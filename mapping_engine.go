// mapping_engine.go — Rule index, resolver, state management, and legacy parser.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
)

// ---------------------------------------------------------------------------
// Rule index — pre-compiled for O(1) source lookup
// ---------------------------------------------------------------------------

// RuleIndex is a pre-compiled lookup structure for fast rule matching.
type RuleIndex struct {
	bySource     map[string][]*MappingRule
	appOverrides map[string]map[string][]*MappingRule // source -> window_class -> rules
}

// BuildRuleIndex compiles a MappingProfile into a RuleIndex.
func BuildRuleIndex(p *MappingProfile) *RuleIndex {
	idx := &RuleIndex{
		bySource:     make(map[string][]*MappingRule),
		appOverrides: make(map[string]map[string][]*MappingRule),
	}

	for i := range p.Mappings {
		r := &p.Mappings[i]
		idx.bySource[r.Input] = append(idx.bySource[r.Input], r)
	}

	for _, override := range p.AppOverrides {
		for i := range override.Mappings {
			r := &override.Mappings[i]
			if idx.appOverrides[r.Input] == nil {
				idx.appOverrides[r.Input] = make(map[string][]*MappingRule)
			}
			idx.appOverrides[r.Input][override.WindowClass] = append(
				idx.appOverrides[r.Input][override.WindowClass], r,
			)
		}
	}

	return idx
}

// Resolve finds the best matching rule for a given input source and state.
func (idx *RuleIndex) Resolve(source string, state *EngineState) *MappingRule {
	state.mu.RLock()
	defer state.mu.RUnlock()

	// App-specific overrides take priority.
	if appMap, ok := idx.appOverrides[source]; ok {
		if rules, ok := appMap[state.ActiveApp]; ok {
			if r := matchBest(rules, state); r != nil {
				return r
			}
		}
	}

	// Fall back to default mappings.
	if rules, ok := idx.bySource[source]; ok {
		return matchBest(rules, state)
	}

	return nil
}

// matchBest finds the highest-priority rule whose modifiers and conditions match.
func matchBest(rules []*MappingRule, state *EngineState) *MappingRule {
	var best *MappingRule
	for _, r := range rules {
		if !modifiersMatch(r.Modifiers, state.ActiveModifiers) {
			continue
		}
		if r.Condition != nil && !r.Condition.Evaluate(state.Variables) {
			continue
		}
		if best == nil || r.Priority > best.Priority {
			best = r
		}
	}
	return best
}

