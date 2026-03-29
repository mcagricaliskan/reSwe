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
	Type       string    `json:"type"`        // "git" or "folder"
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

// AgentQuestion is a question the agent asks the user during planning (pause/resume)
type AgentQuestion struct {
	ID        int64     `json:"id"`
	RunID     int64     `json:"run_id"`
	TaskID    int64     `json:"task_id"`
	Question  string    `json:"question"`
	Options   []string  `json:"options,omitempty"` // optional predefined choices
	Answer    string    `json:"answer"`
	Answered  bool      `json:"answered"`
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
	Status         string    `json:"status"` // running, completed, error, cancelled, waiting
	FinalResult    string    `json:"final_result,omitempty"`
	Error          string    `json:"error,omitempty"`
	SystemPrompt   string    `json:"system_prompt,omitempty"`
	RepoSnapshot   string    `json:"repo_snapshot,omitempty"`
	PausedMessages string    `json:"paused_messages,omitempty"` // JSON: conversation history at pause point
	StepCount      int       `json:"step_count"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	Steps          []AgentStep     `json:"steps,omitempty"`
	Questions      []AgentQuestion `json:"questions,omitempty"`
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
	WSTypeAgentOutput  WSMessageType = "agent_output"
	WSTypeAgentStep    WSMessageType = "agent_step"
	WSTypeAgentDone    WSMessageType = "agent_done"
	WSTypeAgentError   WSMessageType = "agent_error"
	WSTypeAgentWaiting WSMessageType = "agent_waiting"
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

// ExcludeRule is a single global exclude pattern with a default on/off state.
// Managed in Settings, inherited by all projects unless overridden.
type ExcludeRule struct {
	ID               int64     `json:"id"`
	Pattern          string    `json:"pattern"`
	EnabledByDefault bool      `json:"enabled_by_default"`
	CreatedAt        time.Time `json:"created_at"`
}

// ProjectExcludeOverride overrides a global rule's on/off state for a specific project.
type ProjectExcludeOverride struct {
	ID        int64 `json:"id"`
	ProjectID int64 `json:"project_id"`
	RuleID    int64 `json:"rule_id"`
	Enabled   bool  `json:"enabled"`
}

// ProjectCustomPattern is a project-specific pattern (not from global rules).
type ProjectCustomPattern struct {
	ID        int64     `json:"id"`
	ProjectID int64     `json:"project_id"`
	Pattern   string    `json:"pattern"`
	CreatedAt time.Time `json:"created_at"`
}

// ResolvedRule is a global rule with its effective state for a specific project.
type ResolvedRule struct {
	ID               int64  `json:"id"`
	Pattern          string `json:"pattern"`
	EnabledByDefault bool   `json:"enabled_by_default"`
	Enabled          bool   `json:"enabled"`  // effective: override if exists, else default
	Overridden       bool   `json:"overridden"` // true if project has an override
}

// ProjectExcludeConfig is the full exclude configuration for a project.
type ProjectExcludeConfig struct {
	Rules          []ResolvedRule        `json:"rules"`
	CustomPatterns []ProjectCustomPattern `json:"custom_patterns"`
	Effective      []string              `json:"effective"` // all enabled patterns merged
}

// ProjectFile represents a file in a project's codebase (for @-mention search)
type ProjectFile struct {
	ID        int64  `json:"id"`
	ProjectID int64  `json:"project_id"`
	RepoID    int64  `json:"repo_id"`
	RelPath   string `json:"rel_path"`
	Size      int64  `json:"size"`
	IsDir     bool   `json:"is_dir"`
}

// Provider config
type ProviderConfig struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	BaseURL string `json:"base_url,omitempty"`
	APIKey  string `json:"api_key,omitempty"`
	Model   string `json:"model"`
}
