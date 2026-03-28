package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/cagri/reswe/internal/models"
	"github.com/cagri/reswe/internal/provider"
)

const maxSteps = 25

// Step represents one iteration of the agent loop
type Step struct {
	Number      int    `json:"number"`
	Think       string `json:"think"`
	Action      string `json:"action"`
	ActionArg   string `json:"action_arg"`
	Observation string `json:"observation"`
	IsFinal     bool   `json:"is_final"`
}

// StepCallback is called each time the agent completes a step
type StepCallback func(step Step)

// LoopConfig configures an agent loop run
type LoopConfig struct {
	Provider     provider.Provider
	Model        string
	SystemPrompt string
	TaskContext   string                  // task title + description for first message
	History      []models.ChatMessage     // pre-existing conversation (for continuation)
	Tools        *ToolSet
	OnStep       StepCallback
	OnStream     provider.StreamCallback
}

// RunLoop executes the ReAct agent loop
func RunLoop(ctx context.Context, cfg LoopConfig) (string, error) {
	fullSystem := cfg.SystemPrompt + "\n\n" + cfg.Tools.ToolDescriptionBlock()

	var messages []models.ChatMessage

	if len(cfg.History) > 0 {
		// Continuing a conversation — rebuild with system prompt + history + new user message
		messages = append(messages, models.ChatMessage{Role: "system", Content: fullSystem})
		messages = append(messages, cfg.History...)
		if cfg.TaskContext != "" {
			messages = append(messages, models.ChatMessage{Role: "user", Content: cfg.TaskContext})
		}
	} else {
		// Fresh start
		messages = []models.ChatMessage{
			{Role: "system", Content: fullSystem},
			{Role: "user", Content: cfg.TaskContext},
		}
	}

	var finalResult string

	for step := 1; step <= maxSteps; step++ {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return finalResult, ctx.Err()
		default:
		}

		// Call LLM
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
			return finalResult, fmt.Errorf("step %d: %w", step, err)
		}

		llmOutput := response.String()

		// Parse the LLM output for THINK / ACTION / ARG
		think, action, arg := parseReActOutput(llmOutput)

		stepData := Step{
			Number:    step,
			Think:     think,
			Action:    action,
			ActionArg: arg,
		}

		if action == "" {
			// LLM didn't follow format — nudge it
			messages = append(messages,
				models.ChatMessage{Role: "assistant", Content: llmOutput},
				models.ChatMessage{Role: "user", Content: "You must use a tool. Use the format:\n\nTHINK: <reasoning>\nACTION: <tool_name>\nARG: <argument>\n\nAvailable tools: read_file, search_code, list_dir, ask_user, done"},
			)
			stepData.Observation = "(Agent did not call a tool, nudging...)"
			if cfg.OnStep != nil {
				cfg.OnStep(stepData)
			}
			continue
		}

		// Execute the tool
		call := ToolCall{
			Tool:   action,
			Args:   map[string]string{"path": arg, "query": arg, "question": arg, "result": arg},
			Reason: think,
		}
		result := cfg.Tools.Execute(call)

		stepData.Observation = result.Output
		stepData.IsFinal = action == "done"

		if cfg.OnStep != nil {
			cfg.OnStep(stepData)
		}

		if action == "done" {
			finalResult = arg
			return finalResult, nil
		}

		// Add to conversation: assistant's response + observation
		messages = append(messages,
			models.ChatMessage{Role: "assistant", Content: llmOutput},
			models.ChatMessage{Role: "user", Content: fmt.Sprintf("OBSERVATION:\n%s\n\nContinue with your next THINK and ACTION.", result.Output)},
		)
	}

	return finalResult, fmt.Errorf("agent exceeded maximum steps (%d)", maxSteps)
}

// parseReActOutput extracts THINK, ACTION, ARG from LLM output
func parseReActOutput(output string) (think, action, arg string) {
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		upper := strings.ToUpper(trimmed)

		if strings.HasPrefix(upper, "THINK:") {
			think = strings.TrimSpace(trimmed[6:])
		} else if strings.HasPrefix(upper, "ACTION:") {
			action = strings.TrimSpace(trimmed[7:])
			// Clean up: remove backticks, extra formatting
			action = strings.Trim(action, "`* ")
			action = strings.ToLower(action)
		} else if strings.HasPrefix(upper, "ARG:") {
			arg = strings.TrimSpace(trimmed[4:])
		}
	}

	// If ARG wasn't found on its own line, try to get multi-line arg
	// (everything after "ARG:" until end)
	if arg == "" && action != "" {
		idx := strings.Index(strings.ToUpper(output), "ARG:")
		if idx >= 0 {
			arg = strings.TrimSpace(output[idx+4:])
		}
	}

	// Also handle if THINK spans multiple lines before ACTION
	if think == "" {
		thinkIdx := strings.Index(strings.ToUpper(output), "THINK:")
		actionIdx := strings.Index(strings.ToUpper(output), "ACTION:")
		if thinkIdx >= 0 && actionIdx > thinkIdx {
			think = strings.TrimSpace(output[thinkIdx+6 : actionIdx])
		}
	}

	return think, action, arg
}
