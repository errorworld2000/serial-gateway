package gateway

import (
	"bytes"
	"log"
	"net"
	"time"
)

const (
	telnetSE   byte = 240
	telnetSB   byte = 250
	telnetWILL byte = 251
	telnetWONT byte = 252
	telnetDO   byte = 253
	telnetDONT byte = 254
	telnetIAC  byte = 255

	telnetBinary       byte = 0
	telnetEcho         byte = 1
	telnetSuppressGA   byte = 3
	telnetTerminalType byte = 24
	telnetNAWS         byte = 31
	telnetSend         byte = 1
)

var telnetGreeting = []byte{
	telnetIAC, telnetWILL, telnetEcho,
	telnetIAC, telnetWILL, telnetSuppressGA,
	telnetIAC, telnetDO, telnetSuppressGA,
	telnetIAC, telnetWILL, telnetBinary,
	telnetIAC, telnetDO, telnetBinary,
	telnetIAC, telnetDO, telnetNAWS,
	telnetIAC, telnetDO, telnetTerminalType,
}

func (g *SerialGateway) EnsureTelnet() error {
	if g.closed.Load() {
		return net.ErrClosed
	}
	if g.telnetAddress == "" {
		return nil
	}
	g.telnetListenerMu.Lock()
	defer g.telnetListenerMu.Unlock()
	if g.telnetListener != nil {
		return nil
	}
	listener, err := net.Listen("tcp", g.telnetAddress)
	if err != nil {
		return err
	}
	g.telnetListener = listener
	go g.telnetAcceptLoop(listener)
	return nil
}

func (g *SerialGateway) telnetAcceptLoop(listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			g.telnetListenerMu.Lock()
			if g.telnetListener == listener {
				g.telnetListener = nil
			}
			g.telnetListenerMu.Unlock()
			if !g.closed.Load() {
				log.Printf("Telnet accept error on %s: %v", g.name, err)
			}
			return
		}
		go g.handleTelnet(conn)
	}
}

func (g *SerialGateway) handleTelnet(conn net.Conn) {
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		_ = tcpConn.SetNoDelay(true)
		_ = tcpConn.SetKeepAlive(true)
	}
	remote := conn.RemoteAddr().String()
	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if err := writeAll(conn, telnetGreeting); err != nil {
		_ = conn.Close()
		return
	}
	subscriber := g.Subscribe("telnet", remote, func(data []byte) error {
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		return writeAll(conn, encodeTelnetData(data))
	}, conn.Close)
	defer g.Unsubscribe(subscriber)
	log.Printf("Telnet client connected to %s: %s", g.name, remote)

	input := terminalInputWriter{gateway: g, options: g.tcpInput}
	decoder := telnetDecoder{}
	buffer := make([]byte, 4096)
	for {
		n, err := conn.Read(buffer)
		if n > 0 {
			data, reply := decoder.Decode(buffer[:n])
			if len(reply) > 0 {
				_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
				if writeErr := writeAll(conn, reply); writeErr != nil {
					break
				}
			}
			data = bytes.ReplaceAll(data, []byte{'\r', 0}, []byte{'\r'})
			if len(data) > 0 {
				if writeErr := input.Write(data); writeErr != nil {
					log.Printf("Telnet serial write error on %s: %v", g.name, writeErr)
					break
				}
			}
		}
		if err != nil {
			break
		}
	}
	log.Printf("Telnet client disconnected from %s: %s", g.name, remote)
}

func encodeTelnetData(data []byte) []byte {
	if !bytes.Contains(data, []byte{telnetIAC}) {
		return data
	}
	encoded := make([]byte, 0, len(data)+1)
	for _, value := range data {
		encoded = append(encoded, value)
		if value == telnetIAC {
			encoded = append(encoded, telnetIAC)
		}
	}
	return encoded
}

type telnetDecoder struct {
	state   byte
	command byte
}

func (d *telnetDecoder) Decode(input []byte) (data []byte, reply []byte) {
	for _, value := range input {
		switch d.state {
		case 0:
			if value == telnetIAC {
				d.state = 1
			} else {
				data = append(data, value)
			}
		case 1:
			switch value {
			case telnetIAC:
				data = append(data, telnetIAC)
				d.state = 0
			case telnetDO, telnetDONT, telnetWILL, telnetWONT:
				d.command = value
				d.state = 2
			case telnetSB:
				d.state = 3
			default:
				d.state = 0
			}
		case 2:
			if d.command == telnetDO && value != telnetEcho && value != telnetSuppressGA && value != telnetBinary {
				reply = append(reply, telnetIAC, telnetWONT, value)
			}
			if d.command == telnetWILL && value != telnetSuppressGA && value != telnetBinary && value != telnetNAWS && value != telnetTerminalType {
				reply = append(reply, telnetIAC, telnetDONT, value)
			}
			if d.command == telnetWILL && value == telnetTerminalType {
				reply = append(reply, telnetIAC, telnetSB, telnetTerminalType, telnetSend, telnetIAC, telnetSE)
			}
			d.state = 0
		case 3:
			if value == telnetIAC {
				d.state = 4
			}
		case 4:
			if value == telnetSE {
				d.state = 0
			} else {
				d.state = 3
			}
		}
	}
	return data, reply
}
