package dotfiles

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/hairglasses-studio/mcpkit/registry"
)

// ---------------------------------------------------------------------------
// dotfiles_validate_config — extended table-driven tests
// ---------------------------------------------------------------------------

func TestValidateConfig_TableDriven(t *testing.T) {
	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_validate_config")

	tests := []struct {
		name      string
		content   string
		format    string
		wantValid bool
		wantError bool // expect IsError result (input validation failure)
	}{
		{
			name:      "valid TOML with nested tables",
			content:   "[server]\nhost = \"localhost\"\nport = 8080\n\n[database]\nurl = \"postgres://localhost/db\"",
			format:    "toml",
			wantValid: true,
		},
		{
			name:      "valid TOML array of tables",
			content:   "[[items]]\nname = \"a\"\n\n[[items]]\nname = \"b\"",
			format:    "toml",
			wantValid: true,
		},
		{
			name:      "invalid TOML missing bracket",
			content:   "[section\nkey = \"val\"",
			format:    "toml",
			wantValid: false,
		},
		{
			name:      "invalid TOML bad value",
			content:   "[section]\nkey = unquoted string",
			format:    "toml",
			wantValid: false,
		},
		{
			name:      "valid JSON nested",
			content:   `{"a":1,"b":{"c":[1,2,3]}}`,
			format:    "json",
			wantValid: true,
		},
		{
			name:      "valid JSON array",
			content:   `[1, 2, 3]`,
			format:    "json",
			wantValid: true,
		},
		{
			name:      "invalid JSON trailing comma",
			content:   `{"a": 1,}`,
			format:    "json",
			wantValid: false,
		},
		{
			name:      "invalid JSON unclosed brace",
			content:   `{"a": 1`,
			format:    "json",
			wantValid: false,
		},
		{
			name:      "unsupported format yaml",
			content:   "key: value",
			format:    "yaml",
			wantError: true,
		},
		{
			name:      "unsupported format xml",
			content:   "<root/>",
			format:    "xml",
			wantError: true,
		},
		{
			name:      "empty content",
			content:   "",
			format:    "toml",
			wantError: true,
		},
		{
			name:      "empty format",
			content:   "[section]\nkey = 1",
			format:    "",
			wantError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := registry.CallToolRequest{}
			req.Params.Arguments = map[string]any{
				"content": tc.content,
				"format":  tc.format,
			}

			result, err := td.Handler(context.Background(), req)
			if err != nil {
				t.Fatalf("handler returned error: %v", err)
			}

			if tc.wantError {
				if result == nil || !result.IsError {
					t.Fatal("expected IsError result for invalid input")
				}
				return
			}

			if result == nil {
				t.Fatal("result is nil")
			}
			if result.IsError {
				t.Fatalf("unexpected error result: %v", result)
			}

			out := unmarshalValidateOutput(t, result)
			if out.Valid != tc.wantValid {
				t.Errorf("valid = %v, want %v (error=%s)", out.Valid, tc.wantValid, out.Error)
			}
			if !tc.wantValid && out.Error == "" {
				t.Error("expected non-empty error for invalid content")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// dotfiles_list_configs — temp dir tests
// ---------------------------------------------------------------------------

func TestListConfigs_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOTFILES_DIR", dir)

	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_list_configs")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatal("expected successful result for empty dir")
	}

	text := extractText(t, result)
	var out ListConfigsOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Configs) != 0 {
		t.Errorf("expected 0 configs, got %d", len(out.Configs))
	}
}

func TestListConfigs_WithSubdirs(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOTFILES_DIR", dir)

	// Create subdirectories with config files.
	os.MkdirAll(filepath.Join(dir, "ghostty"), 0755)
	os.WriteFile(filepath.Join(dir, "ghostty", "config.toml"), []byte("[font]\nsize = 12"), 0644)

	os.MkdirAll(filepath.Join(dir, "nvim"), 0755)
	os.WriteFile(filepath.Join(dir, "nvim", "init.json"), []byte(`{}`), 0644)

	os.MkdirAll(filepath.Join(dir, "plain"), 0755)
	// No config files in this one — should be "unknown" format.

	// Hidden dir should be skipped.
	os.MkdirAll(filepath.Join(dir, ".hidden"), 0755)

	// File (not dir) should be skipped.
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# dotfiles"), 0644)

	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_list_configs")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatal("expected successful result")
	}

	text := extractText(t, result)
	var out ListConfigsOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(out.Configs) != 3 {
		t.Fatalf("expected 3 configs (ghostty, nvim, plain), got %d", len(out.Configs))
	}

	configMap := make(map[string]ConfigEntry)
	for _, c := range out.Configs {
		configMap[c.Name] = c
	}

	// Verify ghostty has toml format detected.
	if g, ok := configMap["ghostty"]; ok {
		if g.Format != "toml" {
			t.Errorf("ghostty format = %q, want toml", g.Format)
		}
		if g.Path != filepath.Join(dir, "ghostty") {
			t.Errorf("ghostty path = %q, want %q", g.Path, filepath.Join(dir, "ghostty"))
		}
	} else {
		t.Error("missing ghostty config")
	}

	// Verify nvim has json format.
	if n, ok := configMap["nvim"]; ok {
		if n.Format != "json" {
			t.Errorf("nvim format = %q, want json", n.Format)
		}
	} else {
		t.Error("missing nvim config")
	}

	// Verify plain has unknown format.
	if p, ok := configMap["plain"]; ok {
		if p.Format != "unknown" {
			t.Errorf("plain format = %q, want unknown", p.Format)
		}
	} else {
		t.Error("missing plain config")
	}

	// Verify hidden dir is not listed.
	if _, ok := configMap[".hidden"]; ok {
		t.Error("hidden directory should not be listed")
	}
}

