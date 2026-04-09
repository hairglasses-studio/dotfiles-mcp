// Package mapping defines the unified controller mapping engine types and runtime.
//
// It provides the data model for mapping any input device (MIDI, gamepad, keyboard,
// HID) to any output target (keys, commands, OSC, WebSocket, MIDI out, D-Bus).
// The TOML schema is backwards-compatible with existing makima profiles.
package mapping

import (
	"fmt"
	"math"
)

// ---------------------------------------------------------------------------
// Input classification
// ---------------------------------------------------------------------------

// InputClass categorizes the broad device family.
type InputClass string

const (
	InputClassGamepad  InputClass = "gamepad"
	InputClassKeyboard InputClass = "keyboard"
	InputClassMouse    InputClass = "mouse"
	InputClassMIDI     InputClass = "midi"
	InputClassHID      InputClass = "hid"
)

// ValueKind describes the semantic type of an input value.
type ValueKind int

const (
	ValueDiscrete ValueKind = iota // 0/1/2 digital press/release/repeat
	ValueAbsolute                  // 0.0-1.0 normalized continuous
	ValueRelative                  // signed delta (encoders, scroll)
	ValueBipolar                   // -1.0 to 1.0 (analog sticks, pitch bend)
)

// ---------------------------------------------------------------------------
// Output types
// ---------------------------------------------------------------------------

// OutputType enumerates what a mapping produces.
type OutputType string

const (
	OutputKey         OutputType = "key"
	OutputCommand     OutputType = "command"
	OutputMovement    OutputType = "movement"
	OutputOSC         OutputType = "osc"
	OutputWebSocket   OutputType = "websocket"
	OutputMIDI        OutputType = "midi_out"
	OutputDBus        OutputType = "dbus"
	OutputSequence    OutputType = "sequence"
	OutputLayerSwitch OutputType = "layer_switch"
	OutputToggleVar   OutputType = "toggle_var"
	OutputSetVar      OutputType = "set_var"
)

// ---------------------------------------------------------------------------
// Value transform
// ---------------------------------------------------------------------------

// CurveType names a transfer function for value scaling.
type CurveType string

const (
	CurveLinear      CurveType = "linear"
	CurveLogarithmic CurveType = "logarithmic"
	CurveExponential CurveType = "exponential"
	CurveSCurve      CurveType = "scurve"
)

// ValueTransform configures how a continuous input value is scaled
// and mapped to the output domain.
type ValueTransform struct {
	InputRange  [2]float64 `toml:"input_range,omitempty" json:"input_range,omitempty"`
	OutputRange [2]float64 `toml:"output_range,omitempty" json:"output_range,omitempty"`
	Curve       CurveType  `toml:"curve,omitempty" json:"curve,omitempty"`
	CurveParam  float64    `toml:"curve_param,omitempty" json:"curve_param,omitempty"`
	Mode        string     `toml:"mode,omitempty" json:"mode,omitempty"`           // "absolute" (default) or "relative"
	Step        float64    `toml:"step,omitempty" json:"step,omitempty"`           // increment for relative mode
	Threshold   float64    `toml:"threshold,omitempty" json:"threshold,omitempty"` // continuous → discrete threshold
	Pickup      string     `toml:"pickup,omitempty" json:"pickup,omitempty"`       // "jump" (default) or "pickup"
}

// ApplyCurve maps a normalized [0,1] value through the selected curve.
func (vt *ValueTransform) ApplyCurve(normalized float64) float64 {
	param := vt.CurveParam
	if param == 0 {
		param = 2.0
	}
	switch vt.Curve {
	case CurveLogarithmic:
		if normalized <= 0 {
			return 0
		}
		return math.Log(1+normalized*param) / math.Log(1+param)
	case CurveExponential:
		return (math.Exp(normalized*param) - 1) / (math.Exp(param) - 1)
	case CurveSCurve:
		if param == 2.0 {
			return normalized * normalized * (3 - 2*normalized)
		}
		return 0.5 * (1 + math.Tanh(param*(normalized-0.5))/math.Tanh(param*0.5))
	default: // linear
		return normalized
	}
}

