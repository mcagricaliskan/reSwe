package agent

import (
	"fmt"
	"strings"

	"github.com/cagri/reswe/internal/models"
)

// PhaseInfo describes what each agent phase does
type PhaseInfo struct {
	Phase       string `json:"phase"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
}

var Phases = []PhaseInfo{
	{
		Phase:       "plan",
		Name:        "Plan",
		Description: "Agent reads your codebase, understands the task, and proposes a detailed implementation plan. It will explore files, search for patterns, and ask you questions if anything is unclear.",
		Icon:        "file-text",
	},
	{
		Phase:       "execute",
		Name:        "Execute",
		Description: "Agent follows the approved plan to generate the actual code changes. Reads current file contents and outputs the modifications.",
		Icon:        "play",
	},
}

// PromptPreview contains the assembled prompts that would be sent to the AI
type PromptPreview struct {
	Phase        string `json:"phase"`
	SystemPrompt string `json:"system_prompt"`
	UserPrompt   string `json:"user_prompt"`
	ContextSize  int    `json:"context_size_chars"`
}

// Default system prompts
var DefaultSystemPrompts = map[string]string{
	"plan": `You are a senior software engineer. You've been given a task and access to a codebase.

Your job:
1. Explore the codebase to understand the architecture, patterns, and relevant files
2. If anything about the task is unclear, use ask_user to get clarification
3. Create a detailed implementation plan

How to work:
- Start by listing directories and reading key files (README, config, entry points)
- Search for relevant code patterns, function definitions, and existing implementations
- Read the actual files you'll need to modify — don't guess
- If you're unsure about requirements or approach, ask the user before planning

Your final plan (via the done tool) should be in markdown with:
1. **Summary** — What you understood and your approach
2. **Files to modify** — Each file with specific changes needed
3. **Files to create** — New files if any
4. **Steps** — Ordered implementation steps with detail
5. **Questions** — Any remaining concerns (if none, skip this)

Be specific. Reference real file paths and actual code you read.`,

	"execute": `You are a senior software engineer implementing code changes.
You have an approved implementation plan. Your job is to generate the actual code.

How to work:
- Read each file that needs modification to see its current content
- Generate the complete new file content (not diffs)
- Follow the plan closely but use your judgment on implementation details
- Match the existing code style

Your final output (via the done tool) should list each file change:

=== FILE: repo-name/path/to/file.ext ===
ACTION: create|modify|delete
` + "```" + `
<complete file content>
` + "```" + `

Rules:
- Show complete file content for modifications (not partial diffs)
- Include all imports
- Match existing code style
- One file block per file, no explanations between them`,
}

// BuildUserPrompt assembles the user prompt for a given phase
func BuildUserPrompt(phase string, task *models.Task, codebaseCtx string) string {
	switch phase {
	case "plan":
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
		return fmt.Sprintf(`## Task
Title: %s
Description: %s
%s
Explore the codebase and create an implementation plan. If anything is unclear, ask me.`, task.Title, task.Description, extra.String())

	case "execute":
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
		return fmt.Sprintf(`## Task
Title: %s
Description: %s
%s
Read the files and implement the changes according to the plan.`, task.Title, task.Description, extra.String())
	}
	return ""
}
