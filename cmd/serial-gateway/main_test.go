package main

import (
	"testing"

	"go.bug.st/serial"
)

func TestParseParity(t *testing.T) {
	tests := map[string]serial.Parity{
		"none":  serial.NoParity,
		"N":     serial.NoParity,
		"odd":   serial.OddParity,
		"even":  serial.EvenParity,
		"mark":  serial.MarkParity,
		"space": serial.SpaceParity,
	}
	for input, want := range tests {
		got, err := parseParity(input)
		if err != nil || got != want {
			t.Errorf("parseParity(%q) = %v, %v; want %v", input, got, err, want)
		}
	}
	if _, err := parseParity("invalid"); err == nil {
		t.Fatal("parseParity accepted an invalid value")
	}
}

func TestParseStopBits(t *testing.T) {
	tests := map[string]serial.StopBits{
		"1":   serial.OneStopBit,
		"1.5": serial.OnePointFiveStopBits,
		"2":   serial.TwoStopBits,
	}
	for input, want := range tests {
		got, err := parseStopBits(input)
		if err != nil || got != want {
			t.Errorf("parseStopBits(%q) = %v, %v; want %v", input, got, err, want)
		}
	}
	if _, err := parseStopBits("3"); err == nil {
		t.Fatal("parseStopBits accepted an invalid value")
	}
}
