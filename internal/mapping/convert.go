package mapping

import (
	"sort"
	"strconv"
	"strings"
)

// ConvertLegacyToUnified transforms a legacy makima profile into the unified format.
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
	ConvertLegacySettings(unified.Settings, p.LegacySettings)

	// Build mapping rules from legacy sections.
	var mappings []MappingRule

	for input, keys := range p.Remap {
		rule := ParseLegacyInput(input)
		rule.Output = OutputAction{Type: OutputKey, Keys: keys}
		mappings = append(mappings, rule)
	}
	for input, cmds := range p.Commands {
		rule := ParseLegacyInput(input)
		rule.Output = OutputAction{Type: OutputCommand, Exec: cmds}
		mappings = append(mappings, rule)
	}
	for input, target := range p.Movements {
		rule := ParseLegacyInput(input)
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

// ParseLegacyInput handles the "MOD1-MOD2-EVENT" string format from makima.
func ParseLegacyInput(input string) MappingRule {
	parts := strings.Split(input, "-")
	if len(parts) == 1 {
		return MappingRule{Input: parts[0]}
	}
	trigger := parts[len(parts)-1]
	mods := parts[:len(parts)-1]
	return MappingRule{Input: trigger, Modifiers: mods}
}

// ConvertLegacySettings converts a flat key-value settings map to structured MappingSettings.
func ConvertLegacySettings(s *MappingSettings, legacy map[string]string) {
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
