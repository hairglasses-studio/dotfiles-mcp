package main

import (
	"context"
	"testing"

	"github.com/hairglasses-studio/mcpkit/registry"
)

// ---------------------------------------------------------------------------
// hyprInstanceSig — env override
// ---------------------------------------------------------------------------

func TestHyprInstanceSig_FromEnv(t *testing.T) {
	t.Setenv("HYPRLAND_INSTANCE_SIGNATURE", "test-sig-12345")
	got := hyprInstanceSig()
	if got != "test-sig-12345" {
		t.Errorf("hyprInstanceSig() = %q, want test-sig-12345", got)
	}
}

// ---------------------------------------------------------------------------
// Input validation tests — these test error paths without needing Hyprland
// ---------------------------------------------------------------------------

func TestHyprFocusWindow_NoSelector(t *testing.T) {
	m := &HyprlandModule{}
	td := findHyprTool(t, m, "hypr_focus_window")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{}

	result, err := td.Handler(context.Background(), req)
	if err == nil && (result == nil || !result.IsError) {
		t.Error("expected error when neither address nor class is provided")
	}
}

func TestHyprTypeText_Empty(t *testing.T) {
	m := &HyprlandModule{}
	td := findHyprTool(t, m, "hypr_type_text")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{"text": ""}

	result, err := td.Handler(context.Background(), req)
	if err == nil && (result == nil || !result.IsError) {
		t.Error("expected error for empty text")
	}
}

func TestHyprKey_Empty(t *testing.T) {
	m := &HyprlandModule{}
	td := findHyprTool(t, m, "hypr_key")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{"keys": ""}

	result, err := td.Handler(context.Background(), req)
	if err == nil && (result == nil || !result.IsError) {
		t.Error("expected error for empty keys")
	}
}

func TestHyprSetMonitor_MissingName(t *testing.T) {
	m := &HyprlandModule{}
	td := findHyprTool(t, m, "hypr_set_monitor")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{"name": ""}

	result, err := td.Handler(context.Background(), req)
	if err == nil && (result == nil || !result.IsError) {
		t.Error("expected error for empty monitor name")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func findHyprTool(t *testing.T, m *HyprlandModule, name string) registry.ToolDefinition {
	t.Helper()
	for _, td := range m.Tools() {
		if td.Tool.Name == name {
			return td
		}
	}
	t.Fatalf("tool %q not found in HyprlandModule", name)
	return registry.ToolDefinition{}
}
