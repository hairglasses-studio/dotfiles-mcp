package dotfiles

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/hairglasses-studio/mcpkit/registry"
)

// ---------------------------------------------------------------------------
// detectFormat
// ---------------------------------------------------------------------------

func TestDetectFormat(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"config.toml", "toml"},
		{"config.TOML", "toml"},
		{"settings.json", "json"},
		{"settings.jsonc", "json"},
		{"config.yaml", "yaml"},
		{"config.yml", "yaml"},
		{"config.ini", "ini"},
		{"config.conf", "ini"},
		{"Makefile", "unknown"},
		{"script.sh", "unknown"},
		{"", "unknown"},
		{"noext", "unknown"},
	}
	for _, tc := range tests {
		got := detectFormat(tc.path)
		if got != tc.want {
			t.Errorf("detectFormat(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// parseStringArray
// ---------------------------------------------------------------------------

func TestParseStringArray(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		want    []string
		wantErr bool
	}{
		{
			name:  "valid string array",
			input: []any{"a", "b", "c"},
			want:  []string{"a", "b", "c"},
		},
		{
			name:  "mixed types (non-strings skipped)",
			input: []any{"a", 42, "b", true},
			want:  []string{"a", "b"},
		},
		{
			name:  "empty array",
			input: []any{},
			want:  nil,
		},
		{
			name:    "nil input",
			input:   nil,
			wantErr: true,
		},
		{
			name:    "non-array input",
			input:   "not an array",
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseStringArray(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("index %d: got %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// dotfilesDir
// ---------------------------------------------------------------------------

func TestDotfilesDir_EnvOverride(t *testing.T) {
	t.Setenv("DOTFILES_DIR", "/tmp/test-dotfiles")
	got := dotfilesDir()
	if got != "/tmp/test-dotfiles" {
		t.Errorf("dotfilesDir() = %q, want /tmp/test-dotfiles", got)
	}
}

func TestDotfilesDir_Default(t *testing.T) {
	t.Setenv("DOTFILES_DIR", "")
	got := dotfilesDir()
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, "hairglasses-studio", "dotfiles")
	if got != want {
		t.Errorf("dotfilesDir() = %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// ValidateConfig handler -- additional edge cases
// ---------------------------------------------------------------------------

func TestValidateConfig_InvalidJSON(t *testing.T) {
	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_validate_config")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"content": `{"key": broken}`,
		"format":  "json",
	}

	result, err := td.Handler(nil, req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	out := unmarshalValidateOutput(t, result)
	if out.Valid {
		t.Error("expected valid=false for broken JSON")
	}
	if out.Error == "" {
		t.Error("expected non-empty error for broken JSON")
	}
}

func TestValidateConfig_EmptyContent(t *testing.T) {
	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_validate_config")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"content": "",
		"format":  "toml",
	}

	result, err := td.Handler(nil, req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	// Empty content should be rejected as invalid param
	if result == nil || !result.IsError {
		t.Fatal("expected error result for empty content")
	}
}

func TestValidateConfig_EmptyJSON(t *testing.T) {
	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_validate_config")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"content": "{}",
		"format":  "json",
	}

	result, err := td.Handler(nil, req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	out := unmarshalValidateOutput(t, result)
	if !out.Valid {
		t.Errorf("expected valid=true for empty JSON object, got error: %s", out.Error)
	}
}

// ---------------------------------------------------------------------------
// reloadCommands map
// ---------------------------------------------------------------------------

func TestReloadCommands_AllPresent(t *testing.T) {
	expected := []string{"hyprland", "ironbar", "mako", "waybar", "sway", "tmux"}
	for _, svc := range expected {
		if _, ok := reloadCommands[svc]; !ok {
			t.Errorf("missing reload command for %s", svc)
		}
	}
}

func writeTestInstallScript(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, "install.sh")
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write install.sh: %v", err)
	}
	return path
}

// ---------------------------------------------------------------------------
// Installer-backed link inventory
// ---------------------------------------------------------------------------

func TestParseLinkSpecsOutput(t *testing.T) {
	links, err := parseLinkSpecsOutput("/src/a|/dst/a\n/src/b|/dst/b\n")
	if err != nil {
		t.Fatalf("parseLinkSpecsOutput returned error: %v", err)
	}
	if len(links) != 2 {
		t.Fatalf("got %d links, want 2", len(links))
	}
	if links[0].src != "/src/a" || links[0].dst != "/dst/a" {
		t.Fatalf("unexpected first link: %+v", links[0])
	}
}

func TestParseLinkSpecsOutput_InvalidLine(t *testing.T) {
	if _, err := parseLinkSpecsOutput("/src-only-without-separator\n"); err == nil {
		t.Fatal("expected error for malformed line")
	}
}

func TestLoadManagedLinkSpecs_UsesInstallScript(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	src1 := filepath.Join(dir, "ghostty")
	src2 := filepath.Join(dir, "kitty")
	dst1 := filepath.Join(home, ".config/ghostty")
	dst2 := filepath.Join(home, ".config/kitty")

	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}

	writeTestInstallScript(t, dir, fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" != "--print-link-specs" ]]; then
  printf 'unexpected args\n' >&2
  exit 2
fi
printf '%s|%s\n' %q %q
printf '%s|%s\n' %q %q
`, "%s", "%s", src1, dst1, "%s", "%s", src2, dst2))

	t.Setenv("DOTFILES_DIR", dir)
	t.Setenv("HOME", home)

	links, err := loadManagedLinkSpecs()
	if err != nil {
		t.Fatalf("loadManagedLinkSpecs returned error: %v", err)
	}
	if len(links) != 2 {
		t.Fatalf("got %d links, want 2", len(links))
	}
	if links[0].src != src1 || links[0].dst != dst1 {
		t.Fatalf("unexpected first link: %+v", links[0])
	}
	if links[1].src != src2 || links[1].dst != dst2 {
		t.Fatalf("unexpected second link: %+v", links[1])
	}
}

// ---------------------------------------------------------------------------
// Module Name/Description for all modules
// ---------------------------------------------------------------------------

func TestAllModuleNameDescription(t *testing.T) {
	modules := dotfilesModules()
	if len(modules) == 0 {
		t.Fatal("dotfilesModules() returned empty list")
	}

	for _, m := range modules {
		name := m.Name()
		desc := m.Description()
		if name == "" {
			t.Error("module has empty name")
		}
		if desc == "" {
			t.Errorf("module %q has empty description", name)
		}
	}
}

// ---------------------------------------------------------------------------
// dotfilesModules exhaustive list
// ---------------------------------------------------------------------------

func TestDotfilesModules_Count(t *testing.T) {
	modules := dotfilesModules()
	if len(modules) < 18 {
		t.Errorf("expected at least 18 modules, got %d", len(modules))
	}

	names := make(map[string]bool)
	for _, m := range modules {
		names[m.Name()] = true
	}

	expected := []string{
		"dotfiles", "hyprland", "shader", "input", "bluetooth",
		"controller", "midi", "workflow", "oss",
		"mapping", "learn", "mapping_status", "mapping_daemon",
		"screen", "input_simulate", "claude_session", "prompt_registry",
	}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("missing module %q in dotfilesModules()", name)
		}
	}
}

// ---------------------------------------------------------------------------
// All module tools have valid structure (generic assertion)
// ---------------------------------------------------------------------------

func TestAllModuleToolsValid(t *testing.T) {
	modules := dotfilesModules()
	for _, m := range modules {
		t.Run(m.Name(), func(t *testing.T) {
			tools := m.Tools()
			if len(tools) == 0 {
				t.Fatalf("module %q has no tools", m.Name())
			}

			seen := make(map[string]bool)
			for _, td := range tools {
				if td.Tool.Name == "" {
					t.Error("tool has empty name")
				}
				if td.Tool.Description == "" {
					t.Errorf("tool %q has empty description", td.Tool.Name)
				}
				if td.Handler == nil {
					t.Errorf("tool %q has nil handler", td.Tool.Name)
				}
				if seen[td.Tool.Name] {
					t.Errorf("duplicate tool name: %s", td.Tool.Name)
				}
				seen[td.Tool.Name] = true
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Validate JSON round-trip for output types
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Input path helpers
// ---------------------------------------------------------------------------

func TestInputPathHelpers(t *testing.T) {
	t.Setenv("DOTFILES_DIR", "/tmp/test-dotfiles")
	if got := makimaDir(); got != "/tmp/test-dotfiles/makima" {
		t.Errorf("makimaDir() = %q", got)
	}
	if got := midiDir(); got != "/tmp/test-dotfiles/midi" {
		t.Errorf("midiDir() = %q", got)
	}
}

// ---------------------------------------------------------------------------
// Validate JSON round-trip for output types
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Input device regexes (from mod_input.go)
// ---------------------------------------------------------------------------

func TestDeviceRe(t *testing.T) {
	tests := []struct {
		input string
		match bool
		mac   string
		name  string
	}{
		{"Device AA:BB:CC:DD:EE:FF MX Master 4", true, "AA:BB:CC:DD:EE:FF", "MX Master 4"},
		{"Device 12:34:56:78:9a:bc My Device", true, "12:34:56:78:9a:bc", "My Device"},
		{"Not a device line", false, "", ""},
		{"", false, "", ""},
	}

	for _, tc := range tests {
		m := deviceRe.FindStringSubmatch(tc.input)
		if tc.match {
			if m == nil {
				t.Errorf("expected match for %q", tc.input)
				continue
			}
			if m[1] != tc.mac {
				t.Errorf("mac = %q, want %q", m[1], tc.mac)
			}
			if m[2] != tc.name {
				t.Errorf("name = %q, want %q", m[2], tc.name)
			}
		} else if m != nil {
			t.Errorf("unexpected match for %q", tc.input)
		}
	}
}

func TestMacRe(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"AA:BB:CC:DD:EE:FF", true},
		{"12:34:56:78:9a:bc", true},
		{"00:00:00:00:00:00", true},
		{"not-a-mac", false},
		{"AA:BB:CC:DD:EE", false},
		{"AA:BB:CC:DD:EE:FF:GG", false},
		{"", false},
	}
	for _, tc := range tests {
		got := macRe.MatchString(tc.input)
		if got != tc.want {
			t.Errorf("macRe.MatchString(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestBatteryRe(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Battery (85)", "85"},
		{"Battery (100)", "100"},
		{"Battery (0)", "0"},
		{"No battery", ""},
	}
	for _, tc := range tests {
		m := batteryRe.FindStringSubmatch(tc.input)
		if tc.want != "" {
			if m == nil || m[1] != tc.want {
				got := ""
				if m != nil {
					got = m[1]
				}
				t.Errorf("batteryRe on %q: got %q, want %q", tc.input, got, tc.want)
			}
		} else {
			if m != nil {
				t.Errorf("expected no match for %q, got %v", tc.input, m)
			}
		}
	}
}

func TestRiceCheckOutput_JSONRoundTrip(t *testing.T) {
	out := RiceCheckOutput{
		Compositor: "hyprland",
		Shader:     "none",
		Wallpaper:  "none",
		Services: []ServiceReloadStatus{
			{Service: "ironbar", Action: "running"},
		},
	}
	data, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded RiceCheckOutput
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Compositor != "hyprland" {
		t.Errorf("compositor = %q, want hyprland", decoded.Compositor)
	}
}
