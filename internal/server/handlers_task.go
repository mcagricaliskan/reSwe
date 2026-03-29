package server

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cagri/reswe/internal/agent"
	"github.com/cagri/reswe/internal/models"
	"github.com/gofiber/fiber/v3"
)

func (s *Server) handleListTasks(c fiber.Ctx) error {
	projectID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return writeError(c, 400, "invalid project id")
	}
	tasks, err := s.store.ListTasks(projectID)
	if err != nil {
		return writeError(c, 500, err.Error())
	}
	return writeJSON(c, 200, tasks)
}

func (s *Server) handleCreateTask(c fiber.Ctx) error {
	projectID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return writeError(c, 400, "invalid project id")
	}
	var req struct {
		Title       string `json:"title"`
		Description string `json:"description"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return writeError(c, 400, "invalid request body")
	}
	if req.Title == "" {
		return writeError(c, 400, "title is required")
	}
	task, err := s.store.CreateTask(projectID, req.Title, req.Description)
	if err != nil {
		return writeError(c, 500, err.Error())
	}
	return writeJSON(c, 201, task)
}

func (s *Server) handleGetTask(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("taskId"), 10, 64)
	if err != nil {
		return writeError(c, 400, "invalid task id")
	}
	task, err := s.store.GetTask(id)
	if err != nil {
		return writeError(c, 404, "task not found")
	}
	return writeJSON(c, 200, task)
}

func (s *Server) handleUpdateTask(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("taskId"), 10, 64)
	if err != nil {
		return writeError(c, 400, "invalid task id")
	}
	task, err := s.store.GetTask(id)
	if err != nil {
		return writeError(c, 404, "task not found")
	}
	var req struct {
		Title       *string `json:"title"`
		Description *string `json:"description"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return writeError(c, 400, "invalid request body")
	}
	if req.Title != nil {
		task.Title = *req.Title
	}
	if req.Description != nil {
		task.Description = *req.Description
	}
	if err := s.store.UpdateTask(task); err != nil {
		return writeError(c, 500, err.Error())
	}
	return writeJSON(c, 200, task)
}

func (s *Server) handleDeleteTask(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("taskId"), 10, 64)
	if err != nil {
		return writeError(c, 400, "invalid task id")
	}
	if err := s.store.DeleteTask(id); err != nil {
		return writeError(c, 500, err.Error())
	}
	return writeJSON(c, 200, fiber.Map{"status": "deleted"})
}

// --- Agent actions ---

func (s *Server) parseRunConfig(c fiber.Ctx) (int64, agent.RunConfig, error) {
	taskID, err := strconv.ParseInt(c.Params("taskId"), 10, 64)
	if err != nil {
		return 0, agent.RunConfig{}, err
	}
	var cfg agent.RunConfig
	if err := c.Bind().JSON(&cfg); err != nil {
		return 0, agent.RunConfig{}, err
	}
	if cfg.Provider == "" {
		cfg.Provider = "ollama"
	}
	if cfg.Model == "" {
		cfg.Model = "qwen3.5:27b"
	}
	return taskID, cfg, nil
}

func (s *Server) handleGetPhases(c fiber.Ctx) error {
	return writeJSON(c, 200, agent.Phases)
}

func (s *Server) handlePreviewPrompt(c fiber.Ctx) error {
	taskID, err := strconv.ParseInt(c.Params("taskId"), 10, 64)
	if err != nil {
		return writeError(c, 400, "invalid task id")
	}
	phase := c.Params("phase")
	if _, ok := agent.DefaultSystemPrompts[phase]; !ok {
		return writeError(c, 400, "invalid phase: "+phase)
	}
	preview, err := s.orchestrator.PreviewPrompt(taskID, phase)
	if err != nil {
		return writeError(c, 500, err.Error())
	}
	return writeJSON(c, 200, preview)
}

func (s *Server) handlePlan(c fiber.Ctx) error {
	taskID, cfg, err := s.parseRunConfig(c)
	if err != nil {
		return writeError(c, 400, "invalid request")
	}
	go func() {
		if err := s.orchestrator.Plan(taskID, cfg); err != nil {
			s.hub.Broadcast(models_ws_error(taskID, err.Error()))
		}
	}()
	return writeJSON(c, 202, fiber.Map{"status": "started", "phase": "plan"})
}