// Transform takes a raw input value and returns the scaled output value.
func (vt *ValueTransform) Transform(raw float64) float64 {
	inMin, inMax := vt.InputRange[0], vt.InputRange[1]
	if inMax == inMin {
		return vt.OutputRange[0]
	}
	normalized := (raw - inMin) / (inMax - inMin)
	normalized = math.Max(0, math.Min(1, normalized))
	curved := vt.ApplyCurve(normalized)
	outMin, outMax := vt.OutputRange[0], vt.OutputRange[1]
	return outMin + curved*(outMax-outMin)
}

// ---------------------------------------------------------------------------
// Output action
// ---------------------------------------------------------------------------

// OutputAction defines what a mapping rule produces when triggered.
type OutputAction struct {
	Type OutputType `toml:"type" json:"type"`

	// Key output
	Keys []string `toml:"keys,omitempty" json:"keys,omitempty"`

	// Command output
	Exec []string `toml:"exec,omitempty" json:"exec,omitempty"`

	// Movement output
	Target string `toml:"target,omitempty" json:"target,omitempty"`

	// OSC output
	Address string `toml:"address,omitempty" json:"address,omitempty"`
	Port    int    `toml:"port,omitempty" json:"port,omitempty"`
	Host    string `toml:"host,omitempty" json:"host,omitempty"`

	// WebSocket output
	URL     string `toml:"url,omitempty" json:"url,omitempty"`
	Message string `toml:"message,omitempty" json:"message,omitempty"`

	// Layer switching
	Layer string `toml:"layer,omitempty" json:"layer,omitempty"`

	// Variable control
	Variable string `toml:"variable,omitempty" json:"variable,omitempty"`
	Values   []any  `toml:"values,omitempty" json:"values,omitempty"`

	// Sequence steps
	Steps []SequenceStep `toml:"steps,omitempty" json:"steps,omitempty"`
}

// SequenceStep is one action within a sequence chain.
type SequenceStep struct {
	Type    OutputType `toml:"type" json:"type"`
	Keys    []string   `toml:"keys,omitempty" json:"keys,omitempty"`
	Exec    []string   `toml:"exec,omitempty" json:"exec,omitempty"`
	Target  string     `toml:"target,omitempty" json:"target,omitempty"`
	Address string     `toml:"address,omitempty" json:"address,omitempty"`
	Port    int        `toml:"port,omitempty" json:"port,omitempty"`
	Host    string     `toml:"host,omitempty" json:"host,omitempty"`
	DelayMs int        `toml:"delay_ms,omitempty" json:"delay_ms,omitempty"`
}

// ---------------------------------------------------------------------------
// Condition
// ---------------------------------------------------------------------------

// Condition gates a mapping rule on engine state.
type Condition struct {
	Variable    string  `toml:"variable" json:"variable"`
	Equals      any     `toml:"equals,omitempty" json:"equals,omitempty"`
	NotEqual    any     `toml:"not_equal,omitempty" json:"not_equal,omitempty"`
	GreaterThan float64 `toml:"greater_than,omitempty" json:"greater_than,omitempty"`
	LessThan    float64 `toml:"less_than,omitempty" json:"less_than,omitempty"`
}

// Evaluate checks the condition against a variable store.
func (c *Condition) Evaluate(vars map[string]any) bool {
	val, exists := vars[c.Variable]
	if !exists {
		return false
	}
	if c.Equals != nil {
		return fmt.Sprintf("%v", val) == fmt.Sprintf("%v", c.Equals)
	}
	if c.NotEqual != nil {
		return fmt.Sprintf("%v", val) != fmt.Sprintf("%v", c.NotEqual)
	}
	fval, ok := ToFloat64(val)
	if !ok {
		return false
	}
	if c.GreaterThan != 0 && fval <= c.GreaterThan {
		return false
	}
	if c.LessThan != 0 && fval >= c.LessThan {
		return false
	}
	return true
}

