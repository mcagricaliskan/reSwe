package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cagri/reswe/internal/models"
	"github.com/cagri/reswe/internal/scanner"
)

// Tool represents an action the agent can take
type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  string `json:"parameters"`
}

// ToolCall is a parsed tool invocation from the LLM
type ToolCall struct {
	Tool   string            `json:"tool"`
	Args   map[string]string `json:"args"`
	Reason string            `json:"reason"`
}

// ToolResult is what comes back after executing a tool
type ToolResult struct {
	Tool    string `json:"tool"`
	Success bool   `json:"success"`
	Output  string `json:"output"`
}

// ToolSet holds the available tools and the context to execute them
type ToolSet struct {
	scanner *scanner.Scanner
	repos   []models.Repo
	// For ask_user: pending questions get emitted, answers come back later
	pendingQuestions []string
	userAnswers     map[string]string
}

func NewToolSet(sc *scanner.Scanner, repos []models.Repo) *ToolSet {
	return &ToolSet{
		scanner:     sc,
		repos:       repos,
		userAnswers: make(map[string]string),
	}
}

// Available returns the tool definitions for the system prompt
func (ts *ToolSet) Available() []Tool {
	return []Tool{
		{
			Name:        "read_file",
			Description: "Read the contents of a file. Use repo-prefixed paths like 'repo-name/src/main.go' or relative paths within a single-repo project.",
			Parameters:  "path: string (e.g. 'my-service/src/main.go' or 'src/main.go')",
		},
		{
			Name:        "search_code",
			Description: "Search for a text pattern across all files in all repos. Returns matching lines with repo-prefixed file paths.",
			Parameters:  "query: string (text or pattern to search for)",
		},
		{
			Name:        "list_dir",
			Description: "List files and subdirectories. Use '.' for root (shows all repos), or 'repo-name/subfolder' to list within a specific repo.",
			Parameters:  "path: string (e.g. '.', 'my-service', 'my-service/src')",
		},
		{
			Name:        "ask_user",
			Description: "Ask the user a question when you need clarification or a decision that you cannot determine from the code.",
			Parameters:  "question: string (specific, actionable question for the user)",
		},
		{
			Name:        "done",
			Description: "Signal that you have completed your task. Include your final output/conclusion.",
			Parameters:  "result: string (your final output — analysis, plan, questions summary, or code changes)",
		},
	}
}

// ToolDescriptionBlock returns a formatted string for the system prompt
func (ts *ToolSet) ToolDescriptionBlock() string {
	var b strings.Builder
	b.WriteString("## Available Tools\n\n")
	b.WriteString("You MUST use tools to explore the codebase. Do NOT guess about file contents or structure.\n")
	b.WriteString("Call tools using this exact format:\n\n")
	b.WriteString("```\n")
	b.WriteString("THINK: <your reasoning about what to do next>\n")
	b.WriteString("ACTION: <tool_name>\n")
	b.WriteString("ARG: <argument>\n")
	b.WriteString("```\n\n")
	b.WriteString("Wait for the OBSERVATION before continuing. Then THINK again.\n\n")

	// Show repo names so agent knows how to address them
	b.WriteString("## Repos in this project\n")
	for _, r := range ts.repos {
		b.WriteString(fmt.Sprintf("- **%s** (%s)\n", r.Name, r.Path))
	}
	b.WriteString("\nUse `repo-name/path` format for paths. Example: `list_dir(my-service/src)` or `read_file(my-service/go.mod)`\n")
	b.WriteString("Use `.` to list all repos at the top level.\n\n")

	b.WriteString("## Tools\n\n")
	for _, t := range ts.Available() {
		b.WriteString(fmt.Sprintf("- **%s** — %s\n  Parameter: %s\n\n", t.Name, t.Description, t.Parameters))
	}
	b.WriteString("\nWhen you are finished, use the `done` tool with your final result.\n")
	b.WriteString("IMPORTANT: You must call at least one tool before calling `done`. Actually read files and explore.\n")
	return b.String()
}

