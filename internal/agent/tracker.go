package agent

import (
	"context"
	"sync"
	"time"
)

// AgentRun represents a currently running or recently completed agent run
type AgentRun struct {
	TaskID    int64     `json:"task_id"`
	Phase     string    `json:"phase"`
	Provider  string    `json:"provider"`
	Model     string    `json:"model"`
	Status    string    `json:"status"` // "running", "completed", "error"
	Output    string    `json:"output"`
	Error     string    `json:"error,omitempty"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at,omitempty"`

	cancel context.CancelFunc `json:"-"`
	mu     sync.Mutex         `json:"-"`
}

func (r *AgentRun) AppendOutput(chunk string) {
	r.mu.Lock()
	r.Output += chunk
	r.mu.Unlock()
}

func (r *AgentRun) GetOutput() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.Output
}

// Tracker tracks all active and recent agent runs
type Tracker struct {
	mu   sync.RWMutex
	runs map[int64]*AgentRun // keyed by task ID
}

func NewTracker() *Tracker {
	return &Tracker{
		runs: make(map[int64]*AgentRun),
	}
}

// Start registers a new agent run for a task. Cancels any existing run on the same task.
func (t *Tracker) Start(taskID int64, phase, provider, model string) (context.Context, *AgentRun) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Cancel existing run on this task if any
	if existing, ok := t.runs[taskID]; ok && existing.Status == "running" {
		if existing.cancel != nil {
			existing.cancel()
		}
		existing.Status = "cancelled"
		existing.EndedAt = time.Now()
	}

	ctx, cancel := context.WithCancel(context.Background())

	run := &AgentRun{
		TaskID:    taskID,
		Phase:     phase,
		Provider:  provider,
		Model:     model,
		Status:    "running",
		StartedAt: time.Now(),
		cancel:    cancel,
	}

	t.runs[taskID] = run
	return ctx, run
}

// Complete marks a run as done
func (t *Tracker) Complete(taskID int64, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	run, ok := t.runs[taskID]
	if !ok {
		return
	}

	run.EndedAt = time.Now()
	if err != nil {
		run.Status = "error"
		run.Error = err.Error()
	} else {
		run.Status = "completed"
	}
}

// Cancel stops an agent run
func (t *Tracker) Cancel(taskID int64) {
	t.mu.RLock()
	run, ok := t.runs[taskID]
	t.mu.RUnlock()

	if ok && run.Status == "running" && run.cancel != nil {
		run.cancel()
		run.mu.Lock()
		run.Status = "cancelled"
		run.EndedAt = time.Now()
		run.mu.Unlock()
	}
}

// Get returns the current run for a task (or nil)
func (t *Tracker) Get(taskID int64) *AgentRun {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.runs[taskID]
}

// GetAll returns all runs (for dashboard)
func (t *Tracker) GetAllActive() []*AgentRun {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var active []*AgentRun
	for _, run := range t.runs {
		if run.Status == "running" {
			active = append(active, run)
		}
	}
	return active
}

// GetAllRecent returns runs from the last N minutes
func (t *Tracker) GetAllRecent(since time.Duration) []*AgentRun {
	t.mu.RLock()
	defer t.mu.RUnlock()

	cutoff := time.Now().Add(-since)
	var recent []*AgentRun
	for _, run := range t.runs {
		if run.StartedAt.After(cutoff) {
			recent = append(recent, run)
		}
	}
	return recent
}