// ToFloat64 attempts to convert a value to float64.
func ToFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case string:
		return 0, false
	default:
		return 0, false
	}
}

// ---------------------------------------------------------------------------
// Mapping rule
// ---------------------------------------------------------------------------

// MappingRule is one input-to-output binding — the atomic unit of the engine.
type MappingRule struct {
	Input       string          `toml:"input" json:"input"`
	Modifiers   []string        `toml:"modifiers,omitempty" json:"modifiers,omitempty"`
	Output      OutputAction    `toml:"output" json:"output"`
	Value       *ValueTransform `toml:"value,omitempty" json:"value,omitempty"`
	Condition   *Condition      `toml:"condition,omitempty" json:"condition,omitempty"`
	Description string          `toml:"description,omitempty" json:"description,omitempty"`
	Priority    int             `toml:"priority,omitempty" json:"priority,omitempty"`
	Layer       int             `toml:"layer,omitempty" json:"layer,omitempty"` // 0 = all layers (default)
}

// ---------------------------------------------------------------------------
// Device config
// ---------------------------------------------------------------------------

// DeviceConfig identifies the physical device a profile targets.
type DeviceConfig struct {
	Name      string     `toml:"name" json:"name"`
	Type      InputClass `toml:"type" json:"type"`
	VendorID  string     `toml:"vendor_id,omitempty" json:"vendor_id,omitempty"`
	ProductID string     `toml:"product_id,omitempty" json:"product_id,omitempty"`
}

// ---------------------------------------------------------------------------
// Settings
// ---------------------------------------------------------------------------

// StickConfig holds analog stick tuning parameters.
type StickConfig struct {
	Function            string   `toml:"function" json:"function"` // cursor|scroll|bind|disabled
	Sensitivity         int      `toml:"sensitivity" json:"sensitivity"`
	Deadzone            int      `toml:"deadzone" json:"deadzone"`
	ActivationModifiers []string `toml:"activation_modifiers,omitempty" json:"activation_modifiers,omitempty"`
}

// MovementConfig holds cursor/scroll speed tuning.
type MovementConfig struct {
	Speed        int     `toml:"speed" json:"speed"`
	Acceleration float64 `toml:"acceleration" json:"acceleration"`
}

// MappingSettings holds device-level tuning.
type MappingSettings struct {
	GrabDevice       bool            `toml:"grab_device,omitempty" json:"grab_device,omitempty"`
	Axis16Bit        bool            `toml:"axis_16bit,omitempty" json:"axis_16bit,omitempty"`
	ChainOnly        bool            `toml:"chain_only,omitempty" json:"chain_only,omitempty"`
	InvertCursorAxis bool            `toml:"invert_cursor_axis,omitempty" json:"invert_cursor_axis,omitempty"`
	InvertScrollAxis bool            `toml:"invert_scroll_axis,omitempty" json:"invert_scroll_axis,omitempty"`
	CustomModifiers  []string        `toml:"custom_modifiers,omitempty" json:"custom_modifiers,omitempty"`
	LayoutSwitcher   string          `toml:"layout_switcher,omitempty" json:"layout_switcher,omitempty"`
	LStick           *StickConfig    `toml:"lstick,omitempty" json:"lstick,omitempty"`
	RStick           *StickConfig    `toml:"rstick,omitempty" json:"rstick,omitempty"`
	Cursor           *MovementConfig `toml:"cursor,omitempty" json:"cursor,omitempty"`
	Scroll           *MovementConfig `toml:"scroll,omitempty" json:"scroll,omitempty"`
}

// ---------------------------------------------------------------------------
// App override
// ---------------------------------------------------------------------------

// AppOverride provides per-window-class mapping overrides.
type AppOverride struct {
	WindowClass string           `toml:"window_class" json:"window_class"`
	Description string           `toml:"description,omitempty" json:"description,omitempty"`
	Mappings    []MappingRule    `toml:"mapping" json:"mapping"`
	Settings    *MappingSettings `toml:"settings,omitempty" json:"settings,omitempty"`
}

// ---------------------------------------------------------------------------
// Profile metadata
// ---------------------------------------------------------------------------

