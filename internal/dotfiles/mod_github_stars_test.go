package dotfiles

import (
	"testing"

	"github.com/hairglasses-studio/mcpkit/mcptest"
	"github.com/hairglasses-studio/mcpkit/registry"
)

func TestGitHubStarsModuleRegistration(t *testing.T) {
	m := &GitHubStarsModule{}

	if m.Name() != "github_stars" {
		t.Fatalf("expected name github_stars, got %s", m.Name())
	}

	tools := m.Tools()
	if len(tools) != 14 {
		t.Fatalf("expected 14 github stars tools, got %d", len(tools))
	}

	reg := registry.NewToolRegistry()
	reg.RegisterModule(m)
	srv := mcptest.NewServer(t, reg)

	for _, want := range []string{
		"gh_stars_list",
		"gh_stars_summary",
		"gh_star_lists_list",
		"gh_star_lists_ensure",
		"gh_star_lists_rename",
		"gh_star_lists_delete",
		"gh_stars_set",
		"gh_star_membership_set",
		"gh_stars_taxonomy_suggest",
		"gh_stars_cleanup_candidates",
		"gh_stars_taxonomy_audit",
		"gh_stars_taxonomy_sync",
		"gh_stars_bootstrap",
		"gh_stars_install_codex_mcp",
	} {
		if !srv.HasTool(want) {
			t.Errorf("missing tool: %s", want)
		}
	}
}
