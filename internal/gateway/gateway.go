package gateway

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

var errPortDisconnected = errors.New("serial port is disconnected")

const (
	subscriberQueueSize = 128
	maxHistoryBytes     = 1024 * 1024
	rxCoalesceWindow    = 10 * time.Millisecond
	maxRXChunkBytes     = 4096
)

type Record struct {
	Sequence  uint64    `json:"sequence"`
	Timestamp time.Time `json:"timestamp"`
	Direction string    `json:"direction"`
	Data      []byte    `json:"-"`
}

type SerialSettings struct {
	BaudRate int    `json:"baud_rate"`
	DataBits int    `json:"data_bits"`
	Parity   string `json:"parity"`
	StopBits string `json:"stop_bits"`
}

func (s SerialSettings) String() string {
	return fmt.Sprintf("%d %d%s%s", s.BaudRate, s.DataBits, s.Parity, s.StopBits)
}

type Subscriber struct {
	kind      string
	remote    string
	send      chan []byte
	done      chan struct{}
	closeOnce sync.Once
	close     func() error
}

func (s *Subscriber) stop() {
	s.closeOnce.Do(func() {
		close(s.done)
		if s.close != nil {
			_ = s.close()
		}
	})
}

func (s *Subscriber) enqueue(data []byte) bool {
	select {
	case <-s.done:
		return false
	default:
	}
	select {
	case s.send <- data:
		return true
	case <-s.done:
		return false
	default:
		return false
	}
}

type GatewayStatus struct {
	Name             string `json:"name"`
	BaudRate         int    `json:"baud_rate"`
	DataBits         int    `json:"data_bits"`
	Parity           string `json:"parity"`
	StopBits         string `json:"stop_bits"`
	Connected        bool   `json:"connected"`
	TCPAddress       string `json:"tcp_address"`
	TCPListening     bool   `json:"tcp_listening"`
	TCPClients       int    `json:"tcp_clients"`
	WebSocketClients int    `json:"websocket_clients"`
	LatestSequence   uint64 `json:"latest_sequence"`
}

type SerialGateway struct {
	name       string
	serial     SerialSettings
	tcpAddress string

	portMu     sync.RWMutex
	port       io.ReadWriteCloser
	generation uint64
	writeMu    sync.Mutex

	subscribersMu sync.RWMutex
	subscribers   map[*Subscriber]struct{}

	historyMu    sync.Mutex
	history      []Record
	historyBytes int
	sequence     atomic.Uint64
	notify       chan struct{}

	listenerMu sync.Mutex
	listener   net.Listener
	closed     atomic.Bool
}

func NewSerialGateway(name string, settings SerialSettings, tcpHost string, tcpPort int) *SerialGateway {
	return &SerialGateway{
		name:        name,
		serial:      settings,
		tcpAddress:  net.JoinHostPort(tcpHost, fmt.Sprintf("%d", tcpPort)),
		subscribers: make(map[*Subscriber]struct{}),
		notify:      make(chan struct{}),
	}
}

func (g *SerialGateway) Name() string       { return g.name }
func (g *SerialGateway) TCPAddress() string { return g.tcpAddress }

func (g *SerialGateway) Connected() bool {
	g.portMu.RLock()
	defer g.portMu.RUnlock()
	return g.port != nil
}

func (g *SerialGateway) Attach(port io.ReadWriteCloser) bool {
	if g.closed.Load() {
		return false
	}
	g.portMu.Lock()
	if g.port != nil {
		g.portMu.Unlock()
		return false
	}
	g.generation++
	generation := g.generation
	g.port = port
	g.portMu.Unlock()
	go g.readLoop(port, generation)
	return true
}

func (g *SerialGateway) Detach() {
	g.portMu.Lock()
	port := g.port
	g.port = nil
	g.generation++
	g.portMu.Unlock()
	if port != nil {
		_ = port.Close()
	}
}