// ProfileMeta holds optional metadata in the [profile] section.
type ProfileMeta struct {
	SchemaVersion int      `toml:"schema_version,omitempty" json:"schema_version,omitempty"`
	Type          string   `toml:"type,omitempty" json:"type,omitempty"`
	Device        string   `toml:"device,omitempty" json:"device,omitempty"`
	AppClass      string   `toml:"app_class,omitempty" json:"app_class,omitempty"`
	Layer         int      `toml:"layer,omitempty" json:"layer,omitempty"`
	Inherits      string   `toml:"inherits,omitempty" json:"inherits,omitempty"`
	Template      string   `toml:"template,omitempty" json:"template,omitempty"`
	Tags          []string `toml:"tags,omitempty" json:"tags,omitempty"`
	Author        string   `toml:"author,omitempty" json:"author,omitempty"`
	Description   string   `toml:"description,omitempty" json:"description,omitempty"`

	Hardware *HardwareMeta `toml:"hardware,omitempty" json:"hardware,omitempty"`
}

// HardwareMeta describes device capabilities for matching and validation.
type HardwareMeta struct {
	VendorIDs  []string `toml:"vendor_ids,omitempty" json:"vendor_ids,omitempty"`
	ProductIDs []string `toml:"product_ids,omitempty" json:"product_ids,omitempty"`
	Brand      string   `toml:"brand,omitempty" json:"brand,omitempty"`
	Features   []string `toml:"features,omitempty" json:"features,omitempty"`
}

// ---------------------------------------------------------------------------
// Profile — the complete mapping configuration
// ---------------------------------------------------------------------------

// MappingProfile is the complete mapping configuration for one device.
type MappingProfile struct {
	// Unified format fields
	Profile      *ProfileMeta     `toml:"profile,omitempty" json:"profile,omitempty"`
	Device       *DeviceConfig    `toml:"device,omitempty" json:"device,omitempty"`
	Settings     *MappingSettings `toml:"settings,omitempty" json:"settings,omitempty"`
	Mappings     []MappingRule    `toml:"mapping,omitempty" json:"mapping,omitempty"`
	AppOverrides []AppOverride    `toml:"app_override,omitempty" json:"app_override,omitempty"`

	// Legacy makima format fields (parsed when [profile] absent)
	Remap          map[string][]string `toml:"remap,omitempty" json:"remap,omitempty"`
	Commands       map[string][]string `toml:"commands,omitempty" json:"commands,omitempty"`
	Movements      map[string]string   `toml:"movements,omitempty" json:"movements,omitempty"`
	LegacySettings map[string]string   `toml:"settings_legacy,omitempty" json:"-"`

	// Source file info (not serialized)
	SourcePath string `toml:"-" json:"source_path,omitempty"`
	SourceName string `toml:"-" json:"source_name,omitempty"`
}

// IsUnifiedFormat returns true if the profile uses the new unified schema.
func (p *MappingProfile) IsUnifiedFormat() bool {
	return p.Profile != nil && p.Profile.SchemaVersion > 0
}

// IsLegacyFormat returns true if the profile uses the makima v1 flat format.
func (p *MappingProfile) IsLegacyFormat() bool {
	return !p.IsUnifiedFormat() && (len(p.Remap) > 0 || len(p.Commands) > 0 || len(p.Movements) > 0)
}

// MappingCount returns the total number of mappings in the profile.
func (p *MappingProfile) MappingCount() int {
	if p.IsUnifiedFormat() {
		count := len(p.Mappings)
		for _, ov := range p.AppOverrides {
			count += len(ov.Mappings)
		}
		return count
	}
	return len(p.Remap) + len(p.Commands) + len(p.Movements)
}

// DeviceName returns the device name from metadata or source filename.
func (p *MappingProfile) DeviceName() string {
	if p.Profile != nil && p.Profile.Device != "" {
		return p.Profile.Device
	}
	if p.Device != nil && p.Device.Name != "" {
		return p.Device.Name
	}
	return p.SourceName
}
