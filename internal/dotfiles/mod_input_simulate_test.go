package main

import (
	"testing"
)

// ---------------------------------------------------------------------------
// ydotoolButtonCode
// ---------------------------------------------------------------------------

func TestYdotoolButtonCode(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"left", "0x110"},
		{"LEFT", "0x110"},
		{"Left", "0x110"},
		{"right", "0x111"},
		{"RIGHT", "0x111"},
		{"middle", "0x112"},
		{"MIDDLE", "0x112"},
		{"", "0x110"},        // default is left
		{"unknown", "0x110"}, // default is left
	}
	for _, tc := range tests {
		got := ydotoolButtonCode(tc.input)
		if got != tc.want {
			t.Errorf("ydotoolButtonCode(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// hasCmd
// ---------------------------------------------------------------------------

func TestHasCmd(t *testing.T) {
	// "go" should exist on any machine running go tests
	if !hasCmd("go") {
		t.Error("expected 'go' command to be available")
	}
	// An obviously nonexistent command
	if hasCmd("this_command_definitely_does_not_exist_12345") {
		t.Error("expected nonexistent command to return false")
	}
}
