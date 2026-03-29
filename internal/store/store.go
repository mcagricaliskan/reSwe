package store

import (
	"github.com/cagri/reswe/internal/models"
)

type Store interface {
	// Projects
	CreateProject(name, description string) (*models.Project, error)
	GetProject(id int64) (*models.Project, error)
	ListProjects() ([]models.Project, error)
	UpdateProject(id int64, name, description string) (*models.Project, error)
	DeleteProject(id int64) error

	// Repos
	AddRepo(projectID int64, path, name string) (*models.Repo, error)
	AddRepoFull(projectID int64, path, name, remoteURL, identifier, headRef string) (*models.Repo, error)
	SyncRepo(id int64, path, remoteURL, headRef string) error
	ListRepos(projectID int64) ([]models.Repo, error)
	DeleteRepo(id int64) error

	// Tasks
	CreateTask(projectID int64, title, description string) (*models.Task, error)
	GetTask(id int64) (*models.Task, error)
	ListTasks(projectID int64) ([]models.Task, error)
	UpdateTask(task *models.Task) error
	DeleteTask(id int64) error

	// Clarifications
	AddClarification(taskID int64, question string) (*models.Clarification, error)
	AnswerClarification(id int64, answer string) error
	ListClarifications(taskID int64) ([]models.Clarification, error)

	// Executions
	CreateExecution(taskID int64, provider, model string) (*models.Execution, error)
	UpdateExecution(exec *models.Execution) error
	ListExecutions(taskID int64) ([]models.Execution, error)

	// Chat Sessions
	CreateSession(taskID int64) (*models.ChatSession, error)
	GetActiveSession(taskID int64) (*models.ChatSession, error)
	ArchiveSession(sessionID int64) error
	ReactivateSession(sessionID int64) error
	ListSessions(taskID int64) ([]models.ChatSession, error)
	GetSession(sessionID int64) (*models.ChatSession, error)

	// Plan Messages (conversation history)
	AddPlanMessage(taskID int64, role, content string) (*models.PlanMessage, error)
	ListPlanMessages(taskID int64) ([]models.PlanMessage, error)
	ClearPlanMessages(taskID int64) error

	// General task chat
	AddTaskMessage(taskID int64, role, content string) (*models.TaskMessage, error)
	ListTaskMessages(taskID int64) ([]models.TaskMessage, error)
	ClearTaskMessages(taskID int64) error

	// Agent Runs
	CreateAgentRun(taskID int64, phase, provider, model, systemPrompt string) (*models.AgentRun, error)
	UpdateAgentRun(run *models.AgentRun) error
	GetAgentRun(id int64) (*models.AgentRun, error)
	ListAgentRuns(taskID int64) ([]models.AgentRun, error)
	GetLatestAgentRun(taskID int64) (*models.AgentRun, error)

	// Agent Steps
	CreateAgentStep(runID int64, step *models.AgentStep) (*models.AgentStep, error)
	ListAgentSteps(runID int64) ([]models.AgentStep, error)

	// Plan TODOs
	CreatePlanTodo(todo *models.PlanTodo) (*models.PlanTodo, error)
	ListPlanTodos(taskID int64) ([]models.PlanTodo, error)
	UpdatePlanTodo(todo *models.PlanTodo) error
	ClearPlanTodos(taskID int64) error
	GetPlanTodo(id int64) (*models.PlanTodo, error)

	// Agent Questions (pause/resume)
	CreateAgentQuestion(runID, taskID int64, question string, options []string) (*models.AgentQuestion, error)
	AnswerAgentQuestion(id int64, answer string) error
	ListAgentQuestions(runID int64) ([]models.AgentQuestion, error)
	ListPendingQuestions(taskID int64) ([]models.AgentQuestion, error)

	// Settings
	GetSetting(key string) (string, error)
	SetSetting(key, value string) error

	// Exclude Rules (global)
	ListExcludeRules() ([]models.ExcludeRule, error)
	CreateExcludeRule(pattern string, enabledByDefault bool) (*models.ExcludeRule, error)
	UpdateExcludeRule(id int64, pattern string, enabledByDefault bool) error
	DeleteExcludeRule(id int64) error

	// Project exclude overrides
	SetProjectExcludeOverride(projectID, ruleID int64, enabled bool) error
	DeleteProjectExcludeOverride(projectID, ruleID int64) error
	ListProjectExcludeOverrides(projectID int64) ([]models.ProjectExcludeOverride, error)

	// Project custom patterns
	ListProjectCustomPatterns(projectID int64) ([]models.ProjectCustomPattern, error)
	AddProjectCustomPattern(projectID int64, pattern string) (*models.ProjectCustomPattern, error)
	DeleteProjectCustomPattern(id int64) error

	// Combined: effective patterns for a project (rules + overrides + custom)
	GetEffectiveExcludePatterns(projectID int64) ([]string, error)

	// Project Files (for @-mention search)
	SyncProjectFiles(projectID int64, files []models.ProjectFile) error
	SearchProjectFiles(projectID int64, query string, limit int) ([]models.ProjectFile, error)

	Close() error
}