func (g *SerialGateway) readLoop(port io.ReadWriteCloser, generation uint64) {
	type readResult struct {
		data []byte
		err  error
	}
	reads := make(chan readResult, 64)
	go func() {
		buffer := make([]byte, maxRXChunkBytes)
		for {
			n, err := port.Read(buffer)
			result := readResult{err: err}
			if n > 0 {
				result.data = append([]byte(nil), buffer[:n]...)
			}
			reads <- result
			if err != nil {
				return
			}
		}
	}()

	var pending []byte
	var timer *time.Timer
	var timerC <-chan time.Time
	flush := func() {
		if len(pending) == 0 {
			return
		}
		data := pending
		pending = nil
		g.addRecord("rx", data)
		g.broadcast(data)
	}
	defer func() {
		if timer != nil {
			timer.Stop()
		}
		flush()
		g.portMu.Lock()
		if g.generation == generation {
			g.port = nil
			g.generation++
		}
		g.portMu.Unlock()
		_ = port.Close()
	}()

	for {
		select {
		case result := <-reads:
			if len(result.data) > 0 {
				pending = append(pending, result.data...)
				if len(pending) >= maxRXChunkBytes {
					if timer != nil {
						timer.Stop()
						timer = nil
						timerC = nil
					}
					flush()
				} else if timer == nil {
					timer = time.NewTimer(rxCoalesceWindow)
					timerC = timer.C
				}
			}
			if result.err != nil {
				g.portMu.RLock()
				current := g.generation == generation
				g.portMu.RUnlock()
				if current && !g.closed.Load() {
					log.Printf("Serial read error on %s: %v", g.name, result.err)
				}
				return
			}
		case <-timerC:
			timer = nil
			timerC = nil
			flush()
		}
	}
}

func (g *SerialGateway) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	g.writeMu.Lock()
	defer g.writeMu.Unlock()
	g.portMu.RLock()
	port := g.port
	if port == nil {
		g.portMu.RUnlock()
		return 0, errPortDisconnected
	}
	n, err := port.Write(data)
	g.portMu.RUnlock()
	if n > 0 {
		g.addRecord("tx", append([]byte(nil), data[:n]...))
	}
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	return n, err
}

func (g *SerialGateway) addRecord(direction string, data []byte) {
	g.historyMu.Lock()
	sequence := g.sequence.Add(1)
	g.history = append(g.history, Record{Sequence: sequence, Timestamp: time.Now().UTC(), Direction: direction, Data: data})
	g.historyBytes += len(data)
	for g.historyBytes > maxHistoryBytes && len(g.history) > 1 {
		g.historyBytes -= len(g.history[0].Data)
		g.history = g.history[1:]
	}
	close(g.notify)
	g.notify = make(chan struct{})
	g.historyMu.Unlock()
}

func (g *SerialGateway) RecordsAfter(after uint64, limit int) ([]Record, uint64, uint64, <-chan struct{}) {
	g.historyMu.Lock()
	defer g.historyMu.Unlock()
	start := len(g.history)
	for i := range g.history {
		if g.history[i].Sequence > after {
			start = i
			break
		}
	}
	end := len(g.history)
	if limit > 0 && end-start > limit {
		end = start + limit
	}
	records := make([]Record, end-start)
	copy(records, g.history[start:end])
	var oldest uint64
	if len(g.history) > 0 {
		oldest = g.history[0].Sequence
	}
	return records, g.sequence.Load(), oldest, g.notify
}

func (g *SerialGateway) Subscribe(kind, remote string, write func([]byte) error, closeFn func() error) *Subscriber {
	subscriber := &Subscriber{kind: kind, remote: remote, send: make(chan []byte, subscriberQueueSize), done: make(chan struct{}), close: closeFn}
	g.subscribersMu.Lock()
	g.subscribers[subscriber] = struct{}{}
	g.subscribersMu.Unlock()
	go func() {
		defer g.Unsubscribe(subscriber)
		for {
			select {
			case data := <-subscriber.send:
				if err := write(data); err != nil {
					return
				}
			case <-subscriber.done:
				return
			}
		}
	}()
	return subscriber
}

