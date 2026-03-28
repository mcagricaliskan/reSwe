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
  created_at: string
}

export const listAgentRuns = (taskId: number | string) =>
  request<PersistedAgentRun[]>(`/tasks/${taskId}/runs`)
export const getLatestRun = (taskId: number | string) =>
  request<PersistedAgentRun | null>(`/tasks/${taskId}/runs/latest`)
export const getAgentRun = (runId: number) =>
  request<PersistedAgentRun>(`/runs/${runId}`)

// System
export interface PickDirectoryResult {
  path?: string
  cancelled: boolean
}
export const pickDirectory = (title = 'Select Directory', startDir = '') =>
  request<PickDirectoryResult>('/pick-directory', { method: 'POST', body: JSON.stringify({ title, start_dir: startDir }) })
