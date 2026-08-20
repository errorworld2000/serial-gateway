package main

import (
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"sync"

	"serial-gateway/internal/config"
)

type portConfigRuntime interface {
	DetectedPorts() []string
	ApplyAllowedPorts([]string) error
}

type configController struct {
	mu      sync.RWMutex
	path    string
	current config.Config
	runtime portConfigRuntime
}

func newConfigController(path string, current config.Config, runtime portConfigRuntime) *configController {
	return &configController{path: path, current: cloneConfig(current), runtime: runtime}
}

func (c *configController) Handle(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		c.handleGet(w)
	case http.MethodPut:
		c.handlePut(w, r)
	default:
		w.Header().Set("Allow", "GET, PUT")
		writeConfigJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

func (c *configController) handleGet(w http.ResponseWriter) {
	c.mu.RLock()
	current := cloneConfig(c.current)
	c.mu.RUnlock()
	writeConfigJSON(w, http.StatusOK, map[string]any{
		"config":         current,
		"detected_ports": c.runtime.DetectedPorts(),
	})
}

func (c *configController) handlePut(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024))
	decoder.DisallowUnknownFields()
	var updated config.Config
	if err := decoder.Decode(&updated); err != nil {
		writeConfigJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid configuration: " + err.Error()})
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeConfigJSON(w, http.StatusBadRequest, map[string]any{"error": "configuration must contain one JSON object"})
		return
	}
	if err := updated.Validate(); err != nil {
		writeConfigJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	c.mu.Lock()
	previous := cloneConfig(c.current)
	if err := config.Save(c.path, updated); err != nil {
		c.mu.Unlock()
		writeConfigJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	c.current = cloneConfig(updated)
	c.mu.Unlock()

	applyError := ""
	if err := c.runtime.ApplyAllowedPorts(updated.Serial.Ports); err != nil {
		applyError = err.Error()
	}
	writeConfigJSON(w, http.StatusOK, map[string]any{
		"saved":            true,
		"restart_required": restartRequired(previous, updated),
		"apply_error":      applyError,
		"config":           updated,
		"detected_ports":   c.runtime.DetectedPorts(),
	})
}

func restartRequired(previous, updated config.Config) bool {
	previous.Serial.Ports = nil
	updated.Serial.Ports = nil
	return !reflect.DeepEqual(previous, updated)
}

func cloneConfig(value config.Config) config.Config {
	value.Serial.Ports = append([]string(nil), value.Serial.Ports...)
	return value
}

func writeConfigJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
