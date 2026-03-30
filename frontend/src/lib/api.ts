const BASE = '/api'

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  })
  const data = await res.json()
  if (!res.ok) throw new Error(data.error || 'Request failed')
  return data as T
}

export interface Project {
  id: number
  name: string
  description: string
  status: string
  created_at: string
  updated_at: string
  repos?: Repo[]
}

export interface Repo {
  id: number
  project_id: number
  path: string
  name: string
  type: string // "git" or "folder"
  created_at: string
}

export interface DiscoveredRepo {
  path: string
  name: string
}

export interface Task {
  id: number
  project_id: number
  title: string
  description: string
  status: string
  enhanced_description: string
  research_notes: string
  implementation_plan: string
  created_at: string
  updated_at: string
  clarifications?: Clarification[]
  executions?: Execution[]
  todos?: PlanTodo[]
}

export interface PlanTodo {
  id: number
  task_id: number
  run_id: number
  order_index: number
  title: string
  description: string
  status: string // pending, in_progress, done, failed
  depends_on: number[]
  result: string
  created_at: string
  updated_at: string
}

export interface Clarification {
  id: number
  task_id: number
  question: string
  answer: string
  created_at: string
}

export interface Execution {
  id: number
  task_id: number
  provider: string
  model: string
  status: string
  files_changed: string[]
  log: string
  created_at: string
  updated_at: string
}

export interface PackageInfo {
  path: string
  name: string
  rel_path: string
  manager: string
}

export interface FolderAnalysis {
  path: string
  name: string
  is_git: boolean
  submodules?: DiscoveredRepo[]
  nested_repos?: DiscoveredRepo[]
  packages?: PackageInfo[]
  type: string // "single-repo" | "monorepo" | "multi-repo" | "plain-folder"
}

export interface ScanResult {
  discovered: number
  added: number
  repos: Repo[]
}

// Projects
export const listProjects = () => request<Project[]>('/projects')
export const createProject = (name: string, description: string) =>
  request<Project>('/projects', { method: 'POST', body: JSON.stringify({ name, description }) })
export const getProject = (id: number | string) => request<Project>(`/projects/${id}`)
export const updateProject = (id: number | string, name: string, description: string) =>
  request<Project>(`/projects/${id}`, { method: 'PUT', body: JSON.stringify({ name, description }) })
export const deleteProject = (id: number | string) =>
  request(`/projects/${id}`, { method: 'DELETE' })

// Repos
export const addRepo = (projectId: number | string, path: string) =>
  request<Repo>(`/projects/${projectId}/repos`, { method: 'POST', body: JSON.stringify({ path }) })
export const listRepos = (projectId: number | string) => request<Repo[]>(`/projects/${projectId}/repos`)
export const deleteRepo = (id: number) => request(`/repos/${id}`, { method: 'DELETE' })

// Discover & scan
export const discoverRepos = (path: string, maxDepth = 3) =>
  request<DiscoveredRepo[]>('/discover-repos', { method: 'POST', body: JSON.stringify({ path, max_depth: maxDepth }) })
export const scanDirectory = (projectId: number | string, path: string, maxDepth = 3) =>
  request<ScanResult>(`/projects/${projectId}/scan-directory`, { method: 'POST', body: JSON.stringify({ path, max_depth: maxDepth }) })
export const analyzeFolder = (path: string) =>
  request<FolderAnalysis>('/analyze-folder', { method: 'POST', body: JSON.stringify({ path }) })

// Exclude Rules (global settings — individual patterns with on/off)
export interface ExcludeRule {
  id: number
  pattern: string
  enabled_by_default: boolean
  created_at: string
}

export interface ResolvedRule {
  id: number
  pattern: string
  enabled_by_default: boolean
  enabled: boolean    // effective state for this project
  overridden: boolean // true if project has an override
}

export interface ProjectCustomPattern {
  id: number
  project_id: number
  pattern: string
  created_at: string
}

export interface ProjectExcludeConfig {
  rules: ResolvedRule[]
  custom_patterns: ProjectCustomPattern[]
  effective: string[]
}

export const listExcludeRules = () => request<ExcludeRule[]>('/exclude-rules')
export const createExcludeRule = (pattern: string, enabledByDefault: boolean) =>
  request<ExcludeRule>('/exclude-rules', { method: 'POST', body: JSON.stringify({ pattern, enabled_by_default: enabledByDefault }) })
export const updateExcludeRule = (id: number, pattern: string, enabledByDefault: boolean) =>
  request(`/exclude-rules/${id}`, { method: 'PUT', body: JSON.stringify({ pattern, enabled_by_default: enabledByDefault }) })
