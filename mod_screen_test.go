package main

import (
	"testing"
	"time"

	"github.com/hairglasses-studio/mcpkit/mcptest"
	"github.com/hairglasses-studio/mcpkit/registry"
)

// ---------------------------------------------------------------------------
// Screen helper tests
// ---------------------------------------------------------------------------

func TestScreenTimestamp_Format(t *testing.T) {
	ts := screenTimestamp()
	if len(ts) == 0 {
		t.Fatal("expected non-empty timestamp")
	}
	// Should be parseable with the same format
	_, err := time.Parse("20060102-150405", ts)
	if err != nil {
		t.Errorf("timestamp %q not parseable: %v", ts, err)
	}
}

func TestScreenCheckTool_GoExists(t *testing.T) {
	if err := screenCheckTool("go"); err != nil {
		t.Errorf("expected 'go' to exist on PATH: %v", err)
	}
}

func TestScreenCheckTool_Missing(t *testing.T) {
	err := screenCheckTool("this_tool_absolutely_does_not_exist_99999")
	if err == nil {
		t.Error("expected error for missing tool")
	}
}

// ---------------------------------------------------------------------------
// ScreenModule full registration with mcptest
// ---------------------------------------------------------------------------

func TestScreenModule_MCPTestServer(t *testing.T) {
	m := &ScreenModule{}
	reg := registry.NewToolRegistry()
	reg.RegisterModule(m)
	srv := mcptest.NewServer(t, reg)

	names := srv.ToolNames()
	if len(names) != 8 {
		t.Fatalf("expected 8 registered tools, got %d: %v", len(names), names)
	}
}
