package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"

	"serial-gateway/internal/config"
)

type fakePortConfigRuntime struct {
	detected []string
	applied  []string
	applyErr error
}

func (f *fakePortConfigRuntime) DetectedPorts() []string {
	return append([]string(nil), f.detected...)
}

func (f *fakePortConfigRuntime) ApplyAllowedPorts(ports []string) error {
	f.applied = append([]string(nil), ports...)
	return f.applyErr
}

func TestConfigControllerGetAndUpdatePorts(t *testing.T) {
	path := filepath.Join(t.TempDir(), config.FileName)
	initial := config.Default()
	if err := config.Save(path, initial); err != nil {
		t.Fatal(err)
	}
	runtime := &fakePortConfigRuntime{detected: []string{"COM11", "COM13"}}
	controller := newConfigController(path, initial, runtime)

	getResponse := httptest.NewRecorder()
	controller.Handle(getResponse, httptest.NewRequest(http.MethodGet, "/api/v1/config", nil))
	if getResponse.Code != http.StatusOK || !bytes.Contains(getResponse.Body.Bytes(), []byte(`"COM13"`)) {
		t.Fatalf("unexpected GET response: %d %s", getResponse.Code, getResponse.Body.String())
	}

	updated := initial
	updated.Serial.Ports = []string{"COM13"}
	body, _ := json.Marshal(updated)
	putResponse := httptest.NewRecorder()
	controller.Handle(putResponse, httptest.NewRequest(http.MethodPut, "/api/v1/config", bytes.NewReader(body)))
	if putResponse.Code != http.StatusOK {
		t.Fatalf("PUT status=%d body=%s", putResponse.Code, putResponse.Body.String())
	}
	var result struct {
		Saved           bool `json:"saved"`
		RestartRequired bool `json:"restart_required"`
	}
	if err := json.NewDecoder(putResponse.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if !result.Saved || result.RestartRequired {
		t.Fatalf("unexpected PUT result: %#v", result)
	}
	if !reflect.DeepEqual(runtime.applied, []string{"COM13"}) {
		t.Fatalf("applied ports=%v", runtime.applied)
	}
	loaded, _, err := config.LoadOrCreate(path)
	if err != nil || !reflect.DeepEqual(loaded.Serial.Ports, []string{"COM13"}) {
		t.Fatalf("saved ports=%v err=%v", loaded.Serial.Ports, err)
	}
}

func TestConfigControllerReportsRestartForSerialChanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), config.FileName)
	initial := config.Default()
	if err := config.Save(path, initial); err != nil {
		t.Fatal(err)
	}
	controller := newConfigController(path, initial, &fakePortConfigRuntime{})
	updated := initial
	updated.Serial.BaudRate = 9600
	body, _ := json.Marshal(updated)
	response := httptest.NewRecorder()
	controller.Handle(response, httptest.NewRequest(http.MethodPut, "/api/v1/config", bytes.NewReader(body)))
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"restart_required":true`)) {
		t.Fatalf("unexpected response: %d %s", response.Code, response.Body.String())
	}
}
