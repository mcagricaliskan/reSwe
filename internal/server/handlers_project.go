package server

import (
	"os"
	"path/filepath"
	"strconv"

	"github.com/cagri/reswe/internal/scanner"
	"github.com/gofiber/fiber/v3"
)

func (s *Server) handleListProjects(c fiber.Ctx) error {
	projects, err := s.store.ListProjects()
	if err != nil {
		return writeError(c, 500, err.Error())
	}
	return writeJSON(c, 200, projects)
}

func (s *Server) handleCreateProject(c fiber.Ctx) error {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return writeError(c, 400, "invalid request body")
	}
	if req.Name == "" {
		return writeError(c, 400, "name is required")
	}
	project, err := s.store.CreateProject(req.Name, req.Description)
	if err != nil {
		return writeError(c, 500, err.Error())
	}
	return writeJSON(c, 201, project)
}

func (s *Server) handleGetProject(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return writeError(c, 400, "invalid id")
	}
	project, err := s.store.GetProject(id)
	if err != nil {
		return writeError(c, 404, "project not found")
	}
	return writeJSON(c, 200, project)
}

func (s *Server) handleUpdateProject(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return writeError(c, 400, "invalid id")
	}
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return writeError(c, 400, "invalid request body")
	}
	project, err := s.store.UpdateProject(id, req.Name, req.Description)
	if err != nil {
		return writeError(c, 500, err.Error())
	}
	return writeJSON(c, 200, project)
}

func (s *Server) handleDeleteProject(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return writeError(c, 400, "invalid id")
	}
	if err := s.store.DeleteProject(id); err != nil {
		return writeError(c, 500, err.Error())
	}
	return writeJSON(c, 200, fiber.Map{"status": "deleted"})
}

func (s *Server) handleAddRepo(c fiber.Ctx) error {
	projectID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return writeError(c, 400, "invalid project id")
	}
	var req struct {
		Path string `json:"path"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return writeError(c, 400, "invalid request body")
	}
	if req.Path == "" {
		return writeError(c, 400, "path is required")
	}

	// Verify path exists and is a directory
	info, err := os.Stat(req.Path)
	if err != nil || !info.IsDir() {
		return writeError(c, 400, "path does not exist or is not a directory")
	}

	name := filepath.Base(req.Path)
	remoteURL := ""
	headRef := ""
	if scanner.IsGitRepo(req.Path) {
		repoInfo := scanner.BuildRepoInfo(req.Path)
		remoteURL = repoInfo.RemoteURL
		headRef = repoInfo.HeadRef
	}
	identifier := scanner.GetRepoIdentifier(req.Path)

	repo, err := s.store.AddRepoFull(projectID, req.Path, name, remoteURL, identifier, headRef)
	if err != nil {
		return writeError(c, 500, err.Error())
	}

	// Write .reswe marker inside the folder so we can find it if it moves
	scanner.WriteMarker(req.Path, scanner.FolderMarker{
		UUID:      repo.UUID,
		ProjectID: projectID,
		Name:      name,
		AddedAt:   repo.CreatedAt,
	})

	return writeJSON(c, 201, repo)
}

func (s *Server) handleAnalyzeFolder(c fiber.Ctx) error {
	var req struct {
		Path string `json:"path"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return writeError(c, 400, "invalid request body")
	}
	if req.Path == "" {
		return writeError(c, 400, "path is required")
	}

	analysis, err := s.scanner.AnalyzeFolder(req.Path)
	if err != nil {
		return writeError(c, 400, err.Error())
	}

	return writeJSON(c, 200, analysis)
}

func (s *Server) handleListRepos(c fiber.Ctx) error {
	projectID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return writeError(c, 400, "invalid project id")
	}
	repos, err := s.store.ListRepos(projectID)
	if err != nil {
		return writeError(c, 500, err.Error())
	}
	return writeJSON(c, 200, repos)
}

func (s *Server) handleDeleteRepo(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("repoId"), 10, 64)
	if err != nil {
		return writeError(c, 400, "invalid repo id")
	}
	if err := s.store.DeleteRepo(id); err != nil {
		return writeError(c, 500, err.Error())
	}
	return writeJSON(c, 200, fiber.Map{"status": "deleted"})
}

