package agent

import (
	"context"
	"fmt"

	"github.com/cagri/reswe/internal/models"
	"github.com/cagri/reswe/internal/provider"
)

func runChat(ctx context.Context, p provider.Provider, model string, task *models.Task, tools *ToolSet, systemPrompt string, history []models.ChatMessage, userMessage string, onStep StepCallback, onStream provider.StreamCallback) LoopResult {
	var planSection string
	if task.ImplementationPlan != "" {
		planSection = fmt.Sprintf("\n\n## Current Implementation Plan\n%s", task.ImplementationPlan)
	}

	taskContext := fmt.Sprintf(`## Task Context
Title: %s
Description: %s
%s
Answer the user's question or help with their request. Use your tools to explore the codebase when needed.`, task.Title, task.Description, planSection)

	if len(history) > 0 {
		taskContext = ""
		if userMessage != "" {
			history = append(history, models.ChatMessage{Role: "user", Content: userMessage})
		}
	} else if userMessage != "" {
		taskContext += fmt.Sprintf("\n\nUser: %s", userMessage)
	}

	return RunLoop(ctx, LoopConfig{
		Provider:     p,
		Model:        model,
		SystemPrompt: systemPrompt,
		TaskContext:   taskContext,
		History:      history,
		Tools:        tools,
		OnStep:       onStep,
		OnStream:     onStream,
	})
}
