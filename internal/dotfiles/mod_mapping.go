// mod_mapping.go — Unified controller mapping MCP tools.
package dotfiles

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/hairglasses-studio/mapping"
	"github.com/hairglasses-studio/mcpkit/handler"
	"github.com/hairglasses-studio/mcpkit/registry"
)

// ===========================================================================
// I/O types
// ===========================================================================

// ── List ──

type MappingListInput struct {
	Format string `json:"format,omitempty" jsonschema:"description=Filter by format,enum=unified,enum=legacy"`
	Device string `json:"device,omitempty" jsonschema:"description=Filter by device name substring"`
	Tag    string `json:"tag,omitempty" jsonschema:"description=Filter by tag"`
}

type MappingListOutput struct {
	Profiles []mapping.MappingProfileSummary `json:"profiles"`
	Total    int                             `json:"total"`
}

// ── Get ──

type MappingGetInput struct {
	Name string `json:"name" jsonschema:"required,description=Profile name (without .toml extension)"`
}

type MappingGetOutput struct {
	Profile *mapping.MappingProfile `json:"profile"`
	Format  string                  `json:"format"`
	Raw     string                  `json:"raw"`
}

// ── Set ──

type MappingSetInput struct {
	Name    string `json:"name" jsonschema:"required,description=Profile name (without .toml extension)"`
	Content string `json:"content" jsonschema:"required,description=Full TOML content for the mapping profile"`
}

type MappingSetOutput struct {
	Written string                    `json:"written"`
	Valid   bool                      `json:"valid"`
	Issues  []mapping.ValidationIssue `json:"issues,omitempty"`
}

// ── Delete ──

type MappingDeleteInput struct {
	Name string `json:"name" jsonschema:"required,description=Profile name to delete"`
}

type MappingDeleteOutput struct {
	Deleted string `json:"deleted"`
}

// ── Validate ──

type MappingValidateInput struct {
	Content string `json:"content,omitempty" jsonschema:"description=TOML content to validate (mutually exclusive with name)"`
	Name    string `json:"name,omitempty" jsonschema:"description=Profile name to validate from disk"`
}

type MappingValidateOutput struct {
	Valid        bool                      `json:"valid"`
	Format       string                    `json:"format"`
	MappingCount int                       `json:"mapping_count"`
	DeviceName   string                    `json:"device_name,omitempty"`
	Issues       []mapping.ValidationIssue `json:"issues"`
}

// ── Migrate Legacy ──

type MappingMigrateInput struct {
	Name   string `json:"name,omitempty" jsonschema:"description=Legacy profile name to migrate. If empty, lists all legacy profiles."`
	DryRun bool   `json:"dry_run,omitempty" jsonschema:"description=Preview migration without writing. Defaults to true (safe). Pass false to actually execute the migration."`
}

type MigrationResult struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // "migrated", "already_unified", "dry_run", "error"
	Preview string `json:"preview,omitempty"`
	Written string `json:"written,omitempty"`
	Error   string `json:"error,omitempty"`
}

type MappingMigrateOutput struct {
	Results []MigrationResult `json:"results"`
	DryRun  bool              `json:"dry_run"`
}

// ── Resolve Test ──

type MappingResolveTestInput struct {
	Name     string `json:"name" jsonschema:"required,description=Profile name to test resolution against"`
	Source   string `json:"source" jsonschema:"required,description=Input source to simulate (e.g. BTN_SOUTH or midi:cc:1)"`
	App      string `json:"app,omitempty" jsonschema:"description=Window class context for resolution"`
	DeviceID string `json:"device_id,omitempty" jsonschema:"description=Device ID for layer-aware testing"`
	Layer    int    `json:"layer,omitempty" jsonschema:"description=Active layer for the device (0 = default)"`
}

type MappingResolveTestOutput struct {
	Matched     bool                 `json:"matched"`
	Rule        *mapping.MappingRule `json:"rule,omitempty"`
	Context     string               `json:"context"` // "default", "app_override"
	Description string               `json:"description,omitempty"`
}

