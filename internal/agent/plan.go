package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/cagri/reswe/internal/models"
	"github.com/cagri/reswe/internal/provider"
)

func runPlan(ctx context.Context, p provider.Provider, model string, task *models.Task, tools *ToolSet, systemPrompt string, history []models.ChatMessage, userMessage string, onStep StepCallback, onStream provider.StreamCallback) LoopResult {
	var extra strings.Builder
	if task.ImplementationPlan != "" {
		extra.WriteString(fmt.Sprintf("\n## Previous Plan (user wants revision)\n%s", task.ImplementationPlan))
	}
	if len(task.Clarifications) > 0 {
		extra.WriteString("\n## Previous Q&A\n")
		for _, c := range task.Clarifications {
			extra.WriteString(fmt.Sprintf("Q: %s\n", c.Question))
			if c.Answer != "" {
				extra.WriteString(fmt.Sprintf("A: %s\n", c.Answer))
			}
		}
	}

	taskContext := fmt.Sprintf(`## Task
Title: %s
Description: %s
%s
Explore the codebase and create an implementation plan. If anything is unclear, ask me.`, task.Title, task.Description, extra.String())

	if len(history) > 0 {
		taskContext = ""
		if userMessage != "" {
			history = append(history, models.ChatMessage{Role: "user", Content: userMessage})
		}
	} else if userMessage != "" {
		taskContext += fmt.Sprintf("\n\n## Additional User Instructions\n%s", userMessage)
	}

	return RunLoop(ctx, LoopConfig{
		Provider:     p,
		Model:        model,
		SystemPrompt: systemPrompt,
		TaskContext:  taskContext,
		History:      history,
		Tools:        tools,
		OnStep:       onStep,
		OnStream:     onStream,
	})
}
