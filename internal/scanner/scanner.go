package scanner

import (
	"bufio"
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