// ── Generate ──

type MappingGenerateInput struct {
	DeviceName string `json:"device_name" jsonschema:"required,description=Device name for the profile"`
	Template   string `json:"template" jsonschema:"required,description=Template to generate from,enum=desktop,enum=claude-code,enum=gaming,enum=media,enum=macropad,enum=volume-mixer,enum=shader-control"`
	AppClass   string `json:"app_class,omitempty" jsonschema:"description=App class for per-app profile"`
}

type MappingGenerateOutput struct {
	Content      string `json:"content"`
	PreviewPath  string `json:"preview_path"`
	MappingCount int    `json:"mapping_count"`
}

// ── Export ──

type MappingExportInput struct {
	Name     string `json:"name" jsonschema:"required,description=Profile name to export"`
	Sanitize bool   `json:"sanitize,omitempty" jsonschema:"description=Strip local paths and user-specific data (default false)"`
}

type MappingExportOutput struct {
	Name    string `json:"name"`
	Content string `json:"content"`
	Format  string `json:"format"`
	Size    int    `json:"size_bytes"`
}

// ── Import ──

type MappingImportInput struct {
	Content string `json:"content" jsonschema:"required,description=TOML content to import"`
	Name    string `json:"name,omitempty" jsonschema:"description=Override profile name (default: derived from content)"`
	Force   bool   `json:"force,omitempty" jsonschema:"description=Overwrite existing profile (default false)"`
	DryRun  bool   `json:"dry_run,omitempty" jsonschema:"description=Validate without writing (default false)"`
}

type MappingImportOutput struct {
	Valid   bool                      `json:"valid"`
	Written string                    `json:"written,omitempty"`
	Name    string                    `json:"name"`
	Format  string                    `json:"format"`
	Issues  []mapping.ValidationIssue `json:"issues,omitempty"`
	DryRun  bool                      `json:"dry_run"`
	Exists  bool                      `json:"exists,omitempty"`
}

// ── Diff ──

type MappingDiffInput struct {
	Name1 string `json:"name1" jsonschema:"required,description=First profile name"`
	Name2 string `json:"name2" jsonschema:"required,description=Second profile name"`
}

type MappingDiffEntry struct {
	Input  string `json:"input"`
	Status string `json:"status"` // "added", "removed", "changed", "unchanged"
	Left   string `json:"left,omitempty"`
	Right  string `json:"right,omitempty"`
}

type MappingDiffOutput struct {
	Name1   string             `json:"name1"`
	Name2   string             `json:"name2"`
	Added   int                `json:"added"`
	Removed int                `json:"removed"`
	Changed int                `json:"changed"`
	Entries []MappingDiffEntry `json:"entries"`
}

// ===========================================================================
// Module
// ===========================================================================

// MappingEngineModule provides MCP tools for unified controller mapping management.
type MappingEngineModule struct{}

func (m *MappingEngineModule) Name() string { return "mapping" }
func (m *MappingEngineModule) Description() string {
	return "Unified controller mapping engine: profile management, validation, migration, and generation"
}

