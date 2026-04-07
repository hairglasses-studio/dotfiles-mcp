package dotfiles

import (
	"testing"

	"github.com/hairglasses-studio/mcpkit/mcptest"
	"github.com/hairglasses-studio/mcpkit/registry"
)

func TestClipboardModuleRegistration(t *testing.T) {
	m := &ClipboardModule{}

	if m.Name() != "clipboard" {
		t.Fatalf("expected name clipboard, got %s", m.Name())
	}
	if m.Description() == "" {
		t.Fatal("expected non-empty description")
	}

	tools := m.Tools()
	if len(tools) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(tools))
	}

	reg := registry.NewToolRegistry()
	reg.RegisterModule(m)
	srv := mcptest.NewServer(t, reg)

	for _, want := range []string{
		"clipboard_read",
		"clipboard_write",
		"clipboard_read_image",
	} {
		if !srv.HasTool(want) {
			t.Errorf("missing tool: %s", want)
		}
	}
}
