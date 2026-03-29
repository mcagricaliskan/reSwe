package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cagri/reswe/internal/models"
	"github.com/cagri/reswe/internal/provider"
	"github.com/cagri/reswe/internal/scanner"
	"github.com/cagri/reswe/internal/store"
)

// EventCallback sends real-time events to the WebSocket hub
type EventCallback func(msg models.WSMessage)

// RunConfig contains all the options for running an agent phase
type RunConfig struct {
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	SystemPrompt string `json:"system_prompt"`
	UserPrompt   string `json:"user_prompt"` // unused now — agent builds its own context
}

type Orchestrator struct {
	store    store.Store
	registry *provider.Registry
	scanner  *scanner.Scanner
	Tracker  *Tracker
	onEvent  EventCallback
}

func NewOrchestrator(s store.Store, r *provider.Registry, sc *scanner.Scanner, onEvent EventCallback) *Orchestrator {
	return &Orchestrator{
		store:    s,
		registry: r,
		scanner:  sc,
		Tracker:  NewTracker(),
		onEvent:  onEvent,
	}
}

func (o *Orchestrator) emit(taskID int64, msgType models.WSMessageType, payload interface{}) {
	if o.onEvent != nil {
		o.onEvent(models.WSMessage{
			Type:    msgType,
			TaskID:  taskID,
			Payload: payload,
		})
	}
}

func (o *Orchestrator) getProvider(providerName string) (provider.Provider, error) {
	p, ok := o.registry.Get(providerName)
	if !ok {
		return nil, fmt.Errorf("provider %q not found", providerName)
	}
	return p, nil
}

func (o *Orchestrator) getTaskWithRepos(taskID int64) (*models.Task, []models.Repo, []string, error) {
	task, err := o.store.GetTask(taskID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("get task: %w", err)
	}

	repos, err := o.store.ListRepos(task.ProjectID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("list repos: %w", err)
	}

	// Load project-specific exclude patterns (presets + custom)
	excludePatterns, err := o.store.GetEffectiveExcludePatterns(task.ProjectID)
	if err != nil {
		// Non-fatal: fall back to defaults
		excludePatterns = nil
	}

	return task, repos, excludePatterns, nil
}

// makeStepCallback creates callbacks that emit structured steps + raw stream to WebSocket AND persist to DB
func (o *Orchestrator) makeStepCallback(taskID int64, phase string, run *AgentRun, dbRun *models.AgentRun) (StepCallback, provider.StreamCallback) {
	onStep := func(step Step) {
		// Persist step to DB
		dbStep := &models.AgentStep{
			StepNumber:  step.Number,
			Think:       step.Think,
			Action:      step.Action,
			ActionArg:   step.ActionArg,
			Observation: step.Observation,
			IsFinal:     step.IsFinal,
		}
		o.store.CreateAgentStep(dbRun.ID, dbStep)

		// Update run step count
		dbRun.StepCount = step.Number
		o.store.UpdateAgentRun(dbRun)

		// Emit to WebSocket
		o.emit(taskID, models.WSTypeAgentStep, map[string]interface{}{
			"phase":       phase,
			"run_id":      dbRun.ID,
			"step":        step.Number,
			"think":       step.Think,
			"action":      step.Action,
			"action_arg":  step.ActionArg,
			"observation": step.Observation,
			"is_final":    step.IsFinal,
		})
	}

	onStream := func(chunk string) {
		run.AppendOutput(chunk)
		o.emit(taskID, models.WSTypeAgentOutput, map[string]interface{}{
			"phase": phase,
			"chunk": chunk,
		})
	}

	return onStep, onStream
}

// completeRun finalizes a DB run record
func (o *Orchestrator) completeRun(dbRun *models.AgentRun, result string, err error) {
	if err != nil {
		dbRun.Status = "error"
		dbRun.Error = err.Error()
	} else {
		dbRun.Status = "completed"
		dbRun.FinalResult = result
	}
	o.store.UpdateAgentRun(dbRun)
}

// PreviewPrompt returns the prompts that would be sent for a given phase
func (o *Orchestrator) PreviewPrompt(taskID int64, phase string) (*PromptPreview, error) {
	task, repos, excludePatterns, err := o.getTaskWithRepos(taskID)
	if err != nil {
		return nil, err
	}

	tools := NewToolSet(o.scanner, repos, excludePatterns)
	systemPrompt := DefaultSystemPrompts[phase] + "\n\n" + tools.ToolDescriptionBlock()
	userPrompt := BuildUserPrompt(phase, task, "(codebase will be explored via tools)")

	return &PromptPreview{
		Phase:        phase,
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		ContextSize:  len(systemPrompt) + len(userPrompt),
	}, nil
}

// initRun is shared setup for all agent phases
func (o *Orchestrator) initRun(taskID int64, phase string, cfg RunConfig) (*models.Task, []models.Repo, provider.Provider, *ToolSet, string, error) {
	task, repos, excludePatterns, err := o.getTaskWithRepos(taskID)
	if err != nil {
		return nil, nil, nil, nil, "", err
	}

	p, err := o.getProvider(cfg.Provider)
	if err != nil {
		return nil, nil, nil, nil, "", err
	}

	tools := NewToolSet(o.scanner, repos, excludePatterns)

	sysPrompt := cfg.SystemPrompt
	if sysPrompt == "" {
		sysPrompt = DefaultSystemPrompts[phase]
	}

	return task, repos, p, tools, sysPrompt, nil
}

