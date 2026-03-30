package server

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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

// --- General task chat ---

func (s *Server) handleTaskChat(c fiber.Ctx) error {
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
	message := s.injectFileRefs(taskID, req.Message)

	go func() {
		if err := s.orchestrator.Chat(taskID, message, cfg); err != nil {
			s.hub.Broadcast(models_ws_error(taskID, err.Error()))
		}
	}()
	return writeJSON(c, 202, fiber.Map{"status": "started", "phase": "chat"})
}

func (s *Server) handleListTaskMessages(c fiber.Ctx) error {
	taskID, err := strconv.ParseInt(c.Params("taskId"), 10, 64)
	if err != nil {
		return writeError(c, 400, "invalid task id")
	}
	msgs, err := s.store.ListTaskMessages(taskID)
	if err != nil {
		return writeError(c, 500, err.Error())
	}
	return writeJSON(c, 200, msgs)
}

func (s *Server) handleClearTaskMessages(c fiber.Ctx) error {
	taskID, err := strconv.ParseInt(c.Params("taskId"), 10, 64)
	if err != nil {
		return writeError(c, 400, "invalid task id")
	}
	if err := s.store.ClearTaskMessages(taskID); err != nil {
		return writeError(c, 500, err.Error())
	}
	return writeJSON(c, 200, fiber.Map{"status": "cleared"})
}

func (s *Server) handleExecute(c fiber.Ctx) error {
	taskID, cfg, err := s.parseRunConfig(c)
	if err != nil {
		return writeError(c, 400, "invalid request")
	}

	// If task has TODOs, use TODO-based execution
	todos, _ := s.store.ListPlanTodos(taskID)
	if len(todos) > 0 {
		go func() {
			if err := s.orchestrator.ExecuteTodos(taskID, cfg); err != nil {
				s.hub.Broadcast(models_ws_error(taskID, err.Error()))
			}
		}()
		return writeJSON(c, 202, fiber.Map{"status": "started", "phase": "execute-todo", "todo_count": len(todos)})
	}

	go func() {
		if err := s.orchestrator.Execute(taskID, cfg); err != nil {
			s.hub.Broadcast(models_ws_error(taskID, err.Error()))
		}
	}()
	return writeJSON(c, 202, fiber.Map{"status": "started", "phase": "execute"})
}

func (s *Server) handleListTodos(c fiber.Ctx) error {
	taskID, err := strconv.ParseInt(c.Params("taskId"), 10, 64)
	if err != nil {
		return writeError(c, 400, "invalid task id")
	}
	todos, err := s.store.ListPlanTodos(taskID)
	if err != nil {
		return writeError(c, 500, err.Error())
	}
	return writeJSON(c, 200, todos)
}