export const deleteExcludeRule = (id: number) =>
  request(`/exclude-rules/${id}`, { method: 'DELETE' })

// Project exclude config
export const getProjectExcludeConfig = (projectId: number | string) =>
  request<ProjectExcludeConfig>(`/projects/${projectId}/exclude-config`)
export const setProjectExcludeOverride = (projectId: number | string, ruleId: number, enabled: boolean) =>
  request(`/projects/${projectId}/exclude-override`, { method: 'POST', body: JSON.stringify({ rule_id: ruleId, enabled }) })
export const deleteProjectExcludeOverride = (projectId: number | string, ruleId: number) =>
  request(`/projects/${projectId}/exclude-override`, { method: 'DELETE', body: JSON.stringify({ rule_id: ruleId }) })
export const addProjectCustomPattern = (projectId: number | string, pattern: string) =>
  request<ProjectCustomPattern>(`/projects/${projectId}/custom-patterns`, { method: 'POST', body: JSON.stringify({ pattern }) })
export const deleteProjectCustomPattern = (patternId: number) =>
  request(`/custom-patterns/${patternId}`, { method: 'DELETE' })

// Tasks
export const listTasks = (projectId: number | string) => request<Task[]>(`/projects/${projectId}/tasks`)
export const createTask = (projectId: number | string, title: string, description: string) =>
  request<Task>(`/projects/${projectId}/tasks`, { method: 'POST', body: JSON.stringify({ title, description }) })
export const getTask = (id: number | string) => request<Task>(`/tasks/${id}`)
export const updateTask = (id: number | string, data: Partial<Task>) =>
  request<Task>(`/tasks/${id}`, { method: 'PUT', body: JSON.stringify(data) })
export const deleteTask = (id: number | string) => request(`/tasks/${id}`, { method: 'DELETE' })

// Agent actions
export interface RunConfig {
  provider: string
  model: string
  system_prompt?: string
  user_prompt?: string
}

export interface PhaseInfo {
  phase: string
  name: string
  description: string
  icon: string
}

export interface PromptPreview {
  phase: string
  system_prompt: string
  user_prompt: string
  context_size_chars: number
}

export const getPhases = () => request<PhaseInfo[]>('/phases')
export const previewPrompt = (taskId: number | string, phase: string) =>
  request<PromptPreview>(`/tasks/${taskId}/preview/${phase}`)

const defaultCfg = (overrides?: Partial<RunConfig>): RunConfig => ({
  provider: 'ollama',
  model: 'qwen3.5:27b',
  ...overrides,
})

export const runPlan = (taskId: number | string, cfg?: Partial<RunConfig>) =>
  request(`/tasks/${taskId}/plan`, { method: 'POST', body: JSON.stringify(defaultCfg(cfg)) })
export const planChat = (taskId: number | string, message: string) =>
  request(`/tasks/${taskId}/plan/chat`, { method: 'POST', body: JSON.stringify({ message }) })
export const listPlanMessages = (taskId: number | string) =>
  request<PlanMessage[]>(`/tasks/${taskId}/plan/messages`)
export const submitPlanAnswers = (taskId: number | string, answers: { question_id: number; answer: string }[]) =>
  request(`/tasks/${taskId}/plan/answer`, { method: 'POST', body: JSON.stringify({ answers }) })
export const getPendingQuestions = (taskId: number | string) =>
  request<AgentQuestion[]>(`/tasks/${taskId}/questions`)

export interface AgentQuestion {
  id: number
  run_id: number
  task_id: number
  question: string
  options?: string[]
  answer: string
  answered: boolean
  created_at: string
}

export interface PlanMessage {
  id: number
  session_id: number
  task_id: number
  role: string
  content: string
  created_at: string
}

export interface ChatSession {
  id: number
  task_id: number
  status: string
  created_at: string
  updated_at: string
  messages?: PlanMessage[]
}

// General task chat
export interface TaskMessage {
  id: number
  task_id: number
  role: string
  content: string
  created_at: string
}

// Pending Changes (diff review)
export interface PendingChange {
  id: string
  run_id: number
  todo_id: number
  task_id: number
  tool: string
  file_path: string
  rel_path: string
  old_content: string
  new_content: string
  diff: string
  status: string // pending, accepted, rejected
  reject_reason: string
  created_at: string
}

export const acceptChange = (changeId: string) =>
  request(`/changes/${changeId}/accept`, { method: 'POST' })
