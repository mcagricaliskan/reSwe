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

	// Agent Runs
	CreateAgentRun(taskID int64, phase, provider, model, systemPrompt string) (*models.AgentRun, error)
	UpdateAgentRun(run *models.AgentRun) error
	GetAgentRun(id int64) (*models.AgentRun, error)
	ListAgentRuns(taskID int64) ([]models.AgentRun, error)
	GetLatestAgentRun(taskID int64) (*models.AgentRun, error)

	// Agent Steps
	CreateAgentStep(runID int64, step *models.AgentStep) (*models.AgentStep, error)
	ListAgentSteps(runID int64) ([]models.AgentStep, error)

	// Settings
	GetSetting(key string) (string, error)
	SetSetting(key, value string) error

	Close() error
}