func TestListConfigs_NonexistentDir(t *testing.T) {
	t.Setenv("DOTFILES_DIR", "/tmp/nonexistent-dotfiles-test-dir-abc123")

	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_list_configs")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	// Should produce an IsError result for missing dir.
	if result == nil || !result.IsError {
		t.Fatal("expected error result for nonexistent dotfiles dir")
	}
}

// ---------------------------------------------------------------------------
// dotfiles_check_symlinks — temp dir tests
// ---------------------------------------------------------------------------

func TestCheckSymlinks_TempDir(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	srcHealthy := filepath.Join(dir, "ghostty")
	dstHealthy := filepath.Join(home, ".config", "ghostty")
	srcMissing := filepath.Join(dir, "kitty")
	dstMissing := filepath.Join(home, ".config", "kitty")

	if err := os.MkdirAll(filepath.Dir(dstHealthy), 0o755); err != nil {
		t.Fatalf("mkdir healthy target dir: %v", err)
	}
	if err := os.WriteFile(srcHealthy, []byte("ghostty"), 0o644); err != nil {
		t.Fatalf("write healthy source: %v", err)
	}
	if err := os.Symlink(srcHealthy, dstHealthy); err != nil {
		t.Fatalf("symlink healthy target: %v", err)
	}

	writeTestInstallScript(t, dir, fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" != "--print-link-specs" ]]; then
  exit 2
fi
printf '%s|%s\n' %q %q
printf '%s|%s\n' %q %q
`, "%s", "%s", srcHealthy, dstHealthy, "%s", "%s", srcMissing, dstMissing))

	t.Setenv("DOTFILES_DIR", dir)
	t.Setenv("HOME", home)

	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_check_symlinks")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatal("expected successful result")
	}

	text := extractText(t, result)
	var out CheckSymlinksOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(out.Symlinks) == 0 {
		t.Fatal("expected non-empty symlinks list")
	}

	if len(out.Symlinks) != 2 {
		t.Fatalf("got %d symlinks, want 2", len(out.Symlinks))
	}
	if out.Symlinks[0].Status != "healthy" {
		t.Fatalf("healthy symlink status = %q, want healthy", out.Symlinks[0].Status)
	}
	if out.Symlinks[1].Status != "missing" {
		t.Fatalf("missing symlink status = %q, want missing", out.Symlinks[1].Status)
	}
}

func TestCheckSymlinks_InventoryFailure(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOTFILES_DIR", dir)

	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_check_symlinks")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected IsError result when install inventory is unavailable")
	}
}

// ---------------------------------------------------------------------------
// dotfiles_reload_service — input validation tests
// ---------------------------------------------------------------------------

func TestReloadService_EmptyService(t *testing.T) {
	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_reload_service")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"service": "",
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected error result for empty service")
	}
}

func TestReloadService_UnknownService(t *testing.T) {
	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_reload_service")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"service": "nonexistent-service-xyz",
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected error result for unknown service")
	}
}
