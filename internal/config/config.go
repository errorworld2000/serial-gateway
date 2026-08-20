package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const FileName = "serial-gateway.json"

type Config struct {
	Serial         SerialConfig `json:"serial"`
	HTTP           HTTPConfig   `json:"http"`
	TCP            TCPConfig    `json:"tcp"`
	Telnet         TelnetConfig `json:"telnet"`
	ScanIntervalMS int          `json:"scan_interval_ms"`
}

type SerialConfig struct {
	Ports    []string `json:"ports"`
	BaudRate int      `json:"baud_rate"`
	DataBits int      `json:"data_bits"`
	Parity   string   `json:"parity"`
	StopBits string   `json:"stop_bits"`
}

type HTTPConfig struct {
	Address string `json:"address"`
}

type TCPConfig struct {
	Host          string `json:"host"`
	BasePort      int    `json:"base_port"`
	EscapeDelayMS int    `json:"escape_delay_ms"`
	NormalizeCRLF bool   `json:"normalize_crlf"`
}

type TelnetConfig struct {
	Enabled  bool   `json:"enabled"`
	Host     string `json:"host"`
	BasePort int    `json:"base_port"`
}

func Default() Config {
	return Config{
		Serial: SerialConfig{
			Ports:    []string{},
			BaudRate: 115200,
			DataBits: 8,
			Parity:   "none",
			StopBits: "1",
		},
		HTTP: HTTPConfig{Address: "127.0.0.1:8080"},
		TCP: TCPConfig{
			Host:          "127.0.0.1",
			BasePort:      7000,
			EscapeDelayMS: 5,
			NormalizeCRLF: true,
		},
		Telnet: TelnetConfig{
			Enabled:  true,
			Host:     "127.0.0.1",
			BasePort: 8000,
		},
		ScanIntervalMS: 2000,
	}
}

func PathNextToExecutable() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("find executable: %w", err)
	}
	return filepath.Join(filepath.Dir(executable), FileName), nil
}

func LoadOrCreate(path string) (Config, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		value := Default()
		if err := value.Validate(); err != nil {
			return Config{}, false, fmt.Errorf("invalid default configuration: %w", err)
		}
		data, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return Config{}, false, fmt.Errorf("encode default configuration: %w", err)
		}
		data = append(data, '\n')
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return Config{}, false, fmt.Errorf("create configuration %s: %w", path, err)
		}
		return value, true, nil
	}
	if err != nil {
		return Config{}, false, fmt.Errorf("read configuration %s: %w", path, err)
	}

	value := Default()
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return Config{}, false, fmt.Errorf("decode configuration %s: %w", path, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Config{}, false, fmt.Errorf("decode configuration %s: unexpected trailing content", path)
	}
	if err := value.Validate(); err != nil {
		return Config{}, false, fmt.Errorf("validate configuration %s: %w", path, err)
	}
	return value, false, nil
}

func Save(path string, value Config) error {
	if err := value.Validate(); err != nil {
		return fmt.Errorf("validate configuration: %w", err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode configuration: %w", err)
	}
	data = append(data, '\n')
	temporary, err := os.CreateTemp(filepath.Dir(path), ".serial-gateway-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary configuration: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary configuration: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync temporary configuration: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary configuration: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace configuration %s: %w", path, err)
	}
	return nil
}

func (c Config) Validate() error {
	if c.Serial.BaudRate <= 0 {
		return errors.New("serial.baud_rate must be positive")
	}
	if c.Serial.DataBits < 5 || c.Serial.DataBits > 8 {
		return errors.New("serial.data_bits must be 5, 6, 7, or 8")
	}
	switch strings.ToLower(c.Serial.Parity) {
	case "none", "n", "odd", "o", "even", "e", "mark", "m", "space", "s":
	default:
		return errors.New("serial.parity must be none, odd, even, mark, or space")
	}
	switch c.Serial.StopBits {
	case "1", "1.5", "2":
	default:
		return errors.New("serial.stop_bits must be 1, 1.5, or 2")
	}
	if err := validateAddress("http.address", c.HTTP.Address); err != nil {
		return err
	}
	if strings.TrimSpace(c.TCP.Host) == "" {
		return errors.New("tcp.host must not be empty")
	}
	if c.TCP.BasePort < 1 || c.TCP.BasePort > 65535 {
		return errors.New("tcp.base_port must be between 1 and 65535")
	}
	if c.TCP.EscapeDelayMS < 0 {
		return errors.New("tcp.escape_delay_ms must not be negative")
	}
	if strings.TrimSpace(c.Telnet.Host) == "" {
		return errors.New("telnet.host must not be empty")
	}
	if c.Telnet.BasePort < 1 || c.Telnet.BasePort > 65535 {
		return errors.New("telnet.base_port must be between 1 and 65535")
	}
	if c.ScanIntervalMS <= 0 {
		return errors.New("scan_interval_ms must be positive")
	}
	return nil
}

func (c Config) ScanInterval() time.Duration {
	return time.Duration(c.ScanIntervalMS) * time.Millisecond
}

func (c Config) EscapeDelay() time.Duration {
	return time.Duration(c.TCP.EscapeDelayMS) * time.Millisecond
}

func validateAddress(name, address string) error {
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("%s must be a host:port address: %w", name, err)
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return fmt.Errorf("%s port must be between 1 and 65535", name)
	}
	return nil
}
