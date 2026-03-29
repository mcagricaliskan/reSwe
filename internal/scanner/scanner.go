package scanner

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var execCommand = exec.Command

// FileInfo represents a file in the codebase
type FileInfo struct {
	Path    string `json:"path"`
	RelPath string `json:"rel_path"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size"`
}

// Scanner scans a codebase directory
type Scanner struct {
	ignorePatterns []string
}

// Default patterns to always ignore
var defaultIgnore = []string{
	".git", "node_modules", "__pycache__", ".venv", "venv",
	".idea", ".vscode", ".DS_Store", "dist", "build",
	"*.pyc", "*.pyo", "*.class", "*.o", "*.so", "*.dylib",
	"*.exe", "*.dll", "*.bin", "*.lock", "package-lock.json",
	"yarn.lock", "go.sum", "*.min.js", "*.min.css",
	"*.map", "*.wasm", "*.png", "*.jpg", "*.jpeg", "*.gif",
	"*.ico", "*.svg", "*.woff", "*.woff2", "*.ttf", "*.eot",
	"*.mp3", "*.mp4", "*.avi", "*.mov", "*.zip", "*.tar",
	"*.gz", "*.rar", "*.7z", "*.pdf",
}

// sensitivePatterns are files the agent is never allowed to read
var sensitivePatterns = []string{
	".env", ".env.*",
	"*.pem", "*.key", "*.p12", "*.pfx", "*.keystore",
	"*.secret",
	"credentials.json", "service-account*.json",
	"id_rsa", "id_ed25519", "id_dsa", "id_ecdsa",
	".npmrc", ".pypirc", ".netrc", ".docker/config.json",
}

// DefaultSensitivePatterns returns a copy of the hardcoded sensitive patterns (used as fallback).
func DefaultSensitivePatterns() []string {
	result := make([]string, len(sensitivePatterns))
	copy(result, sensitivePatterns)
	return result
}

// IsSensitive returns true if the file path matches the hardcoded sensitive patterns.
// Use IsSensitiveFor with custom patterns from the database when available.
func IsSensitive(path string) bool {
	return IsSensitiveFor(path, sensitivePatterns)
}

// IsSensitiveFor returns true if the file path matches any of the given patterns.
func IsSensitiveFor(path string, patterns []string) bool {
	name := filepath.Base(path)
	for _, pattern := range patterns {
		if name == pattern {
			return true
		}
		if matched, _ := filepath.Match(pattern, name); matched {
			return true
		}
	}
	return false
}

// PackageInfo represents a detected workspace package within a monorepo
type PackageInfo struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	RelPath string `json:"rel_path"`
	Manager string `json:"manager"` // "npm", "go", "cargo", "pip", etc.
}

// FolderAnalysis is the result of analyzing a folder's structure
type FolderAnalysis struct {
	Path        string        `json:"path"`
	Name        string        `json:"name"`
	IsGit       bool          `json:"is_git"`
	Submodules  []RepoInfo    `json:"submodules,omitempty"`
	NestedRepos []RepoInfo    `json:"nested_repos,omitempty"`
	Packages    []PackageInfo `json:"packages,omitempty"`
	Type        string        `json:"type"` // "single-repo", "monorepo", "multi-repo", "plain-folder"
}

// RepoInfo represents a discovered code project (git repo or plain folder)
type RepoInfo struct {
	Path       string `json:"path"`
	Name       string `json:"name"`
	Type       string `json:"type"`        // "git" or "folder"
	RemoteURL  string `json:"remote_url"`  // git remote origin URL (empty for non-git)
	HeadRef    string `json:"head_ref"`    // current branch (empty for non-git)
	RootCommit string `json:"root_commit"` // first commit hash (empty for non-git)
}

func New() *Scanner {
	return &Scanner{
		ignorePatterns: defaultIgnore,
	}
}

// DiscoverFolders lists all immediate subdirectories under a root directory.
// No filtering — user decides what to add. If a folder is a git repo, we note it.
func (s *Scanner) DiscoverFolders(root string) ([]RepoInfo, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	var folders []RepoInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		// Skip hidden dirs and known junk
		if strings.HasPrefix(name, ".") || name == "node_modules" || name == "__pycache__" || name == ".venv" || name == "venv" {
			continue
		}
		folders = append(folders, BuildRepoInfo(filepath.Join(root, name)))
	}

	return folders, nil
}

