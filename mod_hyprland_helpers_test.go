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
// hypr_screenshot_window — input validation
// ---------------------------------------------------------------------------

func TestHyprScreenshotWindow_NoSelector(t *testing.T) {
	m := &HyprlandModule{}
	td := findHyprTool(t, m, "hypr_screenshot_window")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Error("expected error result when neither address nor class is provided")
	}
}

// ---------------------------------------------------------------------------
// windowRegion — scale math
// ---------------------------------------------------------------------------

func TestWindowRegion(t *testing.T) {
	tests := []struct {
		name                   string
		x, y, w, h            int
		scale                  float64
		wantX, wantY, wantW, wantH int
	}{
		{
			name:  "scale 1x (no scaling)",
			x:     100, y: 200, w: 800, h: 600,
			scale: 1.0,
			wantX: 100, wantY: 200, wantW: 800, wantH: 600,
		},
		{
			name:  "scale 2x",
			x:     100, y: 200, w: 800, h: 600,
			scale: 2.0,
			wantX: 200, wantY: 400, wantW: 1600, wantH: 1200,
		},
		{
			name:  "scale 1.5x",
			x:     100, y: 200, w: 800, h: 600,
			scale: 1.5,
			wantX: 150, wantY: 300, wantW: 1200, wantH: 900,
		},
		{
			name:  "fractional rounding",
			x:     101, y: 203, w: 799, h: 601,
			scale: 1.333333,
			wantX: 135, wantY: 271, wantW: 1065, wantH: 801,
		},
		{
			name:  "zero position",
			x:     0, y: 0, w: 1920, h: 1080,
			scale: 2.0,
			wantX: 0, wantY: 0, wantW: 3840, wantH: 2160,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			px, py, pw, ph := windowRegion(tt.x, tt.y, tt.w, tt.h, tt.scale)
			if px != tt.wantX || py != tt.wantY || pw != tt.wantW || ph != tt.wantH {
				t.Errorf("windowRegion(%d,%d,%d,%d, %.2f) = (%d,%d,%d,%d), want (%d,%d,%d,%d)",
					tt.x, tt.y, tt.w, tt.h, tt.scale,
					px, py, pw, ph,
					tt.wantX, tt.wantY, tt.wantW, tt.wantH)
			}
		})
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
