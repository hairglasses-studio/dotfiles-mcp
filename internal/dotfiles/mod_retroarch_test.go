package dotfiles

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hairglasses-studio/mcpkit/registry"
	"github.com/mark3labs/mcp-go/mcp"
)

// TestRetroArchModuleMetadata confirms the module exposes the expected
// three tools and carries the documented Name/Description.
func TestRetroArchModuleMetadata(t *testing.T) {
	m := &RetroArchModule{}
	if m.Name() != "retroarch" {
		t.Errorf("Name() = %q, want %q", m.Name(), "retroarch")
	}
	if !strings.Contains(m.Description(), "RetroArch") {
		t.Errorf("Description() missing 'RetroArch': %q", m.Description())
	}
	tools := m.Tools()
	names := make(map[string]bool, len(tools))
	for _, td := range tools {
		names[td.Tool.Name] = true
	}
	for _, want := range []string{"retroarch_audit", "retroarch_command", "retroarch_now_playing"} {
		if !names[want] {
			t.Errorf("missing tool: %q (got %v)", want, mapKeys(names))
		}
	}
}

// TestRetroArchCommandRejectsEmpty verifies the command handler
// returns an invalid-param error when `command` is empty.
func TestRetroArchCommandRejectsEmpty(t *testing.T) {
	m := &RetroArchModule{}
	var commandHandler registry.ToolHandlerFunc
	for _, td := range m.Tools() {
		if td.Tool.Name == "retroarch_command" {
			commandHandler = td.Handler
			break
		}
	}
	if commandHandler == nil {
		t.Fatal("retroarch_command handler not found")
	}
	req := registry.CallToolRequest{
		Params: mcp.CallToolParams{Arguments: map[string]any{"command": ""}},
	}
	res, err := commandHandler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if res == nil || !res.IsError {
		t.Errorf("expected IsError result for empty command, got %+v", res)
	}
}

// TestRetroArchNowPlayingHandlesMissingCache verifies the tool emits
// a well-formed JSON payload even when /tmp/bar-retroarch.txt does
// not exist — the ticker_label field should be empty, and
// retroarch_live should be false, but the tool must not fail.
func TestRetroArchNowPlayingHandlesMissingCache(t *testing.T) {
	// Stash any existing cache so we can test the missing-cache branch.
	cache := "/tmp/bar-retroarch.txt"
	saved := ""
	if data, err := os.ReadFile(cache); err == nil {
		saved = string(data)
		_ = os.Remove(cache)
	}
	defer func() {
		if saved != "" {
			_ = os.WriteFile(cache, []byte(saved), 0o644)
		}
	}()

	// Also point HOME at a tmpdir so content_history lookup misses cleanly.
	origHome := os.Getenv("HOME")
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	defer t.Setenv("HOME", origHome)

	m := &RetroArchModule{}
	var nowHandler registry.ToolHandlerFunc
	for _, td := range m.Tools() {
		if td.Tool.Name == "retroarch_now_playing" {
			nowHandler = td.Handler
			break
		}
	}
	if nowHandler == nil {
		t.Fatal("retroarch_now_playing handler not found")
	}

	res, err := nowHandler(context.Background(), registry.CallToolRequest{})
	if err != nil {
		t.Fatalf("handler err: %v", err)
	}
	if res == nil || len(res.Content) == 0 {
		t.Fatal("empty result")
	}
	tc, ok := res.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("unexpected content type: %T", res.Content[0])
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(tc.Text), &doc); err != nil {
		t.Fatalf("result is not JSON: %v  text=%s", err, tc.Text)
	}
	if label, _ := doc["ticker_label"].(string); label != "" {
		t.Errorf("expected empty ticker_label, got %q", label)
	}
	if live, _ := doc["retroarch_live"].(bool); live {
		t.Errorf("expected retroarch_live=false on missing cache, got true")
	}
	// Confirm history_path reflects the tmpdir HOME.
	if hp, _ := doc["history_path"].(string); !strings.HasPrefix(hp, tmp) {
		t.Errorf("history_path did not pick up HOME override: %q", hp)
	}
}

// TestRetroArchModuleRegistered confirms the module is wired into the
// bundle registration list in discovery.go via the same instance shape
// the rest of the modules use.
func TestRetroArchModuleRegistered(t *testing.T) {
	// Locate discovery.go and grep for our registration.
	root := os.Getenv("DOTFILES_DIR")
	if root == "" {
		if cwd, err := os.Getwd(); err == nil {
			// We're under mcp/dotfiles-mcp/ when running tests; walk up
			// two dirs to hit the repo root.
			root = filepath.Join(cwd, "..", "..")
		}
	}
	data, err := os.ReadFile(filepath.Join(root, "mcp", "dotfiles-mcp", "discovery.go"))
	if err != nil {
		t.Fatalf("read discovery.go: %v", err)
	}
	if !strings.Contains(string(data), "&RetroArchModule{}") {
		t.Error("discovery.go does not register &RetroArchModule{} in the bundle list")
	}
}

func mapKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
