package main

import (
	"testing"
)

func TestSandboxID_Unique(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := sandboxID()
		if len(id) != 8 {
			t.Errorf("sandboxID length = %d, want 8", len(id))
		}
		if seen[id] {
			t.Errorf("duplicate sandbox ID after %d iterations: %s", i, id)
		}
		seen[id] = true
	}
}

func TestGetDotfilesPath(t *testing.T) {
	path := getDotfilesPath()
	// Should find the dotfiles directory (we're inside it)
	if path == "" {
		t.Skip("dotfiles path not found (expected in dev environment)")
	}
	// Path should be absolute
	if path[0] != '/' {
		t.Errorf("dotfiles path %q is not absolute", path)
	}
}

func TestGetSandbox_NotFound(t *testing.T) {
	_, err := getSandbox("nonexistent-id")
	if err == nil {
		t.Error("expected error for nonexistent sandbox")
	}
}

func TestSandboxFileExists(t *testing.T) {
	// File that exists
	if !sandboxFileExists("/dev/null") {
		t.Error("sandboxFileExists(/dev/null) = false, want true")
	}
	// File that doesn't
	if sandboxFileExists("/nonexistent/path/12345") {
		t.Error("sandboxFileExists(nonexistent) = true, want false")
	}
}

func TestDockerCmd_NotRunning(t *testing.T) {
	// Test with invalid container — should return error, not panic
	_, err := dockerCmd("inspect", "nonexistent-container-12345")
	if err == nil {
		t.Error("expected error for nonexistent container")
	}
}
