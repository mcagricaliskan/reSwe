package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cagri/reswe/internal/models"
)

// newTestStore creates an in-memory SQLite store for testing.
func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	s, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// seedProject creates a project + repo and returns their IDs.
func seedProject(t *testing.T, s *SQLiteStore) (projectID, repoID int64) {
	t.Helper()
	p, err := s.CreateProject("test-project", "")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	r, err := s.AddRepoFull(p.ID, "/fake/path", "fakerepo", "", "ident", "main")
	if err != nil {
		t.Fatalf("add repo: %v", err)
	}
	return p.ID, r.ID
}

// fakeTree builds a []ProjectFile that mirrors testdata/fakerepo.
// Paths are prefixed with repo name (like real sync does: "fakerepo/cmd/serve.go").
func fakeTree(projectID, repoID int64) []models.ProjectFile {
	const repo = "fakerepo"

	dirs := []string{
		repo,
		repo + "/cmd",
		repo + "/docs",
		repo + "/internal",
		repo + "/internal/handlers",
		repo + "/internal/utils",
		repo + "/pkg",
		repo + "/pkg/models",
	}
	files := []struct {
		path string
		size int64
	}{
		{repo + "/Makefile", 120},
		{repo + "/README.md", 450},
		{repo + "/go.mod", 80},
		{repo + "/main.go", 300},
		{repo + "/cmd/migrate.go", 200},
		{repo + "/cmd/serve.go", 250},
		{repo + "/docs/setup.md", 900},
		{repo + "/internal/handlers/auth.go", 1500},
		{repo + "/internal/handlers/user.go", 2000},
		{repo + "/internal/utils/helpers.go", 600},
		{repo + "/pkg/models/project.go", 1100},
		{repo + "/pkg/models/user.go", 800},
	}

	var out []models.ProjectFile
	for _, d := range dirs {
		out = append(out, models.ProjectFile{
			ProjectID: projectID, RepoID: repoID,
			RelPath: d, Size: 0, IsDir: true,
		})
	}
	for _, f := range files {
		out = append(out, models.ProjectFile{
			ProjectID: projectID, RepoID: repoID,
			RelPath: f.path, Size: f.size, IsDir: false,
		})
	}
	return out
}

func TestSyncProjectFiles(t *testing.T) {
	s := newTestStore(t)
	pid, rid := seedProject(t, s)
	tree := fakeTree(pid, rid)

	if err := s.SyncProjectFiles(pid, tree); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// Should find everything
	all, err := s.SearchProjectFiles(pid, "", 100)
	if err != nil {
		t.Fatalf("search all: %v", err)
	}
	if len(all) != len(tree) {
		t.Errorf("expected %d entries, got %d", len(tree), len(all))
	}

	// Re-sync with fewer files should replace, not append
	small := tree[:5]
	if err := s.SyncProjectFiles(pid, small); err != nil {
		t.Fatalf("re-sync: %v", err)
	}
	all, _ = s.SearchProjectFiles(pid, "", 100)
	if len(all) != 5 {
		t.Errorf("after re-sync expected 5, got %d", len(all))
	}
}

func TestSearchFoldersFirst(t *testing.T) {
	s := newTestStore(t)
	pid, rid := seedProject(t, s)
	if err := s.SyncProjectFiles(pid, fakeTree(pid, rid)); err != nil {
		t.Fatal(err)
	}

	results, err := s.SearchProjectFiles(pid, "internal", 20)
	if err != nil {
		t.Fatal(err)
	}

	if len(results) == 0 {
		t.Fatal("expected results for 'internal'")
	}

	// First results should be directories
	if !results[0].IsDir {
		t.Errorf("expected first result to be a directory, got file: %s", results[0].RelPath)
	}

	// All dirs should come before any file
	seenFile := false
	for _, r := range results {
		if !r.IsDir {
			seenFile = true
		}
		if r.IsDir && seenFile {
			t.Errorf("directory %q appeared after a file — folders should come first", r.RelPath)
			break
		}
	}
}