func (g *SerialGateway) Unsubscribe(subscriber *Subscriber) {
	g.subscribersMu.Lock()
	delete(g.subscribers, subscriber)
	g.subscribersMu.Unlock()
	subscriber.stop()
}

func (g *SerialGateway) broadcast(data []byte) {
	g.subscribersMu.RLock()
	subscribers := make([]*Subscriber, 0, len(g.subscribers))
	for subscriber := range g.subscribers {
		subscribers = append(subscribers, subscriber)
	}
	g.subscribersMu.RUnlock()
	for _, subscriber := range subscribers {
		if !subscriber.enqueue(data) {
			log.Printf("Dropping slow %s client %s on %s", subscriber.kind, subscriber.remote, g.name)
			g.Unsubscribe(subscriber)
		}
	}
}

func (g *SerialGateway) Status() GatewayStatus {
	status := GatewayStatus{
		Name:           g.name,
		BaudRate:       g.serial.BaudRate,
		DataBits:       g.serial.DataBits,
		Parity:         g.serial.Parity,
		StopBits:       g.serial.StopBits,
		Connected:      g.Connected(),
		TCPAddress:     g.tcpAddress,
		LatestSequence: g.sequence.Load(),
	}
	g.listenerMu.Lock()
	status.TCPListening = g.listener != nil
	g.listenerMu.Unlock()
	g.subscribersMu.RLock()
	for subscriber := range g.subscribers {
		switch subscriber.kind {
		case "tcp":
			status.TCPClients++
		case "websocket":
			status.WebSocketClients++
		}
	}
	g.subscribersMu.RUnlock()
	return status
}

func (g *SerialGateway) EnsureTCP() error {
	if g.closed.Load() {
		return net.ErrClosed
	}
	g.listenerMu.Lock()
	defer g.listenerMu.Unlock()
	if g.listener != nil {
		return nil
	}
	listener, err := net.Listen("tcp", g.tcpAddress)
	if err != nil {
		return err
	}
	g.listener = listener
	go g.acceptLoop(listener)
	return nil
}

func (g *SerialGateway) acceptLoop(listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			g.listenerMu.Lock()
			if g.listener == listener {
				g.listener = nil
			}
			g.listenerMu.Unlock()
			if !g.closed.Load() {
				log.Printf("TCP accept error on %s: %v", g.name, err)
			}
			return
		}
		go g.handleTCP(conn)
	}
}

func (g *SerialGateway) handleTCP(conn net.Conn) {
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		_ = tcpConn.SetNoDelay(true)
		_ = tcpConn.SetKeepAlive(true)
	}
	remote := conn.RemoteAddr().String()
	subscriber := g.Subscribe("tcp", remote, func(data []byte) error {
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		return writeAll(conn, data)
	}, conn.Close)
	defer g.Unsubscribe(subscriber)
	log.Printf("Raw TCP client connected to %s: %s", g.name, remote)
	buffer := make([]byte, 4096)
	for {
		n, err := conn.Read(buffer)
		if n > 0 {
			if _, writeErr := g.Write(buffer[:n]); writeErr != nil {
				log.Printf("Raw TCP serial write error on %s: %v", g.name, writeErr)
				break
			}
		}
		if err != nil {
			break
		}
	}
	log.Printf("Raw TCP client disconnected from %s: %s", g.name, remote)
}

func writeAll(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := writer.Write(data)
		if n > 0 {
			data = data[n:]
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}

func (g *SerialGateway) Close() {
	if !g.closed.CompareAndSwap(false, true) {
		return
	}
	g.listenerMu.Lock()
	listener := g.listener
	g.listener = nil
	g.listenerMu.Unlock()
	if listener != nil {
		_ = listener.Close()
	}
	g.Detach()
	g.subscribersMu.RLock()
	subscribers := make([]*Subscriber, 0, len(g.subscribers))
	for subscriber := range g.subscribers {
		subscribers = append(subscribers, subscriber)
	}
	g.subscribersMu.RUnlock()
	for _, subscriber := range subscribers {
		g.Unsubscribe(subscriber)
	}
}