func (s *Server) handleDiscoverRepos(c fiber.Ctx) error {
	var req struct {
		Path     string `json:"path"`
		MaxDepth int    `json:"max_depth"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return writeError(c, 400, "invalid request body")
	}
	if req.Path == "" {
		return writeError(c, 400, "path is required")
	}
	repos, err := s.scanner.DiscoverRepos(req.Path, req.MaxDepth)
	if err != nil {
		return writeError(c, 500, err.Error())
	}
	return writeJSON(c, 200, repos)
}

func (s *Server) handleScanDirectory(c fiber.Ctx) error {
	projectID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return writeError(c, 400, "invalid project id")
	}
	var req struct {
		Path     string `json:"path"`
		MaxDepth int    `json:"max_depth"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return writeError(c, 400, "invalid request body")
	}
	if req.Path == "" {
		return writeError(c, 400, "path is required")
	}

	discovered, err := s.scanner.DiscoverRepos(req.Path, req.MaxDepth)
	if err != nil {
		return writeError(c, 500, err.Error())
	}

	var added []interface{}
	for _, d := range discovered {
		identifier := d.RemoteURL
		if identifier == "" {
			identifier = d.Path
		}
		repo, err := s.store.AddRepoFull(projectID, d.Path, d.Name, d.RemoteURL, identifier, d.HeadRef)
		if err != nil {
			continue
		}
		// Write .reswe marker
		scanner.WriteMarker(d.Path, scanner.FolderMarker{
			UUID:      repo.UUID,
			ProjectID: projectID,
			Name:      d.Name,
			AddedAt:   repo.CreatedAt,
		})
		added = append(added, repo)
	}

	return writeJSON(c, 201, fiber.Map{
		"discovered": len(discovered),
		"added":      len(added),
		"repos":      added,
	})
}

// handleReconcile checks all folders in a project — if a path is broken, tries to find
// the folder by its .reswe marker UUID. Reports what's valid, broken, and relocated.
func (s *Server) handleReconcile(c fiber.Ctx) error {
	projectID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return writeError(c, 400, "invalid project id")
	}

	var req struct {
		ScanPath string `json:"scan_path"` // optional: where to search for relocated folders
	}
	c.Bind().JSON(&req)

	repos, err := s.store.ListRepos(projectID)
	if err != nil {
		return writeError(c, 500, err.Error())
	}

	type reconcileResult struct {
		ID      int64  `json:"id"`
		UUID    string `json:"uuid"`
		Name    string `json:"name"`
		OldPath string `json:"old_path"`
		NewPath string `json:"new_path"`
		Status  string `json:"status"` // "ok", "broken", "relocated"
	}

	var results []reconcileResult

	// If a scan path was provided, scan it for markers to build a UUID→path lookup
	markerMap := make(map[string]string)
	if req.ScanPath != "" {
		markerMap = scanner.ScanForMarkers(req.ScanPath, 4)
	}

	for _, repo := range repos {
		r := reconcileResult{
			ID:      repo.ID,
			UUID:    repo.UUID,
			Name:    repo.Name,
			OldPath: repo.Path,
			NewPath: repo.Path,
		}

		// Check if current path still exists
		if _, err := os.Stat(repo.Path); err == nil {
			r.Status = "ok"
			r.Name = filepath.Base(repo.Path) // always use current folder name
			// Sync name + git info
			remoteURL := ""
			headRef := ""
			if scanner.IsGitRepo(repo.Path) {
				info := scanner.BuildRepoInfo(repo.Path)
				remoteURL = info.RemoteURL
				headRef = info.HeadRef
			}
			s.store.SyncRepo(repo.ID, repo.Path, remoteURL, headRef)
		} else {
			// Path is broken — try to find by marker UUID
			if newPath, ok := markerMap[repo.UUID]; ok {
				r.Status = "relocated"
				r.NewPath = newPath
				// Update the stored path
				remoteURL := ""
				headRef := ""
				if scanner.IsGitRepo(newPath) {
					info := scanner.BuildRepoInfo(newPath)
					remoteURL = info.RemoteURL
					headRef = info.HeadRef
				}
				s.store.SyncRepo(repo.ID, newPath, remoteURL, headRef)
			} else {
				r.Status = "broken"
			}
		}

		results = append(results, r)
	}

	return writeJSON(c, 200, results)
}

func (s *Server) handlePickDirectory(c fiber.Ctx) error {
	var req struct {
		Title    string `json:"title"`
		StartDir string `json:"start_dir"`
	}
	c.Bind().JSON(&req)

	if req.Title == "" {
		req.Title = "Select Directory"
	}

	dir, err := pickDirectory(req.Title, req.StartDir)
	if err != nil {
		if err.Error() == "dialog canceled" || err.Error() == "cancelled" {
			return writeJSON(c, 200, fiber.Map{"cancelled": true})
		}
		return writeError(c, 500, err.Error())
	}

	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return writeError(c, 400, "selected path is not a valid directory")
	}

	return writeJSON(c, 200, fiber.Map{"path": dir, "cancelled": false})
}
