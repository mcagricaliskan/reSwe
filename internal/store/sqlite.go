package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/cagri/reswe/internal/models"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

func newUUID() string {
	return uuid.New().String()
}

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	s := &SQLiteStore{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return s, nil
}

func (s *SQLiteStore) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS projects (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		uuid TEXT NOT NULL DEFAULT '',
		name TEXT NOT NULL,
		description TEXT DEFAULT '',
		status TEXT DEFAULT 'active',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS repos (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		uuid TEXT NOT NULL DEFAULT '',
		project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
		path TEXT NOT NULL,
		name TEXT NOT NULL,
		remote_url TEXT DEFAULT '',
		root_commit TEXT DEFAULT '',
		identifier TEXT DEFAULT '',
		head_ref TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS tasks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
		title TEXT NOT NULL,
		description TEXT DEFAULT '',
		status TEXT DEFAULT 'open',
		enhanced_description TEXT DEFAULT '',
		research_notes TEXT DEFAULT '',
		implementation_plan TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS clarifications (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
		question TEXT NOT NULL,
		answer TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS executions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
		provider TEXT NOT NULL,
		model TEXT NOT NULL,
		status TEXT DEFAULT 'pending',
		files_changed TEXT DEFAULT '[]',
		log TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS chat_sessions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
		status TEXT DEFAULT 'active',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS plan_messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id INTEGER NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
		task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
		role TEXT NOT NULL,
		content TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS agent_runs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
		project_id INTEGER DEFAULT 0,
		project_uuid TEXT DEFAULT '',
		phase TEXT NOT NULL,
		provider TEXT NOT NULL,
		model TEXT NOT NULL,
		status TEXT DEFAULT 'running',
		final_result TEXT DEFAULT '',
		error TEXT DEFAULT '',
		system_prompt TEXT DEFAULT '',
		repo_snapshot TEXT DEFAULT '[]',
		step_count INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS agent_steps (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		run_id INTEGER NOT NULL REFERENCES agent_runs(id) ON DELETE CASCADE,
		step_number INTEGER NOT NULL,
		think TEXT DEFAULT '',
		action TEXT DEFAULT '',
		action_arg TEXT DEFAULT '',
		observation TEXT DEFAULT '',
		is_final INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);
	`
	_, err := s.db.Exec(schema)
	return err
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// --- Projects ---

func (s *SQLiteStore) CreateProject(name, description string) (*models.Project, error) {
	now := time.Now()
	uid := newUUID()
	res, err := s.db.Exec(
		"INSERT INTO projects (uuid, name, description, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		uid, name, description, now, now,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &models.Project{
		ID:          id,
		UUID:        uid,
		Name:        name,
		Description: description,
		Status:      models.ProjectStatusActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

func (s *SQLiteStore) GetProject(id int64) (*models.Project, error) {
	p := &models.Project{}
	err := s.db.QueryRow(
		"SELECT id, uuid, name, description, status, created_at, updated_at FROM projects WHERE id = ?", id,
	).Scan(&p.ID, &p.UUID, &p.Name, &p.Description, &p.Status, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}

	repos, err := s.ListRepos(id)
	if err != nil {
		return nil, err
	}
	p.Repos = repos

	return p, nil
}

func (s *SQLiteStore) ListProjects() ([]models.Project, error) {
	rows, err := s.db.Query("SELECT id, uuid, name, description, status, created_at, updated_at FROM projects ORDER BY updated_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []models.Project
	for rows.Next() {
		var p models.Project
		if err := rows.Scan(&p.ID, &p.UUID, &p.Name, &p.Description, &p.Status, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}
	return projects, nil
}

func (s *SQLiteStore) UpdateProject(id int64, name, description string) (*models.Project, error) {
	now := time.Now()
	_, err := s.db.Exec(
		"UPDATE projects SET name = ?, description = ?, updated_at = ? WHERE id = ?",
		name, description, now, id,
	)
	if err != nil {
		return nil, err
	}
	return s.GetProject(id)
}

func (s *SQLiteStore) DeleteProject(id int64) error {
	_, err := s.db.Exec("DELETE FROM projects WHERE id = ?", id)
	return err
}

// --- Repos ---

func (s *SQLiteStore) AddRepo(projectID int64, path, name string) (*models.Repo, error) {
	return s.AddRepoFull(projectID, path, name, "", "", "")
}

func (s *SQLiteStore) AddRepoFull(projectID int64, path, name, remoteURL, identifier, headRef string) (*models.Repo, error) {
	now := time.Now()
	uid := newUUID()

	// Extract root commit for the true stable identity
	rootCommit := ""
	cmd := exec.Command("git", "-C", path, "rev-list", "--max-parents=0", "HEAD")
	if out, err := cmd.Output(); err == nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(lines) > 0 {
			rootCommit = strings.TrimSpace(lines[0])
		}
	}

	// Build identifier: root_commit > remote > path
	if rootCommit != "" {
		identifier = "commit:" + rootCommit
	} else if remoteURL != "" {
		identifier = "remote:" + remoteURL
	} else {
		identifier = "path:" + path
	}

	res, err := s.db.Exec(
		`INSERT INTO repos (uuid, project_id, path, name, remote_url, root_commit, identifier, head_ref, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		uid, projectID, path, name, remoteURL, rootCommit, identifier, headRef, now, now,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()

	s.db.Exec("UPDATE projects SET updated_at = ? WHERE id = ?", now, projectID)

	return &models.Repo{
		ID:         id,
		UUID:       uid,
		ProjectID:  projectID,
		Path:       path,
		Name:       name,
		RemoteURL:  remoteURL,
		RootCommit: rootCommit,
		Identifier: identifier,
		HeadRef:    headRef,
		CreatedAt:  now,
		UpdatedAt:  now,
	}, nil
}

// SyncRepo updates a repo's mutable fields while keeping stable identity.
// Name is always derived from the current folder name on disk.
func (s *SQLiteStore) SyncRepo(id int64, path, remoteURL, headRef string) error {
	now := time.Now()
	name := filepath.Base(path)
	_, err := s.db.Exec(
		"UPDATE repos SET path = ?, name = ?, remote_url = ?, head_ref = ?, updated_at = ? WHERE id = ?",
		path, name, remoteURL, headRef, now, id,
	)
	return err
}

func (s *SQLiteStore) ListRepos(projectID int64) ([]models.Repo, error) {
	rows, err := s.db.Query(
		`SELECT id, uuid, project_id, path, name, remote_url, root_commit, identifier, head_ref, created_at, updated_at
		 FROM repos WHERE project_id = ? ORDER BY name`, projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var repos []models.Repo
	for rows.Next() {
		var r models.Repo
		if err := rows.Scan(&r.ID, &r.UUID, &r.ProjectID, &r.Path, &r.Name,
			&r.RemoteURL, &r.RootCommit, &r.Identifier, &r.HeadRef, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		repos = append(repos, r)
	}
	return repos, nil
}

func (s *SQLiteStore) DeleteRepo(id int64) error {
	_, err := s.db.Exec("DELETE FROM repos WHERE id = ?", id)
	return err
}

// --- Tasks ---

func (s *SQLiteStore) CreateTask(projectID int64, title, description string) (*models.Task, error) {
	now := time.Now()
	res, err := s.db.Exec(
		"INSERT INTO tasks (project_id, title, description, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		projectID, title, description, now, now,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()

	s.db.Exec("UPDATE projects SET updated_at = ? WHERE id = ?", now, projectID)

	return &models.Task{
		ID:          id,
		ProjectID:   projectID,
		Title:       title,
		Description: description,
		Status:      models.TaskStatusOpen,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

func (s *SQLiteStore) GetTask(id int64) (*models.Task, error) {
	t := &models.Task{}
	err := s.db.QueryRow(
		`SELECT id, project_id, title, description, status, enhanced_description,
		 research_notes, implementation_plan, created_at, updated_at
		 FROM tasks WHERE id = ?`, id,
	).Scan(&t.ID, &t.ProjectID, &t.Title, &t.Description, &t.Status,
		&t.EnhancedDescription, &t.ResearchNotes, &t.ImplementationPlan,
		&t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}

	clarifications, _ := s.ListClarifications(id)
	t.Clarifications = clarifications

	executions, _ := s.ListExecutions(id)
	t.Executions = executions

	return t, nil
}

func (s *SQLiteStore) ListTasks(projectID int64) ([]models.Task, error) {
	rows, err := s.db.Query(
		`SELECT id, project_id, title, description, status, enhanced_description,
		 research_notes, implementation_plan, created_at, updated_at
		 FROM tasks WHERE project_id = ? ORDER BY updated_at DESC`, projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []models.Task
	for rows.Next() {
		var t models.Task
		if err := rows.Scan(&t.ID, &t.ProjectID, &t.Title, &t.Description, &t.Status,
			&t.EnhancedDescription, &t.ResearchNotes, &t.ImplementationPlan,
			&t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

func (s *SQLiteStore) UpdateTask(task *models.Task) error {
	task.UpdatedAt = time.Now()
	_, err := s.db.Exec(
		`UPDATE tasks SET title = ?, description = ?, status = ?, enhanced_description = ?,
		 research_notes = ?, implementation_plan = ?, updated_at = ? WHERE id = ?`,
		task.Title, task.Description, task.Status, task.EnhancedDescription,
		task.ResearchNotes, task.ImplementationPlan, task.UpdatedAt, task.ID,
	)
	if err != nil {
		return err
	}
	s.db.Exec("UPDATE projects SET updated_at = ? WHERE id = ?", task.UpdatedAt, task.ProjectID)
	return nil
}

func (s *SQLiteStore) DeleteTask(id int64) error {
	_, err := s.db.Exec("DELETE FROM tasks WHERE id = ?", id)
	return err
}

// --- Chat Sessions ---

func (s *SQLiteStore) CreateSession(taskID int64) (*models.ChatSession, error) {
	now := time.Now()
	res, err := s.db.Exec(
		"INSERT INTO chat_sessions (task_id, status, created_at, updated_at) VALUES (?, 'active', ?, ?)",
		taskID, now, now,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &models.ChatSession{ID: id, TaskID: taskID, Status: "active", CreatedAt: now, UpdatedAt: now}, nil
}

func (s *SQLiteStore) GetActiveSession(taskID int64) (*models.ChatSession, error) {
	sess := &models.ChatSession{}
	err := s.db.QueryRow(
		"SELECT id, task_id, status, created_at, updated_at FROM chat_sessions WHERE task_id = ? AND status = 'active' ORDER BY created_at DESC LIMIT 1",
		taskID,
	).Scan(&sess.ID, &sess.TaskID, &sess.Status, &sess.CreatedAt, &sess.UpdatedAt)
	if err != nil {
		return nil, err
	}
	msgs, _ := s.listSessionMessages(sess.ID)
	sess.Messages = msgs
	return sess, nil
}

func (s *SQLiteStore) ArchiveSession(sessionID int64) error {
	now := time.Now()
	_, err := s.db.Exec(
		"UPDATE chat_sessions SET status = 'archived', updated_at = ? WHERE id = ?",
		now, sessionID,
	)
	return err
}

func (s *SQLiteStore) ReactivateSession(sessionID int64) error {
	now := time.Now()
	_, err := s.db.Exec(
		"UPDATE chat_sessions SET status = 'active', updated_at = ? WHERE id = ?",
		now, sessionID,
	)
	return err
}

func (s *SQLiteStore) ListSessions(taskID int64) ([]models.ChatSession, error) {
	rows, err := s.db.Query(
		"SELECT id, task_id, status, created_at, updated_at FROM chat_sessions WHERE task_id = ? ORDER BY created_at DESC",
		taskID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []models.ChatSession
	for rows.Next() {
		var sess models.ChatSession
		if err := rows.Scan(&sess.ID, &sess.TaskID, &sess.Status, &sess.CreatedAt, &sess.UpdatedAt); err != nil {
			return nil, err
		}
		// Count messages instead of loading all
		var count int
		s.db.QueryRow("SELECT COUNT(*) FROM plan_messages WHERE session_id = ?", sess.ID).Scan(&count)
		if count > 0 {
			// Load first user message as preview
			var preview string
			s.db.QueryRow("SELECT content FROM plan_messages WHERE session_id = ? AND role = 'user' ORDER BY created_at LIMIT 1", sess.ID).Scan(&preview)
			if len(preview) > 100 {
				preview = preview[:100] + "..."
			}
			sess.Messages = []models.PlanMessage{{Content: preview, Role: "user"}}
		}
		sessions = append(sessions, sess)
	}
	return sessions, nil
}

func (s *SQLiteStore) GetSession(sessionID int64) (*models.ChatSession, error) {
	sess := &models.ChatSession{}
	err := s.db.QueryRow(
		"SELECT id, task_id, status, created_at, updated_at FROM chat_sessions WHERE id = ?",
		sessionID,
	).Scan(&sess.ID, &sess.TaskID, &sess.Status, &sess.CreatedAt, &sess.UpdatedAt)
	if err != nil {
		return nil, err
	}
	msgs, _ := s.listSessionMessages(sess.ID)
	sess.Messages = msgs
	return sess, nil
}

func (s *SQLiteStore) listSessionMessages(sessionID int64) ([]models.PlanMessage, error) {
	rows, err := s.db.Query(
		"SELECT id, session_id, task_id, role, content, created_at FROM plan_messages WHERE session_id = ? ORDER BY created_at",
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []models.PlanMessage
	for rows.Next() {
		var m models.PlanMessage
		if err := rows.Scan(&m.ID, &m.SessionID, &m.TaskID, &m.Role, &m.Content, &m.CreatedAt); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, nil
}

// --- Plan Messages ---

// AddPlanMessage adds a message to the active session (creates one if none exists)
func (s *SQLiteStore) AddPlanMessage(taskID int64, role, content string) (*models.PlanMessage, error) {
	// Get or create active session
	sess, err := s.GetActiveSession(taskID)
	if err != nil {
		sess, err = s.CreateSession(taskID)
		if err != nil {
			return nil, err
		}
	}

	now := time.Now()
	res, err := s.db.Exec(
		"INSERT INTO plan_messages (session_id, task_id, role, content, created_at) VALUES (?, ?, ?, ?, ?)",
		sess.ID, taskID, role, content, now,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()

	// Touch session updated_at
	s.db.Exec("UPDATE chat_sessions SET updated_at = ? WHERE id = ?", now, sess.ID)

	return &models.PlanMessage{ID: id, SessionID: sess.ID, TaskID: taskID, Role: role, Content: content, CreatedAt: now}, nil
}

// ListPlanMessages returns messages from the active session
func (s *SQLiteStore) ListPlanMessages(taskID int64) ([]models.PlanMessage, error) {
	sess, err := s.GetActiveSession(taskID)
	if err != nil {
		return nil, nil // no active session = no messages
	}
	return s.listSessionMessages(sess.ID)
}

// ClearPlanMessages archives the current session (keeps history) and starts fresh
func (s *SQLiteStore) ClearPlanMessages(taskID int64) error {
	sess, err := s.GetActiveSession(taskID)
	if err != nil {
		return nil // no session to clear
	}
	return s.ArchiveSession(sess.ID)
}

// --- Clarifications ---

func (s *SQLiteStore) AddClarification(taskID int64, question string) (*models.Clarification, error) {
	now := time.Now()
	res, err := s.db.Exec(
		"INSERT INTO clarifications (task_id, question, created_at) VALUES (?, ?, ?)",
		taskID, question, now,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &models.Clarification{
		ID:        id,
		TaskID:    taskID,
		Question:  question,
		CreatedAt: now,
	}, nil
}

func (s *SQLiteStore) AnswerClarification(id int64, answer string) error {
	_, err := s.db.Exec("UPDATE clarifications SET answer = ? WHERE id = ?", answer, id)
	return err
}

func (s *SQLiteStore) ListClarifications(taskID int64) ([]models.Clarification, error) {
	rows, err := s.db.Query(
		"SELECT id, task_id, question, answer, created_at FROM clarifications WHERE task_id = ? ORDER BY created_at",
		taskID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []models.Clarification
	for rows.Next() {
		var c models.Clarification
		if err := rows.Scan(&c.ID, &c.TaskID, &c.Question, &c.Answer, &c.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, c)
	}
	return items, nil
}

// --- Executions ---

func (s *SQLiteStore) CreateExecution(taskID int64, provider, model string) (*models.Execution, error) {
	now := time.Now()
	res, err := s.db.Exec(
		"INSERT INTO executions (task_id, provider, model, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		taskID, provider, model, now, now,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &models.Execution{
		ID:        id,
		TaskID:    taskID,
		Provider:  provider,
		Model:     model,
		Status:    "pending",
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (s *SQLiteStore) UpdateExecution(exec *models.Execution) error {
	exec.UpdatedAt = time.Now()
	filesJSON, _ := json.Marshal(exec.FilesChanged)
	_, err := s.db.Exec(
		`UPDATE executions SET status = ?, files_changed = ?, log = ?, updated_at = ? WHERE id = ?`,
		exec.Status, string(filesJSON), exec.Log, exec.UpdatedAt, exec.ID,
	)
	return err
}

func (s *SQLiteStore) ListExecutions(taskID int64) ([]models.Execution, error) {
	rows, err := s.db.Query(
		"SELECT id, task_id, provider, model, status, files_changed, log, created_at, updated_at FROM executions WHERE task_id = ? ORDER BY created_at DESC",
		taskID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []models.Execution
	for rows.Next() {
		var e models.Execution
		var filesJSON string
		if err := rows.Scan(&e.ID, &e.TaskID, &e.Provider, &e.Model, &e.Status,
			&filesJSON, &e.Log, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(filesJSON), &e.FilesChanged)
		items = append(items, e)
	}
	return items, nil
}

// --- Settings ---

func (s *SQLiteStore) GetSetting(key string) (string, error) {
	var value string
	err := s.db.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	return value, err
}

func (s *SQLiteStore) SetSetting(key, value string) error {
	_, err := s.db.Exec(
		"INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = ?",
		key, value, value,
	)
	return err
}

// --- Agent Runs ---

func (s *SQLiteStore) CreateAgentRun(taskID int64, phase, provider, model, systemPrompt string) (*models.AgentRun, error) {
	now := time.Now()

	// Look up project info from task
	var projectID int64
	var projectUUID string
	err := s.db.QueryRow("SELECT p.id, p.uuid FROM projects p JOIN tasks t ON t.project_id = p.id WHERE t.id = ?", taskID).Scan(&projectID, &projectUUID)
	if err != nil {
		projectID = 0
		projectUUID = ""
	}

	// Snapshot repos at time of run
	var repoSnapshot string
	repos, _ := s.ListRepos(projectID)
	if repos != nil {
		snapshotJSON, _ := json.Marshal(repos)
		repoSnapshot = string(snapshotJSON)
	}

	res, err := s.db.Exec(
		`INSERT INTO agent_runs (task_id, project_id, project_uuid, phase, provider, model, system_prompt, repo_snapshot, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		taskID, projectID, projectUUID, phase, provider, model, systemPrompt, repoSnapshot, now, now,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &models.AgentRun{
		ID:           id,
		TaskID:       taskID,
		ProjectID:    projectID,
		ProjectUUID:  projectUUID,
		Phase:        phase,
		Provider:     provider,
		Model:        model,
		Status:       "running",
		SystemPrompt: systemPrompt,
		RepoSnapshot: repoSnapshot,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

func (s *SQLiteStore) UpdateAgentRun(run *models.AgentRun) error {
	run.UpdatedAt = time.Now()
	_, err := s.db.Exec(
		`UPDATE agent_runs SET status = ?, final_result = ?, error = ?, step_count = ?, updated_at = ? WHERE id = ?`,
		run.Status, run.FinalResult, run.Error, run.StepCount, run.UpdatedAt, run.ID,
	)
	return err
}

const agentRunCols = `id, task_id, project_id, project_uuid, phase, provider, model, status, final_result, error, system_prompt, repo_snapshot, step_count, created_at, updated_at`

func scanAgentRun(row interface{ Scan(...interface{}) error }) (*models.AgentRun, error) {
	r := &models.AgentRun{}
	err := row.Scan(&r.ID, &r.TaskID, &r.ProjectID, &r.ProjectUUID, &r.Phase, &r.Provider, &r.Model, &r.Status,
		&r.FinalResult, &r.Error, &r.SystemPrompt, &r.RepoSnapshot, &r.StepCount, &r.CreatedAt, &r.UpdatedAt)
	return r, err
}

func (s *SQLiteStore) GetAgentRun(id int64) (*models.AgentRun, error) {
	r, err := scanAgentRun(s.db.QueryRow(
		`SELECT `+agentRunCols+` FROM agent_runs WHERE id = ?`, id,
	))
	if err != nil {
		return nil, err
	}
	r.Steps, _ = s.ListAgentSteps(id)
	return r, nil
}

func (s *SQLiteStore) ListAgentRuns(taskID int64) ([]models.AgentRun, error) {
	rows, err := s.db.Query(
		`SELECT `+agentRunCols+` FROM agent_runs WHERE task_id = ? ORDER BY created_at DESC`, taskID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []models.AgentRun
	for rows.Next() {
		r, err := scanAgentRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, *r)
	}
	return runs, nil
}

func (s *SQLiteStore) GetLatestAgentRun(taskID int64) (*models.AgentRun, error) {
	r, err := scanAgentRun(s.db.QueryRow(
		`SELECT `+agentRunCols+` FROM agent_runs WHERE task_id = ? ORDER BY created_at DESC LIMIT 1`, taskID,
	))
	if err != nil {
		return nil, err
	}
	r.Steps, _ = s.ListAgentSteps(r.ID)
	return r, nil
}

// --- Agent Steps ---

func (s *SQLiteStore) CreateAgentStep(runID int64, step *models.AgentStep) (*models.AgentStep, error) {
	now := time.Now()
	isFinal := 0
	if step.IsFinal {
		isFinal = 1
	}
	res, err := s.db.Exec(
		`INSERT INTO agent_steps (run_id, step_number, think, action, action_arg, observation, is_final, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		runID, step.StepNumber, step.Think, step.Action, step.ActionArg, step.Observation, isFinal, now,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	step.ID = id
	step.RunID = runID
	step.CreatedAt = now
	return step, nil
}

func (s *SQLiteStore) ListAgentSteps(runID int64) ([]models.AgentStep, error) {
	rows, err := s.db.Query(
		`SELECT id, run_id, step_number, think, action, action_arg, observation, is_final, created_at
		 FROM agent_steps WHERE run_id = ? ORDER BY step_number`, runID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var steps []models.AgentStep
	for rows.Next() {
		var st models.AgentStep
		var isFinal int
		if err := rows.Scan(&st.ID, &st.RunID, &st.StepNumber, &st.Think, &st.Action,
			&st.ActionArg, &st.Observation, &isFinal, &st.CreatedAt); err != nil {
			return nil, err
		}
		st.IsFinal = isFinal != 0
		steps = append(steps, st)
	}
	return steps, nil
}
