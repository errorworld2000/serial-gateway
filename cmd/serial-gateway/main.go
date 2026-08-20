package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"serial-gateway/internal/config"
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
	configPath, err := config.PathNextToExecutable()
	if err != nil {
		log.Fatal(err)
	}
	runtimeConfig, created, err := config.LoadOrCreate(configPath)
	if err != nil {
		log.Fatal(err)
	}
	if created {
		log.Printf("Created default configuration: %s", configPath)
	}
	log.Printf("Using configuration: %s", configPath)

	parity, err := parseParity(runtimeConfig.Serial.Parity)
	if err != nil {
		log.Fatal(err)
	}
	stopBits, err := parseStopBits(runtimeConfig.Serial.StopBits)
	if err != nil {
		log.Fatal(err)
	}

	mode := serial.Mode{BaudRate: runtimeConfig.Serial.BaudRate, DataBits: runtimeConfig.Serial.DataBits, Parity: parity, StopBits: stopBits}
	opener := func(name string) (io.ReadWriteCloser, error) {
		return serial.Open(name, &mode)
	}
	settings := gateway.SerialSettings{
		BaudRate: runtimeConfig.Serial.BaudRate,
		DataBits: runtimeConfig.Serial.DataBits,
		Parity:   strings.ToUpper(runtimeConfig.Serial.Parity[:1]),
		StopBits: runtimeConfig.Serial.StopBits,
	}
	app := gateway.NewApp(settings, runtimeConfig.TCP.Host, runtimeConfig.TCP.BasePort, opener)
	app.SetBuildInfo(gateway.BuildInfo{Version: version, Commit: commit, BuildDate: buildDate})
	app.SetTCPInputOptions(gateway.TCPInputOptions{EscapeDelay: runtimeConfig.EscapeDelay(), NormalizeCRLF: runtimeConfig.TCP.NormalizeCRLF})
	app.SetAllowedPorts(runtimeConfig.Serial.Ports)
	ports, err := serial.GetPortsList()
	if err != nil {
		log.Printf("Initial serial scan failed: %v", err)
	} else {
		app.Reconcile(ports)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go app.ScanLoop(ctx, runtimeConfig.ScanInterval())

	mux := gateway.NewMux(app, webui.FileSystem())
	configController := newConfigController(configPath, runtimeConfig, app)
	mux.HandleFunc("/api/v1/config", configController.Handle)
	server := &http.Server{
		Addr:              runtimeConfig.HTTP.Address,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	serverErrors := make(chan error, 1)
	go func() {
		log.Printf("Serial Gateway build %s (commit %s, built %s)", version, commit, buildDate)
		log.Printf("Serial mode: %s", settings)
		log.Printf("Web UI and AI API started at http://%s", runtimeConfig.HTTP.Address)
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
		return serial.NoParity, fmt.Errorf("invalid serial.parity %q: expected none, odd, even, mark, or space", value)
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
		return serial.OneStopBit, fmt.Errorf("invalid serial.stop_bits %q: expected 1, 1.5, or 2", value)
	}
}
