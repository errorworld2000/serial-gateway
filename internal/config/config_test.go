package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadOrCreate(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	value, created, err := LoadOrCreate(path)
	if err != nil {
		t.Fatalf("create default config: %v", err)
	}
	if !created || !reflect.DeepEqual(value, Default()) {
		t.Fatalf("unexpected default config: created=%v value=%#v", created, value)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read generated config: %v", err)
	}
	if !strings.Contains(string(data), `"normalize_crlf": true`) {
		t.Fatalf("generated config missing terminal option: %s", data)
	}
	if !strings.Contains(string(data), `"base_port": 8000`) {
		t.Fatalf("generated config missing Telnet option: %s", data)
	}

	loaded, created, err := LoadOrCreate(path)
	if err != nil || created || !reflect.DeepEqual(loaded, value) {
		t.Fatalf("reload config: created=%v value=%#v err=%v", created, loaded, err)
	}
}

func TestLoadLegacyConfigAddsTelnetDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	legacy := `{"serial":{"ports":["COM13"],"baud_rate":115200,"data_bits":8,"parity":"none","stop_bits":"1"},"http":{"address":"127.0.0.1:8080"},"tcp":{"host":"127.0.0.1","base_port":7000,"escape_delay_ms":5,"normalize_crlf":true},"scan_interval_ms":2000}`
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, created, err := LoadOrCreate(path)
	if err != nil || created {
		t.Fatalf("load legacy config: created=%v err=%v", created, err)
	}
	if !loaded.Telnet.Enabled || loaded.Telnet.Host != "127.0.0.1" || loaded.Telnet.BasePort != 8000 {
		t.Fatalf("legacy Telnet defaults = %#v", loaded.Telnet)
	}
}

func TestLoadRejectsUnknownAndInvalidValues(t *testing.T) {
	tests := []string{
		`{"unexpected":true}`,
		`{"serial":{"baud_rate":0,"data_bits":8,"parity":"none","stop_bits":"1"},"http":{"address":"127.0.0.1:8080"},"tcp":{"host":"127.0.0.1","base_port":7000},"scan_interval_ms":2000}`,
	}
	for index, contents := range tests {
		path := filepath.Join(t.TempDir(), FileName)
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, _, err := LoadOrCreate(path); err == nil {
			t.Fatalf("case %d accepted invalid configuration", index)
		}
	}
}

func TestSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	value := Default()
	value.Serial.Ports = []string{"COM13"}
	value.Serial.BaudRate = 9600
	if err := Save(path, value); err != nil {
		t.Fatalf("save config: %v", err)
	}
	loaded, created, err := LoadOrCreate(path)
	if err != nil || created || !reflect.DeepEqual(loaded, value) {
		t.Fatalf("load saved config: created=%v value=%#v err=%v", created, loaded, err)
	}
}
