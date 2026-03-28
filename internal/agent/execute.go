package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/cagri/reswe/internal/models"
	"github.com/cagri/reswe/internal/provider"
)

func runExecute(ctx context.Context, p provider.Provider, model string, task *models.Task, tools *ToolSet, systemPrompt string, onStep StepCallback, onStream provider.StreamCallback) (string, error) {
	var extra strings.Builder
	if task.ImplementationPlan != "" {
		extra.WriteString(fmt.Sprintf("\n## Implementation Plan\n%s", task.ImplementationPlan))
	}
	if len(task.Clarifications) > 0 {
		extra.WriteString("\n## Q&A\n")
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
Read the files and implement the changes according to the plan.`, task.Title, task.Description, extra.String())

	result, err := RunLoop(ctx, LoopConfig{
		Provider:     p,
		Model:        model,
		SystemPrompt: systemPrompt,
		TaskContext:   taskContext,
		Tools:        tools,
		OnStep:       onStep,
		OnStream:     onStream,
	})
	if err != nil {
		return "", fmt.Errorf("execute agent: %w", err)
	}

	return result, nil
}
