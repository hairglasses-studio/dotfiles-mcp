package dotfiles

import (
	"testing"

	"github.com/hairglasses-studio/mcpkit/registry"
)

func TestBuildContractSnapshotBundle(t *testing.T) {
	bundle, err := BuildContractSnapshotBundle("default")
	if err != nil {
		t.Fatalf("BuildContractSnapshotBundle: %v", err)
	}

	if bundle.Overview.TotalTools == 0 {
		t.Fatal("expected non-zero tool count")
	}
	if bundle.Overview.TotalTools != len(bundle.Tools) {
		t.Fatalf("overview total_tools = %d, want %d", bundle.Overview.TotalTools, len(bundle.Tools))
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
	if bundle.Overview.PublishMirror {
		t.Fatal("expected canonical snapshot to report publish_mirror=false")
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

func TestBuildContractSnapshotBundleDesktopProfile(t *testing.T) {
	bundle, err := BuildContractSnapshotBundle("desktop")
	if err != nil {
		t.Fatalf("BuildContractSnapshotBundle(desktop): %v", err)
	}

	if bundle.Overview.Profile != "desktop" {
		t.Fatalf("profile = %q, want desktop", bundle.Overview.Profile)
	}

	hyprDeferred := true
	btDeferred := false
	desktopStatusDeferred := true
	for _, tool := range bundle.Tools {
		switch tool.Name {
		case "hypr_list_windows":
			hyprDeferred = tool.Deferred
		case "bt_connect":
			btDeferred = tool.Deferred
		case "dotfiles_desktop_status":
			desktopStatusDeferred = tool.Deferred
		}
	}
	if hyprDeferred {
		t.Fatal("expected hypr_list_windows to be eager in desktop profile")
	}
	if !btDeferred {
		t.Fatal("expected bt_connect to remain deferred in desktop profile")
	}
	if desktopStatusDeferred {
		t.Fatal("expected dotfiles_desktop_status to be eager in desktop profile")
	}
}

func TestBuildContractSnapshotBundleOpsProfile(t *testing.T) {
	bundle, err := BuildContractSnapshotBundle("ops")
	if err != nil {
		t.Fatalf("BuildContractSnapshotBundle(ops): %v", err)
	}

	if bundle.Overview.Profile != "ops" {
		t.Fatalf("profile = %q, want ops", bundle.Overview.Profile)
	}

	deferred := make(map[string]bool, len(bundle.Tools))
	for _, tool := range bundle.Tools {
		deferred[tool.Name] = tool.Deferred
	}

	if deferred["dotfiles_validate_config"] {
		t.Fatal("expected dotfiles_validate_config to be eager in ops profile")
	}
	if deferred["dotfiles_rice_check"] {
		t.Fatal("expected dotfiles_rice_check to be eager in ops profile")
	}
	if deferred["workflow_sync"] {
		t.Fatal("expected workflow_sync to be eager in ops profile")
	}
	if deferred["oss_score"] {
		t.Fatal("expected oss_score to be eager in ops profile")
	}
	if !deferred["hypr_list_windows"] {
		t.Fatal("expected hypr_list_windows to remain deferred in ops profile")
	}
	if !deferred["shader_list"] {
		t.Fatal("expected shader_list to remain deferred in ops profile")
	}
}

func TestBuildContractSnapshotBundleFrontDoorParity(t *testing.T) {
	bundle, err := BuildContractSnapshotBundle("default")
	if err != nil {
		t.Fatalf("BuildContractSnapshotBundle(default): %v", err)
	}

	reg := registry.NewToolRegistry()
	promptReg := buildDotfilesPromptRegistry()
	resReg := buildDotfilesResourceRegistry(reg, promptReg)

	if len(bundle.Resources) != resReg.ResourceCount() {
		t.Fatalf("bundle resources = %d, want %d", len(bundle.Resources), resReg.ResourceCount())
	}
	if len(bundle.Templates) != resReg.TemplateCount() {
		t.Fatalf("bundle templates = %d, want %d", len(bundle.Templates), resReg.TemplateCount())
	}
	if len(bundle.Prompts) != promptReg.PromptCount() {
		t.Fatalf("bundle prompts = %d, want %d", len(bundle.Prompts), promptReg.PromptCount())
	}
	if bundle.Overview.ResourceCount != len(bundle.Resources)+len(bundle.Templates) {
		t.Fatalf("overview resource_count = %d, want %d", bundle.Overview.ResourceCount, len(bundle.Resources)+len(bundle.Templates))
	}
	if bundle.Overview.TemplateCount != len(bundle.Templates) {
		t.Fatalf("overview template_count = %d, want %d", bundle.Overview.TemplateCount, len(bundle.Templates))
	}
	if bundle.Overview.PromptCount != len(bundle.Prompts) {
		t.Fatalf("overview prompt_count = %d, want %d", bundle.Overview.PromptCount, len(bundle.Prompts))
	}

	resourceSet := make(map[string]struct{}, len(bundle.Resources))
	for _, resource := range bundle.Resources {
		resourceSet[resource.URI] = struct{}{}
	}
	templateSet := make(map[string]struct{}, len(bundle.Templates))
	for _, template := range bundle.Templates {
		templateSet[template.URITemplate] = struct{}{}
	}
	promptSet := make(map[string]struct{}, len(bundle.Prompts))
	for _, prompt := range bundle.Prompts {
		promptSet[prompt.Name] = struct{}{}
	}

	for _, rd := range resReg.GetAllResourceDefinitions() {
		if _, ok := resourceSet[rd.Resource.URI]; !ok {
			t.Fatalf("resource %q missing from contract bundle", rd.Resource.URI)
		}
	}
	for _, td := range resReg.GetAllTemplateDefinitions() {
		if _, ok := templateSet[td.Template.URITemplate.Raw()]; !ok {
			t.Fatalf("template %q missing from contract bundle", td.Template.URITemplate.Raw())
		}
	}
	for _, pd := range promptReg.GetAllPromptDefinitions() {
		if _, ok := promptSet[pd.Prompt.Name]; !ok {
			t.Fatalf("prompt %q missing from contract bundle", pd.Prompt.Name)
		}
	}

	for _, workflow := range dotfilesWorkflowCatalog() {
		if workflow.ResourceURI != "" {
			if _, ok := resourceSet[workflow.ResourceURI]; !ok {
				t.Fatalf("workflow %q resource %q missing from contract bundle", workflow.Name, workflow.ResourceURI)
			}
		}
		if workflow.PromptName != "" {
			if _, ok := promptSet[workflow.PromptName]; !ok {
				t.Fatalf("workflow %q prompt %q missing from contract bundle", workflow.Name, workflow.PromptName)
			}
		}
	}
}

func TestCanonicalWellKnownManifestParity(t *testing.T) {
	bundle, err := BuildContractSnapshotBundle("default")
	if err != nil {
		t.Fatalf("BuildContractSnapshotBundle: %v", err)
	}

	if bundle.Manifest.PublishMirror {
		t.Fatal("expected publish_mirror=false")
	}
	if bundle.Manifest.CanonicalSource != canonicalSourceURL {
		t.Fatalf("canonical_source = %q", bundle.Manifest.CanonicalSource)
	}
	if bundle.Manifest.Repository != "https://github.com/hairglasses-studio/dotfiles" {
		t.Fatalf("repository = %q", bundle.Manifest.Repository)
	}
	if !bundle.Manifest.Capabilities.Tools || !bundle.Manifest.Capabilities.Resources || !bundle.Manifest.Capabilities.Prompts {
		t.Fatal("expected tools/resources/prompts capabilities to be true")
	}
	if bundle.Manifest.ToolCount != bundle.Overview.TotalTools {
		t.Fatalf("tool_count = %d, want %d", bundle.Manifest.ToolCount, bundle.Overview.TotalTools)
	}
	if bundle.Manifest.ToolCount != len(bundle.Tools) {
		t.Fatalf("manifest tool_count = %d, want %d", bundle.Manifest.ToolCount, len(bundle.Tools))
	}
	if bundle.Manifest.ResourceCount != bundle.Overview.ResourceCount {
		t.Fatalf("resource_count = %d, want %d", bundle.Manifest.ResourceCount, bundle.Overview.ResourceCount)
	}
	if bundle.Manifest.PromptCount != bundle.Overview.PromptCount {
		t.Fatalf("prompt_count = %d, want %d", bundle.Manifest.PromptCount, bundle.Overview.PromptCount)
	}
}
