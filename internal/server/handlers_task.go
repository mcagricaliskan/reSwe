package server

import (
	"strconv"
	"time"

	"github.com/cagri/reswe/internal/agent"
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

	go func() {
		if err := s.orchestrator.PlanChat(taskID, req.Message, cfg); err != nil {
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
		return writeJSON(c, 200, nil)
	}
	return writeJSON(c, 200, run)
}
