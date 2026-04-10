package enhancer

import "strings"

// TaskType represents the type of task a prompt is requesting
type TaskType string

const (
	TaskTypeCode            TaskType = "code"
	TaskTypeCreative        TaskType = "creative"
	TaskTypeAnalysis        TaskType = "analysis"
	TaskTypeTroubleshooting TaskType = "troubleshooting"
	TaskTypeWorkflow        TaskType = "workflow"
	TaskTypeGeneral         TaskType = "general"
)

// taskTypePatterns maps keyword patterns to task types
var taskTypePatterns = map[TaskType][]string{
	TaskTypeTroubleshooting: {
		"debug", "error", "fix", "broken", "not working", "crash", "fail",
		"issue", "problem", "troubleshoot", "diagnose", "wrong", "stuck",
		"bug", "exception", "timeout", "disconnect",
	},
	TaskTypeCode: {
		"create", "build", "implement", "code", "write", "function",
		"class", "api", "endpoint", "refactor", "add feature", "develop",
		"program", "script", "module", "package", "library",
	},
	TaskTypeAnalysis: {
		"review", "analyze", "compare", "evaluate", "assess", "audit",
		"inspect", "examine", "investigate", "explain", "understand",
		"performance", "benchmark", "profile", "measure",
	},
	TaskTypeCreative: {
		"design", "visual", "music", "audio", "video", "art",
		"creative", "mood", "aesthetic", "style", "theme", "vibe",
		"show", "performance", "set", "mix", "lighting",
	},
	TaskTypeWorkflow: {
		"workflow", "automate", "sequence", "pipeline", "chain",
		"schedule", "orchestrate", "batch", "process", "routine",
		"startup", "shutdown", "backup",
	},
}

// Classify determines the most likely task type for a prompt
func Classify(prompt string) TaskType {
	lower := strings.ToLower(prompt)
	scores := make(map[TaskType]int)

	for taskType, patterns := range taskTypePatterns {
		for _, pattern := range patterns {
			if strings.Contains(lower, pattern) {
				scores[taskType]++
			}
		}
	}

	bestType := TaskTypeGeneral
	bestScore := 0
	for taskType, score := range scores {
		if score > bestScore {
			bestScore = score
			bestType = taskType
		}
	}

	return bestType
}

// ValidTaskType checks if a string is a valid task type
func ValidTaskType(s string) TaskType {
	switch TaskType(strings.ToLower(s)) {
	case TaskTypeCode, TaskTypeCreative, TaskTypeAnalysis,
		TaskTypeTroubleshooting, TaskTypeWorkflow, TaskTypeGeneral:
		return TaskType(strings.ToLower(s))
	default:
		return ""
	}
}
