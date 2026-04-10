package enhancer

import (
	"fmt"
	"strings"
)

// PromptTemplate represents a reusable prompt template
type PromptTemplate struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	TaskType    TaskType `json:"task_type"`
	Variables   []string `json:"variables"`
	Example     string   `json:"example"`
	Template    string   `json:"-"`
}

var builtinTemplates = []PromptTemplate{
	{
		Name:        "troubleshoot",
		Description: "Structured diagnostic prompt for system issues",
		TaskType:    TaskTypeTroubleshooting,
		Variables:   []string{"system", "symptoms", "context"},
		Example:     `prompt-improver template troubleshoot --system resolume --symptoms "clips not triggering"`,
		Template: `<role>You are a systems diagnostician.</role>

<context>
System: {{system}}
Symptoms: {{symptoms}}
Additional context: {{context}}
</context>

<instructions>
1. Check the current status of {{system}}
2. Identify which component is likely failing based on the symptoms
3. Run targeted diagnostics for that component
4. Propose a fix with step-by-step instructions
</instructions>

<constraints>
- Start with the least disruptive diagnostic steps
- Do not restart services unless other options are exhausted
- Report findings in a structured format
</constraints>

<output_format>
## Diagnosis
- **Affected system**: ...
- **Root cause**: ...
- **Confidence**: high/medium/low

## Steps Taken
1. ...

## Recommended Fix
1. ...
</output_format>`,
	},
	{
		Name:        "code_review",
		Description: "Structured code review with focus areas and severity levels",
		TaskType:    TaskTypeAnalysis,
		Variables:   []string{"code", "language", "focus"},
		Example:     `prompt-improver template code_review --language Go --focus "error handling"`,
		Template: `<role>You are a senior code reviewer specializing in {{language}}.</role>

<context>
Language: {{language}}
Focus areas: {{focus}}
Code to review:
{{code}}
</context>

<instructions>
1. Read the code and understand its purpose
2. Check for correctness, edge cases, and error handling
3. Evaluate idiomatic usage for {{language}}
4. Focus specifically on: {{focus}}
5. Suggest concrete improvements with code examples
</instructions>

<constraints>
- Reference line numbers or function names
- Distinguish between critical issues, suggestions, and nitpicks
- Show targeted diffs, not full rewrites
- Acknowledge what is done well
</constraints>

<output_format>
## Summary
One-line assessment.

## Critical Issues
- [critical] description — suggested fix

## Suggestions
- [suggestion] description — example

## Nitpicks
- [nitpick] description
</output_format>`,
	},
	{
		Name:        "workflow_create",
		Description: "Design a multi-step automation workflow from a goal",
		TaskType:    TaskTypeWorkflow,
		Variables:   []string{"goal", "systems", "constraints"},
		Example:     `prompt-improver template workflow_create --goal "pre-show startup sequence"`,
		Template: `<role>You are a workflow architect.</role>

<context>
Goal: {{goal}}
Target systems: {{systems}}
Constraints: {{constraints}}
</context>

<instructions>
1. Identify the tools and steps needed
2. Design the step sequence with dependencies
3. Add error handling and rollback for each step
4. Validate the workflow can be executed
</instructions>

<constraints>
- Each step must have a clear success/failure condition
- Steps without dependencies should run in parallel
- Include health checks before destructive operations
</constraints>

<output_format>
## Workflow: {{goal}}

### Steps
| # | Action | Depends On | On Failure |
|---|--------|-----------|------------|
| 1 | ... | — | stop |

### Error Handling
- ...
</output_format>`,
	},
	{
		Name:        "data_analysis",
		Description: "Structured analysis prompt for datasets and metrics",
		TaskType:    TaskTypeAnalysis,
		Variables:   []string{"dataset", "questions", "format"},
		Example:     `prompt-improver template data_analysis --dataset "server logs" --questions "what caused the spike"`,
		Template: `<role>You are a data analyst.</role>

<context>
Dataset: {{dataset}}
Questions to answer: {{questions}}
Preferred output format: {{format}}
</context>

<instructions>
1. Examine the specified dataset
2. Clean and validate the data
3. Answer each question with supporting data
4. Identify unexpected patterns or anomalies
5. Provide actionable recommendations
</instructions>

<constraints>
- Support claims with specific numbers
- Distinguish correlation from causation
- Note data quality issues
</constraints>

<output_format>
## Key Findings
1. ...

## Detailed Answers
**Q: ...**
A: ...

## Anomalies
- ...

## Recommendations
1. ...
</output_format>`,
	},
	{
		Name:        "creative_brief",
		Description: "Creative direction prompt for design work",
		TaskType:    TaskTypeCreative,
		Variables:   []string{"mood", "constraints", "references", "medium"},
		Example:     `prompt-improver template creative_brief --mood "dark techno" --medium "visuals"`,
		Template: `<role>You are a creative director.</role>

<context>
Medium: {{medium}}
Mood/aesthetic: {{mood}}
References: {{references}}
Technical constraints: {{constraints}}
</context>

<instructions>
1. Interpret the mood into concrete parameters
2. Suggest specific configurations that achieve the aesthetic
3. Provide parameter ranges (colors, intensity, effects)
4. Design a progression arc
</instructions>

<constraints>
- Stay within available capabilities
- Provide specific numbers, not vague descriptions
- Balance ambition with reliability
</constraints>

<output_format>
## Creative Brief: {{mood}}

### Palette
- Colors: ...
- Textures: ...
- Movement: ...

### Parameters
| Element | Parameter | Value/Range |
|---------|-----------|-------------|
| ... | ... | ... |

### Progression
1. Opening: ...
2. Build: ...
3. Peak: ...
4. Cooldown: ...
</output_format>`,
	},
}

// GetTemplate returns a template by name
func GetTemplate(name string) *PromptTemplate {
	for i := range builtinTemplates {
		if builtinTemplates[i].Name == name {
			return &builtinTemplates[i]
		}
	}
	return nil
}

// ListTemplates returns all available templates
func ListTemplates() []PromptTemplate {
	return builtinTemplates
}

// FillTemplate replaces {{variable}} placeholders with provided values
func FillTemplate(tmpl *PromptTemplate, vars map[string]string) string {
	result := tmpl.Template
	for _, v := range tmpl.Variables {
		placeholder := "{{" + v + "}}"
		value := vars[v]
		if value == "" {
			value = "(not specified)"
		}
		result = strings.ReplaceAll(result, placeholder, value)
	}
	return result
}

// TemplateListSummary returns a formatted summary of all templates
func TemplateListSummary() string {
	var b strings.Builder
	b.WriteString("# Available Prompt Templates\n\n")
	for _, t := range builtinTemplates {
		fmt.Fprintf(&b, "## %s\n", t.Name)
		fmt.Fprintf(&b, "- **Description**: %s\n", t.Description)
		fmt.Fprintf(&b, "- **Task type**: %s\n", t.TaskType)
		fmt.Fprintf(&b, "- **Variables**: %s\n", strings.Join(t.Variables, ", "))
		fmt.Fprintf(&b, "- **Example**: `%s`\n\n", t.Example)
	}
	return b.String()
}
