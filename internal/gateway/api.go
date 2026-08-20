package gateway

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const maxAPIWriteBytes = 1024 * 1024

type encodedRecord struct {
	Sequence  uint64    `json:"sequence"`
	Timestamp time.Time `json:"timestamp"`
	Direction string    `json:"direction"`
	Encoding  string    `json:"encoding"`
	Data      string    `json:"data"`
}

type writeRequest struct {
	Encoding string `json:"encoding"`
	Data     string `json:"data"`
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeAPIError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": message, "status": status})
}

func requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method == method {
		return true
	}
	w.Header().Set("Allow", method)
	writeAPIError(w, http.StatusMethodNotAllowed, "method not allowed")
	return false
}

func (a *App) HandleLegacyPorts(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	ports := make([]string, 0)
	for _, gateway := range a.gatewayList() {
		if gateway.Connected() {
			ports = append(ports, gateway.Name())
		}
	}
	writeJSON(w, http.StatusOK, ports)
}

func (a *App) HandlePortStatus(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	statuses := make([]GatewayStatus, 0)
	for _, gateway := range a.gatewayList() {
		statuses = append(statuses, gateway.Status())
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ports": statuses,
		"usage": map[string]string{
			"securecrt": "Create a Telnet session using the port's telnet_address",
			"raw":       "Use tcp_address for byte-transparent TCP tools",
			"read":      "GET /api/v1/read?port=COM3&after=0&wait_ms=30000&encoding=hex",
			"write":     "POST /api/v1/write?port=COM3 with JSON {\"encoding\":\"text|hex|base64\",\"data\":\"...\"}",
		},
	})
}

func (a *App) HandleInfo(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	a.mu.RLock()
	build := a.build
	settings := a.serial
	a.mu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"service":         "serial-gateway",
		"api_version":     "v1",
		"build":           build,
		"serial_defaults": settings,
	})
}

func (a *App) HandleHealth(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	connected := 0
	for _, gateway := range a.gatewayList() {
		if gateway.Connected() {
			connected++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "connected_ports": connected})
}

func (a *App) HandleRead(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	gateway, ok := a.gateway(r.URL.Query().Get("port"))
	if !ok {
		writeAPIError(w, http.StatusNotFound, "unknown port")
		return
	}
	after, err := parseUintQuery(r, "after", 0)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	limit64, err := parseUintQuery(r, "limit", 256)
	if err != nil || limit64 == 0 || limit64 > 1000 {
		writeAPIError(w, http.StatusBadRequest, "limit must be between 1 and 1000")
		return
	}
	waitMS, err := parseUintQuery(r, "wait_ms", 0)
	if err != nil || waitMS > 30000 {
		writeAPIError(w, http.StatusBadRequest, "wait_ms must be between 0 and 30000")
		return
	}
	encoding := strings.ToLower(r.URL.Query().Get("encoding"))
	if encoding == "" {
		encoding = "base64"
	}
	if encoding != "text" && encoding != "hex" && encoding != "base64" {
		writeAPIError(w, http.StatusBadRequest, "encoding must be text, hex, or base64")
		return
	}
	records, latest, oldest, notify := gateway.RecordsAfter(after, int(limit64))
	if len(records) == 0 && waitMS > 0 {
		timer := time.NewTimer(time.Duration(waitMS) * time.Millisecond)
		defer timer.Stop()
		select {
		case <-notify:
		case <-timer.C:
		case <-r.Context().Done():
			return
		}
		records, latest, oldest, _ = gateway.RecordsAfter(after, int(limit64))
	}
	result := make([]encodedRecord, 0, len(records))
	nextAfter := after
	for _, record := range records {
		result = append(result, encodedRecord{Sequence: record.Sequence, Timestamp: record.Timestamp, Direction: record.Direction, Encoding: encoding, Data: encodeData(record.Data, encoding)})
		nextAfter = record.Sequence
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"port":              gateway.Name(),
		"connected":         gateway.Connected(),
		"oldest_sequence":   oldest,
		"latest_sequence":   latest,
		"next_after":        nextAfter,
		"has_more":          nextAfter < latest,
		"history_truncated": len(records) > 0 && oldest > after+1,
		"records":           result,
	})
}

func (a *App) HandleWrite(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	gateway, ok := a.gateway(r.URL.Query().Get("port"))
	if !ok {
		writeAPIError(w, http.StatusNotFound, "unknown port")
		return
	}
	var data []byte
	var err error
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		var request writeRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAPIWriteBytes))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		data, err = decodeData(request.Data, request.Encoding)
	} else {
		data, err = io.ReadAll(http.MaxBytesReader(w, r.Body, maxAPIWriteBytes))
	}
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	written, err := gateway.Write(data)
	if errors.Is(err, errPortDisconnected) {
		writeAPIError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"port": gateway.Name(), "bytes_written": written, "latest_sequence": gateway.Status().LatestSequence})
}

func parseUintQuery(r *http.Request, name string, defaultValue uint64) (uint64, error) {
	value := r.URL.Query().Get(name)
	if value == "" {
		return defaultValue, nil
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be an unsigned integer", name)
	}
	return parsed, nil
}

func encodeData(data []byte, encoding string) string {
	switch encoding {
	case "text":
		return string(data)
	case "hex":
		return hex.EncodeToString(data)
	default:
		return base64.StdEncoding.EncodeToString(data)
	}
}

func decodeData(data, encoding string) ([]byte, error) {
	switch strings.ToLower(encoding) {
	case "", "text", "utf8", "utf-8":
		return []byte(data), nil
	case "hex":
		compact := strings.NewReplacer(" ", "", "\n", "", "\r", "", "\t", "").Replace(data)
		decoded, err := hex.DecodeString(compact)
		if err != nil {
			return nil, fmt.Errorf("invalid hex data: %w", err)
		}
		return decoded, nil
	case "base64":
		decoded, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			return nil, fmt.Errorf("invalid base64 data: %w", err)
		}
		return decoded, nil
	default:
		return nil, fmt.Errorf("encoding must be text, hex, or base64")
	}
}
