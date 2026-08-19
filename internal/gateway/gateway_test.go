package gateway

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeSerialPort struct {
	reads     chan []byte
	closed    chan struct{}
	closeOnce sync.Once
	writesMu  sync.Mutex
	writes    bytes.Buffer
}

func newFakeSerialPort() *fakeSerialPort {
	return &fakeSerialPort{reads: make(chan []byte, 8), closed: make(chan struct{})}
}

func (p *fakeSerialPort) Read(buffer []byte) (int, error) {
	select {
	case data := <-p.reads:
		return copy(buffer, data), nil
	case <-p.closed:
		return 0, io.EOF
	}
}

func (p *fakeSerialPort) Write(data []byte) (int, error) {
	p.writesMu.Lock()
	defer p.writesMu.Unlock()
	return p.writes.Write(data)
}

func (p *fakeSerialPort) Close() error {
	p.closeOnce.Do(func() { close(p.closed) })
	return nil
}

func (p *fakeSerialPort) written() string {
	p.writesMu.Lock()
	defer p.writesMu.Unlock()
	return p.writes.String()
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for condition")
		}
		time.Sleep(time.Millisecond)
	}
}

var testSerialSettings = SerialSettings{BaudRate: 115200, DataBits: 8, Parity: "N", StopBits: "1"}

func TestStableCOMTCPMapping(t *testing.T) {
	app := NewApp(testSerialSettings, "127.0.0.1", 7000, nil)
	if port := app.allocateTCPPort("COM3"); port != 7003 {
		t.Fatalf("COM3 mapped to %d, want 7003", port)
	}
	if port := app.allocateTCPPort("COM7"); port != 7007 {
		t.Fatalf("COM7 mapped to %d, want 7007", port)
	}
	if port := app.allocateTCPPort("/dev/ttyUSB0"); port != 7000 {
		t.Fatalf("non-COM port mapped to %d, want 7000", port)
	}
}

