package dotfiles

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/mcpkit/prompts"
	"github.com/hairglasses-studio/mcpkit/registry"
	"github.com/hairglasses-studio/mcpkit/resources"
)

func TestBuildDotfilesPromptRegistry(t *testing.T) {
	promptReg := buildDotfilesPromptRegistry()
	if promptReg.PromptCount() != 4 {
		t.Fatalf("expected 4 prompts, got %d", promptReg.PromptCount())
	}
	if _, ok := promptReg.GetPrompt("dotfiles_audit_fleet"); !ok {
		t.Fatal("expected dotfiles_audit_fleet prompt to be registered")
	}
}

func TestBuildDotfilesResourceRegistry(t *testing.T) {
	reg := registry.NewToolRegistry()
	promptReg := buildDotfilesPromptRegistry()
	resReg := buildDotfilesResourceRegistry(reg, promptReg)
	if resReg.ResourceCount() != 8 {
		t.Fatalf("expected 8 resources, got %d", resReg.ResourceCount())
	}
	if _, ok := resReg.GetResource("dotfiles://server/overview"); !ok {
		t.Fatal("expected dotfiles overview resource to be registered")
	}
}

func TestDotfilesOverviewResource(t *testing.T) {
	reg := registry.NewToolRegistry()
	promptReg := buildDotfilesPromptRegistry()
	resModule := &dotfilesResourceModule{reg: reg, promptReg: promptReg}

	var overview resources.ResourceDefinition
	for _, rd := range resModule.Resources() {
		if rd.Resource.URI == "dotfiles://server/overview" {
			overview = rd
			break
		}
	}

	out, err := overview.Handler(context.Background(), mcp.ReadResourceRequest{})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 resource contents entry, got %d", len(out))
	}
	text, ok := out[0].(mcp.TextResourceContents)
	if !ok {
		t.Fatalf("expected text resource contents, got %T", out[0])
	}
	if text.Text == "" {
		t.Fatal("expected non-empty overview text")
	}
}

func TestDotfilesPromptHandler(t *testing.T) {
	promptModule := &dotfilesPromptModule{}

	var pd prompts.PromptDefinition
	for _, candidate := range promptModule.Prompts() {
		if candidate.Prompt.Name == "dotfiles_triage_desktop" {
			pd = candidate
			break
		}
	}

	result, err := pd.Handler(context.Background(), mcp.GetPromptRequest{
		Params: mcp.GetPromptParams{
			Arguments: map[string]string{"symptom": "eww bar missing"},
		},
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || len(result.Messages) == 0 {
		t.Fatal("expected prompt result with messages")
	}
}