// Execute runs a tool and returns the result
func (ts *ToolSet) Execute(call ToolCall) ToolResult {
	switch call.Tool {
	case "read_file":
		return ts.readFile(call.Args["path"])
	case "search_code":
		return ts.searchCode(call.Args["query"])
	case "list_dir":
		return ts.listDir(call.Args["path"])
	case "ask_user":
		return ts.askUser(call.Args["question"])
	case "done":
		return ToolResult{Tool: "done", Success: true, Output: call.Args["result"]}
	default:
		return ToolResult{Tool: call.Tool, Success: false, Output: fmt.Sprintf("Unknown tool: %s", call.Tool)}
	}
}

// resolvePath takes a user-facing path like "repo-name/src/main.go" and returns
// the absolute filesystem path. It tries:
//  1. repo-name prefix match: "my-service/src/main.go" → <repo_path>/src/main.go
//  2. direct relative inside each repo: "src/main.go" → <repo_path>/src/main.go
//  3. absolute path passthrough
func (ts *ToolSet) resolvePath(path string) (string, bool) {
	if path == "" || path == "." {
		return "", false
	}

	// 1. Try repo-name prefix match
	parts := strings.SplitN(path, "/", 2)
	prefix := parts[0]
	for _, repo := range ts.repos {
		if repo.Name == prefix {
			subPath := ""
			if len(parts) > 1 {
				subPath = parts[1]
			}
			if subPath == "" || subPath == "." {
				return repo.Path, true
			}
			return filepath.Join(repo.Path, subPath), true
		}
	}

	// 2. Try as relative path inside each repo (first match wins)
	for _, repo := range ts.repos {
		full := filepath.Join(repo.Path, path)
		if _, err := os.Stat(full); err == nil {
			return full, true
		}
	}

	// 3. Absolute path
	if filepath.IsAbs(path) {
		if _, err := os.Stat(path); err == nil {
			return path, true
		}
	}

	return "", false
}

// repoNameForPath returns the repo name if the absolute path is inside a known repo
func (ts *ToolSet) repoNameForPath(absPath string) string {
	for _, repo := range ts.repos {
		if strings.HasPrefix(absPath, repo.Path) {
			return repo.Name
		}
	}
	return ""
}

func (ts *ToolSet) readFile(path string) ToolResult {
	if path == "" {
		return ToolResult{Tool: "read_file", Success: false, Output: "Error: path is required"}
	}

	resolved, ok := ts.resolvePath(path)
	if !ok {
		// Build helpful error with available repos
		var repos []string
		for _, r := range ts.repos {
			repos = append(repos, r.Name)
		}
		return ToolResult{
			Tool:    "read_file",
			Success: false,
			Output:  fmt.Sprintf("File not found: %s\nAvailable repos: %s\nUse format: repo-name/path/to/file", path, strings.Join(repos, ", ")),
		}
	}

	content, err := ts.scanner.ReadFile(resolved, 200*1024)
	if err != nil {
		return ToolResult{Tool: "read_file", Success: false, Output: fmt.Sprintf("Error reading %s: %v", path, err)}
	}

	if len(content) > 15000 {
		content = content[:15000] + "\n... (truncated, file is " + fmt.Sprintf("%d", len(content)) + " bytes)"
	}

	return ToolResult{Tool: "read_file", Success: true, Output: content}
}

func (ts *ToolSet) searchCode(query string) ToolResult {
	if query == "" {
		return ToolResult{Tool: "search_code", Success: false, Output: "Error: query is required"}
	}

	var results strings.Builder
	matchCount := 0

	for _, repo := range ts.repos {
		files, err := ts.scanner.ScanTree(repo.Path)
		if err != nil {
			continue
		}

		for _, f := range files {
			if f.IsDir || f.Size > 500*1024 {
				continue
			}

			data, err := os.ReadFile(f.Path)
			if err != nil {
				continue
			}

			lines := strings.Split(string(data), "\n")
			for i, line := range lines {
				if strings.Contains(strings.ToLower(line), strings.ToLower(query)) {
					// Always prefix with repo name so agent can use the path
					results.WriteString(fmt.Sprintf("%s/%s:%d: %s\n", repo.Name, f.RelPath, i+1, strings.TrimSpace(line)))
					matchCount++
					if matchCount >= 50 {
						results.WriteString("... (50 match limit reached)\n")
						return ToolResult{Tool: "search_code", Success: true, Output: results.String()}
					}
				}
			}
		}
	}

	if matchCount == 0 {
		return ToolResult{Tool: "search_code", Success: true, Output: fmt.Sprintf("No matches found for: %s", query)}
	}

	return ToolResult{Tool: "search_code", Success: true, Output: results.String()}
}