func (s *Server) handlePlanChat(c fiber.Ctx) error {
	taskID, err := strconv.ParseInt(c.Params("taskId"), 10, 64)
	if err != nil {
		return writeError(c, 400, "invalid task id")
	}
	var req struct {
		Message  string `json:"message"`
		Provider string `json:"provider"`
		Model    string `json:"model"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return writeError(c, 400, "invalid request body")
	}
	if req.Message == "" {
		return writeError(c, 400, "message is required")
	}
	if req.Provider == "" {
		req.Provider = "ollama"
	}
	if req.Model == "" {
		req.Model = "qwen3.5:27b"
	}

	cfg := agent.RunConfig{Provider: req.Provider, Model: req.Model}

	// Inject @file reference contents into the message
	message := s.injectFileRefs(taskID, req.Message)

	go func() {
		if err := s.orchestrator.PlanChat(taskID, message, cfg); err != nil {
			s.hub.Broadcast(models_ws_error(taskID, err.Error()))
		}
	}()
	return writeJSON(c, 202, fiber.Map{"status": "started", "phase": "plan"})
}

func (s *Server) handleListPlanMessages(c fiber.Ctx) error {
	taskID, err := strconv.ParseInt(c.Params("taskId"), 10, 64)
	if err != nil {
		return writeError(c, 400, "invalid task id")
	}
	msgs, err := s.store.ListPlanMessages(taskID)
	if err != nil {
		return writeError(c, 500, err.Error())
	}
	return writeJSON(c, 200, msgs)
}

func (s *Server) handlePlanAnswer(c fiber.Ctx) error {
	taskID, err := strconv.ParseInt(c.Params("taskId"), 10, 64)
	if err != nil {
		return writeError(c, 400, "invalid task id")
	}

	var req struct {
		Answers []struct {
			QuestionID int64  `json:"question_id"`
			Answer     string `json:"answer"`
		} `json:"answers"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return writeError(c, 400, "invalid request body")
	}

	answerMap := make(map[int64]string)
	for _, a := range req.Answers {
		answerMap[a.QuestionID] = a.Answer
	}

	cfg := agent.RunConfig{Provider: "ollama", Model: "qwen3.5:27b"}

	go func() {
		if err := s.orchestrator.ResumePlan(taskID, answerMap, cfg); err != nil {
			s.hub.Broadcast(models_ws_error(taskID, err.Error()))
		}
	}()

	return writeJSON(c, 202, fiber.Map{"status": "resuming"})
}

func (s *Server) handleGetPendingQuestions(c fiber.Ctx) error {
	taskID, err := strconv.ParseInt(c.Params("taskId"), 10, 64)
	if err != nil {
		return writeError(c, 400, "invalid task id")
	}

	questions, err := s.store.ListPendingQuestions(taskID)
	if err != nil {
		return writeError(c, 500, err.Error())
	}
	return writeJSON(c, 200, questions)
}

func (s *Server) handleListSessions(c fiber.Ctx) error {
	taskID, err := strconv.ParseInt(c.Params("taskId"), 10, 64)
	if err != nil {
		return writeError(c, 400, "invalid task id")
	}
	sessions, err := s.store.ListSessions(taskID)
	if err != nil {
		return writeError(c, 500, err.Error())
	}
	return writeJSON(c, 200, sessions)
}

func (s *Server) handleGetSession(c fiber.Ctx) error {
	sessionID, err := strconv.ParseInt(c.Params("sessionId"), 10, 64)
	if err != nil {
		return writeError(c, 400, "invalid session id")
	}
	sess, err := s.store.GetSession(sessionID)
	if err != nil {
		return writeError(c, 404, "session not found")
	}
	return writeJSON(c, 200, sess)
}

// handleRestoreSession archives the current active session and makes the selected one active again
func (s *Server) handleRestoreSession(c fiber.Ctx) error {
	sessionID, err := strconv.ParseInt(c.Params("sessionId"), 10, 64)
	if err != nil {
		return writeError(c, 400, "invalid session id")
	}

	sess, err := s.store.GetSession(sessionID)
	if err != nil {
		return writeError(c, 404, "session not found")
	}

	// Archive current active session for this task
	activeSess, err := s.store.GetActiveSession(sess.TaskID)
	if err == nil && activeSess.ID != sessionID {
		s.store.ArchiveSession(activeSess.ID)
	}

	// Reactivate the selected session
	s.store.ReactivateSession(sessionID)

	return writeJSON(c, 200, sess)
}

func (s *Server) handleExecute(c fiber.Ctx) error {
	taskID, cfg, err := s.parseRunConfig(c)
	if err != nil {
		return writeError(c, 400, "invalid request")
	}
	go func() {
		if err := s.orchestrator.Execute(taskID, cfg); err != nil {
			s.hub.Broadcast(models_ws_error(taskID, err.Error()))
		}
	}()
	return writeJSON(c, 202, fiber.Map{"status": "started", "phase": "execute"})
}

