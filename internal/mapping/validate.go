package mapping

import "fmt"

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
			issues = append(issues, ValidateValueTransform(m.Value, fmt.Sprintf("mapping[%d]", i))...)
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

// ValidateValueTransform checks a value transform for correctness.
func ValidateValueTransform(vt *ValueTransform, prefix string) []ValidationIssue {
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
