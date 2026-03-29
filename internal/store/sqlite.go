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
		type TEXT DEFAULT 'git',
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

	CREATE TABLE IF NOT EXISTS task_messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
		role TEXT NOT NULL,
		content TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
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
		paused_messages TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS agent_questions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		run_id INTEGER NOT NULL REFERENCES agent_runs(id) ON DELETE CASCADE,
		task_id INTEGER NOT NULL,
		question TEXT NOT NULL,
		options TEXT DEFAULT '[]',
		answer TEXT DEFAULT '',
		answered INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS plan_todos (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
		run_id INTEGER DEFAULT 0,
		order_index INTEGER NOT NULL,
		title TEXT NOT NULL,
		description TEXT DEFAULT '',
		status TEXT DEFAULT 'pending',
		depends_on TEXT DEFAULT '[]',
		result TEXT DEFAULT '',
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

	CREATE TABLE IF NOT EXISTS exclude_rules (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		pattern TEXT NOT NULL UNIQUE,
		enabled_by_default INTEGER DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS project_exclude_overrides (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
		rule_id INTEGER NOT NULL REFERENCES exclude_rules(id) ON DELETE CASCADE,
		enabled INTEGER NOT NULL,
		UNIQUE(project_id, rule_id)
	);

	CREATE TABLE IF NOT EXISTS project_custom_patterns (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
		pattern TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS project_files (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
		repo_id INTEGER NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
		rel_path TEXT NOT NULL,
		size INTEGER DEFAULT 0,
		is_dir INTEGER DEFAULT 0,
		UNIQUE(project_id, repo_id, rel_path)
	);
	CREATE INDEX IF NOT EXISTS idx_project_files_search ON project_files(project_id, rel_path);
	`
	_, err := s.db.Exec(schema)
	if err != nil {
		return err
	}

	// Migrations for existing databases
	migrations := []string{
		"ALTER TABLE repos ADD COLUMN type TEXT DEFAULT 'git'",
		"ALTER TABLE project_files ADD COLUMN is_dir INTEGER DEFAULT 0",
		// Agent timing fields
		"ALTER TABLE agent_runs ADD COLUMN started_at DATETIME",
		"ALTER TABLE agent_runs ADD COLUMN completed_at DATETIME",
		"ALTER TABLE agent_runs ADD COLUMN duration_ms INTEGER DEFAULT 0",
		"ALTER TABLE agent_steps ADD COLUMN started_at DATETIME",
		"ALTER TABLE agent_steps ADD COLUMN completed_at DATETIME",
		"ALTER TABLE agent_steps ADD COLUMN duration_ms INTEGER DEFAULT 0",
	}
	for _, m := range migrations {
		s.db.Exec(m) // ignore errors — column may already exist
	}

	// Drop old preset tables if they exist (replaced by exclude_rules)
	s.db.Exec("DROP TABLE IF EXISTS project_presets")
	s.db.Exec("DROP TABLE IF EXISTS project_exclude_patterns")
	s.db.Exec("DROP TABLE IF EXISTS exclude_presets")

	// Seed default exclude rules if none exist
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM exclude_rules").Scan(&count); err == nil && count == 0 {
		defaults := []struct{ pattern string; enabled int }{
			{".env", 1}, {".env.*", 1},
			{"*.pem", 1}, {"*.key", 1}, {"*.p12", 1}, {"*.pfx", 1}, {"*.keystore", 1},
			{"*.secret", 1},
			{"credentials.json", 1}, {"service-account*.json", 1},
			{"id_rsa", 1}, {"id_ed25519", 1}, {"id_dsa", 1}, {"id_ecdsa", 1},
			{".npmrc", 0}, {".pypirc", 0}, {".netrc", 0},
		}
		for _, d := range defaults {
			s.db.Exec("INSERT OR IGNORE INTO exclude_rules (pattern, enabled_by_default) VALUES (?, ?)", d.pattern, d.enabled)
		}
	}

	return nil
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

	// Detect type and extract git info
	repoType := "folder"
	rootCommit := ""
	cmd := exec.Command("git", "-C", path, "rev-list", "--max-parents=0", "HEAD")
	if out, err := cmd.Output(); err == nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(lines) > 0 {
			rootCommit = strings.TrimSpace(lines[0])
		}
		repoType = "git"
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
		`INSERT INTO repos (uuid, project_id, path, name, remote_url, root_commit, identifier, head_ref, type, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		uid, projectID, path, name, remoteURL, rootCommit, identifier, headRef, repoType, now, now,
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
		Type:       repoType,
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
		`SELECT id, uuid, project_id, path, name, remote_url, root_commit, identifier, head_ref, type, created_at, updated_at
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
			&r.RemoteURL, &r.RootCommit, &r.Identifier, &r.HeadRef, &r.Type, &r.CreatedAt, &r.UpdatedAt); err != nil {
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

	todos, _ := s.ListPlanTodos(id)
	t.Todos = todos

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

// --- Task Messages ---

func (s *SQLiteStore) AddTaskMessage(taskID int64, role, content string) (*models.TaskMessage, error) {
	now := time.Now()
	res, err := s.db.Exec(
		"INSERT INTO task_messages (task_id, role, content, created_at) VALUES (?, ?, ?, ?)",
		taskID, role, content, now,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &models.TaskMessage{ID: id, TaskID: taskID, Role: role, Content: content, CreatedAt: now}, nil
}

func (s *SQLiteStore) ListTaskMessages(taskID int64) ([]models.TaskMessage, error) {
	rows, err := s.db.Query(
		"SELECT id, task_id, role, content, created_at FROM task_messages WHERE task_id = ? ORDER BY created_at",
		taskID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []models.TaskMessage
	for rows.Next() {
		var m models.TaskMessage
		if err := rows.Scan(&m.ID, &m.TaskID, &m.Role, &m.Content, &m.CreatedAt); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, nil
}

func (s *SQLiteStore) ClearTaskMessages(taskID int64) error {
	_, err := s.db.Exec("DELETE FROM task_messages WHERE task_id = ?", taskID)
	return err
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
		return []models.PlanMessage{}, nil
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
		`INSERT INTO agent_runs (task_id, project_id, project_uuid, phase, provider, model, system_prompt, repo_snapshot, started_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		taskID, projectID, projectUUID, phase, provider, model, systemPrompt, repoSnapshot, now, now, now,
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
		StartedAt:    now,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

func (s *SQLiteStore) UpdateAgentRun(run *models.AgentRun) error {
	run.UpdatedAt = time.Now()
	_, err := s.db.Exec(
		`UPDATE agent_runs SET status = ?, final_result = ?, error = ?, paused_messages = ?, step_count = ?, completed_at = ?, duration_ms = ?, updated_at = ? WHERE id = ?`,
		run.Status, run.FinalResult, run.Error, run.PausedMessages, run.StepCount, run.CompletedAt, run.DurationMs, run.UpdatedAt, run.ID,
	)
	return err
}

const agentRunCols = `id, task_id, project_id, project_uuid, phase, provider, model, status, final_result, error, system_prompt, repo_snapshot, paused_messages, step_count, started_at, completed_at, duration_ms, created_at, updated_at`

func scanAgentRun(row interface{ Scan(...interface{}) error }) (*models.AgentRun, error) {
	r := &models.AgentRun{}
	err := row.Scan(&r.ID, &r.TaskID, &r.ProjectID, &r.ProjectUUID, &r.Phase, &r.Provider, &r.Model, &r.Status,
		&r.FinalResult, &r.Error, &r.SystemPrompt, &r.RepoSnapshot, &r.PausedMessages, &r.StepCount,
		&r.StartedAt, &r.CompletedAt, &r.DurationMs, &r.CreatedAt, &r.UpdatedAt)
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
	// If CompletedAt not set, use now
	if step.CompletedAt.IsZero() {
		step.CompletedAt = now
	}
	// Calculate duration if StartedAt is set
	if !step.StartedAt.IsZero() {
		step.DurationMs = step.CompletedAt.Sub(step.StartedAt).Milliseconds()
	}
	res, err := s.db.Exec(
		`INSERT INTO agent_steps (run_id, step_number, think, action, action_arg, observation, is_final, started_at, completed_at, duration_ms, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		runID, step.StepNumber, step.Think, step.Action, step.ActionArg, step.Observation, isFinal, step.StartedAt, step.CompletedAt, step.DurationMs, now,
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
		`SELECT id, run_id, step_number, think, action, action_arg, observation, is_final, started_at, completed_at, duration_ms, created_at
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
			&st.ActionArg, &st.Observation, &isFinal, &st.StartedAt, &st.CompletedAt, &st.DurationMs, &st.CreatedAt); err != nil {
			return nil, err
		}
		st.IsFinal = isFinal != 0
		steps = append(steps, st)
	}
	return steps, nil
}

// --- Agent Questions ---

func (s *SQLiteStore) CreateAgentQuestion(runID, taskID int64, question string, options []string) (*models.AgentQuestion, error) {
	now := time.Now()
	optJSON, _ := json.Marshal(options)
	res, err := s.db.Exec(
		"INSERT INTO agent_questions (run_id, task_id, question, options, created_at) VALUES (?, ?, ?, ?, ?)",
		runID, taskID, question, string(optJSON), now,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &models.AgentQuestion{
		ID: id, RunID: runID, TaskID: taskID,
		Question: question, Options: options, CreatedAt: now,
	}, nil
}

func (s *SQLiteStore) AnswerAgentQuestion(id int64, answer string) error {
	_, err := s.db.Exec("UPDATE agent_questions SET answer = ?, answered = 1 WHERE id = ?", answer, id)
	return err
}

func (s *SQLiteStore) ListAgentQuestions(runID int64) ([]models.AgentQuestion, error) {
	rows, err := s.db.Query(
		"SELECT id, run_id, task_id, question, options, answer, answered, created_at FROM agent_questions WHERE run_id = ? ORDER BY id",
		runID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var qs []models.AgentQuestion
	for rows.Next() {
		var q models.AgentQuestion
		var optJSON string
		var answered int
		if err := rows.Scan(&q.ID, &q.RunID, &q.TaskID, &q.Question, &optJSON, &q.Answer, &answered, &q.CreatedAt); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(optJSON), &q.Options)
		q.Answered = answered != 0
		qs = append(qs, q)
	}
	return qs, nil
}

func (s *SQLiteStore) ListPendingQuestions(taskID int64) ([]models.AgentQuestion, error) {
	rows, err := s.db.Query(
		`SELECT q.id, q.run_id, q.task_id, q.question, q.options, q.answer, q.answered, q.created_at
		 FROM agent_questions q
		 JOIN agent_runs r ON q.run_id = r.id
		 WHERE q.task_id = ? AND r.status = 'waiting' AND q.answered = 0
		 ORDER BY q.id`, taskID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var qs []models.AgentQuestion
	for rows.Next() {
		var q models.AgentQuestion
		var optJSON string
		var answered int
		if err := rows.Scan(&q.ID, &q.RunID, &q.TaskID, &q.Question, &optJSON, &q.Answer, &answered, &q.CreatedAt); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(optJSON), &q.Options)
		q.Answered = answered != 0
		qs = append(qs, q)
	}
	return qs, nil
}

// --- Plan TODOs ---

func (s *SQLiteStore) CreatePlanTodo(todo *models.PlanTodo) (*models.PlanTodo, error) {
	now := time.Now()
	depsJSON, _ := json.Marshal(todo.DependsOn)
	res, err := s.db.Exec(
		`INSERT INTO plan_todos (task_id, run_id, order_index, title, description, status, depends_on, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		todo.TaskID, todo.RunID, todo.OrderIndex, todo.Title, todo.Description, todo.Status, string(depsJSON), now, now,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	todo.ID = id
	todo.CreatedAt = now
	todo.UpdatedAt = now
	return todo, nil
}

func (s *SQLiteStore) ListPlanTodos(taskID int64) ([]models.PlanTodo, error) {
	rows, err := s.db.Query(
		`SELECT id, task_id, run_id, order_index, title, description, status, depends_on, result, created_at, updated_at
		 FROM plan_todos WHERE task_id = ? ORDER BY order_index`, taskID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var todos []models.PlanTodo
	for rows.Next() {
		var t models.PlanTodo
		var depsJSON string
		if err := rows.Scan(&t.ID, &t.TaskID, &t.RunID, &t.OrderIndex, &t.Title, &t.Description,
			&t.Status, &depsJSON, &t.Result, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(depsJSON), &t.DependsOn)
		if t.DependsOn == nil {
			t.DependsOn = []int64{}
		}
		todos = append(todos, t)
	}
	return todos, nil
}

func (s *SQLiteStore) UpdatePlanTodo(todo *models.PlanTodo) error {
	todo.UpdatedAt = time.Now()
	depsJSON, _ := json.Marshal(todo.DependsOn)
	_, err := s.db.Exec(
		`UPDATE plan_todos SET title = ?, description = ?, status = ?, depends_on = ?, result = ?, updated_at = ? WHERE id = ?`,
		todo.Title, todo.Description, todo.Status, string(depsJSON), todo.Result, todo.UpdatedAt, todo.ID,
	)
	return err
}

func (s *SQLiteStore) ClearPlanTodos(taskID int64) error {
	_, err := s.db.Exec("DELETE FROM plan_todos WHERE task_id = ?", taskID)
	return err
}

func (s *SQLiteStore) GetPlanTodo(id int64) (*models.PlanTodo, error) {
	t := &models.PlanTodo{}
	var depsJSON string
	err := s.db.QueryRow(
		`SELECT id, task_id, run_id, order_index, title, description, status, depends_on, result, created_at, updated_at
		 FROM plan_todos WHERE id = ?`, id,
	).Scan(&t.ID, &t.TaskID, &t.RunID, &t.OrderIndex, &t.Title, &t.Description,
		&t.Status, &depsJSON, &t.Result, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	json.Unmarshal([]byte(depsJSON), &t.DependsOn)
	if t.DependsOn == nil {
		t.DependsOn = []int64{}
	}
	return t, nil
}

// --- Exclude Rules (global) ---

func (s *SQLiteStore) ListExcludeRules() ([]models.ExcludeRule, error) {
	rows, err := s.db.Query("SELECT id, pattern, enabled_by_default, created_at FROM exclude_rules ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []models.ExcludeRule
	for rows.Next() {
		var r models.ExcludeRule
		var enabled int
		if err := rows.Scan(&r.ID, &r.Pattern, &enabled, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.EnabledByDefault = enabled != 0
		rules = append(rules, r)
	}
	return rules, nil
}

func (s *SQLiteStore) CreateExcludeRule(pattern string, enabledByDefault bool) (*models.ExcludeRule, error) {
	now := time.Now()
	e := 0
	if enabledByDefault {
		e = 1
	}
	res, err := s.db.Exec("INSERT INTO exclude_rules (pattern, enabled_by_default, created_at) VALUES (?, ?, ?)", pattern, e, now)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &models.ExcludeRule{ID: id, Pattern: pattern, EnabledByDefault: enabledByDefault, CreatedAt: now}, nil
}

func (s *SQLiteStore) UpdateExcludeRule(id int64, pattern string, enabledByDefault bool) error {
	e := 0
	if enabledByDefault {
		e = 1
	}
	_, err := s.db.Exec("UPDATE exclude_rules SET pattern = ?, enabled_by_default = ? WHERE id = ?", pattern, e, id)
	return err
}

func (s *SQLiteStore) DeleteExcludeRule(id int64) error {
	_, err := s.db.Exec("DELETE FROM exclude_rules WHERE id = ?", id)
	return err
}

// --- Project Exclude Overrides ---

func (s *SQLiteStore) SetProjectExcludeOverride(projectID, ruleID int64, enabled bool) error {
	e := 0
	if enabled {
		e = 1
	}
	_, err := s.db.Exec(
		"INSERT INTO project_exclude_overrides (project_id, rule_id, enabled) VALUES (?, ?, ?) ON CONFLICT(project_id, rule_id) DO UPDATE SET enabled = ?",
		projectID, ruleID, e, e,
	)
	return err
}

func (s *SQLiteStore) DeleteProjectExcludeOverride(projectID, ruleID int64) error {
	_, err := s.db.Exec("DELETE FROM project_exclude_overrides WHERE project_id = ? AND rule_id = ?", projectID, ruleID)
	return err
}

func (s *SQLiteStore) ListProjectExcludeOverrides(projectID int64) ([]models.ProjectExcludeOverride, error) {
	rows, err := s.db.Query("SELECT id, project_id, rule_id, enabled FROM project_exclude_overrides WHERE project_id = ?", projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var overrides []models.ProjectExcludeOverride
	for rows.Next() {
		var o models.ProjectExcludeOverride
		var enabled int
		if err := rows.Scan(&o.ID, &o.ProjectID, &o.RuleID, &enabled); err != nil {
			return nil, err
		}
		o.Enabled = enabled != 0
		overrides = append(overrides, o)
	}
	return overrides, nil
}

// --- Project Custom Patterns ---

func (s *SQLiteStore) ListProjectCustomPatterns(projectID int64) ([]models.ProjectCustomPattern, error) {
	rows, err := s.db.Query("SELECT id, project_id, pattern, created_at FROM project_custom_patterns WHERE project_id = ? ORDER BY id", projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var patterns []models.ProjectCustomPattern
	for rows.Next() {
		var p models.ProjectCustomPattern
		if err := rows.Scan(&p.ID, &p.ProjectID, &p.Pattern, &p.CreatedAt); err != nil {
			return nil, err
		}
		patterns = append(patterns, p)
	}
	return patterns, nil
}

func (s *SQLiteStore) AddProjectCustomPattern(projectID int64, pattern string) (*models.ProjectCustomPattern, error) {
	now := time.Now()
	res, err := s.db.Exec("INSERT INTO project_custom_patterns (project_id, pattern, created_at) VALUES (?, ?, ?)", projectID, pattern, now)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &models.ProjectCustomPattern{ID: id, ProjectID: projectID, Pattern: pattern, CreatedAt: now}, nil
}

func (s *SQLiteStore) DeleteProjectCustomPattern(id int64) error {
	_, err := s.db.Exec("DELETE FROM project_custom_patterns WHERE id = ?", id)
	return err
}

// GetEffectiveExcludePatterns returns all enabled patterns for a project:
// global rules (with per-project overrides applied) + project custom patterns.
func (s *SQLiteStore) GetEffectiveExcludePatterns(projectID int64) ([]string, error) {
	rules, err := s.ListExcludeRules()
	if err != nil {
		return nil, err
	}
	overrides, err := s.ListProjectExcludeOverrides(projectID)
	if err != nil {
		return nil, err
	}

	overrideMap := make(map[int64]bool)
	for _, o := range overrides {
		overrideMap[o.RuleID] = o.Enabled
	}

	seen := make(map[string]bool)
	var result []string

	for _, r := range rules {
		enabled := r.EnabledByDefault
		if ov, ok := overrideMap[r.ID]; ok {
			enabled = ov
		}
		if enabled && !seen[r.Pattern] {
			seen[r.Pattern] = true
			result = append(result, r.Pattern)
		}
	}

	custom, err := s.ListProjectCustomPatterns(projectID)
	if err != nil {
		return nil, err
	}
	for _, c := range custom {
		if !seen[c.Pattern] {
			seen[c.Pattern] = true
			result = append(result, c.Pattern)
		}
	}

	return result, nil
}

// --- Project Files ---

func (s *SQLiteStore) SyncProjectFiles(projectID int64, files []models.ProjectFile) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM project_files WHERE project_id = ?", projectID); err != nil {
		return err
	}

	// Bulk insert in batches of 500
	const batchSize = 500
	for i := 0; i < len(files); i += batchSize {
		end := i + batchSize
		if end > len(files) {
			end = len(files)
		}
		batch := files[i:end]

		var b strings.Builder
		b.WriteString("INSERT INTO project_files (project_id, repo_id, rel_path, size, is_dir) VALUES ")
		args := make([]interface{}, 0, len(batch)*5)
		for j, f := range batch {
			if j > 0 {
				b.WriteString(",")
			}
			isDir := 0
			if f.IsDir {
				isDir = 1
			}
			b.WriteString("(?,?,?,?,?)")
			args = append(args, f.ProjectID, f.RepoID, f.RelPath, f.Size, isDir)
		}
		if _, err := tx.Exec(b.String(), args...); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *SQLiteStore) SearchProjectFiles(projectID int64, query string, limit int) ([]models.ProjectFile, error) {
	// Build LIKE pattern: "internal/ext/x" → "%internal%/%ext%/%x%"
	parts := strings.Split(query, "/")
	var pattern strings.Builder
	pattern.WriteString("%")
	for i, part := range parts {
		if i > 0 {
			pattern.WriteString("/%")
		}
		// Escape LIKE wildcards in user input
		part = strings.ReplaceAll(part, "%", "\\%")
		part = strings.ReplaceAll(part, "_", "\\_")
		pattern.WriteString(part)
		pattern.WriteString("%")
	}

	rows, err := s.db.Query(
		`SELECT id, project_id, repo_id, rel_path, size, is_dir FROM project_files
		 WHERE project_id = ? AND rel_path LIKE ? ESCAPE '\'
		 ORDER BY is_dir DESC, rel_path LIMIT ?`,
		projectID, pattern.String(), limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []models.ProjectFile
	for rows.Next() {
		var f models.ProjectFile
		var isDir int
		if err := rows.Scan(&f.ID, &f.ProjectID, &f.RepoID, &f.RelPath, &f.Size, &isDir); err != nil {
			return nil, err
		}
		f.IsDir = isDir != 0
		files = append(files, f)
	}
	return files, nil
}