// loadHistory loads the persisted plan conversation as ChatMessages
func (o *Orchestrator) loadHistory(taskID int64) []models.ChatMessage {
	msgs, err := o.store.ListPlanMessages(taskID)
	if err != nil {
		return nil
	}
	var history []models.ChatMessage
	for _, m := range msgs {
		history = append(history, models.ChatMessage{Role: m.Role, Content: m.Content})
	}
	return history
}

// saveTurn saves both the user message and agent response to conversation history
func (o *Orchestrator) saveTurn(taskID int64, userMsg, agentMsg string) {
	if userMsg != "" {
		o.store.AddPlanMessage(taskID, "user", userMsg)
	}
	if agentMsg != "" {
		o.store.AddPlanMessage(taskID, "assistant", agentMsg)
	}
}

// handleLoopResult processes a LoopResult — handles done, waiting, and error states
func (o *Orchestrator) handleLoopResult(taskID int64, result LoopResult, dbRun *models.AgentRun, task *models.Task, phase string) error {
	switch result.Status {
	case "done":
		o.Tracker.Complete(taskID, nil)
		o.completeRun(dbRun, result.FinalResult, nil)

		if phase == "plan" {
			task.ImplementationPlan = result.FinalResult
			task.Status = models.TaskStatusOpen
		} else if phase == "execute" {
			task.Status = models.TaskStatusReview
		}
		o.store.UpdateTask(task)
		o.emit(taskID, models.WSTypeAgentDone, map[string]interface{}{"phase": phase, "run_id": dbRun.ID})
		return nil

	case "waiting_for_user":
		o.Tracker.Complete(taskID, nil)

		// Save conversation history for resume
		msgJSON, _ := json.Marshal(result.Messages)
		dbRun.Status = "waiting"
		dbRun.PausedMessages = string(msgJSON)
		dbRun.StepCount = result.StepCount
		o.store.UpdateAgentRun(dbRun)

		// Save questions to DB
		var qIDs []int64
		for _, q := range result.Questions {
			aq, err := o.store.CreateAgentQuestion(dbRun.ID, taskID, q, nil)
			if err != nil {
				continue
			}
			qIDs = append(qIDs, aq.ID)
		}

		task.Status = models.TaskStatusOpen
		o.store.UpdateTask(task)

		// Emit to frontend
		o.emit(taskID, models.WSTypeAgentWaiting, map[string]interface{}{
			"run_id":    dbRun.ID,
			"questions": result.Questions,
		})
		return nil

	case "error", "max_steps":
		o.Tracker.Complete(taskID, result.Error)
		o.completeRun(dbRun, "", result.Error)
		errMsg := "unknown error"
		if result.Error != nil {
			errMsg = result.Error.Error()
		}
		o.emit(taskID, models.WSTypeAgentError, map[string]interface{}{"error": errMsg})
		return result.Error

	default:
		return fmt.Errorf("unexpected loop status: %s", result.Status)
	}
}

// Plan starts a new planning conversation
func (o *Orchestrator) Plan(taskID int64, cfg RunConfig) error {
	task, _, p, tools, sysPrompt, err := o.initRun(taskID, "plan", cfg)
	if err != nil {
		return err
	}

	o.store.ClearPlanMessages(taskID)

	ctx, memRun := o.Tracker.Start(taskID, "plan", cfg.Provider, cfg.Model)
	dbRun, err := o.store.CreateAgentRun(taskID, "plan", cfg.Provider, cfg.Model, sysPrompt)
	if err != nil {
		return err
	}

	task.Status = models.TaskStatusPlanning
	o.store.UpdateTask(task)
	o.emit(taskID, models.WSTypeTaskUpdate, map[string]interface{}{"status": task.Status, "run_id": dbRun.ID})

	onStep, onStream := o.makeStepCallback(taskID, "plan", memRun, dbRun)

	userMsg := BuildUserPrompt("plan", task, "")
	result := runPlan(ctx, p, cfg.Model, task, tools, sysPrompt, nil, onStep, onStream)

	// Save conversation turn if agent produced output
	if result.FinalResult != "" {
		o.saveTurn(taskID, userMsg, result.FinalResult)
	}

	return o.handleLoopResult(taskID, result, dbRun, task, "plan")
}

