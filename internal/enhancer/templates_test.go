package enhancer

import (
	"strings"
	"testing"
)

func TestListTemplates(t *testing.T) {
	templates := ListTemplates()
	if len(templates) != 5 {
		t.Errorf("expected 5 templates, got %d", len(templates))
	}
	for _, tmpl := range templates {
		if tmpl.Name == "" {
			t.Error("template has empty name")
		}
		if tmpl.Description == "" {
			t.Errorf("template %q has empty description", tmpl.Name)
		}
		if tmpl.TaskType == "" {
			t.Errorf("template %q has empty task type", tmpl.Name)
		}
		if len(tmpl.Variables) == 0 {
			t.Errorf("template %q has no variables", tmpl.Name)
		}
		if tmpl.Template == "" {
			t.Errorf("template %q has empty template body", tmpl.Name)
		}
		if tmpl.Example == "" {
			t.Errorf("template %q has empty example", tmpl.Name)
		}
	}
}

func TestTemplateListSummary(t *testing.T) {
	summary := TemplateListSummary()
	expectedNames := []string{"troubleshoot", "code_review", "workflow_create", "data_analysis", "creative_brief"}
	for _, name := range expectedNames {
		assertContains(t, summary, name)
	}
	assertContains(t, summary, "# Available Prompt Templates")
}

func TestFillTemplate_AllTemplates(t *testing.T) {
	for _, tmpl := range ListTemplates() {
		t.Run(tmpl.Name, func(t *testing.T) {
			filled := FillTemplate(&tmpl, map[string]string{})
			for _, v := range tmpl.Variables {
				assertNotContains(t, filled, "{{"+v+"}}")
				assertContains(t, filled, "(not specified)")
			}
		})
	}
}

func TestFillTemplate_NilVars(t *testing.T) {
	tmpl := GetTemplate("troubleshoot")
	if tmpl == nil {
		t.Fatal("troubleshoot template should exist")
	}
	// Should not panic
	filled := FillTemplate(tmpl, nil)
	if filled == "" {
		t.Error("filled template should not be empty")
	}
	assertContains(t, filled, "(not specified)")
}

func TestGetTemplate_AllNames(t *testing.T) {
	names := []string{"troubleshoot", "code_review", "workflow_create", "data_analysis", "creative_brief"}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			tmpl := GetTemplate(name)
			if tmpl == nil {
				t.Errorf("GetTemplate(%q) returned nil", name)
			}
		})
	}
}

func TestFillTemplate_WithVars(t *testing.T) {
	tmpl := GetTemplate("code_review")
	filled := FillTemplate(tmpl, map[string]string{
		"language": "Go",
		"focus":    "error handling",
		"code":     "func main() {}",
	})
	assertContains(t, filled, "Go")
	assertContains(t, filled, "error handling")
	assertContains(t, filled, "func main() {}")
	assertNotContains(t, filled, "(not specified)")
}

func TestGetTemplate_Nonexistent(t *testing.T) {
	tmpl := GetTemplate("nonexistent")
	if tmpl != nil {
		t.Error("should return nil for nonexistent template")
	}
}

func TestTemplateListSummary_Format(t *testing.T) {
	summary := TemplateListSummary()
	// Should have markdown formatting
	if !strings.HasPrefix(summary, "#") {
		t.Error("summary should start with markdown header")
	}
	assertContains(t, summary, "**Description**")
	assertContains(t, summary, "**Task type**")
	assertContains(t, summary, "**Variables**")
}
