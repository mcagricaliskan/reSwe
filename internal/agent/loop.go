package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cagri/reswe/internal/models"
	"github.com/cagri/reswe/internal/provider"
)

const maxSteps = 25

// Step represents one iteration of the agent loop
type Step struct {
	Number      int       `json:"number"`
	Think       string    `json:"think"`
	Action      string    `json:"action"`
	ActionArg   string    `json:"action_arg"`
	Observation string    `json:"observation"`
	IsFinal     bool      `json:"is_final"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
	DurationMs  int64     `json:"duration_ms"`
}

// StepCallback is called each time the agent completes a step
type StepCallback func(step Step)

// LoopResult is the structured return from RunLoop
type LoopResult struct {
	FinalResult string               `json:"final_result"`
	Status      string               `json:"status"` // "done", "waiting_for_user", "max_steps", "error"
	Questions   []string             `json:"questions,omitempty"`
	Messages    []models.ChatMessage `json:"messages"`
	StepCount   int                  `json:"step_count"`
	Error       error                `json:"-"`
}

// LoopConfig configures an agent loop run
type LoopConfig struct {
	Provider     provider.Provider
	Model        string
	SystemPrompt string
	TaskContext   string
	History      []models.ChatMessage
	Tools        *ToolSet
	OnStep       StepCallback
	OnStream     provider.StreamCallback
}

// RunLoop executes the ReAct agent loop.
func RunLoop(ctx context.Context, cfg LoopConfig) LoopResult {
	fullSystem := cfg.SystemPrompt + "\n\n" + cfg.Tools.ToolDescriptionBlock()

	var messages []models.ChatMessage

	if len(cfg.History) > 0 {
		messages = append(messages, models.ChatMessage{Role: "system", Content: fullSystem})
		messages = append(messages, cfg.History...)
		if cfg.TaskContext != "" {
			messages = append(messages, models.ChatMessage{Role: "user", Content: cfg.TaskContext})
		}
	} else {
		messages = []models.ChatMessage{
			{Role: "system", Content: fullSystem},
			{Role: "user", Content: cfg.TaskContext},
		}
	}

	for step := 1; step <= maxSteps; step++ {
		stepStartedAt := time.Now()

		select {
		case <-ctx.Done():
			return LoopResult{Status: "error", Error: ctx.Err(), Messages: messages, StepCount: step}
		default:
		}

		var response strings.Builder
		req := models.ChatRequest{
			Model:    cfg.Model,
			Messages: messages,
			Stream:   true,
		}

		_, err := cfg.Provider.ChatStream(ctx, req, func(chunk string) {
			response.WriteString(chunk)
			if cfg.OnStream != nil {
				cfg.OnStream(chunk)
			}
		})
		if err != nil {
			return LoopResult{Status: "error", Error: fmt.Errorf("step %d: %w", step, err), Messages: messages, StepCount: step}
		}

		llmOutput := response.String()
		think, action, arg := parseReActOutput(llmOutput)

		stepData := Step{
			Number:    step,
			Think:     think,
			Action:    action,
			ActionArg: arg,
			StartedAt: stepStartedAt,
		}

		if action == "" {
			// LLM didn't follow format — check if the entire output IS the plan
			// (some models just dump the plan without THINK/ACTION/ARG format)
			if step > 3 && looksLikePlan(llmOutput) {
				now := time.Now()
				stepData.Action = "done"
				stepData.ActionArg = llmOutput
				stepData.IsFinal = true
				stepData.CompletedAt = now
				stepData.DurationMs = now.Sub(stepStartedAt).Milliseconds()
				if cfg.OnStep != nil {
					cfg.OnStep(stepData)
				}
				return LoopResult{
					FinalResult: llmOutput,
					Status:      "done",
					Messages:    messages,
					StepCount:   step,
				}
			}

			messages = append(messages,
				models.ChatMessage{Role: "assistant", Content: llmOutput},
				models.ChatMessage{Role: "user", Content: "You MUST call a tool. Format:\n\nTHINK: your reasoning (all thinking goes here)\nACTION: tool_name\nARG: argument\n\nFor done: ARG is your clean deliverable document — no thinking, no 'I', just the content.\n\nAvailable: read_file, search_code, list_dir, write_file, edit_file, ask_user, done"},
			)
			now := time.Now()
			stepData.Observation = "(Agent did not call a tool, nudging...)"
			stepData.CompletedAt = now
			stepData.DurationMs = now.Sub(stepStartedAt).Milliseconds()
			if cfg.OnStep != nil {
				cfg.OnStep(stepData)
			}
			continue
		}

		// Execute the tool — parse structured args for multi-param tools
		args := parseToolArgs(action, arg)
		call := ToolCall{
			Tool:   action,
			Args:   args,
			Reason: think,
		}
		result := cfg.Tools.Execute(call)

		now := time.Now()
		stepData.Observation = result.Output
		stepData.IsFinal = action == "done"
		stepData.CompletedAt = now
		stepData.DurationMs = now.Sub(stepStartedAt).Milliseconds()

		if cfg.OnStep != nil {
			cfg.OnStep(stepData)
		}

		if action == "done" {
			return LoopResult{
				FinalResult: arg,
				Status:      "done",
				Messages:    messages,
				StepCount:   step,
			}
		}

		messages = append(messages,
			models.ChatMessage{Role: "assistant", Content: llmOutput},
			models.ChatMessage{Role: "user", Content: fmt.Sprintf("OBSERVATION:\n%s\n\nContinue. Use THINK/ACTION/ARG format.", result.Output)},
		)

		if result.Pause {
			return LoopResult{
				Status:    "waiting_for_user",
				Questions: cfg.Tools.GetPendingQuestions(),
				Messages:  messages,
				StepCount: step,
			}
		}
	}

	return LoopResult{Status: "max_steps", Error: fmt.Errorf("agent exceeded maximum steps (%d)", maxSteps), Messages: messages, StepCount: maxSteps}
}

// looksLikePlan checks if raw output looks like a plan (has markdown headers, multiple sections)
func looksLikePlan(output string) bool {
	headers := 0
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			headers++
		}
	}
	return headers >= 2 && len(output) > 200
}

// parseToolArgs converts the raw ARG string into a key→value map.
// For single-param tools (read_file, list_dir, search_code, ask_user, done),
// the whole ARG is mapped to every common key.
// For multi-param tools (edit_file, write_file), we parse structured delimiters:
//
//	FILE: <path>
//	OLD:
//	<old content>
//	NEW:
//	<new content>
//
// or for write_file:
//
//	FILE: <path>
//	CONTENT:
//	<file content>
func parseToolArgs(action, arg string) map[string]string {
	switch action {
	case "edit_file":
		return parseEditFileArgs(arg)
	case "write_file":
		return parseWriteFileArgs(arg)
	default:
		// Single-param tools: map arg to all common keys
		return map[string]string{"path": arg, "query": arg, "question": arg, "result": arg}
	}
}

// parseEditFileArgs parses: FILE: <path>\nOLD:\n<old>\nNEW:\n<new>
func parseEditFileArgs(arg string) map[string]string {
	result := map[string]string{}
	lines := strings.Split(arg, "\n")

	var filePath string
	var oldLines, newLines []string
	section := "" // "", "old", "new"

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		upper := strings.ToUpper(trimmed)

		if strings.HasPrefix(upper, "FILE:") {
			filePath = strings.TrimSpace(trimmed[5:])
			section = ""
			continue
		}
		if upper == "OLD:" || strings.HasPrefix(upper, "OLD:") {
			section = "old"
			// Check if there's content on the same line as OLD:
			rest := strings.TrimSpace(trimmed[4:])
			if rest != "" {
				oldLines = append(oldLines, rest)
			}
			continue
		}
		if upper == "NEW:" || strings.HasPrefix(upper, "NEW:") {
			section = "new"
			rest := strings.TrimSpace(trimmed[4:])
			if rest != "" {
				newLines = append(newLines, rest)
			}
			continue
		}

		switch section {
		case "old":
			oldLines = append(oldLines, line)
		case "new":
			newLines = append(newLines, line)
		}
	}

	result["path"] = filePath
	result["old_content"] = trimTrailingEmptyLines(strings.Join(oldLines, "\n"))
	result["new_content"] = trimTrailingEmptyLines(strings.Join(newLines, "\n"))
	return result
}

// parseWriteFileArgs parses: FILE: <path>\nCONTENT:\n<content>
func parseWriteFileArgs(arg string) map[string]string {
	result := map[string]string{}
	lines := strings.Split(arg, "\n")

	var filePath string
	var contentLines []string
	inContent := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		upper := strings.ToUpper(trimmed)

		if strings.HasPrefix(upper, "FILE:") {
			filePath = strings.TrimSpace(trimmed[5:])
			continue
		}
		if upper == "CONTENT:" || strings.HasPrefix(upper, "CONTENT:") {
			inContent = true
			rest := strings.TrimSpace(trimmed[8:])
			if rest != "" {
				contentLines = append(contentLines, rest)
			}
			continue
		}

		if inContent {
			contentLines = append(contentLines, line)
		}
	}

	result["path"] = filePath
	result["content"] = trimTrailingEmptyLines(strings.Join(contentLines, "\n"))
	return result
}

// trimTrailingEmptyLines removes trailing blank lines but preserves a final newline
func trimTrailingEmptyLines(s string) string {
	s = strings.TrimRight(s, " \t\n")
	if s != "" {
		s += "\n"
	}
	return s
}

// parseReActOutput extracts THINK, ACTION, ARG from LLM output.
// ARG captures everything after "ARG:" to end of output (multi-line for done tool).
func parseReActOutput(output string) (think, action, arg string) {
	lines := strings.Split(output, "\n")

	thinkLines := []string{}
	inThink := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		upper := strings.ToUpper(trimmed)

		if strings.HasPrefix(upper, "THINK:") {
			inThink = true
			rest := strings.TrimSpace(trimmed[6:])
			if rest != "" {
				thinkLines = append(thinkLines, rest)
			}
			continue
		}

		if strings.HasPrefix(upper, "ACTION:") {
			inThink = false
			action = strings.TrimSpace(trimmed[7:])
			action = strings.Trim(action, "`* ")
			action = strings.ToLower(action)
			continue
		}

		if strings.HasPrefix(upper, "ARG:") {
			inThink = false
			// Everything from here to end of output is the argument
			// (critical for 'done' which has multi-line markdown plan)
			rest := strings.TrimSpace(trimmed[4:])
			remaining := strings.Join(lines[i+1:], "\n")
			if rest != "" && remaining != "" {
				arg = rest + "\n" + remaining
			} else if rest != "" {
				arg = rest
			} else {
				arg = strings.TrimSpace(remaining)
			}
			break // stop parsing — everything after ARG: is the argument
		}

		if inThink {
			thinkLines = append(thinkLines, trimmed)
		}
	}

	think = strings.Join(thinkLines, " ")

	return think, action, arg
}
