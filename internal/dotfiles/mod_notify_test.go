package dotfiles

import (
	"testing"

	"github.com/hairglasses-studio/mcpkit/mcptest"
	"github.com/hairglasses-studio/mcpkit/registry"
)

func TestNotifyModuleRegistration(t *testing.T) {
	m := &NotifyModule{}

	if m.Name() != "notify" {
		t.Fatalf("expected name notify, got %s", m.Name())
	}
	if m.Description() == "" {
		t.Fatal("expected non-empty description")
	}

	tools := m.Tools()
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}

	reg := registry.NewToolRegistry()
	reg.RegisterModule(m)
	srv := mcptest.NewServer(t, reg)

	if !srv.HasTool("notify_send") {
		t.Error("missing tool: notify_send")
	}
}
