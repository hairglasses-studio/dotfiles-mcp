package dotfiles

import "testing"

func TestBuildContractSnapshotBundle(t *testing.T) {
	bundle, err := BuildContractSnapshotBundle("default")
	if err != nil {
		t.Fatalf("BuildContractSnapshotBundle: %v", err)
	}

	if bundle.Overview.TotalTools == 0 {
		t.Fatal("expected non-zero tool count")
	}
	if bundle.Overview.ResourceCount == 0 {
		t.Fatal("expected non-zero resource count")
	}
	if bundle.Overview.PromptCount == 0 {
		t.Fatal("expected non-zero prompt count")
	}
	if bundle.Overview.Profile != "default" {
		t.Fatalf("profile = %q, want default", bundle.Overview.Profile)
	}

	seenJuhradial := false
	seenLegacyLogiops := false
	for _, tool := range bundle.Tools {
		switch tool.Name {
		case "input_get_juhradial_config":
			seenJuhradial = true
		case "input_get_logiops_config":
			seenLegacyLogiops = true
		}
	}
	if !seenJuhradial {
		t.Fatal("expected juhradial tool in contract snapshot")
	}
	if seenLegacyLogiops {
		t.Fatal("unexpected legacy logiops tool in contract snapshot")
	}
}

func TestBuildContractSnapshotBundle_DesktopProfile(t *testing.T) {
	bundle, err := BuildContractSnapshotBundle("desktop")
	if err != nil {
		t.Fatalf("BuildContractSnapshotBundle(desktop): %v", err)
	}

	if bundle.Overview.Profile != "desktop" {
		t.Fatalf("profile = %q, want desktop", bundle.Overview.Profile)
	}

	hyprDeferred := true
	btDeferred := false
	for _, tool := range bundle.Tools {
		switch tool.Name {
		case "hypr_list_windows":
			hyprDeferred = tool.Deferred
		case "bt_connect":
			btDeferred = tool.Deferred
		}
	}
	if hyprDeferred {
		t.Fatal("expected hypr_list_windows to be eager in desktop profile")
	}
	if !btDeferred {
		t.Fatal("expected bt_connect to remain deferred in desktop profile")
	}
}

func TestWellKnownManifestParity(t *testing.T) {
	bundle, err := BuildContractSnapshotBundle("default")
	if err != nil {
		t.Fatalf("BuildContractSnapshotBundle: %v", err)
	}

	if !bundle.Manifest.PublishMirror {
		t.Fatal("expected publish_mirror=true")
	}
	if bundle.Manifest.CanonicalSource != canonicalSourceURL {
		t.Fatalf("canonical_source = %q", bundle.Manifest.CanonicalSource)
	}
	if !bundle.Manifest.Capabilities.Tools || !bundle.Manifest.Capabilities.Resources || !bundle.Manifest.Capabilities.Prompts {
		t.Fatal("expected tools/resources/prompts capabilities to be true")
	}
	if bundle.Manifest.ToolCount != bundle.Overview.TotalTools {
		t.Fatalf("tool_count = %d, want %d", bundle.Manifest.ToolCount, bundle.Overview.TotalTools)
	}
	if bundle.Manifest.ResourceCount != bundle.Overview.ResourceCount {
		t.Fatalf("resource_count = %d, want %d", bundle.Manifest.ResourceCount, bundle.Overview.ResourceCount)
	}
	if bundle.Manifest.PromptCount != bundle.Overview.PromptCount {
		t.Fatalf("prompt_count = %d, want %d", bundle.Manifest.PromptCount, bundle.Overview.PromptCount)
	}
}