func (m *MappingEngineModule) Tools() []registry.ToolDefinition {
	return []registry.ToolDefinition{
		// ── mapping_list_profiles ──
		handler.TypedHandler[MappingListInput, MappingListOutput](
			"mapping_list_profiles",
			"List all mapping profiles with device info, format, and mapping counts. Supports filtering by format (unified/legacy), device name, and tag.",
			func(_ context.Context, input MappingListInput) (MappingListOutput, error) {
				profiles, err := listMappingProfiles()
				if err != nil {
					return MappingListOutput{}, fmt.Errorf("list profiles: %w", err)
				}

				var filtered []mapping.MappingProfileSummary
				for _, p := range profiles {
					if input.Format != "" && p.Format != input.Format {
						continue
					}
					if input.Device != "" && !strings.Contains(strings.ToLower(p.DeviceName), strings.ToLower(input.Device)) {
						continue
					}
					if input.Tag != "" {
						found := false
						for _, t := range p.Tags {
							if strings.EqualFold(t, input.Tag) {
								found = true
								break
							}
						}
						if !found {
							continue
						}
					}
					filtered = append(filtered, p)
				}

				return MappingListOutput{
					Profiles: filtered,
					Total:    len(filtered),
				}, nil
			},
		),

		// ── mapping_get_profile ──
		handler.TypedHandler[MappingGetInput, MappingGetOutput](
			"mapping_get_profile",
			"Read a mapping profile with parsed structure and raw TOML content.",
			func(_ context.Context, input MappingGetInput) (MappingGetOutput, error) {
				if input.Name == "" {
					return MappingGetOutput{}, fmt.Errorf("[%s] name is required", handler.ErrInvalidParam)
				}
				path := resolveMappingPath(input.Name)
				raw, err := os.ReadFile(path)
				if err != nil {
					return MappingGetOutput{}, fmt.Errorf("[%s] profile %q not found: %w", handler.ErrNotFound, input.Name, err)
				}
				p, err := mapping.ParseMappingProfile(string(raw), path)
				if err != nil {
					return MappingGetOutput{}, fmt.Errorf("parse profile: %w", err)
				}
				format := "legacy"
				if p.IsUnifiedFormat() {
					format = "unified"
				}
				return MappingGetOutput{
					Profile: p,
					Format:  format,
					Raw:     string(raw),
				}, nil
			},
		),

		// ── mapping_set_profile ──
		handler.TypedHandler[MappingSetInput, MappingSetOutput](
			"mapping_set_profile",
			"Create or update a mapping profile. Validates TOML syntax and schema before writing.",
			func(_ context.Context, input MappingSetInput) (MappingSetOutput, error) {
				if input.Name == "" {
					return MappingSetOutput{}, fmt.Errorf("[%s] name is required", handler.ErrInvalidParam)
				}
				if input.Content == "" {
					return MappingSetOutput{}, fmt.Errorf("[%s] content is required", handler.ErrInvalidParam)
				}

				// Validate TOML syntax.
				var raw map[string]any
				if _, err := toml.Decode(input.Content, &raw); err != nil {
					return MappingSetOutput{Valid: false, Issues: []mapping.ValidationIssue{{
						Severity: "error",
						Field:    "content",
						Message:  fmt.Sprintf("invalid TOML: %v", err),
					}}}, nil
				}

				// Parse and validate semantically.
				p, err := mapping.ParseMappingProfile(input.Content, input.Name+".toml")
				if err != nil {
					return MappingSetOutput{Valid: false, Issues: []mapping.ValidationIssue{{
						Severity: "error",
						Field:    "content",
						Message:  err.Error(),
					}}}, nil
				}

				issues := mapping.ValidateProfile(p)
				hasErrors := false
				for _, issue := range issues {
					if issue.Severity == "error" {
						hasErrors = true
						break
					}
				}
				if hasErrors {
					return MappingSetOutput{Valid: false, Issues: issues}, nil
				}

				// Write to makima directory.
				path := resolveMappingPath(input.Name)
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					return MappingSetOutput{}, fmt.Errorf("create directory: %w", err)
				}
				if err := os.WriteFile(path, []byte(input.Content), 0644); err != nil {
					return MappingSetOutput{}, fmt.Errorf("write profile: %w", err)
				}

				return MappingSetOutput{
					Written: path,
					Valid:   true,
					Issues:  issues,
				}, nil
			},
		),

		// ── mapping_delete_profile ──
		handler.TypedHandler[MappingDeleteInput, MappingDeleteOutput](
			"mapping_delete_profile",
			"Delete a mapping profile from disk.",
			func(_ context.Context, input MappingDeleteInput) (MappingDeleteOutput, error) {
				if input.Name == "" {
					return MappingDeleteOutput{}, fmt.Errorf("[%s] name is required", handler.ErrInvalidParam)
				}
				path := resolveMappingPath(input.Name)
				if _, err := os.Stat(path); os.IsNotExist(err) {
					return MappingDeleteOutput{}, fmt.Errorf("[%s] profile %q not found", handler.ErrNotFound, input.Name)
				}
				if err := os.Remove(path); err != nil {
					return MappingDeleteOutput{}, fmt.Errorf("delete profile: %w", err)
				}
				return MappingDeleteOutput{Deleted: path}, nil
			},
		),

		// ── mapping_validate ──
		handler.TypedHandler[MappingValidateInput, MappingValidateOutput](
			"mapping_validate",
			"Validate a mapping profile for schema compliance. Provide either content (TOML string) or name (profile on disk).",
			func(_ context.Context, input MappingValidateInput) (MappingValidateOutput, error) {
				var content string
				if input.Content != "" {
					content = input.Content
				} else if input.Name != "" {
					path := resolveMappingPath(input.Name)
					data, err := os.ReadFile(path)
					if err != nil {
						return MappingValidateOutput{}, fmt.Errorf("[%s] profile %q not found: %w", handler.ErrNotFound, input.Name, err)
					}
					content = string(data)
				} else {
					return MappingValidateOutput{}, fmt.Errorf("[%s] provide either content or name", handler.ErrInvalidParam)
				}

				p, err := mapping.ParseMappingProfile(content, input.Name+".toml")
				if err != nil {
					return MappingValidateOutput{
						Valid: false,
						Issues: []mapping.ValidationIssue{{
							Severity: "error",
							Field:    "content",
							Message:  err.Error(),
						}},
					}, nil
				}

				issues := mapping.ValidateProfile(p)
				hasErrors := false
				for _, issue := range issues {
					if issue.Severity == "error" {
						hasErrors = true
						break
					}
				}

				format := "legacy"
				if p.IsUnifiedFormat() {
					format = "unified"
				}

				return MappingValidateOutput{
					Valid:        !hasErrors,
					Format:       format,
					MappingCount: p.MappingCount(),
					DeviceName:   p.DeviceName(),
					Issues:       issues,
				}, nil
			},
		),

		// ── mapping_migrate_legacy ──
		handler.TypedHandler[MappingMigrateInput, MappingMigrateOutput](
			"mapping_migrate_legacy",
			"Convert legacy makima profiles to the unified mapping format. Dry-run by default.",
			func(_ context.Context, input MappingMigrateInput) (MappingMigrateOutput, error) {
				// dry_run defaults to true (safe). Pass dry_run:false to execute.
				// Go zero-value false → !false → true (dry-run). Explicit false → execute.
				dryRun := !input.DryRun

				profiles, err := listMappingProfiles()
				if err != nil {
					return MappingMigrateOutput{}, fmt.Errorf("list profiles: %w", err)
				}

				var results []MigrationResult
				for _, summary := range profiles {
					if input.Name != "" && summary.Name != input.Name {
						continue
					}
					if summary.Format != "legacy" {
						results = append(results, MigrationResult{
							Name:   summary.Name,
							Status: "already_unified",
						})
						continue
					}

					p, err := mapping.LoadMappingProfile(summary.Path)
					if err != nil {
						results = append(results, MigrationResult{
							Name:   summary.Name,
							Status: "error",
							Error:  err.Error(),
						})
						continue
					}

					unified := mapping.ConvertLegacyToUnified(p)

					var buf bytes.Buffer
					enc := toml.NewEncoder(&buf)
					if err := enc.Encode(unified); err != nil {
						results = append(results, MigrationResult{
							Name:   summary.Name,
							Status: "error",
							Error:  fmt.Sprintf("encode unified: %v", err),
						})
						continue
					}

					if dryRun {
						results = append(results, MigrationResult{
							Name:    summary.Name,
							Status:  "dry_run",
							Preview: buf.String(),
						})
					} else {
						outPath := summary.Path
						if err := os.WriteFile(outPath, buf.Bytes(), 0644); err != nil {
							results = append(results, MigrationResult{
								Name:   summary.Name,
								Status: "error",
								Error:  fmt.Sprintf("write: %v", err),
							})
							continue
						}
						results = append(results, MigrationResult{
							Name:    summary.Name,
							Status:  "migrated",
							Written: outPath,
						})
					}
				}

				return MappingMigrateOutput{
					Results: results,
					DryRun:  dryRun,
				}, nil
			},
		),

		// ── mapping_resolve_test ──
		handler.TypedHandler[MappingResolveTestInput, MappingResolveTestOutput](
			"mapping_resolve_test",
			"Simulate rule resolution: given an input source and app context, show which mapping rule would fire.",
			func(_ context.Context, input MappingResolveTestInput) (MappingResolveTestOutput, error) {
				if input.Name == "" || input.Source == "" {
					return MappingResolveTestOutput{}, fmt.Errorf("[%s] name and source are required", handler.ErrInvalidParam)
				}

				path := resolveMappingPath(input.Name)
				p, err := mapping.LoadMappingProfile(path)
				if err != nil {
					return MappingResolveTestOutput{}, fmt.Errorf("[%s] profile %q: %w", handler.ErrNotFound, input.Name, err)
				}

				// Convert legacy if needed for resolution.
				if p.IsLegacyFormat() {
					p = mapping.ConvertLegacyToUnified(p)
				}

				idx := mapping.BuildRuleIndex(p)
				state := mapping.NewEngineState()
				state.ActiveApp = input.App

				if input.DeviceID != "" && input.Layer != 0 {
					state.SetActiveLayer(input.DeviceID, input.Layer)
				}
				rule := idx.Resolve(input.Source, state, input.DeviceID)
				if rule == nil {
					return MappingResolveTestOutput{
						Matched: false,
						Context: resolveContext(input.App),
					}, nil
				}

				return MappingResolveTestOutput{
					Matched:     true,
					Rule:        rule,
					Context:     resolveContext(input.App),
					Description: rule.Description,
				}, nil
			},
		),

		// ── mapping_generate ──
		handler.TypedHandler[MappingGenerateInput, MappingGenerateOutput](
			"mapping_generate",
			"Generate a mapping profile from a built-in template for a specified device.",
			func(_ context.Context, input MappingGenerateInput) (MappingGenerateOutput, error) {
				if input.DeviceName == "" || input.Template == "" {
					return MappingGenerateOutput{}, fmt.Errorf("[%s] device_name and template are required", handler.ErrInvalidParam)
				}

				// Check for controller templates first (from existing mod_input.go).
				if tmpl, ok := controllerTemplates[input.Template]; ok {
					content := fmt.Sprintf("# Generated from template: %s\n# Device: %s\n\n%s",
						input.Template, input.DeviceName, tmpl)
					name := input.DeviceName
					if input.AppClass != "" {
						name += "::" + input.AppClass
					}
					path := filepath.Join(makimaDir(), name+".toml")
					return MappingGenerateOutput{
						Content:      content,
						PreviewPath:  path,
						MappingCount: countTOMLMappings(content),
					}, nil
				}

				// Check MIDI templates.
				if tmpl, ok := midiTemplates[input.Template]; ok {
					content := fmt.Sprintf("# Generated from template: %s\n# Device: %s\n\n%s",
						input.Template, input.DeviceName, tmpl)
					name := input.DeviceName
					path := filepath.Join(midiDir(), name+".toml")
					return MappingGenerateOutput{
						Content:      content,
						PreviewPath:  path,
						MappingCount: countTOMLMappings(content),
					}, nil
				}

				return MappingGenerateOutput{}, fmt.Errorf("[%s] unknown template: %q", handler.ErrInvalidParam, input.Template)
			},
		),

		// ── mapping_export ──
		handler.TypedHandler[MappingExportInput, MappingExportOutput](
			"mapping_export",
			"Export a mapping profile for sharing. Optionally sanitize to strip local paths.",
			func(_ context.Context, input MappingExportInput) (MappingExportOutput, error) {
				if input.Name == "" {
					return MappingExportOutput{}, fmt.Errorf("[%s] name is required", handler.ErrInvalidParam)
				}
				path := resolveMappingPath(input.Name)
				data, err := os.ReadFile(path)
				if err != nil {
					return MappingExportOutput{}, fmt.Errorf("[%s] profile %q not found: %w", handler.ErrNotFound, input.Name, err)
				}

				content := string(data)
				if input.Sanitize {
					content = sanitizeProfile(content)
				}

				format := "legacy"
				if strings.Contains(content, "[profile]") {
					format = "unified"
				}

				return MappingExportOutput{
					Name:    input.Name,
					Content: content,
					Format:  format,
					Size:    len(content),
				}, nil
			},
		),

		// ── mapping_import ──
		handler.TypedHandler[MappingImportInput, MappingImportOutput](
			"mapping_import",
			"Import a mapping profile from TOML content. Validates before writing. Dry-run mode available.",
			func(_ context.Context, input MappingImportInput) (MappingImportOutput, error) {
				if input.Content == "" {
					return MappingImportOutput{}, fmt.Errorf("[%s] content is required", handler.ErrInvalidParam)
				}

				// Parse and validate.
				p, err := mapping.ParseMappingProfile(input.Content, "import.toml")
				if err != nil {
					return MappingImportOutput{
						Valid: false,
						Issues: []mapping.ValidationIssue{{
							Severity: "error",
							Field:    "content",
							Message:  err.Error(),
						}},
					}, nil
				}

				issues := mapping.ValidateProfile(p)
				hasErrors := false
				for _, issue := range issues {
					if issue.Severity == "error" {
						hasErrors = true
						break
					}
				}

				// Determine name.
				name := input.Name
				if name == "" {
					name = p.DeviceName()
				}
				if name == "" {
					name = "imported-profile"
				}

				format := "legacy"
				if p.IsUnifiedFormat() {
					format = "unified"
				}

				result := MappingImportOutput{
					Valid:  !hasErrors,
					Name:   name,
					Format: format,
					Issues: issues,
					DryRun: input.DryRun,
				}

				if hasErrors {
					return result, nil
				}

				// Check if profile already exists.
				path := resolveMappingPath(name)
				if _, err := os.Stat(path); err == nil {
					result.Exists = true
					if !input.Force && !input.DryRun {
						result.Issues = append(result.Issues, mapping.ValidationIssue{
							Severity: "warning",
							Field:    "name",
							Message:  fmt.Sprintf("Profile %q already exists. Use force=true to overwrite.", name),
						})
						return result, nil
					}
				}

				if input.DryRun {
					return result, nil
				}

				// Write the profile.
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					return result, fmt.Errorf("create directory: %w", err)
				}
				if err := os.WriteFile(path, []byte(input.Content), 0644); err != nil {
					return result, fmt.Errorf("write profile: %w", err)
				}
				result.Written = path

				return result, nil
			},
		),

		// ── mapping_diff ──
		handler.TypedHandler[MappingDiffInput, MappingDiffOutput](
			"mapping_diff",
			"Compare two mapping profiles and show differences in their bindings.",
			func(_ context.Context, input MappingDiffInput) (MappingDiffOutput, error) {
				if input.Name1 == "" || input.Name2 == "" {
					return MappingDiffOutput{}, fmt.Errorf("[%s] name1 and name2 are required", handler.ErrInvalidParam)
				}

				p1, err := mapping.LoadMappingProfile(resolveMappingPath(input.Name1))
				if err != nil {
					return MappingDiffOutput{}, fmt.Errorf("[%s] profile %q: %w", handler.ErrNotFound, input.Name1, err)
				}
				p2, err := mapping.LoadMappingProfile(resolveMappingPath(input.Name2))
				if err != nil {
					return MappingDiffOutput{}, fmt.Errorf("[%s] profile %q: %w", handler.ErrNotFound, input.Name2, err)
				}

				// Extract mapping keys from both profiles.
				left := extractMappingKeys(p1)
				right := extractMappingKeys(p2)

				var entries []MappingDiffEntry
				added, removed, changed := 0, 0, 0

				// Find items in left.
				for key, lval := range left {
					if rval, ok := right[key]; ok {
						if lval == rval {
							entries = append(entries, MappingDiffEntry{Input: key, Status: "unchanged"})
						} else {
							entries = append(entries, MappingDiffEntry{Input: key, Status: "changed", Left: lval, Right: rval})
							changed++
						}
					} else {
						entries = append(entries, MappingDiffEntry{Input: key, Status: "removed", Left: lval})
						removed++
					}
				}

				// Find items only in right.
				for key, rval := range right {
					if _, ok := left[key]; !ok {
						entries = append(entries, MappingDiffEntry{Input: key, Status: "added", Right: rval})
						added++
					}
				}

				return MappingDiffOutput{
					Name1:   input.Name1,
					Name2:   input.Name2,
					Added:   added,
					Removed: removed,
					Changed: changed,
					Entries: entries,
				}, nil
			},
		),
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func resolveMappingPath(name string) string {
	if !strings.HasSuffix(name, ".toml") {
		name += ".toml"
	}
	return filepath.Join(makimaDir(), name)
}

