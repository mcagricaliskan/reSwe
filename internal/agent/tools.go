package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	Pause   bool   `json:"pause"` // if true, agent loop pauses and waits for user input
}

// ToolSet holds the available tools and the context to execute them
type ToolSet struct {
	scanner         *scanner.Scanner
	repos           []models.Repo
	excludePatterns []string
	// For ask_user: pending questions
	pendingQuestions []string
	userAnswers     map[string]string
	// Review mode: when true, write/edit operations produce diffs and pause instead of applying
	ReviewMode    bool
	PendingChange *models.PendingChange // the last proposed change (for approval flow)
	CurrentTodoID int64                  // which TODO is being executed (for linking changes)
	TaskID        int64                  // for linking pending changes
	RunID         int64                  // for linking pending changes
}

func NewToolSet(sc *scanner.Scanner, repos []models.Repo, excludePatterns []string) *ToolSet {
	// Fall back to hardcoded defaults if no patterns configured
	if len(excludePatterns) == 0 {
		excludePatterns = scanner.DefaultSensitivePatterns()
	}
	return &ToolSet{
		scanner:         sc,
		repos:           repos,
		excludePatterns: excludePatterns,
		userAnswers:     make(map[string]string),
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
			Name:        "write_file",
			Description: "Create a new file or completely overwrite an existing file. Use for new files or when most of the file changes.",
			Parameters:  "FILE: string (repo-prefixed path), CONTENT: string (complete file content)",
		},
		{
			Name:        "edit_file",
			Description: "Replace a specific section of a file. The OLD content must exactly match text in the file (including indentation). Read the file first with read_file, then copy the exact text you want to replace. Include 3-5 lines of context to ensure a unique match.",
			Parameters:  "FILE: string (repo-prefixed path), OLD: string (exact text to find), NEW: string (replacement text)",
		},
		{
			Name:        "ask_user",
			Description: "Ask the user a question. The agent loop will PAUSE until the user answers. Use this when: requirements are ambiguous, there are multiple valid approaches, something seems risky or against best practices, or you need a decision you can't make from code alone.",
			Parameters:  "question: string (specific, actionable question)",
		},
		{
			Name:        "done",
			Description: "Finish and output your final result. The ARG is the DELIVERABLE — a clean document, not your thinking process. Write it as if handing a document to another engineer. No 'I', no 'Let me', no reasoning — just the content.",
			Parameters:  "result: string (clean markdown document — your final deliverable)",
		},
	}
}