func (s *Server) handleAnswer(c fiber.Ctx) error {
	clarificationID, err := strconv.ParseInt(c.Params("clarificationId"), 10, 64)
	if err != nil {
		return writeError(c, 400, "invalid clarification id")
	}
	var req struct {
		Answer string `json:"answer"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return writeError(c, 400, "invalid request body")
	}
	if err := s.store.AnswerClarification(clarificationID, req.Answer); err != nil {
		return writeError(c, 500, err.Error())
	}
	return writeJSON(c, 200, fiber.Map{"status": "answered"})
}

func (s *Server) handleListProviders(c fiber.Ctx) error {
	return writeJSON(c, 200, s.orchestrator_registry_list())
}

// --- Agent status ---

func (s *Server) handleGetAgentStatus(c fiber.Ctx) error {
	taskID, err := strconv.ParseInt(c.Params("taskId"), 10, 64)
	if err != nil {
		return writeError(c, 400, "invalid task id")
	}
	run := s.orchestrator.Tracker.Get(taskID)
	if run == nil {
		return writeJSON(c, 200, fiber.Map{"active": false})
	}
	return writeJSON(c, 200, fiber.Map{
		"active":     run.Status == "running",
		"phase":      run.Phase,
		"status":     run.Status,
		"output":     run.GetOutput(),
		"provider":   run.Provider,
		"model":      run.Model,
		"started_at": run.StartedAt,
		"ended_at":   run.EndedAt,
		"error":      run.Error,
	})
}

func (s *Server) handleGetActiveAgents(c fiber.Ctx) error {
	active := s.orchestrator.Tracker.GetAllActive()
	recent := s.orchestrator.Tracker.GetAllRecent(30 * time.Minute)
	return writeJSON(c, 200, fiber.Map{"active": active, "recent": recent})
}

func (s *Server) handleCancelAgent(c fiber.Ctx) error {
	taskID, err := strconv.ParseInt(c.Params("taskId"), 10, 64)
	if err != nil {
		return writeError(c, 400, "invalid task id")
	}
	s.orchestrator.Tracker.Cancel(taskID)
	return writeJSON(c, 200, fiber.Map{"status": "cancelled"})
}

// --- Persisted runs ---

func (s *Server) handleListAgentRuns(c fiber.Ctx) error {
	taskID, err := strconv.ParseInt(c.Params("taskId"), 10, 64)
	if err != nil {
		return writeError(c, 400, "invalid task id")
	}
	runs, err := s.store.ListAgentRuns(taskID)
	if err != nil {
		return writeError(c, 500, err.Error())
	}
	return writeJSON(c, 200, runs)
}

func (s *Server) handleGetAgentRun(c fiber.Ctx) error {
	runID, err := strconv.ParseInt(c.Params("runId"), 10, 64)
	if err != nil {
		return writeError(c, 400, "invalid run id")
	}
	run, err := s.store.GetAgentRun(runID)
	if err != nil {
		return writeError(c, 404, "run not found")
	}
	return writeJSON(c, 200, run)
}

func (s *Server) handleGetLatestRun(c fiber.Ctx) error {
	taskID, err := strconv.ParseInt(c.Params("taskId"), 10, 64)
	if err != nil {
		return writeError(c, 400, "invalid task id")
	}
	run, err := s.store.GetLatestAgentRun(taskID)
	if err != nil {
		return writeJSON(c, 200, []interface{}{})
	}
	return writeJSON(c, 200, run)
}

// --- File reference helpers ---

var fileRefRe = regexp.MustCompile(`@([\w./\-]+[\w\-])`)

func extractFileRefs(msg string) []string {
	matches := fileRefRe.FindAllStringSubmatch(msg, -1)
	seen := make(map[string]bool)
	var refs []string
	for _, m := range matches {
		path := m[1]
		if !seen[path] {
			seen[path] = true
			refs = append(refs, path)
		}
	}
	return refs
}

func (s *Server) readFileFromRepos(repos []models.Repo, relPath string) string {
	for _, repo := range repos {
		fullPath := filepath.Join(repo.Path, relPath)
		info, err := os.Stat(fullPath)
		if err != nil {
			continue
		}
		if info.IsDir() {
			// List directory contents for folder references
			entries, err := os.ReadDir(fullPath)
			if err != nil {
				continue
			}
			var lines []string
			for _, e := range entries {
				name := e.Name()
				if e.IsDir() {
					name += "/"
				}
				lines = append(lines, name)
			}
			return strings.Join(lines, "\n")
		}
		content, err := s.scanner.ReadFile(fullPath, 100*1024)
		if err == nil {
			return content
		}
	}
	return ""
}

func (s *Server) injectFileRefs(taskID int64, message string) string {
	refs := extractFileRefs(message)
	if len(refs) == 0 {
		return message
	}

	task, err := s.store.GetTask(taskID)
	if err != nil {
		return message
	}
	repos, err := s.store.ListRepos(task.ProjectID)
	if err != nil || len(repos) == 0 {
		return message
	}

	var b strings.Builder
	b.WriteString(message)
	b.WriteString("\n\n--- Referenced Files ---\n")
	for _, ref := range refs {
		content := s.readFileFromRepos(repos, ref)
		if content != "" {
			b.WriteString(fmt.Sprintf("\n### %s\n```\n%s\n```\n", ref, content))
		}
	}
	return b.String()
}
