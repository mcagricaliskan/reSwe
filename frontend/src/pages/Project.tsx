import { useState, useEffect } from 'react'
import { useParams, Link } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Badge } from '@/components/ui/badge'
import { Separator } from '@/components/ui/separator'
import { Plus, Trash2, FolderSearch, FolderOpen, GitBranch, ChevronRight, Loader2 } from 'lucide-react'
import { toast } from 'sonner'
import type { Project, Repo, Task, DiscoveredRepo } from '@/lib/api'
import * as api from '@/lib/api'
import { wsClient } from '@/lib/ws'

const statusVariant: Record<string, 'default' | 'secondary' | 'destructive' | 'outline'> = {
  open: 'secondary',
  researching: 'default',
  clarifying: 'default',
  planning: 'default',
  executing: 'default',
  review: 'outline',
  done: 'secondary',
}

export default function ProjectPage() {
  const { id } = useParams<{ id: string }>()
  const [project, setProject] = useState<Project | null>(null)
  const [tasks, setTasks] = useState<Task[]>([])
  const [repos, setRepos] = useState<Repo[]>([])

  // Forms
  const [showTaskForm, setShowTaskForm] = useState(false)
  const [showRepoForm, setShowRepoForm] = useState(false)
  const [showScanForm, setShowScanForm] = useState(false)
  const [taskTitle, setTaskTitle] = useState('')
  const [taskDesc, setTaskDesc] = useState('')
  const [repoPath, setRepoPath] = useState('')
  const [scanPath, setScanPath] = useState('')
  const [discovered, setDiscovered] = useState<DiscoveredRepo[]>([])
  const [scanning, setScanning] = useState(false)
  const [activeAgentTasks, setActiveAgentTasks] = useState<Set<number>>(new Set())

  useEffect(() => {
    if (id) {
      loadProject()
      loadTasks()
      loadActiveAgents()
    }
  }, [id])

  // Listen for task updates via WebSocket to refresh the list
  useEffect(() => {
    const unsubs: Array<() => void> = []
    unsubs.push(wsClient.on('task_update', () => { loadTasks(); loadActiveAgents() }))
    unsubs.push(wsClient.on('agent_done', () => { loadTasks(); loadActiveAgents() }))
    unsubs.push(wsClient.on('agent_error', () => { loadTasks(); loadActiveAgents() }))
    return () => unsubs.forEach(fn => fn())
  }, [])

  async function loadActiveAgents() {
    try {
      const data = await api.getActiveAgents()
      const activeIds = new Set<number>()
      if (data.active) {
        data.active.forEach(a => activeIds.add(a.task_id))
      }
      setActiveAgentTasks(activeIds)
    } catch {}
  }

  async function loadProject() {
    try {
      const data = await api.getProject(id!)
      setProject(data)
      setRepos(data.repos || [])
    } catch {
      toast.error('Failed to load project')
    }
  }

  async function loadTasks() {
    try {
      const data = await api.listTasks(id!)
      setTasks(data || [])
    } catch {
      toast.error('Failed to load tasks')
    }
  }

  async function handlePickDirectory(target: 'scan' | 'repo') {
    try {
      const result = await api.pickDirectory('Select Directory')
      if (result.cancelled || !result.path) return
      if (target === 'scan') {
        setScanPath(result.path)
      } else {
        setRepoPath(result.path)
      }
    } catch {
      toast.error('Failed to open directory picker')
    }
  }

  async function handleAddRepo(e: React.FormEvent) {
    e.preventDefault()
    if (!repoPath.trim()) return
    try {
      await api.addRepo(id!, repoPath)
      setRepoPath('')
      setShowRepoForm(false)
      toast.success('Repository added')
      loadProject()
    } catch {
      toast.error('Failed to add repo')
    }
  }

  async function handleScanDiscover() {
    if (!scanPath.trim()) return
    setScanning(true)
    try {
      const repos = await api.discoverRepos(scanPath)
      setDiscovered(repos)
      if (repos.length === 0) {
        toast.info('No git repositories found in that directory')
      }
    } catch {
      toast.error('Failed to scan directory')
    } finally {
      setScanning(false)
    }
  }

  async function handleScanAdd() {
    try {
      const result = await api.scanDirectory(id!, scanPath)
      toast.success(`Added ${result.added} repositories`)
      setShowScanForm(false)
      setDiscovered([])
      setScanPath('')
      loadProject()
    } catch {
      toast.error('Failed to add repositories')
    }
  }

  async function handleCreateTask(e: React.FormEvent) {
    e.preventDefault()
    if (!taskTitle.trim()) return
    try {
      await api.createTask(id!, taskTitle, taskDesc)
      setTaskTitle('')
      setTaskDesc('')
      setShowTaskForm(false)
      toast.success('Task created')
      loadTasks()
    } catch {
      toast.error('Failed to create task')
    }
  }

  async function handleDeleteRepo(repoId: number) {
    try {
      await api.deleteRepo(repoId)
      toast.success('Repo removed')
      loadProject()
    } catch {
      toast.error('Failed to remove repo')
    }
  }

  async function handleDeleteTask(taskId: number) {
    try {
      await api.deleteTask(taskId)
      toast.success('Task deleted')
      loadTasks()
    } catch {
      toast.error('Failed to delete task')
    }
  }

  if (!project) return <p className="text-muted-foreground">Loading...</p>

  return (
    <div>
      {/* Breadcrumb */}
      <div className="flex items-center gap-1 text-sm text-muted-foreground mb-4">
        <Link to="/" className="hover:text-foreground transition-colors">Projects</Link>
        <ChevronRight className="h-3 w-3" />
        <span className="text-foreground truncate">{project.name}</span>
      </div>

      <h1 className="text-xl sm:text-2xl font-semibold tracking-tight">{project.name}</h1>
      {project.description && (
        <p className="text-muted-foreground mt-1 mb-6">{project.description}</p>
      )}

      {/* Repositories */}
      <section className="mb-8">
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 mb-3">
          <h2 className="text-sm font-semibold text-muted-foreground uppercase tracking-wider">
            Repositories
          </h2>
          <div className="flex flex-wrap gap-2">
            <Button variant="outline" size="xs" onClick={() => { setShowScanForm(!showScanForm); setShowRepoForm(false) }}>
              <FolderSearch className="h-3 w-3 mr-1" />
              {showScanForm ? 'Cancel' : 'Scan Directory'}
            </Button>
            <Button variant="outline" size="xs" onClick={() => { setShowRepoForm(!showRepoForm); setShowScanForm(false) }}>
              <Plus className="h-3 w-3 mr-1" />
              {showRepoForm ? 'Cancel' : 'Add Repo'}
            </Button>
          </div>
        </div>

        {/* Scan directory form */}
        {showScanForm && (
          <Card className="mb-3">
            <CardContent className="pt-4 space-y-3">
              <p className="text-sm text-muted-foreground">
                Point to a parent directory and we'll find all git repos inside it.
              </p>
              <div className="flex flex-col sm:flex-row gap-2">
                <div className="flex gap-2 flex-1">
                  <Input
                    placeholder="/path/to/your/projects"
                    value={scanPath}
                    onChange={e => setScanPath(e.target.value)}
                    autoFocus
                  />
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => handlePickDirectory('scan')}
                    title="Browse..."
                    className="shrink-0"
                  >
                    <FolderOpen className="h-4 w-4" />
                  </Button>
                </div>
                <Button onClick={handleScanDiscover} disabled={scanning || !scanPath.trim()} size="sm" className="shrink-0">
                  {scanning ? 'Scanning...' : 'Discover'}
                </Button>
              </div>
              {discovered.length > 0 && (
                <div className="space-y-2">
                  <p className="text-sm font-medium">Found {discovered.length} repos:</p>
                  <div className="max-h-48 overflow-auto space-y-1">
                    {discovered.map((r, i) => (
                      <div key={i} className="flex items-center gap-2 text-sm py-1.5 px-2 rounded bg-secondary">
                        <GitBranch className="h-3 w-3 text-muted-foreground shrink-0" />
                        <span className="font-medium shrink-0">{r.name}</span>
                        <span className="text-muted-foreground text-xs truncate hidden sm:inline">{r.path}</span>
                      </div>
                    ))}
                  </div>
                  <Button onClick={handleScanAdd} size="sm">
                    Add All {discovered.length} Repos
                  </Button>
                </div>
              )}
            </CardContent>
          </Card>
        )}

        {/* Manual add repo form */}
        {showRepoForm && (
          <Card className="mb-3">
            <CardContent className="pt-4">
              <form onSubmit={handleAddRepo} className="flex flex-col sm:flex-row gap-2">
                <div className="flex gap-2 flex-1">
                  <Input
                    placeholder="Absolute path to repository"
                    value={repoPath}
                    onChange={e => setRepoPath(e.target.value)}
                    autoFocus
                  />
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => handlePickDirectory('repo')}
                    title="Browse..."
                    className="shrink-0"
                  >
                    <FolderOpen className="h-4 w-4" />
                  </Button>
                </div>
                <Button type="submit" size="sm" className="shrink-0">Add</Button>
              </form>
            </CardContent>
          </Card>
        )}

        {repos.length === 0 ? (
          <Card>
            <CardContent className="py-8 text-center text-muted-foreground text-sm">
              No repositories added. Scan a directory or add repos manually.
            </CardContent>
          </Card>
        ) : (
          <div className="space-y-1">
            {repos.map(r => (
              <div
                key={r.id}
                className="flex items-center justify-between py-2 px-3 rounded-md border bg-card hover:bg-accent/30 transition-colors gap-2"
              >
                <div className="flex items-center gap-2 min-w-0">
                  <GitBranch className="h-4 w-4 text-muted-foreground shrink-0" />
                  <span className="font-medium text-sm shrink-0">{r.name}</span>
                  <span className="text-xs text-muted-foreground truncate hidden sm:inline">{r.path}</span>
                </div>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-7 w-7 text-muted-foreground hover:text-destructive shrink-0"
                  onClick={() => handleDeleteRepo(r.id)}
                >
                  <Trash2 className="h-3 w-3" />
                </Button>
              </div>
            ))}
          </div>
        )}
      </section>

      <Separator className="mb-8" />

      {/* Tasks */}
      <section>
        <div className="flex items-center justify-between mb-3">
          <h2 className="text-sm font-semibold text-muted-foreground uppercase tracking-wider">
            Tasks
          </h2>
          <Button size="xs" onClick={() => setShowTaskForm(!showTaskForm)}>
            <Plus className="h-3 w-3 mr-1" />
            {showTaskForm ? 'Cancel' : 'New Task'}
          </Button>
        </div>

        {showTaskForm && (
          <Card className="mb-3">
            <CardContent className="pt-4">
              <form onSubmit={handleCreateTask} className="space-y-3">
                <Input
                  placeholder="Task title"
                  value={taskTitle}
                  onChange={e => setTaskTitle(e.target.value)}
                  autoFocus
                />
                <Textarea
                  placeholder="Describe what you want to accomplish..."
                  value={taskDesc}
                  onChange={e => setTaskDesc(e.target.value)}
                  rows={3}
                />
                <Button type="submit" size="sm">Create Task</Button>
              </form>
            </CardContent>
          </Card>
        )}

        {tasks.length === 0 ? (
          <Card>
            <CardContent className="py-8 text-center text-muted-foreground text-sm">
              No tasks yet. Create a task to start working with the AI agent.
            </CardContent>
          </Card>
        ) : (
          <div className="space-y-1">
            {tasks.map(t => (
              <Card key={t.id} className="hover:border-primary/30 transition-colors">
                <CardContent className="flex items-center justify-between py-3 gap-2">
                  <Link to={`/tasks/${t.id}`} className="flex items-center gap-3 flex-1 min-w-0">
                    <Badge variant={statusVariant[t.status] || 'secondary'} className="shrink-0">
                      {t.status}
                    </Badge>
                    <span className="font-medium text-sm truncate">{t.title}</span>
                    {activeAgentTasks.has(t.id) && (
                      <Loader2 className="h-3 w-3 animate-spin text-primary shrink-0" />
                    )}
                  </Link>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-7 w-7 text-muted-foreground hover:text-destructive shrink-0"
                    onClick={() => handleDeleteTask(t.id)}
                  >
                    <Trash2 className="h-3 w-3" />
                  </Button>
                </CardContent>
              </Card>
            ))}
          </div>
        )}
      </section>
    </div>
  )
}