// DiscoverRepos is kept as an alias for backward compat with the API.
func (s *Scanner) DiscoverRepos(root string, maxDepth int) ([]RepoInfo, error) {
	return s.DiscoverFolders(root)
}

func IsGitRepo(path string) bool {
	info, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil && info.IsDir()
}

func BuildRepoInfo(path string) RepoInfo {
	info := RepoInfo{
		Path: path,
		Name: filepath.Base(path),
		Type: "folder",
	}
	if IsGitRepo(path) {
		info.Type = "git"
		info.RemoteURL = readGitRemote(path)
		info.HeadRef = readGitHead(path)
		info.RootCommit = readRootCommit(path)
	}
	return info
}

// readGitRemote extracts the remote origin URL from .git/config
func readGitRemote(repoPath string) string {
	configPath := filepath.Join(repoPath, ".git", "config")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return ""
	}

	lines := strings.Split(string(data), "\n")
	inOrigin := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == `[remote "origin"]` {
			inOrigin = true
			continue
		}
		if inOrigin {
			if strings.HasPrefix(trimmed, "[") {
				break // next section
			}
			if strings.HasPrefix(trimmed, "url = ") {
				return strings.TrimPrefix(trimmed, "url = ")
			}
		}
	}
	return ""
}

// readGitHead reads the current HEAD ref
func readGitHead(repoPath string) string {
	headPath := filepath.Join(repoPath, ".git", "HEAD")
	data, err := os.ReadFile(headPath)
	if err != nil {
		return ""
	}
	head := strings.TrimSpace(string(data))
	// "ref: refs/heads/main" → "main"
	if strings.HasPrefix(head, "ref: refs/heads/") {
		return strings.TrimPrefix(head, "ref: refs/heads/")
	}
	// Detached HEAD — return short hash
	if len(head) > 8 {
		return head[:8]
	}
	return head
}

// readRootCommit gets the hash of the very first commit in the repo.
// This is the true identity — same across clones, forks, renames, org transfers.
func readRootCommit(repoPath string) string {
	// Use git rev-list to find the root commit
	// We read it by walking the pack or loose objects, but simplest is exec
	cmd := execCommand("git", "-C", repoPath, "rev-list", "--max-parents=0", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 {
		return ""
	}
	// Return the first root commit (repos can have multiple roots from merges, take the first)
	return strings.TrimSpace(lines[0])
}

// GetRepoIdentifier returns the best stable identifier for a repo.
// Priority: root commit hash > remote URL > path
func GetRepoIdentifier(repoPath string) string {
	root := readRootCommit(repoPath)
	if root != "" {
		return "commit:" + root
	}
	remote := readGitRemote(repoPath)
	if remote != "" {
		return "remote:" + remote
	}
	return "path:" + repoPath
}

// ScanTree returns the file tree of a directory
func (s *Scanner) ScanTree(root string) ([]FileInfo, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	// Load .gitignore if it exists
	gitignore := s.loadGitignore(root)
	allPatterns := append(s.ignorePatterns, gitignore...)

	var files []FileInfo
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}

		relPath, _ := filepath.Rel(root, path)
		if relPath == "." {
			return nil
		}

		// Check ignore patterns
		name := info.Name()
		for _, pattern := range allPatterns {
			if matchPattern(name, relPath, pattern) {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		files = append(files, FileInfo{
			Path:    path,
			RelPath: relPath,
			IsDir:   info.IsDir(),
			Size:    info.Size(),
		})

		return nil
	})

	return files, err
}

