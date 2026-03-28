package models

import "time"

type ProjectStatus string

const (
	ProjectStatusActive   ProjectStatus = "active"
	ProjectStatusArchived ProjectStatus = "archived"
)

type TaskStatus string

const (
	TaskStatusOpen       TaskStatus = "open"
	TaskStatusResearch   TaskStatus = "researching"
	TaskStatusClarify    TaskStatus = "clarifying"
	TaskStatusPlanning   TaskStatus = "planning"
	TaskStatusExecuting  TaskStatus = "executing"
	TaskStatusReview     TaskStatus = "review"
	TaskStatusDone       TaskStatus = "done"
)

type Project struct {
	ID          int64         `json:"id"`
	UUID        string        `json:"uuid"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Status      ProjectStatus `json:"status"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
	Repos       []Repo        `json:"repos,omitempty"`
	Tasks       []Task        `json:"tasks,omitempty"`
}

type Repo struct {
	ID         int64     `json:"id"`
	UUID       string    `json:"uuid"`
	ProjectID  int64     `json:"project_id"`
	Path       string    `json:"path"`
	Name       string    `json:"name"`
	RemoteURL  string    `json:"remote_url"`  // git remote origin (can change on rename)
	RootCommit string    `json:"root_commit"` // hash of first commit — true stable identity across clones/renames
	Identifier string    `json:"identifier"`  // best stable ID: root_commit > remote_url > path
	HeadRef    string    `json:"head_ref"`    // current branch
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type Task struct {
	ID                  int64          `json:"id"`
	ProjectID           int64          `json:"project_id"`
	Title               string         `json:"title"`
	Description         string         `json:"description"`
	Status              TaskStatus     `json:"status"`
	EnhancedDescription string         `json:"enhanced_description,omitempty"`
	ResearchNotes       string         `json:"research_notes,omitempty"`
	ImplementationPlan  string         `json:"implementation_plan,omitempty"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
	Clarifications      []Clarification `json:"clarifications,omitempty"`
	Executions          []Execution    `json:"executions,omitempty"`
}

type Clarification struct {
	ID        int64     `json:"id"`
	TaskID    int64     `json:"task_id"`
	Question  string    `json:"question"`
	Answer    string    `json:"answer,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// ChatSession groups messages into a conversation
type ChatSession struct {
	ID        int64     `json:"id"`
	TaskID    int64     `json:"task_id"`
	Status    string    `json:"status"` // "active", "archived"
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Messages  []PlanMessage `json:"messages,omitempty"`
}

// PlanMessage is a single message in a chat session
type PlanMessage struct {
	ID        int64     `json:"id"`
	SessionID int64     `json:"session_id"`
	TaskID    int64     `json:"task_id"`
	Role      string    `json:"role"` // "user", "assistant"
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type Execution struct {
	ID           int64     `json:"id"`
	TaskID       int64     `json:"task_id"`
	Provider     string    `json:"provider"`
	Model        string    `json:"model"`
	Status       string    `json:"status"`
	FilesChanged []string  `json:"files_changed,omitempty"`
	Log          string    `json:"log,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Agent run represents a single agent execution (persisted)
type AgentRun struct {
	ID           int64     `json:"id"`
	TaskID       int64     `json:"task_id"`
	ProjectID    int64     `json:"project_id"`
	ProjectUUID  string    `json:"project_uuid"`
	Phase        string    `json:"phase"`
	Provider     string    `json:"provider"`
	Model        string    `json:"model"`
	Status       string    `json:"status"` // running, completed, error, cancelled
	FinalResult  string    `json:"final_result,omitempty"`
	Error        string    `json:"error,omitempty"`
	SystemPrompt string    `json:"system_prompt,omitempty"`
	RepoSnapshot string    `json:"repo_snapshot,omitempty"` // JSON: repos + identifiers at time of run
	StepCount    int       `json:"step_count"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Steps        []AgentStep `json:"steps,omitempty"`
}

// AgentStep represents one think/act/observe iteration (persisted)
type AgentStep struct {
	ID          int64     `json:"id"`
	RunID       int64     `json:"run_id"`
	StepNumber  int       `json:"step_number"`
	Think       string    `json:"think"`
	Action      string    `json:"action"`
	ActionArg   string    `json:"action_arg"`
	Observation string    `json:"observation"`
	IsFinal     bool      `json:"is_final"`
	CreatedAt   time.Time `json:"created_at"`
}

// WebSocket message types
type WSMessageType string

const (
	WSTypeAgentOutput WSMessageType = "agent_output"
	WSTypeAgentStep   WSMessageType = "agent_step"
	WSTypeAgentDone   WSMessageType = "agent_done"
	WSTypeAgentError  WSMessageType = "agent_error"
	WSTypeTaskUpdate  WSMessageType = "task_update"
	WSTypeClarify     WSMessageType = "clarify"
)

type WSMessage struct {
	Type    WSMessageType `json:"type"`
	TaskID  int64         `json:"task_id"`
	Payload interface{}   `json:"payload"`
}

// AI provider types
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

type ChatResponse struct {
	Content  string `json:"content"`
	Done     bool   `json:"done"`
	Model    string `json:"model"`
}

// Provider config
type ProviderConfig struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	BaseURL string `json:"base_url,omitempty"`
	APIKey  string `json:"api_key,omitempty"`
	Model   string `json:"model"`
}
