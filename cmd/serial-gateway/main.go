package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"serial-gateway/internal/gateway"
	"serial-gateway/internal/webui"

	"go.bug.st/serial"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

func main() {
	baudRate := flag.Int("baud", 115200, "serial baud rate")
	dataBits := flag.Int("data-bits", 8, "serial data bits: 5, 6, 7, or 8")
	parityName := flag.String("parity", "none", "serial parity: none, odd, even, mark, or space")
	stopBitsName := flag.String("stop-bits", "1", "serial stop bits: 1, 1.5, or 2")
	httpAddress := flag.String("http", "127.0.0.1:8080", "HTTP listen address")
	tcpHost := flag.String("tcp-host", "127.0.0.1", "SecureCRT Raw TCP listen host")
	tcpBase := flag.Int("tcp-base", 7000, "first SecureCRT Raw TCP port")
	scanInterval := flag.Duration("scan-interval", 2*time.Second, "serial hot-plug scan interval")
	portNames := flag.String("ports", "", "comma-separated serial port allowlist, for example COM13,COM14")
	showVersion := flag.Bool("version", false, "print version information and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("serial-gateway %s (commit %s, built %s)\n", version, commit, buildDate)
		return
	}
	if *dataBits < 5 || *dataBits > 8 {
		log.Fatalf("Invalid -data-bits %d: expected 5, 6, 7, or 8", *dataBits)
	}
	parity, err := parseParity(*parityName)
	if err != nil {
		log.Fatal(err)
	}
	stopBits, err := parseStopBits(*stopBitsName)
	if err != nil {
		log.Fatal(err)
	}
	if *tcpBase < 1 || *tcpBase > 65535 {
		log.Fatalf("Invalid -tcp-base %d: expected 1-65535", *tcpBase)
	}
	if *scanInterval <= 0 {
		log.Fatalf("Invalid -scan-interval %s: expected a positive duration", *scanInterval)
	}

	mode := serial.Mode{BaudRate: *baudRate, DataBits: *dataBits, Parity: parity, StopBits: stopBits}
	opener := func(name string) (io.ReadWriteCloser, error) {
		return serial.Open(name, &mode)
	}
	settings := gateway.SerialSettings{BaudRate: *baudRate, DataBits: *dataBits, Parity: strings.ToUpper((*parityName)[:1]), StopBits: *stopBitsName}
	app := gateway.NewApp(settings, *tcpHost, *tcpBase, opener)
	app.SetBuildInfo(gateway.BuildInfo{Version: version, Commit: commit, BuildDate: buildDate})
	if *portNames != "" {
		app.SetAllowedPorts(strings.Split(*portNames, ","))
	}
	ports, err := serial.GetPortsList()
	if err != nil {
		log.Printf("Initial serial scan failed: %v", err)
	} else {
		app.Reconcile(ports)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go app.ScanLoop(ctx, *scanInterval)

	server := &http.Server{
		Addr:              *httpAddress,
		Handler:           gateway.NewMux(app, webui.FileSystem()),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	serverErrors := make(chan error, 1)
	go func() {
		log.Printf("Serial mode: %s", settings)
		log.Printf("Web UI and AI API started at http://%s", *httpAddress)
		if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
		close(serverErrors)
	}()

	select {
	case <-ctx.Done():
		log.Printf("Shutting down")
	case err := <-serverErrors:
		if err != nil {
			log.Printf("HTTP server failed: %v", err)
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP shutdown: %v", err)
	}
	app.Close()
}

func parseParity(value string) (serial.Parity, error) {
	switch strings.ToLower(value) {
	case "none", "n":
		return serial.NoParity, nil
	case "odd", "o":
		return serial.OddParity, nil
	case "even", "e":
		return serial.EvenParity, nil
	case "mark", "m":
		return serial.MarkParity, nil
	case "space", "s":
		return serial.SpaceParity, nil
	default:
		return serial.NoParity, fmt.Errorf("invalid -parity %q: expected none, odd, even, mark, or space", value)
	}
}

func parseStopBits(value string) (serial.StopBits, error) {
	switch value {
	case "1":
		return serial.OneStopBit, nil
	case "1.5":
		return serial.OnePointFiveStopBits, nil
	case "2":
		return serial.TwoStopBits, nil
	default:
		return serial.OneStopBit, fmt.Errorf("invalid -stop-bits %q: expected 1, 1.5, or 2", value)
	}
}
