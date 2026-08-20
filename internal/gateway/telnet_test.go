package gateway

import (
	"bytes"
	"io"
	"net"
	"testing"
	"time"
)

func TestTelnetDecoderStripsNegotiationAcrossReads(t *testing.T) {
	decoder := telnetDecoder{}
	data, reply := decoder.Decode([]byte{'a', telnetIAC, telnetWILL})
	if string(data) != "a" || len(reply) != 0 {
		t.Fatalf("first decode = data %x reply %x", data, reply)
	}
	data, reply = decoder.Decode([]byte{99, 'b', telnetIAC, telnetIAC})
	if !bytes.Equal(data, []byte{'b', telnetIAC}) {
		t.Fatalf("decoded data = %x", data)
	}
	wantReply := []byte{telnetIAC, telnetDONT, 99}
	if !bytes.Equal(reply, wantReply) {
		t.Fatalf("negotiation reply = %x, want %x", reply, wantReply)
	}
}

func TestEncodeTelnetDataEscapesIAC(t *testing.T) {
	want := []byte{'a', telnetIAC, telnetIAC, 'b'}
	if got := encodeTelnetData([]byte{'a', telnetIAC, 'b'}); !bytes.Equal(got, want) {
		t.Fatalf("encoded = %x, want %x", got, want)
	}
}

func TestTelnetBridgeNegotiatesAndWritesSerial(t *testing.T) {
	port := newFakeSerialPort()
	gateway := NewSerialGateway("TEST-TELNET", testSerialSettings, "127.0.0.1", 0)
	gateway.SetTCPInputOptions(TCPInputOptions{NormalizeCRLF: true})
	gateway.SetTelnetAddress("127.0.0.1", 0)
	gateway.Attach(port)
	if err := gateway.EnsureTelnet(); err != nil {
		t.Fatalf("start Telnet listener: %v", err)
	}
	t.Cleanup(gateway.Close)

	gateway.telnetListenerMu.Lock()
	address := gateway.telnetListener.Addr().String()
	gateway.telnetListenerMu.Unlock()
	conn, err := net.Dial("tcp", address)
	if err != nil {
		t.Fatalf("dial Telnet: %v", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(time.Second))
	greeting := make([]byte, len(telnetGreeting))
	if _, err := io.ReadFull(conn, greeting); err != nil {
		t.Fatalf("read negotiation: %v", err)
	}
	if !bytes.Equal(greeting, telnetGreeting) {
		t.Fatalf("negotiation = %x, want %x", greeting, telnetGreeting)
	}

	input := []byte{telnetIAC, telnetDO, telnetEcho, 'e', 't', 'h', '\t', '\r', '\n', telnetIAC, telnetIAC}
	if _, err := conn.Write(input); err != nil {
		t.Fatalf("write Telnet input: %v", err)
	}
	wantSerial := string(append([]byte("eth\t\r"), telnetIAC))
	waitFor(t, func() bool { return port.written() == wantSerial })
}