func modifiersMatch(required []string, active map[string]bool) bool {
	for _, mod := range required {
		if !active[mod] {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// Engine state
// ---------------------------------------------------------------------------

// EngineState holds all mutable runtime state for the mapping engine.
type EngineState struct {
	mu sync.RWMutex

	ActiveModifiers map[string]bool
	ActiveApp       string
	ActiveLayer     map[string]int // deviceID -> layer index
	Variables       map[string]any
	PickupState     map[string]float64 // input source -> last output value
	FaderCrossed    map[string]bool
}

// NewEngineState creates an initialized engine state.
func NewEngineState() *EngineState {
	return &EngineState{
		ActiveModifiers: make(map[string]bool),
		ActiveLayer:     make(map[string]int),
		Variables:       make(map[string]any),
		PickupState:     make(map[string]float64),
		FaderCrossed:    make(map[string]bool),
	}
}

// ---------------------------------------------------------------------------
// Profile loading and parsing
// ---------------------------------------------------------------------------

// profilesDir returns the path for extended profile storage.
func profilesDir() string {
	return filepath.Join(dotfilesDir(), "profiles")
}

// LoadMappingProfile reads and parses a mapping profile from a TOML file.
func LoadMappingProfile(path string) (*MappingProfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read profile: %w", err)
	}

	return ParseMappingProfile(string(data), path)
}

// ParseMappingProfile parses TOML content into a MappingProfile.
// It auto-detects legacy vs unified format.
func ParseMappingProfile(content, sourcePath string) (*MappingProfile, error) {
	// First, decode into a raw map to detect format.
	var raw map[string]any
	if _, err := toml.Decode(content, &raw); err != nil {
		return nil, fmt.Errorf("invalid TOML: %w", err)
	}

	p := &MappingProfile{
		SourcePath: sourcePath,
		SourceName: strings.TrimSuffix(filepath.Base(sourcePath), ".toml"),
	}

	// Check for unified format marker.
	if _, hasProfile := raw["profile"]; hasProfile {
		if _, err := toml.Decode(content, p); err != nil {
			return nil, fmt.Errorf("decode unified profile: %w", err)
		}
		return p, nil
	}

	// Legacy format: parse into the legacy fields.
	// We need a struct that matches makima's flat [remap], [commands], etc.
	type legacyProfile struct {
		Remap     map[string][]string `toml:"remap"`
		Commands  map[string][]string `toml:"commands"`
		Movements map[string]string   `toml:"movements"`
		Settings  map[string]string   `toml:"settings"`
	}
	var legacy legacyProfile
	if _, err := toml.Decode(content, &legacy); err != nil {
		return nil, fmt.Errorf("decode legacy profile: %w", err)
	}

	p.Remap = legacy.Remap
	p.Commands = legacy.Commands
	p.Movements = legacy.Movements
	p.LegacySettings = legacy.Settings

	return p, nil
}

// ConvertLegacyToUnified transforms a legacy makima profile into the unified format.
// The filename convention provides device name and optional app association.
func ConvertLegacyToUnified(p *MappingProfile) *MappingProfile {
	unified := &MappingProfile{
		SourcePath: p.SourcePath,
		SourceName: p.SourceName,
		Profile: &ProfileMeta{
			SchemaVersion: 1,
			Type:          "mapping",
		},
		Settings: &MappingSettings{},
	}

	// Parse source name for device and app class.
	// Convention: "DeviceName.toml" or "DeviceName::window_class.toml"
	base := p.SourceName
	parts := strings.SplitN(base, "::", 2)
	deviceName := parts[0]

	unified.Profile.Device = deviceName
	unified.Device = &DeviceConfig{Name: deviceName}

	var appClass string
	if len(parts) > 1 {
		if _, err := strconv.Atoi(parts[1]); err != nil {
			appClass = parts[1]
		} else {
			layer, _ := strconv.Atoi(parts[1])
			unified.Profile.Layer = layer
		}
	}

	// Convert legacy settings to structured settings.
	convertLegacySettings(unified.Settings, p.LegacySettings)

	// Build mapping rules from legacy sections.
	var mappings []MappingRule

	for input, keys := range p.Remap {
		rule := parseLegacyInput(input)
		rule.Output = OutputAction{Type: OutputKey, Keys: keys}
		mappings = append(mappings, rule)
	}
	for input, cmds := range p.Commands {
		rule := parseLegacyInput(input)
		rule.Output = OutputAction{Type: OutputCommand, Exec: cmds}
		mappings = append(mappings, rule)
	}
	for input, target := range p.Movements {
		rule := parseLegacyInput(input)
		rule.Output = OutputAction{Type: OutputMovement, Target: target}
		mappings = append(mappings, rule)
	}

	// Sort for deterministic output.
	sort.Slice(mappings, func(i, j int) bool {
		return mappings[i].Input < mappings[j].Input
	})

	if appClass == "" {
		unified.Mappings = mappings
	} else {
		unified.Profile.AppClass = appClass
		unified.AppOverrides = []AppOverride{{
			WindowClass: appClass,
			Mappings:    mappings,
		}}
	}

	return unified
}

// parseLegacyInput handles the "MOD1-MOD2-EVENT" string format from makima.
func parseLegacyInput(input string) MappingRule {
	parts := strings.Split(input, "-")
	if len(parts) == 1 {
		return MappingRule{Input: parts[0]}
	}
	// Last part is the trigger; preceding parts are modifiers.
	trigger := parts[len(parts)-1]
	mods := parts[:len(parts)-1]
	return MappingRule{Input: trigger, Modifiers: mods}
}

func convertLegacySettings(s *MappingSettings, legacy map[string]string) {
	if legacy == nil {
		return
	}
	for k, v := range legacy {
		switch strings.ToUpper(k) {
		case "GRAB_DEVICE":
			s.GrabDevice = v == "true"
		case "16_BIT_AXIS":
			s.Axis16Bit = v == "true"
		case "CHAIN_ONLY":
			s.ChainOnly = v == "true"
		case "INVERT_CURSOR_AXIS":
			s.InvertCursorAxis = v == "true"
		case "INVERT_SCROLL_AXIS":
			s.InvertScrollAxis = v == "true"
		case "LAYOUT_SWITCHER":
			s.LayoutSwitcher = v
		case "LSTICK":
			if s.LStick == nil {
				s.LStick = &StickConfig{}
			}
			s.LStick.Function = v
		case "LSTICK_SENSITIVITY":
			if s.LStick == nil {
				s.LStick = &StickConfig{}
			}
			s.LStick.Sensitivity, _ = strconv.Atoi(v)
		case "LSTICK_DEADZONE":
			if s.LStick == nil {
				s.LStick = &StickConfig{}
			}
			s.LStick.Deadzone, _ = strconv.Atoi(v)
		case "RSTICK":
			if s.RStick == nil {
				s.RStick = &StickConfig{}
			}
			s.RStick.Function = v
		case "RSTICK_SENSITIVITY":
			if s.RStick == nil {
				s.RStick = &StickConfig{}
			}
			s.RStick.Sensitivity, _ = strconv.Atoi(v)
		case "RSTICK_DEADZONE":
			if s.RStick == nil {
				s.RStick = &StickConfig{}
			}
			s.RStick.Deadzone, _ = strconv.Atoi(v)
		case "CURSOR_SPEED":
			if s.Cursor == nil {
				s.Cursor = &MovementConfig{}
			}
			s.Cursor.Speed, _ = strconv.Atoi(v)
		case "CURSOR_ACCEL":
			if s.Cursor == nil {
				s.Cursor = &MovementConfig{}
			}
			s.Cursor.Acceleration, _ = strconv.ParseFloat(v, 64)
		case "SCROLL_SPEED":
			if s.Scroll == nil {
				s.Scroll = &MovementConfig{}
			}
			s.Scroll.Speed, _ = strconv.Atoi(v)
		case "SCROLL_ACCEL":
			if s.Scroll == nil {
				s.Scroll = &MovementConfig{}
			}
			s.Scroll.Acceleration, _ = strconv.ParseFloat(v, 64)
		case "CUSTOM_MODIFIERS":
			s.CustomModifiers = strings.Split(v, "-")
		}
	}
}

// ---------------------------------------------------------------------------
// Profile listing and discovery
// ---------------------------------------------------------------------------

// ListMappingProfiles scans the makima and profiles directories for all profiles.
func ListMappingProfiles() ([]MappingProfileSummary, error) {
	var summaries []MappingProfileSummary

	// Scan makima directory (legacy + unified).
	dir := makimaDir()
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read makima dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
			continue
		}
		if strings.HasPrefix(e.Name(), ".") {
			continue // makima ignores dotfiles
		}
		path := filepath.Join(dir, e.Name())
		p, err := LoadMappingProfile(path)
		if err != nil {
			summaries = append(summaries, MappingProfileSummary{
				Name:   strings.TrimSuffix(e.Name(), ".toml"),
				Path:   path,
				Format: "error",
				Error:  err.Error(),
			})
			continue
		}
		s := profileToSummary(p)
		summaries = append(summaries, s)
	}

	return summaries, nil
}