func resolveContext(app string) string {
	if app != "" {
		return "app_override"
	}
	return "default"
}

// countTOMLMappings does a rough count of mapping entries in TOML content
// by counting lines with "=" that are inside [remap], [commands], [cc], [note] sections.
func countTOMLMappings(content string) int {
	count := 0
	inSection := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			switch {
			case strings.HasPrefix(trimmed, "[remap]"),
				strings.HasPrefix(trimmed, "[commands]"),
				strings.HasPrefix(trimmed, "[movements]"),
				strings.HasPrefix(trimmed, "[cc]"),
				strings.HasPrefix(trimmed, "[note]"):
				inSection = true
			default:
				inSection = false
			}
			continue
		}
		if inSection && strings.Contains(trimmed, "=") && !strings.HasPrefix(trimmed, "#") {
			count++
		}
	}
	return count
}

// sanitizeProfile removes local file paths and user-specific data from a profile.
func sanitizeProfile(content string) string {
	lines := strings.Split(content, "\n")
	var sanitized []string
	home := homeDir()
	for _, line := range lines {
		// Replace the active user's home path first so root and other non-/home
		// environments still sanitize correctly under test and in exports.
		if len(home) > 1 && strings.Contains(line, home) {
			line = strings.ReplaceAll(line, home, "~")
		}
		// Fall back to common home-directory prefixes for imported profiles.
		if strings.Contains(line, "/home/") || strings.Contains(line, "/Users/") {
			line = strings.ReplaceAll(line, "/home/", "~/")
			line = strings.ReplaceAll(line, "/Users/", "~/")
		}
		sanitized = append(sanitized, line)
	}
	return strings.Join(sanitized, "\n")
}

// extractMappingKeys returns a map of input->output description for diffing.
func extractMappingKeys(p *mapping.MappingProfile) map[string]string {
	keys := make(map[string]string)

	if p.IsUnifiedFormat() {
		for _, m := range p.Mappings {
			keys[m.Input] = fmt.Sprintf("%s:%v", m.Output.Type, describeOutput(m.Output))
		}
	} else {
		for input, outputs := range p.Remap {
			keys[input] = fmt.Sprintf("key:%s", strings.Join(outputs, "+"))
		}
		for input, cmds := range p.Commands {
			keys[input] = fmt.Sprintf("cmd:%s", strings.Join(cmds, " && "))
		}
		for input, target := range p.Movements {
			keys[input] = fmt.Sprintf("move:%s", target)
		}
	}

	return keys
}

func describeOutput(o mapping.OutputAction) string {
	switch o.Type {
	case mapping.OutputKey:
		return strings.Join(o.Keys, "+")
	case mapping.OutputCommand:
		return strings.Join(o.Exec, " ")
	case mapping.OutputMovement:
		return o.Target
	case mapping.OutputOSC:
		return fmt.Sprintf("%s:%d%s", o.Host, o.Port, o.Address)
	default:
		return string(o.Type)
	}
}