// ReadFile reads a file's content (with size limit)
func (s *Scanner) ReadFile(path string, maxBytes int64) (string, error) {
	if maxBytes <= 0 {
		maxBytes = 100 * 1024 // 100KB default
	}

	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}

	if info.Size() > maxBytes {
		return "", fmt.Errorf("file too large: %d bytes (limit %d)", info.Size(), maxBytes)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

// BuildTreeString creates a text representation of the file tree
func (s *Scanner) BuildTreeString(files []FileInfo) string {
	var b strings.Builder
	for _, f := range files {
		if f.IsDir {
			b.WriteString(f.RelPath + "/\n")
		} else {
			b.WriteString(f.RelPath + "\n")
		}
	}
	return b.String()
}

// FindKeyFiles returns paths to important project files
func (s *Scanner) FindKeyFiles(files []FileInfo) []string {
	keyNames := map[string]bool{
		"README.md": true, "readme.md": true, "README": true,
		"package.json": true, "go.mod": true, "Cargo.toml": true,
		"pyproject.toml": true, "requirements.txt": true, "setup.py": true,
		"Makefile": true, "Dockerfile": true, "docker-compose.yml": true,
		"docker-compose.yaml": true, ".env.example": true,
		"tsconfig.json": true, "pom.xml": true, "build.gradle": true,
	}

	var result []string
	for _, f := range files {
		if f.IsDir {
			continue
		}
		base := filepath.Base(f.RelPath)
		if keyNames[base] {
			result = append(result, f.Path)
		}
	}
	return result
}

func (s *Scanner) loadGitignore(root string) []string {
	path := filepath.Join(root, ".gitignore")
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var patterns []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns
}

// AnalyzeFolder inspects a folder and detects its structure:
// git repo, submodules, nested repos, monorepo workspaces, or plain folder.
func (s *Scanner) AnalyzeFolder(path string) (*FolderAnalysis, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("not a directory: %s", path)
	}

	a := &FolderAnalysis{
		Path:  path,
		Name:  filepath.Base(path),
		IsGit: IsGitRepo(path),
	}

	// Detect submodules
	if a.IsGit {
		a.Submodules = detectSubmodules(path)
	}

	// Detect nested git repos (walk 2 levels, skip the root .git)
	a.NestedRepos = detectNestedRepos(path)

	// Detect monorepo workspace packages
	a.Packages = detectPackages(path)

	// Derive type
	switch {
	case len(a.NestedRepos) > 0 || len(a.Submodules) > 0:
		a.Type = "multi-repo"
	case len(a.Packages) > 0:
		a.Type = "monorepo"
	case a.IsGit:
		a.Type = "single-repo"
	default:
		a.Type = "plain-folder"
	}

	return a, nil
}

func detectSubmodules(repoPath string) []RepoInfo {
	cmd := execCommand("git", "-C", repoPath, "submodule", "status")
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		return nil
	}

	var subs []RepoInfo
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Format: " <hash> <path> (<describe>)" or "-<hash> <path>"
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		subPath := parts[1]
		absPath := filepath.Join(repoPath, subPath)
		subs = append(subs, BuildRepoInfo(absPath))
	}
	return subs
}

func detectNestedRepos(root string) []RepoInfo {
	var repos []RepoInfo
	// Walk 2 levels deep looking for .git dirs
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		child := filepath.Join(root, e.Name())
		if IsGitRepo(child) {
			repos = append(repos, BuildRepoInfo(child))
			continue
		}
		// Check one more level
		subEntries, err := os.ReadDir(child)
		if err != nil {
			continue
		}
		for _, se := range subEntries {
			if !se.IsDir() || strings.HasPrefix(se.Name(), ".") {
				continue
			}
			grandchild := filepath.Join(child, se.Name())
			if IsGitRepo(grandchild) {
				repos = append(repos, BuildRepoInfo(grandchild))
			}
		}
	}
	return repos
}

func detectPackages(root string) []PackageInfo {
	var pkgs []PackageInfo

	// npm/yarn/pnpm workspaces
	pkgs = append(pkgs, detectNpmWorkspaces(root)...)

	// pnpm-workspace.yaml
	pkgs = append(pkgs, detectPnpmWorkspaces(root)...)

	// go.work
	pkgs = append(pkgs, detectGoWorkspaces(root)...)

	// Cargo workspaces
	pkgs = append(pkgs, detectCargoWorkspaces(root)...)

	return pkgs
}