// MappingProfileSummary is a lightweight view of a profile for listing.
type MappingProfileSummary struct {
	Name         string   `json:"name"`
	Path         string   `json:"path"`
	Format       string   `json:"format"` // "unified", "legacy", "error"
	DeviceName   string   `json:"device_name,omitempty"`
	AppClass     string   `json:"app_class,omitempty"`
	MappingCount int      `json:"mapping_count"`
	Tags         []string `json:"tags,omitempty"`
	Description  string   `json:"description,omitempty"`
	Error        string   `json:"error,omitempty"`
}

func profileToSummary(p *MappingProfile) MappingProfileSummary {
	s := MappingProfileSummary{
		Name:         p.SourceName,
		Path:         p.SourcePath,
		MappingCount: p.MappingCount(),
		DeviceName:   p.DeviceName(),
	}
	if p.IsUnifiedFormat() {
		s.Format = "unified"
		if p.Profile != nil {
			s.Tags = p.Profile.Tags
			s.Description = p.Profile.Description
			s.AppClass = p.Profile.AppClass
		}
	} else {
		s.Format = "legacy"
		// Parse app class from filename.
		parts := strings.SplitN(p.SourceName, "::", 2)
		if len(parts) > 1 {
			if _, err := strconv.Atoi(parts[1]); err != nil {
				s.AppClass = parts[1]
			}
		}
	}
	return s
}

