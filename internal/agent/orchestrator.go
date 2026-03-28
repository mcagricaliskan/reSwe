package agent

import (
	"fmt"

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

func (o *Orchestrator) getTaskWithRepos(taskID int64) (*models.Task, []models.Repo, error) {
	task, err := o.store.GetTask(taskID)
	if err != nil {
		return nil, nil, fmt.Errorf("get task: %w", err)
	}

	repos, err := o.store.ListRepos(task.ProjectID)
	if err != nil {
		return nil, nil, fmt.Errorf("list repos: %w", err)
	}

	return task, repos, nil
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
	task, repos, err := o.getTaskWithRepos(taskID)
	if err != nil {
		return nil, err
	}

	tools := NewToolSet(o.scanner, repos)
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
	task, repos, err := o.getTaskWithRepos(taskID)
	if err != nil {
		return nil, nil, nil, nil, "", err
	}

	p, err := o.getProvider(cfg.Provider)
	if err != nil {
		return nil, nil, nil, nil, "", err
	}

	tools := NewToolSet(o.scanner, repos)

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

// Plan starts a new planning conversation — reads codebase, proposes a plan
func (o *Orchestrator) Plan(taskID int64, cfg RunConfig) error {
	task, _, p, tools, sysPrompt, err := o.initRun(taskID, "plan", cfg)
	if err != nil {
		return err
	}

	// Clear old conversation if starting fresh
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

	// First message: the task itself
	userMsg := BuildUserPrompt("plan", task, "")
	plan, err := runPlan(ctx, p, cfg.Model, task, tools, sysPrompt, onStep, onStream)
	o.Tracker.Complete(taskID, err)
	o.completeRun(dbRun, plan, err)

	if err != nil {
		o.emit(taskID, models.WSTypeAgentError, map[string]interface{}{"error": err.Error()})
		return err
	}

	// Save conversation turn
	o.saveTurn(taskID, userMsg, plan)

	// Save questions from ask_user tool
	for _, q := range tools.GetPendingQuestions() {
		c, err := o.store.AddClarification(taskID, q)
		if err != nil {
			continue
		}
		o.emit(taskID, models.WSTypeClarify, map[string]interface{}{"id": c.ID, "question": q})
	}

	task.ImplementationPlan = plan
	task.Status = models.TaskStatusOpen
	o.store.UpdateTask(task)
	o.emit(taskID, models.WSTypeAgentDone, map[string]interface{}{"phase": "plan", "run_id": dbRun.ID})
	return nil
}

// PlanChat sends a follow-up message in the planning conversation.
// The agent sees all previous context and continues without re-reading the whole codebase.
func (o *Orchestrator) PlanChat(taskID int64, userMessage string, cfg RunConfig) error {
	task, _, p, tools, sysPrompt, err := o.initRun(taskID, "plan", cfg)
	if err != nil {
		return err
	}

	// Load existing conversation
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

	// Run loop with history + new user message
	result, err := RunLoop(ctx, LoopConfig{
		Provider:     p,
		Model:        cfg.Model,
		SystemPrompt: sysPrompt,
		History:      history,
		TaskContext:   userMessage,
		Tools:        tools,
		OnStep:       onStep,
		OnStream:     onStream,
	})
	o.Tracker.Complete(taskID, err)
	o.completeRun(dbRun, result, err)

	if err != nil {
		o.emit(taskID, models.WSTypeAgentError, map[string]interface{}{"error": err.Error()})
		return err
	}

	// Save this turn
	o.saveTurn(taskID, userMessage, result)

	// Save questions
	for _, q := range tools.GetPendingQuestions() {
		c, err := o.store.AddClarification(taskID, q)
		if err != nil {
			continue
		}
		o.emit(taskID, models.WSTypeClarify, map[string]interface{}{"id": c.ID, "question": q})
	}

	task.ImplementationPlan = result
	task.Status = models.TaskStatusOpen
	o.store.UpdateTask(task)
	o.emit(taskID, models.WSTypeAgentDone, map[string]interface{}{"phase": "plan", "run_id": dbRun.ID})
	return nil
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

	result, err := runExecute(ctx, p, cfg.Model, task, tools, sysPrompt, onStep, onStream)
	o.Tracker.Complete(taskID, err)
	o.completeRun(dbRun, result, err)

	if err != nil {
		exec.Status = "failed"
		exec.Log = err.Error()
		o.store.UpdateExecution(exec)
		o.emit(taskID, models.WSTypeAgentError, map[string]interface{}{"error": err.Error()})
		return err
	}

	exec.Status = "completed"
	exec.Log = result
	o.store.UpdateExecution(exec)

	task.Status = models.TaskStatusReview
	o.store.UpdateTask(task)
	o.emit(taskID, models.WSTypeAgentDone, map[string]interface{}{"phase": "execute", "run_id": dbRun.ID})
	return nil
}