export const rejectChange = (changeId: string, reason = '') =>
  request(`/changes/${changeId}/reject`, { method: 'POST', body: JSON.stringify({ reason }) })
export const listPendingChanges = (taskId: number | string) =>
  request<PendingChange[]>(`/tasks/${taskId}/pending-changes`)

// Chat
export const taskChat = (taskId: number | string, message: string) =>
  request(`/tasks/${taskId}/chat`, { method: 'POST', body: JSON.stringify({ message }) })
export const listTaskMessages = (taskId: number | string) =>
  request<TaskMessage[]>(`/tasks/${taskId}/chat/messages`)
export const clearTaskMessages = (taskId: number | string) =>
  request(`/tasks/${taskId}/chat/messages`, { method: 'DELETE' })

export const listSessions = (taskId: number | string) =>
  request<ChatSession[]>(`/tasks/${taskId}/sessions`)
export const getSession = (sessionId: number) =>
  request<ChatSession>(`/sessions/${sessionId}`)
export const restoreSession = (sessionId: number) =>
  request<ChatSession>(`/sessions/${sessionId}/restore`, { method: 'POST' })

export const runExecute = (taskId: number | string, cfg?: Partial<RunConfig>) =>
  request(`/tasks/${taskId}/execute`, { method: 'POST', body: JSON.stringify(defaultCfg(cfg)) })
export const answerClarification = (clarificationId: number, answer: string) =>
  request(`/clarifications/${clarificationId}/answer`, { method: 'POST', body: JSON.stringify({ answer }) })

// Providers
export const listProviders = () => request<string[]>('/providers')

// Agent status
export interface AgentStatus {
  active: boolean
  phase?: string
  status?: string
  output?: string
  provider?: string
  model?: string
  started_at?: string
  ended_at?: string
  error?: string
}

export interface ActiveAgentsResponse {
  active: AgentRun[] | null
  recent: AgentRun[] | null
}

export interface AgentRun {
  task_id: number
  phase: string
  provider: string
  model: string
  status: string
  output: string
  error?: string
  started_at: string
  ended_at?: string
}

export const getAgentStatus = (taskId: number | string) =>
  request<AgentStatus>(`/tasks/${taskId}/agent-status`)
export const cancelAgent = (taskId: number | string) =>
  request(`/tasks/${taskId}/cancel`, { method: 'POST' })
export const getActiveAgents = () =>
  request<ActiveAgentsResponse>('/agents/active')

// Persisted agent runs
export interface PersistedAgentRun {
  id: number
  task_id: number
  phase: string
  provider: string
  model: string
  status: string
  final_result: string
  error: string
  system_prompt: string
  step_count: number
  started_at: string
  completed_at?: string
  duration_ms: number
  created_at: string
  updated_at: string
  steps?: PersistedAgentStep[]
}

export interface PersistedAgentStep {
  id: number
  run_id: number
  step_number: number
  think: string
  action: string
  action_arg: string
  observation: string
  is_final: boolean
  started_at: string
  completed_at: string
  duration_ms: number
  created_at: string
}

export const listAgentRuns = (taskId: number | string) =>
  request<PersistedAgentRun[]>(`/tasks/${taskId}/runs`)
export const getLatestRun = (taskId: number | string) =>
  request<PersistedAgentRun | null>(`/tasks/${taskId}/runs/latest`)
export const getAgentRun = (runId: number) =>
  request<PersistedAgentRun>(`/runs/${runId}`)

// Timeline
export interface TimelineEvent {
  id: string
  type: string
  title: string
  description?: string
  status?: string
  run_id?: number
  todo_id?: number
  change_id?: string
  metadata?: Record<string, unknown>
  created_at: string
}
export const getTimeline = (taskId: number | string) =>
  request<TimelineEvent[]>(`/tasks/${taskId}/timeline`)

// Project Files (@-mention)
export interface ProjectFile {
  id: number
  project_id: number
  repo_id: number
  rel_path: string
  size: number
  is_dir: boolean
}
export const searchProjectFiles = (projectId: number | string, q: string) =>
  request<ProjectFile[]>(`/projects/${projectId}/files?q=${encodeURIComponent(q)}`)
export const syncProjectFiles = (projectId: number | string) =>
  request<{ count: number }>(`/projects/${projectId}/sync-files`, { method: 'POST' })

// System
export interface PickDirectoryResult {
  path?: string
  cancelled: boolean
}
export const pickDirectory = (title = 'Select Directory', startDir = '') =>
  request<PickDirectoryResult>('/pick-directory', { method: 'POST', body: JSON.stringify({ title, start_dir: startDir }) })