func TestSearchPathFragments(t *testing.T) {
	s := newTestStore(t)
	pid, rid := seedProject(t, s)
	s.SyncProjectFiles(pid, fakeTree(pid, rid))

	tests := []struct {
		query   string
		wantAny []string // at least these rel_paths should appear
	}{
		// Typing repo name shows the root folder
		{"fakerepo", []string{"fakerepo"}},
		// Typing "int/han" should match internal/handlers (folder) and its files
		{"int/han", []string{"fakerepo/internal/handlers", "fakerepo/internal/handlers/auth.go", "fakerepo/internal/handlers/user.go"}},
		// Typing "pkg/mod" should match pkg/models folder and its files
		{"pkg/mod", []string{"fakerepo/pkg/models", "fakerepo/pkg/models/user.go", "fakerepo/pkg/models/project.go"}},
		// Typing "serve" should match cmd/serve.go
		{"serve", []string{"fakerepo/cmd/serve.go"}},
		// Typing "main" should match main.go
		{"main", []string{"fakerepo/main.go"}},
		// Typing full path
		{"fakerepo/internal/utils/helpers.go", []string{"fakerepo/internal/utils/helpers.go"}},
		// Folder inside repo
		{"fakerepo/cmd", []string{"fakerepo/cmd"}},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			results, err := s.SearchProjectFiles(pid, tt.query, 20)
			if err != nil {
				t.Fatal(err)
			}
			got := make(map[string]bool)
			for _, r := range results {
				got[r.RelPath] = true
			}
			for _, want := range tt.wantAny {
				if !got[want] {
					t.Errorf("query %q: expected %q in results, got %v", tt.query, want, keys(got))
				}
			}
		})
	}
}

func TestSearchIsDirFlag(t *testing.T) {
	s := newTestStore(t)
	pid, rid := seedProject(t, s)
	s.SyncProjectFiles(pid, fakeTree(pid, rid))

	results, _ := s.SearchProjectFiles(pid, "cmd", 20)
	foundDir := false
	foundFile := false
	for _, r := range results {
		if r.RelPath == "fakerepo/cmd" && r.IsDir {
			foundDir = true
		}
		if r.RelPath == "fakerepo/cmd/serve.go" && !r.IsDir {
			foundFile = true
		}
	}
	if !foundDir {
		t.Error("expected fakerepo/cmd directory with IsDir=true")
	}
	if !foundFile {
		t.Error("expected fakerepo/cmd/serve.go with IsDir=false")
	}
}

func TestSearchEmpty(t *testing.T) {
	s := newTestStore(t)
	pid, rid := seedProject(t, s)
	s.SyncProjectFiles(pid, fakeTree(pid, rid))

	// Empty query returns all (up to limit)
	results, err := s.SearchProjectFiles(pid, "", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 5 {
		t.Errorf("expected 5 results for empty query with limit 5, got %d", len(results))
	}
}

func TestSearchNoResults(t *testing.T) {
	s := newTestStore(t)
	pid, rid := seedProject(t, s)
	s.SyncProjectFiles(pid, fakeTree(pid, rid))

	results, err := s.SearchProjectFiles(pid, "zzz_nonexistent_zzz", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestSearchWildcardEscape(t *testing.T) {
	s := newTestStore(t)
	pid, rid := seedProject(t, s)

	// Insert a file with % and _ in path (edge case)
	weird := []models.ProjectFile{
		{ProjectID: pid, RepoID: rid, RelPath: "data/100%_done.txt", Size: 10},
		{ProjectID: pid, RepoID: rid, RelPath: "data/normal.txt", Size: 10},
	}
	s.SyncProjectFiles(pid, weird)

	// Searching for "100%" should match only the weird file, not everything
	results, _ := s.SearchProjectFiles(pid, "100%", 20)
	if len(results) != 1 {
		t.Errorf("expected 1 result for '100%%', got %d", len(results))
	}
}

// TestTestdataFixtureExists verifies the testdata directory is in place.
func TestTestdataFixtureExists(t *testing.T) {
	fixture := filepath.Join("testdata", "fakerepo")
	info, err := os.Stat(fixture)
	if err != nil {
		t.Fatalf("testdata/fakerepo missing: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("testdata/fakerepo is not a directory")
	}

	// Spot-check a few paths
	for _, path := range []string{
		"cmd/serve.go",
		"internal/handlers/auth.go",
		"pkg/models/user.go",
		"docs/setup.md",
	} {
		full := filepath.Join(fixture, path)
		if _, err := os.Stat(full); err != nil {
			t.Errorf("expected fixture file %s: %v", path, err)
		}
	}
}

func keys(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}