func (ts *ToolSet) listDir(path string) ToolResult {
	if path == "" || path == "." {
		// Root listing: show top-level of each repo
		return ts.listAllReposRoot()
	}

	resolved, ok := ts.resolvePath(path)
	if !ok {
		var repos []string
		for _, r := range ts.repos {
			repos = append(repos, r.Name)
		}
		return ToolResult{
			Tool:    "list_dir",
			Success: false,
			Output:  fmt.Sprintf("Directory not found: %s\nAvailable repos: %s\nUse format: repo-name/subfolder", path, strings.Join(repos, ", ")),
		}
	}

	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return ToolResult{Tool: "list_dir", Success: false, Output: fmt.Sprintf("Not a directory: %s", path)}
	}

	entries, err := os.ReadDir(resolved)
	if err != nil {
		return ToolResult{Tool: "list_dir", Success: false, Output: fmt.Sprintf("Error reading directory: %v", err)}
	}

	var results strings.Builder
	results.WriteString(fmt.Sprintf("=== %s ===\n", path))
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") && name != ".github" {
			continue
		}
		if name == "node_modules" || name == "__pycache__" || name == "venv" || name == ".venv" {
			continue
		}
		if e.IsDir() {
			results.WriteString(fmt.Sprintf("  %s/\n", name))
		} else {
			info, _ := e.Info()
			size := int64(0)
			if info != nil {
				size = info.Size()
			}
			results.WriteString(fmt.Sprintf("  %s (%d bytes)\n", name, size))
		}
	}

	return ToolResult{Tool: "list_dir", Success: true, Output: results.String()}
}

func (ts *ToolSet) listAllReposRoot() ToolResult {
	var results strings.Builder
	for _, repo := range ts.repos {
		entries, err := os.ReadDir(repo.Path)
		if err != nil {
			results.WriteString(fmt.Sprintf("=== %s (error: %v) ===\n", repo.Name, err))
			continue
		}

		results.WriteString(fmt.Sprintf("=== %s ===\n", repo.Name))
		for _, e := range entries {
			name := e.Name()
			if strings.HasPrefix(name, ".") && name != ".github" {
				continue
			}
			if name == "node_modules" || name == "__pycache__" || name == "venv" || name == ".venv" {
				continue
			}
			if e.IsDir() {
				results.WriteString(fmt.Sprintf("  %s/\n", name))
			} else {
				info, _ := e.Info()
				size := int64(0)
				if info != nil {
					size = info.Size()
				}
				results.WriteString(fmt.Sprintf("  %s (%d bytes)\n", name, size))
			}
		}
	}

	if results.Len() == 0 {
		return ToolResult{Tool: "list_dir", Success: false, Output: "No repos in this project"}
	}

	return ToolResult{Tool: "list_dir", Success: true, Output: results.String()}
}

func (ts *ToolSet) askUser(question string) ToolResult {
	if question == "" {
		return ToolResult{Tool: "ask_user", Success: false, Output: "Error: question is required"}
	}
	ts.pendingQuestions = append(ts.pendingQuestions, question)
	return ToolResult{
		Tool:    "ask_user",
		Success: true,
		Output:  fmt.Sprintf("Question sent to user: %s\n(Waiting for answer...)", question),
	}
}

// GetPendingQuestions returns questions the agent wants to ask the user
func (ts *ToolSet) GetPendingQuestions() []string {
	return ts.pendingQuestions
}