// PlanChat continues the planning conversation with a user message
func (o *Orchestrator) PlanChat(taskID int64, userMessage string, cfg RunConfig) error {
	task, _, p, tools, sysPrompt, err := o.initRun(taskID, "plan", cfg)
	if err != nil {
		return err
	}

	history := o.loadHistory(taskID)

	ctx, memRun := o.Tracker.Start(taskID, "plan", cfg.Provider, cfg.Model)
	dbRun, err := o.store.CreateAgentRun(taskID, "plan", cfg.Provider, cfg.Model, sysPrompt)
	if err != nil {
		return err
	}

	task.Status = models.TaskStatusPlanning
	o.store.UpdateTask(task)
	o.emit(taskID, models.WSTypeTaskUpdate, map[string]interface{}{"status": task.Status, "run_id": dbRun.ID})

	onStep, onStream := o.makeStepCallback(taskID, "plan", memRun, dbRun)

	result := runPlan(ctx, p, cfg.Model, task, tools, sysPrompt, history, onStep, onStream)

	if result.FinalResult != "" {
		o.saveTurn(taskID, userMessage, result.FinalResult)
		task.ImplementationPlan = result.FinalResult
	}

	return o.handleLoopResult(taskID, result, dbRun, task, "plan")
}

// ResumePlan resumes a paused planning run after user answers questions
func (o *Orchestrator) ResumePlan(taskID int64, answers map[int64]string, cfg RunConfig) error {
	task, _, p, tools, sysPrompt, err := o.initRun(taskID, "plan", cfg)
	if err != nil {
		return err
	}

	// Find the paused run
	dbRun, err := o.store.GetLatestAgentRun(taskID)
	if err != nil || dbRun.Status != "waiting" {
		return fmt.Errorf("no paused run found for task %d", taskID)
	}

	// Save answers to DB
	for qID, answer := range answers {
		o.store.AnswerAgentQuestion(qID, answer)
	}

	// Load paused conversation history
	var pausedMessages []models.ChatMessage
	if err := json.Unmarshal([]byte(dbRun.PausedMessages), &pausedMessages); err != nil {
		return fmt.Errorf("failed to load paused state: %w", err)
	}

	// Build answer observation: inject user's answers into the conversation
	var answerText strings.Builder
	answerText.WriteString("OBSERVATION: User answered your questions:\n")
	questions, _ := o.store.ListAgentQuestions(dbRun.ID)
	for _, q := range questions {
		if q.Answered {
			answerText.WriteString(fmt.Sprintf("Q: %s\nA: %s\n\n", q.Question, q.Answer))
		}
	}
	answerText.WriteString("Continue with your next THINK and ACTION based on these answers.")

	// Append the answer observation to the paused conversation
	pausedMessages = append(pausedMessages, models.ChatMessage{
		Role:    "user",
		Content: answerText.String(),
	})

	// Create a new run for the resumed session
	ctx, memRun := o.Tracker.Start(taskID, "plan", cfg.Provider, cfg.Model)
	newDbRun, err := o.store.CreateAgentRun(taskID, "plan", cfg.Provider, cfg.Model, sysPrompt)
	if err != nil {
		return err
	}

	// Mark old run as resumed
	dbRun.Status = "resumed"
	o.store.UpdateAgentRun(dbRun)

	task.Status = models.TaskStatusPlanning
	o.store.UpdateTask(task)
	o.emit(taskID, models.WSTypeTaskUpdate, map[string]interface{}{"status": task.Status, "run_id": newDbRun.ID})

	onStep, onStream := o.makeStepCallback(taskID, "plan", memRun, newDbRun)

	// Resume the loop from where it paused (with history + answers)
	result := RunLoop(ctx, LoopConfig{
		Provider:     p,
		Model:        cfg.Model,
		SystemPrompt: sysPrompt,
		History:      pausedMessages,
		Tools:        tools,
		OnStep:       onStep,
		OnStream:     onStream,
	})

	if result.FinalResult != "" {
		o.saveTurn(taskID, "(answered questions)", result.FinalResult)
		task.ImplementationPlan = result.FinalResult
	}

	return o.handleLoopResult(taskID, result, newDbRun, task, "plan")
}

// Execute runs the execution agent
func (o *Orchestrator) Execute(taskID int64, cfg RunConfig) error {
	task, _, p, tools, sysPrompt, err := o.initRun(taskID, "execute", cfg)
	if err != nil {
		return err
	}

	ctx, memRun := o.Tracker.Start(taskID, "execute", cfg.Provider, cfg.Model)
	dbRun, err := o.store.CreateAgentRun(taskID, "execute", cfg.Provider, cfg.Model, sysPrompt)
	if err != nil {
		return err
	}

	task.Status = models.TaskStatusExecuting
	o.store.UpdateTask(task)
	o.emit(taskID, models.WSTypeTaskUpdate, map[string]interface{}{"status": task.Status, "run_id": dbRun.ID})

	exec, err := o.store.CreateExecution(taskID, cfg.Provider, cfg.Model)
	if err != nil {
		o.Tracker.Complete(taskID, err)
		o.completeRun(dbRun, "", err)
		return err
	}

	onStep, onStream := o.makeStepCallback(taskID, "execute", memRun, dbRun)

	result := runExecute(ctx, p, cfg.Model, task, tools, sysPrompt, onStep, onStream)

	if result.Status == "done" {
		exec.Status = "completed"
		exec.Log = result.FinalResult
	} else {
		exec.Status = "failed"
		if result.Error != nil {
			exec.Log = result.Error.Error()
		}
	}
	o.store.UpdateExecution(exec)

	return o.handleLoopResult(taskID, result, dbRun, task, "execute")
}