func detectNpmWorkspaces(root string) []PackageInfo {
	pkgPath := filepath.Join(root, "package.json")
	data, err := os.ReadFile(pkgPath)
	if err != nil {
		return nil
	}

	var pkg struct {
		Workspaces interface{} `json:"workspaces"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil || pkg.Workspaces == nil {
		return nil
	}

	// Workspaces can be []string or {"packages": []string}
	var patterns []string
	switch v := pkg.Workspaces.(type) {
	case []interface{}:
		for _, p := range v {
			if s, ok := p.(string); ok {
				patterns = append(patterns, s)
			}
		}
	case map[string]interface{}:
		if p, ok := v["packages"]; ok {
			if arr, ok := p.([]interface{}); ok {
				for _, item := range arr {
					if s, ok := item.(string); ok {
						patterns = append(patterns, s)
					}
				}
			}
		}
	}

	return expandWorkspaceGlobs(root, patterns, "npm")
}

func detectPnpmWorkspaces(root string) []PackageInfo {
	wsPath := filepath.Join(root, "pnpm-workspace.yaml")
	data, err := os.ReadFile(wsPath)
	if err != nil {
		return nil
	}

	// Simple YAML parsing for "packages:" list — no external dependency
	var patterns []string
	inPackages := false
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "packages:" {
			inPackages = true
			continue
		}
		if inPackages {
			if strings.HasPrefix(trimmed, "- ") {
				pattern := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
				pattern = strings.Trim(pattern, "'\"")
				patterns = append(patterns, pattern)
			} else if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
				break
			}
		}
	}

	return expandWorkspaceGlobs(root, patterns, "pnpm")
}

func detectGoWorkspaces(root string) []PackageInfo {
	wsPath := filepath.Join(root, "go.work")
	data, err := os.ReadFile(wsPath)
	if err != nil {
		return nil
	}

	var pkgs []PackageInfo
	inUse := false
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "use (" {
			inUse = true
			continue
		}
		if inUse {
			if trimmed == ")" {
				break
			}
			modPath := strings.TrimSpace(trimmed)
			if modPath == "" || strings.HasPrefix(modPath, "//") {
				continue
			}
			absPath := filepath.Join(root, modPath)
			if info, err := os.Stat(absPath); err == nil && info.IsDir() {
				pkgs = append(pkgs, PackageInfo{
					Path:    absPath,
					Name:    filepath.Base(modPath),
					RelPath: modPath,
					Manager: "go",
				})
			}
		}
	}
	return pkgs
}

func detectCargoWorkspaces(root string) []PackageInfo {
	cargoPath := filepath.Join(root, "Cargo.toml")
	data, err := os.ReadFile(cargoPath)
	if err != nil {
		return nil
	}

	// Simple TOML parsing for [workspace] members
	var patterns []string
	inWorkspace := false
	inMembers := false
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "[workspace]" {
			inWorkspace = true
			continue
		}
		if inWorkspace && strings.HasPrefix(trimmed, "members") {
			inMembers = true
			// Could be on same line: members = ["a", "b"]
			if idx := strings.Index(trimmed, "["); idx >= 0 {
				content := trimmed[idx:]
				content = strings.Trim(content, "[]")
				for _, m := range strings.Split(content, ",") {
					m = strings.TrimSpace(m)
					m = strings.Trim(m, "\"'")
					if m != "" {
						patterns = append(patterns, m)
					}
				}
				if strings.Contains(trimmed, "]") {
					inMembers = false
				}
			}
			continue
		}
		if inMembers {
			if strings.Contains(trimmed, "]") {
				inMembers = false
				continue
			}
			m := strings.TrimSpace(trimmed)
			m = strings.Trim(m, "\"',")
			if m != "" {
				patterns = append(patterns, m)
			}
		}
		if inWorkspace && strings.HasPrefix(trimmed, "[") && trimmed != "[workspace]" {
			break
		}
	}

	return expandWorkspaceGlobs(root, patterns, "cargo")
}

func expandWorkspaceGlobs(root string, patterns []string, manager string) []PackageInfo {
	var pkgs []PackageInfo
	seen := make(map[string]bool)

	for _, pattern := range patterns {
		// Expand glob
		matches, err := filepath.Glob(filepath.Join(root, pattern))
		if err != nil {
			continue
		}
		for _, m := range matches {
			if seen[m] {
				continue
			}
			info, err := os.Stat(m)
			if err != nil || !info.IsDir() {
				continue
			}
			seen[m] = true
			rel, _ := filepath.Rel(root, m)
			pkgs = append(pkgs, PackageInfo{
				Path:    m,
				Name:    filepath.Base(m),
				RelPath: rel,
				Manager: manager,
			})
		}
	}
	return pkgs
}

func matchPattern(name, relPath, pattern string) bool {
	// Strip leading/trailing slashes for directory patterns
	pattern = strings.TrimRight(pattern, "/")

	// Exact name match
	if name == pattern {
		return true
	}

	// Glob match on name
	if matched, _ := filepath.Match(pattern, name); matched {
		return true
	}

	// Glob match on relative path
	if matched, _ := filepath.Match(pattern, relPath); matched {
		return true
	}

	// Check if any path component matches
	parts := strings.Split(relPath, string(filepath.Separator))
	for _, part := range parts {
		if part == pattern {
			return true
		}
		if matched, _ := filepath.Match(pattern, part); matched {
			return true
		}
	}

	return false
}