func TestInfoAPI(t *testing.T) {
	app := NewApp(testSerialSettings, "127.0.0.1", 7000, nil)
	app.SetBuildInfo(BuildInfo{Version: "1.2.3", Commit: "abc123", BuildDate: "2026-08-19T00:00:00Z"})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/info", nil)
	response := httptest.NewRecorder()
	NewMux(app, http.Dir("../../internal/webui/public")).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("info status = %d", response.Code)
	}
	var payload struct {
		Service    string         `json:"service"`
		APIVersion string         `json:"api_version"`
		Build      BuildInfo      `json:"build"`
		Serial     SerialSettings `json:"serial_defaults"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode info: %v", err)
	}
	if payload.Service != "serial-gateway" || payload.APIVersion != "v1" || payload.Build.Version != "1.2.3" || payload.Serial != testSerialSettings {
		t.Fatalf("unexpected info response: %#v", payload)
	}
}

func TestGatewayRecordsRXAndTX(t *testing.T) {
	port := newFakeSerialPort()
	gateway := NewSerialGateway("TEST1", testSerialSettings, "127.0.0.1", 0)
	t.Cleanup(gateway.Close)
	if !gateway.Attach(port) {
		t.Fatal("failed to attach fake port")
	}
	if _, err := gateway.Write([]byte("command\r")); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	port.reads <- []byte("response\r\n")
	waitFor(t, func() bool { return gateway.Status().LatestSequence == 2 })
	records, latest, oldest, _ := gateway.RecordsAfter(0, 10)
	if latest != 2 || len(records) != 2 {
		t.Fatalf("got latest=%d records=%d", latest, len(records))
	}
	if oldest != 1 {
		t.Fatalf("got oldest=%d", oldest)
	}
	if records[0].Direction != "tx" || string(records[0].Data) != "command\r" {
		t.Fatalf("unexpected TX record: %#v", records[0])
	}
	if records[1].Direction != "rx" || string(records[1].Data) != "response\r\n" {
		t.Fatalf("unexpected RX record: %#v", records[1])
	}
}

func TestRXReadsAreCoalesced(t *testing.T) {
	port := newFakeSerialPort()
	gateway := NewSerialGateway("TEST-COALESCE", testSerialSettings, "127.0.0.1", 0)
	t.Cleanup(gateway.Close)
	gateway.Attach(port)
	for _, value := range []byte("root@localhost:~# ") {
		port.reads <- []byte{value}
	}
	waitFor(t, func() bool { return gateway.Status().LatestSequence == 1 })
	records, _, _, _ := gateway.RecordsAfter(0, 10)
	if len(records) != 1 || string(records[0].Data) != "root@localhost:~# " {
		t.Fatalf("RX data was not coalesced: %#v", records)
	}
}

func TestRawTCPBridge(t *testing.T) {
	port := newFakeSerialPort()
	gateway := NewSerialGateway("TEST2", testSerialSettings, "127.0.0.1", 0)
	t.Cleanup(gateway.Close)
	gateway.Attach(port)
	if err := gateway.EnsureTCP(); err != nil {
		t.Fatalf("start TCP listener: %v", err)
	}
	gateway.listenerMu.Lock()
	address := gateway.listener.Addr().String()
	gateway.listenerMu.Unlock()
	conn, err := net.Dial("tcp", address)
	if err != nil {
		t.Fatalf("dial raw TCP: %v", err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte{0x00, 0x41, 0xff}); err != nil {
		t.Fatalf("TCP write: %v", err)
	}
	waitFor(t, func() bool { return port.written() == string([]byte{0x00, 0x41, 0xff}) })
	port.reads <- []byte{0x10, 0x00, 0xfe}
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	received := make([]byte, 3)
	if _, err := io.ReadFull(conn, received); err != nil {
		t.Fatalf("TCP read: %v", err)
	}
	if !bytes.Equal(received, []byte{0x10, 0x00, 0xfe}) {
		t.Fatalf("binary data changed: %x", received)
	}
}

func TestAIAPIWriteAndCursorRead(t *testing.T) {
	port := newFakeSerialPort()
	settings := SerialSettings{BaudRate: 9600, DataBits: 8, Parity: "N", StopBits: "1"}
	gateway := NewSerialGateway("TEST3", settings, "127.0.0.1", 0)
	gateway.Attach(port)
	app := NewApp(settings, "127.0.0.1", 7000, nil)
	app.gateways[gateway.Name()] = gateway
	t.Cleanup(app.Close)
	server := httptest.NewServer(NewMux(app, http.Dir("../../internal/webui/public")))
	defer server.Close()

	requestBody := strings.NewReader(`{"encoding":"hex","data":"41 00 ff"}`)
	response, err := http.Post(server.URL+"/api/v1/write?port=TEST3", "application/json", requestBody)
	if err != nil {
		t.Fatalf("API write: %v", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("API write status: %d", response.StatusCode)
	}
	waitFor(t, func() bool { return port.written() == string([]byte{0x41, 0x00, 0xff}) })
	port.reads <- []byte("ok\r\n")
	waitFor(t, func() bool { return gateway.Status().LatestSequence == 2 })

	response, err = http.Get(server.URL + "/api/v1/read?port=TEST3&after=1&encoding=text")
	if err != nil {
		t.Fatalf("API read: %v", err)
	}
	defer response.Body.Close()
	var payload struct {
		LatestSequence uint64          `json:"latest_sequence"`
		NextAfter      uint64          `json:"next_after"`
		HasMore        bool            `json:"has_more"`
		Records        []encodedRecord `json:"records"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode API response: %v", err)
	}
	if payload.LatestSequence != 2 || payload.NextAfter != 2 || payload.HasMore || len(payload.Records) != 1 || payload.Records[0].Data != "ok\r\n" {
		t.Fatalf("unexpected cursor response: %#v", payload)
	}
}
