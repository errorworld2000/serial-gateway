package gateway

import (
	"context"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.bug.st/serial"
)

var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

type SerialOpener func(name string) (io.ReadWriteCloser, error)

type App struct {
	mu             sync.RWMutex
	gateways       map[string]*SerialGateway
	serial         SerialSettings
	tcpHost        string
	tcpBase        int
	nextTCPPort    int
	usedTCPPort    map[int]bool
	telnet         TelnetOptions
	nextTelnetPort int
	usedTelnetPort map[int]bool
	openSerial     SerialOpener
	build          BuildInfo
	allowed        map[string]bool
	tcpInput       TCPInputOptions
	detected       []string
}

type TelnetOptions struct {
	Enabled  bool
	Host     string
	BasePort int
}

func (a *App) SetAllowedPorts(names []string) {
	allowed := make(map[string]bool, len(names))
	for _, name := range names {
		if normalized := strings.ToUpper(strings.TrimSpace(name)); normalized != "" {
			allowed[normalized] = true
		}
	}
	a.mu.Lock()
	a.allowed = allowed
	a.mu.Unlock()
}

func (a *App) SetTCPInputOptions(options TCPInputOptions) {
	a.mu.Lock()
	a.tcpInput = options
	a.mu.Unlock()
}

func (a *App) SetTelnetOptions(options TelnetOptions) {
	a.mu.Lock()
	a.telnet = options
	a.nextTelnetPort = options.BasePort
	a.mu.Unlock()
}

func (a *App) DetectedPorts() []string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return append([]string(nil), a.detected...)
}

func (a *App) ApplyAllowedPorts(names []string) error {
	a.SetAllowedPorts(names)
	ports, err := serial.GetPortsList()
	if err != nil {
		return err
	}
	a.Reconcile(ports)
	return nil
}

func NewApp(settings SerialSettings, tcpHost string, tcpBase int, opener SerialOpener) *App {
	return &App{
		gateways:       make(map[string]*SerialGateway),
		serial:         settings,
		tcpHost:        tcpHost,
		tcpBase:        tcpBase,
		nextTCPPort:    tcpBase,
		usedTCPPort:    make(map[int]bool),
		usedTelnetPort: make(map[int]bool),
		openSerial:     opener,
		build:          BuildInfo{Version: "dev", Commit: "unknown", BuildDate: "unknown"},
	}
}

type BuildInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"build_date"`
}

func (a *App) SetBuildInfo(info BuildInfo) {
	a.mu.Lock()
	a.build = info
	a.mu.Unlock()
}

func (a *App) allocateTCPPort(name string) int {
	return allocateMappedPort(name, a.tcpBase, &a.nextTCPPort, a.usedTCPPort)
}

func (a *App) allocateTelnetPort(name string) int {
	return allocateMappedPort(name, a.telnet.BasePort, &a.nextTelnetPort, a.usedTelnetPort)
}

func allocateMappedPort(name string, base int, next *int, used map[int]bool) int {
	upperName := strings.ToUpper(name)
	if strings.HasPrefix(upperName, "COM") {
		if number, err := strconv.Atoi(strings.TrimPrefix(upperName, "COM")); err == nil {
			candidate := base + number
			if candidate <= 65535 && !used[candidate] {
				used[candidate] = true
				return candidate
			}
		}
	}
	if *next > 65535 {
		*next = 1024
	}
	for used[*next] {
		*next++
		if *next > 65535 {
			*next = 1024
		}
	}
	port := *next
	used[port] = true
	*next++
	return port
}

func (a *App) gateway(name string) (*SerialGateway, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	gateway, ok := a.gateways[name]
	return gateway, ok
}

func (a *App) gatewayList() []*SerialGateway {
	a.mu.RLock()
	defer a.mu.RUnlock()
	gateways := make([]*SerialGateway, 0, len(a.gateways))
	for _, gateway := range a.gateways {
		gateways = append(gateways, gateway)
	}
	sort.Slice(gateways, func(i, j int) bool { return gateways[i].Name() < gateways[j].Name() })
	return gateways
}

func (a *App) Reconcile(portNames []string) {
	sort.Strings(portNames)
	a.mu.Lock()
	a.detected = append(a.detected[:0], portNames...)
	a.mu.Unlock()
	present := make(map[string]bool, len(portNames))
	for _, name := range portNames {
		a.mu.RLock()
		allowed := len(a.allowed) == 0 || a.allowed[strings.ToUpper(name)]
		a.mu.RUnlock()
		if !allowed {
			continue
		}
		present[name] = true
		gateway, ok := a.gateway(name)
		if !ok {
			a.mu.Lock()
			tcpPort := a.allocateTCPPort(name)
			gateway = NewSerialGateway(name, a.serial, a.tcpHost, tcpPort)
			if a.telnet.Enabled {
				gateway.SetTelnetAddress(a.telnet.Host, a.allocateTelnetPort(name))
			}
			gateway.SetTCPInputOptions(a.tcpInput)
			a.gateways[name] = gateway
			a.mu.Unlock()
			log.Printf("Discovered serial port %s (Raw TCP: %s, SecureCRT Telnet: %s)", name, gateway.TCPAddress(), gateway.TelnetAddress())
		}
		if err := gateway.EnsureTCP(); err != nil {
			log.Printf("TCP listener for %s: %v", name, err)
		}
		if gateway.TelnetAddress() != "" {
			if err := gateway.EnsureTelnet(); err != nil {
				log.Printf("Telnet listener for %s: %v", name, err)
			}
		}
		if !gateway.Connected() {
			port, err := a.openSerial(name)
			if err != nil {
				log.Printf("Serial port %s is unavailable: %v", name, err)
				continue
			}
			if gateway.Attach(port) {
				log.Printf("Opened serial port %s (%s)", name, a.serial)
			} else {
				_ = port.Close()
			}
		}
	}
	for _, gateway := range a.gatewayList() {
		if !present[gateway.Name()] && gateway.Connected() {
			log.Printf("Serial port %s disappeared; waiting for it to return", gateway.Name())
			gateway.Detach()
		}
	}
}

func (a *App) ScanLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ports, err := serial.GetPortsList()
			if err != nil {
				log.Printf("Serial scan failed: %v", err)
				continue
			}
			a.Reconcile(ports)
		}
	}
}

func (a *App) Close() {
	for _, gateway := range a.gatewayList() {
		gateway.Close()
	}
}

func (a *App) HandleWS(w http.ResponseWriter, r *http.Request) {
	portName := r.URL.Query().Get("port")
	gateway, ok := a.gateway(portName)
	if portName == "" {
		http.Error(w, "missing port parameter", http.StatusBadRequest)
		return
	}
	if !ok {
		http.Error(w, "unknown port", http.StatusNotFound)
		return
	}
	if !gateway.Connected() {
		http.Error(w, "port is currently disconnected", http.StatusServiceUnavailable)
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade: %v", err)
		return
	}
	remote := conn.RemoteAddr().String()
	subscriber := gateway.Subscribe("websocket", remote, func(data []byte) error {
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		return conn.WriteMessage(websocket.BinaryMessage, data)
	}, conn.Close)
	defer gateway.Unsubscribe(subscriber)
	log.Printf("WebSocket client connected to %s: %s", portName, remote)
	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			break
		}
		if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
			continue
		}
		if _, err := gateway.Write(data); err != nil {
			log.Printf("Serial write error on %s: %v", portName, err)
			break
		}
	}
	log.Printf("WebSocket client disconnected from %s: %s", portName, remote)
}

func NewMux(app *App, publicFiles http.FileSystem) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/docs", app.HandleAPIDocs)
	mux.HandleFunc("/api/docs/", app.HandleAPIDocs)
	mux.HandleFunc("/api/v1/guide", app.HandleAIGuide)
	mux.HandleFunc("/api/v1/openapi.json", app.HandleOpenAPI)
	mux.HandleFunc("/ws", app.HandleWS)
	mux.HandleFunc("/api/ports", app.HandleLegacyPorts)
	mux.HandleFunc("/api/v1/ports", app.HandlePortStatus)
	mux.HandleFunc("/api/v1/info", app.HandleInfo)
	mux.HandleFunc("/api/v1/read", app.HandleRead)
	mux.HandleFunc("/api/v1/write", app.HandleWrite)
	mux.HandleFunc("/healthz", app.HandleHealth)
	mux.Handle("/", http.FileServer(publicFiles))
	return mux
}