// ToolDescriptionBlock returns a formatted string for the system prompt
func (ts *ToolSet) ToolDescriptionBlock() string {
	var b strings.Builder
	b.WriteString("## Available Tools\n\n")
	b.WriteString("You MUST use tools to explore the codebase. Do NOT guess about file contents or structure.\n")
	b.WriteString("Call tools using this exact format:\n\n")
	b.WriteString("### Single-argument tools (read_file, list_dir, search_code, ask_user, done)\n")
	b.WriteString("```\n")
	b.WriteString("THINK: <your reasoning about what to do next>\n")
	b.WriteString("ACTION: <tool_name>\n")
	b.WriteString("ARG: <argument>\n")
	b.WriteString("```\n\n")
	b.WriteString("### edit_file (modify part of a file)\n")
	b.WriteString("```\n")
	b.WriteString("THINK: <reasoning>\n")
	b.WriteString("ACTION: edit_file\n")
	b.WriteString("ARG:\n")
	b.WriteString("FILE: repo-name/path/to/file.ext\n")
	b.WriteString("OLD:\n")
	b.WriteString("<exact text from the file to replace — must match uniquely>\n")
	b.WriteString("NEW:\n")
	b.WriteString("<replacement text>\n")
	b.WriteString("```\n\n")
	b.WriteString("### write_file (create or overwrite entire file)\n")
	b.WriteString("```\n")
	b.WriteString("THINK: <reasoning>\n")
	b.WriteString("ACTION: write_file\n")
	b.WriteString("ARG:\n")
	b.WriteString("FILE: repo-name/path/to/file.ext\n")
	b.WriteString("CONTENT:\n")
	b.WriteString("<complete file content>\n")
	b.WriteString("```\n\n")
	b.WriteString("IMPORTANT: For edit_file, you MUST read_file first and use the EXACT text from the file in OLD.\n")
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
	b.WriteString("\nWhen finished, call `done`. The ARG of done is your DELIVERABLE — a clean document.\n")
	b.WriteString("Put ALL your reasoning in THINK lines. The done ARG must contain ZERO thinking — only the final output.\n")
	b.WriteString("IMPORTANT: Explore the codebase (list_dir, read_file, search_code) BEFORE making changes.\n")
	b.WriteString("IMPORTANT: ALWAYS read_file BEFORE using edit_file — you need the exact text to match.\n")
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
	case "write_file":
		return ts.writeFile(call.Args["path"], call.Args["content"])
	case "edit_file":
		return ts.editFile(call.Args["path"], call.Args["old_content"], call.Args["new_content"])
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

	// Check sensitive file blocklist (uses configurable patterns)
	if scanner.IsSensitiveFor(path, ts.excludePatterns) {
		return ToolResult{Tool: "read_file", Success: false, Output: fmt.Sprintf("Access denied: %s is a sensitive file", path)}
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

	// Also check the resolved absolute path
	if scanner.IsSensitiveFor(resolved, ts.excludePatterns) {
		return ToolResult{Tool: "read_file", Success: false, Output: fmt.Sprintf("Access denied: %s is a sensitive file", path)}
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

			// Skip sensitive files (uses configurable patterns)
			if scanner.IsSensitiveFor(f.RelPath, ts.excludePatterns) || scanner.IsSensitiveFor(f.Path, ts.excludePatterns) {
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

func (ts *ToolSet) writeFile(path, content string) ToolResult {
	if path == "" {
		return ToolResult{Tool: "write_file", Success: false, Output: "Error: path is required. Use format:\nFILE: repo-name/path/to/file\nCONTENT:\n<file content>"}
	}
	if content == "" {
		return ToolResult{Tool: "write_file", Success: false, Output: "Error: content is required"}
	}

	// Block sensitive files
	if scanner.IsSensitiveFor(path, ts.excludePatterns) {
		return ToolResult{Tool: "write_file", Success: false, Output: fmt.Sprintf("Access denied: %s is a sensitive file", path)}
	}

	// Resolve path — for new files, parent dir must exist
	resolved, ok := ts.resolvePath(path)
	if !ok {
		// Try to resolve the parent directory — file might not exist yet (new file)
		dir := filepath.Dir(path)
		resolvedDir, dirOk := ts.resolvePath(dir)
		if !dirOk {
			var repos []string
			for _, r := range ts.repos {
				repos = append(repos, r.Name)
			}
			return ToolResult{
				Tool:    "write_file",
				Success: false,
				Output:  fmt.Sprintf("Directory not found: %s\nAvailable repos: %s\nUse format: repo-name/path/to/file", dir, strings.Join(repos, ", ")),
			}
		}
		resolved = filepath.Join(resolvedDir, filepath.Base(path))
	}

	if scanner.IsSensitiveFor(resolved, ts.excludePatterns) {
		return ToolResult{Tool: "write_file", Success: false, Output: fmt.Sprintf("Access denied: %s is a sensitive file", path)}
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(resolved), 0755); err != nil {
		return ToolResult{Tool: "write_file", Success: false, Output: fmt.Sprintf("Error creating directory: %v", err)}
	}

	// ReviewMode: propose change instead of applying
	if ts.ReviewMode {
		oldContent := ""
		if data, err := os.ReadFile(resolved); err == nil {
			oldContent = string(data)
		}
		diff := UnifiedDiff(oldContent, content, path)
		ts.PendingChange = &models.PendingChange{
			ID:         fmt.Sprintf("%d-%d", ts.TaskID, time.Now().UnixNano()),
			RunID:      ts.RunID,
			TodoID:     ts.CurrentTodoID,
			TaskID:     ts.TaskID,
			Tool:       "write_file",
			FilePath:   resolved,
			RelPath:    path,
			OldContent: oldContent,
			NewContent: content,
			Diff:       diff,
			Status:     "pending",
			CreatedAt:  time.Now(),
		}
		return ToolResult{
			Tool:    "write_file",
			Success: true,
			Output:  fmt.Sprintf("Change proposed for %s. Waiting for user approval.\n\n%s", path, diff),
			Pause:   true,
		}
	}

	if err := os.WriteFile(resolved, []byte(content), 0644); err != nil {
		return ToolResult{Tool: "write_file", Success: false, Output: fmt.Sprintf("Error writing %s: %v", path, err)}
	}

	return ToolResult{Tool: "write_file", Success: true, Output: fmt.Sprintf("Successfully wrote %s (%d bytes)", path, len(content))}
}

func (ts *ToolSet) editFile(path, oldContent, newContent string) ToolResult {
	if path == "" {
		return ToolResult{Tool: "edit_file", Success: false, Output: "Error: path is required. Use format:\nFILE: repo-name/path/to/file\nOLD:\n<old content>\nNEW:\n<new content>"}
	}
	if oldContent == "" {
		return ToolResult{Tool: "edit_file", Success: false, Output: "Error: OLD content is required — this is the exact text to find and replace"}
	}

	// Block sensitive files
	if scanner.IsSensitiveFor(path, ts.excludePatterns) {
		return ToolResult{Tool: "edit_file", Success: false, Output: fmt.Sprintf("Access denied: %s is a sensitive file", path)}
	}

	resolved, ok := ts.resolvePath(path)
	if !ok {
		var repos []string
		for _, r := range ts.repos {
			repos = append(repos, r.Name)
		}
		return ToolResult{
			Tool:    "edit_file",
			Success: false,
			Output:  fmt.Sprintf("File not found: %s\nAvailable repos: %s\nUse format: repo-name/path/to/file", path, strings.Join(repos, ", ")),
		}
	}

	if scanner.IsSensitiveFor(resolved, ts.excludePatterns) {
		return ToolResult{Tool: "edit_file", Success: false, Output: fmt.Sprintf("Access denied: %s is a sensitive file", path)}
	}

	data, err := os.ReadFile(resolved)
	if err != nil {
		return ToolResult{Tool: "edit_file", Success: false, Output: fmt.Sprintf("Error reading %s: %v", path, err)}
	}

	fileContent := string(data)

	// Count occurrences of old_content
	count := strings.Count(fileContent, oldContent)
	if count == 0 {
		// Try trimming trailing whitespace from each line as a fallback
		oldTrimmed := trimLineEnds(oldContent)
		fileTrimmed := trimLineEnds(fileContent)
		if strings.Count(fileTrimmed, oldTrimmed) > 0 {
			return ToolResult{
				Tool:    "edit_file",
				Success: false,
				Output:  "OLD content not found (exact match). Possible whitespace mismatch — read the file first with read_file and copy the exact content.",
			}
		}
		return ToolResult{
			Tool:    "edit_file",
			Success: false,
			Output:  "OLD content not found in file. Read the file first with read_file and use the exact text you see.",
		}
	}
	if count > 1 {
		return ToolResult{
			Tool:    "edit_file",
			Success: false,
			Output:  fmt.Sprintf("OLD content matches %d locations in the file. Include more surrounding context to make it unique.", count),
		}
	}

	// Exactly one match — apply the replacement
	newFile := strings.Replace(fileContent, oldContent, newContent, 1)

	// ReviewMode: propose change instead of applying
	if ts.ReviewMode {
		diff := UnifiedDiff(fileContent, newFile, path)
		ts.PendingChange = &models.PendingChange{
			ID:         fmt.Sprintf("%d-%d", ts.TaskID, time.Now().UnixNano()),
			RunID:      ts.RunID,
			TodoID:     ts.CurrentTodoID,
			TaskID:     ts.TaskID,
			Tool:       "edit_file",
			FilePath:   resolved,
			RelPath:    path,
			OldContent: fileContent,
			NewContent: newFile,
			Diff:       diff,
			Status:     "pending",
			CreatedAt:  time.Now(),
		}
		return ToolResult{
			Tool:    "edit_file",
			Success: true,
			Output:  fmt.Sprintf("Change proposed for %s. Waiting for user approval.\n\n%s", path, diff),
			Pause:   true,
		}
	}

	if err := os.WriteFile(resolved, []byte(newFile), 0644); err != nil {
		return ToolResult{Tool: "edit_file", Success: false, Output: fmt.Sprintf("Error writing %s: %v", path, err)}
	}

	return ToolResult{Tool: "edit_file", Success: true, Output: fmt.Sprintf("Successfully edited %s (replaced %d bytes with %d bytes)", path, len(oldContent), len(newContent))}
}

// trimLineEnds trims trailing whitespace from each line (for fuzzy whitespace comparison)
func trimLineEnds(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.Join(lines, "\n")
}

func (ts *ToolSet) askUser(question string) ToolResult {
	if question == "" {
		return ToolResult{Tool: "ask_user", Success: false, Output: "Error: question is required"}
	}
	ts.pendingQuestions = append(ts.pendingQuestions, question)
	return ToolResult{
		Tool:    "ask_user",
		Success: true,
		Output:  fmt.Sprintf("Question sent to user: %s\n(Agent will pause and wait for answer.)", question),
		Pause:   true,
	}
}

// GetPendingQuestions returns questions the agent wants to ask the user
func (ts *ToolSet) GetPendingQuestions() []string {
	return ts.pendingQuestions
}
