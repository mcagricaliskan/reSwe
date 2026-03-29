package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/cagri/reswe/internal/models"
	"github.com/cagri/reswe/internal/provider"
)

func runExecute(ctx context.Context, p provider.Provider, model string, task *models.Task, tools *ToolSet, systemPrompt string, onStep StepCallback, onStream provider.StreamCallback) LoopResult {
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

	return RunLoop(ctx, LoopConfig{
		Provider:     p,
		Model:        model,
		SystemPrompt: systemPrompt,
		TaskContext:   taskContext,
		Tools:        tools,
		OnStep:       onStep,
		OnStream:     onStream,
	})
}

// runTodoExecution runs a single TODO item in a focused ReAct loop
func runTodoExecution(ctx context.Context, p provider.Provider, model string, task *models.Task, todo *models.PlanTodo, prevResults string, tools *ToolSet, systemPrompt string, onStep StepCallback, onStream provider.StreamCallback) LoopResult {
	var context strings.Builder

	context.WriteString(fmt.Sprintf("## Current Step (TODO #%d)\n**%s**\n\n%s\n\n", todo.OrderIndex, todo.Title, todo.Description))

	if task.ImplementationPlan != "" {
		context.WriteString(fmt.Sprintf("## Overall Plan\n%s\n\n", task.ImplementationPlan))
	}

	if prevResults != "" {
		context.WriteString(fmt.Sprintf("## Previous Step Results\n%s\n", prevResults))
	}

	context.WriteString("Execute this step. Use tools to read files, then call done with your result.")

	return RunLoop(ctx, LoopConfig{
		Provider:     p,
		Model:        model,
		SystemPrompt: systemPrompt,
		TaskContext:   context.String(),
		Tools:        tools,
		OnStep:       onStep,
		OnStream:     onStream,
	})
}
