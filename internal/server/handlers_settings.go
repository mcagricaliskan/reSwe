package server

import (
	"strconv"

	"github.com/cagri/reswe/internal/models"
	"github.com/gofiber/fiber/v3"
)

// --- Global Exclude Rules ---

func (s *Server) handleListExcludeRules(c fiber.Ctx) error {
	rules, err := s.store.ListExcludeRules()
	if err != nil {
		return writeError(c, 500, err.Error())
	}
	if rules == nil {
		rules = []models.ExcludeRule{}
	}
	return writeJSON(c, 200, rules)
}

func (s *Server) handleCreateExcludeRule(c fiber.Ctx) error {
	var req struct {
		Pattern          string `json:"pattern"`
		EnabledByDefault bool   `json:"enabled_by_default"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return writeError(c, 400, "invalid request body")
	}
	if req.Pattern == "" {
		return writeError(c, 400, "pattern is required")
	}
	rule, err := s.store.CreateExcludeRule(req.Pattern, req.EnabledByDefault)
	if err != nil {
		return writeError(c, 500, err.Error())
	}
	return writeJSON(c, 201, rule)
}

func (s *Server) handleUpdateExcludeRule(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return writeError(c, 400, "invalid rule id")
	}
	var req struct {
		Pattern          string `json:"pattern"`
		EnabledByDefault bool   `json:"enabled_by_default"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return writeError(c, 400, "invalid request body")
	}
	if err := s.store.UpdateExcludeRule(id, req.Pattern, req.EnabledByDefault); err != nil {
		return writeError(c, 500, err.Error())
	}
	return writeJSON(c, 200, fiber.Map{"status": "ok"})
}

func (s *Server) handleDeleteExcludeRule(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return writeError(c, 400, "invalid rule id")
	}
	if err := s.store.DeleteExcludeRule(id); err != nil {
		return writeError(c, 500, err.Error())
	}
	return writeJSON(c, 200, fiber.Map{"status": "deleted"})
}

// --- Project Exclude Config ---

func (s *Server) handleGetProjectExcludeConfig(c fiber.Ctx) error {
	projectID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return writeError(c, 400, "invalid project id")
	}

	rules, err := s.store.ListExcludeRules()
	if err != nil {
		return writeError(c, 500, err.Error())
	}
	overrides, err := s.store.ListProjectExcludeOverrides(projectID)
	if err != nil {
		return writeError(c, 500, err.Error())
	}
	custom, err := s.store.ListProjectCustomPatterns(projectID)
	if err != nil {
		return writeError(c, 500, err.Error())
	}
	effective, err := s.store.GetEffectiveExcludePatterns(projectID)
	if err != nil {
		return writeError(c, 500, err.Error())
	}

	// Build resolved rules
	overrideMap := make(map[int64]bool)
	overriddenSet := make(map[int64]bool)
	for _, o := range overrides {
		overrideMap[o.RuleID] = o.Enabled
		overriddenSet[o.RuleID] = true
	}

	resolved := make([]fiber.Map, 0)
	for _, r := range rules {
		enabled := r.EnabledByDefault
		overridden := false
		if ov, ok := overrideMap[r.ID]; ok {
			enabled = ov
			overridden = true
		}
		resolved = append(resolved, fiber.Map{
			"id":                 r.ID,
			"pattern":            r.Pattern,
			"enabled_by_default": r.EnabledByDefault,
			"enabled":            enabled,
			"overridden":         overridden,
		})
	}

	if custom == nil {
		custom = []models.ProjectCustomPattern{}
	}
	if effective == nil {
		effective = []string{}
	}

	return writeJSON(c, 200, fiber.Map{
		"rules":           resolved,
		"custom_patterns": custom,
		"effective":       effective,
	})
}

func (s *Server) handleSetProjectExcludeOverride(c fiber.Ctx) error {
	projectID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return writeError(c, 400, "invalid project id")
	}
	var req struct {
		RuleID  int64 `json:"rule_id"`
		Enabled bool  `json:"enabled"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return writeError(c, 400, "invalid request body")
	}
	if err := s.store.SetProjectExcludeOverride(projectID, req.RuleID, req.Enabled); err != nil {
		return writeError(c, 500, err.Error())
	}
	return writeJSON(c, 200, fiber.Map{"status": "ok"})
}

func (s *Server) handleDeleteProjectExcludeOverride(c fiber.Ctx) error {
	projectID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return writeError(c, 400, "invalid project id")
	}
	var req struct {
		RuleID int64 `json:"rule_id"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return writeError(c, 400, "invalid request body")
	}
	if err := s.store.DeleteProjectExcludeOverride(projectID, req.RuleID); err != nil {
		return writeError(c, 500, err.Error())
	}
	return writeJSON(c, 200, fiber.Map{"status": "ok"})
}

func (s *Server) handleAddProjectCustomPattern(c fiber.Ctx) error {
	projectID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return writeError(c, 400, "invalid project id")
	}
	var req struct {
		Pattern string `json:"pattern"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return writeError(c, 400, "invalid request body")
	}
	if req.Pattern == "" {
		return writeError(c, 400, "pattern is required")
	}
	pattern, err := s.store.AddProjectCustomPattern(projectID, req.Pattern)
	if err != nil {
		return writeError(c, 500, err.Error())
	}
	return writeJSON(c, 201, pattern)
}

func (s *Server) handleDeleteProjectCustomPattern(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("patternId"), 10, 64)
	if err != nil {
		return writeError(c, 400, "invalid pattern id")
	}
	if err := s.store.DeleteProjectCustomPattern(id); err != nil {
		return writeError(c, 500, err.Error())
	}
	return writeJSON(c, 200, fiber.Map{"status": "deleted"})
}