func (s *Server) handleUpdateTodo(c fiber.Ctx) error {
	todoID, err := strconv.ParseInt(c.Params("todoId"), 10, 64)
	if err != nil {
		return writeError(c, 400, "invalid todo id")
	}
	todo, err := s.store.GetPlanTodo(todoID)
	if err != nil {
		return writeError(c, 404, "todo not found")
	}
	var req struct {
		Title       *string `json:"title"`
		Description *string `json:"description"`
		Status      *string `json:"status"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return writeError(c, 400, "invalid request body")
	}
	if req.Title != nil {
		todo.Title = *req.Title
	}
	if req.Description != nil {
		todo.Description = *req.Description
	}
	if req.Status != nil {
		todo.Status = *req.Status
	}
	if err := s.store.UpdatePlanTodo(todo); err != nil {
		return writeError(c, 500, err.Error())
	}
	return writeJSON(c, 200, todo)
}

func (s *Server) handleExecuteTodos(c fiber.Ctx) error {
	taskID, cfg, err := s.parseRunConfig(c)
	if err != nil {
		return writeError(c, 400, "invalid request")
	}
	go func() {
		if err := s.orchestrator.ExecuteTodos(taskID, cfg); err != nil {
			s.hub.Broadcast(models_ws_error(taskID, err.Error()))
		}
	}()
	return writeJSON(c, 202, fiber.Map{"status": "started", "phase": "execute-todo"})
}

func (s *Server) handleGetTimeline(c fiber.Ctx) error {
	taskID, err := strconv.ParseInt(c.Params("taskId"), 10, 64)
	if err != nil {
		return writeError(c, 400, "invalid task id")
	}

	var events []models.TimelineEvent

	// Agent runs → plan/execute/chat events
	runs, _ := s.store.ListAgentRuns(taskID)
	for _, run := range runs {
		ev := models.TimelineEvent{
			ID:        fmt.Sprintf("run-%d", run.ID),
			RunID:     run.ID,
			Status:    run.Status,
			CreatedAt: run.CreatedAt,
		}

		switch run.Phase {
		case "plan":
			if run.Status == "completed" {
				// Check if this was first plan or revision
				firstPlan := true
				for _, prev := range runs {
					if prev.Phase == "plan" && prev.Status == "completed" && prev.CreatedAt.Before(run.CreatedAt) {
						firstPlan = false
						break
					}
				}
				if firstPlan {
					ev.Type = "plan_created"
					ev.Title = "Plan created"
				} else {
					ev.Type = "plan_revised"
					ev.Title = "Plan revised"
				}
				ev.Metadata = map[string]interface{}{
					"step_count": run.StepCount,
					"duration_ms": run.DurationMs,
				}
			} else if run.Status == "waiting" {
				ev.Type = "plan_waiting"
				ev.Title = "Agent waiting for input"
			} else if run.Status == "error" {
				ev.Type = "plan_error"
				ev.Title = "Planning failed"
				ev.Description = run.Error
			} else {
				continue // skip running/resumed
			}
		case "execute-todo":
			if run.Status == "completed" {
				ev.Type = "todo_executed"
				ev.Title = "Step executed"
				ev.Description = run.FinalResult
				if len(ev.Description) > 100 {
					ev.Description = ev.Description[:100] + "..."
				}
			} else if run.Status == "waiting" {
				ev.Type = "change_pending"
				ev.Title = "Change waiting for approval"
			} else if run.Status == "error" {
				ev.Type = "todo_error"
				ev.Title = "Step failed"
				ev.Description = run.Error
			} else {
				continue
			}
			ev.Metadata = map[string]interface{}{"step_count": run.StepCount, "duration_ms": run.DurationMs}
		case "execute":
			if run.Status == "completed" {
				ev.Type = "execution_completed"
				ev.Title = "Execution completed"
			} else if run.Status == "error" {
				ev.Type = "execution_error"
				ev.Title = "Execution failed"
				ev.Description = run.Error
			} else {
				continue
			}
		case "chat":
			if run.Status == "completed" {
				ev.Type = "chat"
				ev.Title = "Chat message"
				ev.Metadata = map[string]interface{}{"step_count": run.StepCount}
			} else {
				continue
			}
		default:
			continue
		}

		events = append(events, ev)
	}

	// Pending changes → accepted/rejected events
	changes, _ := s.store.ListPendingChanges(taskID)
	for _, ch := range changes {
		if ch.Status == "accepted" {
			events = append(events, models.TimelineEvent{
				ID:        "change-" + ch.ID,
				Type:      "change_accepted",
				Title:     "Change accepted",
				ChangeID:  ch.ID,
				CreatedAt: ch.CreatedAt,
				Metadata:  map[string]interface{}{"file": ch.RelPath, "tool": ch.Tool},
			})
		} else if ch.Status == "rejected" {
			events = append(events, models.TimelineEvent{
				ID:          "change-" + ch.ID,
				Type:        "change_rejected",
				Title:       "Change rejected",
				Description: ch.RejectReason,
				ChangeID:    ch.ID,
				CreatedAt:   ch.CreatedAt,
				Metadata:    map[string]interface{}{"file": ch.RelPath, "tool": ch.Tool},
			})
		}
	}

	// Sort by time descending (most recent first)
	sort.Slice(events, func(i, j int) bool {
		return events[i].CreatedAt.After(events[j].CreatedAt)
	})

	return writeJSON(c, 200, events)
}

func (s *Server) handleAcceptChange(c fiber.Ctx) error {
	changeID := c.Params("changeId")

	// Get the change to find the task
	change, err := s.store.GetPendingChange(changeID)
	if err != nil {
		return writeError(c, 404, "change not found")
	}

	cfg := agent.RunConfig{Provider: "ollama", Model: "qwen3.5:27b"}

	go func() {
		if err := s.orchestrator.AcceptChange(change.TaskID, changeID, cfg); err != nil {
			s.hub.Broadcast(models_ws_error(change.TaskID, err.Error()))
		}
	}()

	return writeJSON(c, 202, fiber.Map{"status": "accepted"})
}

func (s *Server) handleRejectChange(c fiber.Ctx) error {
	changeID := c.Params("changeId")

	change, err := s.store.GetPendingChange(changeID)
	if err != nil {
		return writeError(c, 404, "change not found")
	}

	var req struct {
		Reason string `json:"reason"`
	}
	c.Bind().JSON(&req)

	cfg := agent.RunConfig{Provider: "ollama", Model: "qwen3.5:27b"}

	go func() {
		if err := s.orchestrator.RejectChange(change.TaskID, changeID, req.Reason, cfg); err != nil {
			s.hub.Broadcast(models_ws_error(change.TaskID, err.Error()))
		}
	}()

	return writeJSON(c, 202, fiber.Map{"status": "rejected"})
}

func (s *Server) handleListPendingChanges(c fiber.Ctx) error {
	taskID, err := strconv.ParseInt(c.Params("taskId"), 10, 64)
	if err != nil {
		return writeError(c, 400, "invalid task id")
	}
	changes, err := s.store.ListPendingChanges(taskID)
	if err != nil {
		return writeError(c, 500, err.Error())
	}
	return writeJSON(c, 200, changes)
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
		// Paths are stored as "repoName/sub/path" — strip the repo name prefix
		inner := relPath
		if strings.HasPrefix(relPath, repo.Name+"/") {
			inner = relPath[len(repo.Name)+1:]
		} else if relPath == repo.Name {
			inner = "."
		}
		fullPath := filepath.Join(repo.Path, inner)
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