// ---------------------------------------------------------------------------
// Profile validation
// ---------------------------------------------------------------------------

// ValidationIssue describes a problem found during profile validation.
type ValidationIssue struct {
	Severity string `json:"severity"` // "error", "warning", "info"
	Field    string `json:"field"`
	Message  string `json:"message"`
}

// ValidateProfile checks a profile for correctness.
func ValidateProfile(p *MappingProfile) []ValidationIssue {
	var issues []ValidationIssue

	if p.IsUnifiedFormat() {
		if p.Profile.SchemaVersion < 1 {
			issues = append(issues, ValidationIssue{
				Severity: "error",
				Field:    "profile.schema_version",
				Message:  "schema_version must be >= 1",
			})
		}
		for i, m := range p.Mappings {
			if m.Input == "" {
				issues = append(issues, ValidationIssue{
					Severity: "error",
					Field:    fmt.Sprintf("mapping[%d].input", i),
					Message:  "input is required",
				})
			}
			if m.Output.Type == "" {
				issues = append(issues, ValidationIssue{
					Severity: "error",
					Field:    fmt.Sprintf("mapping[%d].output.type", i),
					Message:  "output type is required",
				})
			}
			issues = append(issues, validateValueTransform(m.Value, fmt.Sprintf("mapping[%d]", i))...)
		}
		for oi, ov := range p.AppOverrides {
			if ov.WindowClass == "" {
				issues = append(issues, ValidationIssue{
					Severity: "error",
					Field:    fmt.Sprintf("app_override[%d].window_class", oi),
					Message:  "window_class is required",
				})
			}
			for mi, m := range ov.Mappings {
				if m.Input == "" {
					issues = append(issues, ValidationIssue{
						Severity: "error",
						Field:    fmt.Sprintf("app_override[%d].mapping[%d].input", oi, mi),
						Message:  "input is required",
					})
				}
			}
		}
	} else if p.IsLegacyFormat() {
		issues = append(issues, ValidationIssue{
			Severity: "info",
			Field:    "format",
			Message:  "Legacy makima format detected. Consider migrating with mapping_migrate_legacy.",
		})
	} else {
		issues = append(issues, ValidationIssue{
			Severity: "warning",
			Field:    "format",
			Message:  "Profile appears to be empty or unrecognized format.",
		})
	}

	return issues
}

func validateValueTransform(vt *ValueTransform, prefix string) []ValidationIssue {
	if vt == nil {
		return nil
	}
	var issues []ValidationIssue
	if vt.InputRange[0] == vt.InputRange[1] && vt.InputRange[0] != 0 {
		issues = append(issues, ValidationIssue{
			Severity: "warning",
			Field:    prefix + ".value.input_range",
			Message:  "input_range min equals max — transform will always return output min",
		})
	}
	switch vt.Curve {
	case "", CurveLinear, CurveLogarithmic, CurveExponential, CurveSCurve:
		// valid
	default:
		issues = append(issues, ValidationIssue{
			Severity: "error",
			Field:    prefix + ".value.curve",
			Message:  fmt.Sprintf("unknown curve type: %q (valid: linear, logarithmic, exponential, scurve)", vt.Curve),
		})
	}
	return issues
}
