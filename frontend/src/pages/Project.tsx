import { useState, useEffect } from 'react'
import { useParams, Link, useSearchParams } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Badge } from '@/components/ui/badge'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'
import { Plus, Trash2, FolderSearch, FolderOpen, GitBranch, Folder, ChevronRight, Loader2, Package, Info, Settings } from 'lucide-react'
import { toast } from 'sonner'
import type { Project, Repo, Task, DiscoveredRepo, FolderAnalysis } from '@/lib/api'
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
  const [searchParams, setSearchParams] = useSearchParams()
  const activeTab = searchParams.get('tab') || 'overview'
  const setActiveTab = (tab: string | number) => setSearchParams({ tab: String(tab) }, { replace: true })
  const [project, setProject] = useState<Project | null>(null)
  const [tasks, setTasks] = useState<Task[]>([])
  const [repos, setRepos] = useState<Repo[]>([])

  const [showTaskForm, setShowTaskForm] = useState(false)
  const [showRepoForm, setShowRepoForm] = useState(false)
  const [showScanForm, setShowScanForm] = useState(false)
  const [taskTitle, setTaskTitle] = useState('')
  const [taskDesc, setTaskDesc] = useState('')
  const [repoPath, setRepoPath] = useState('')
  const [scanPath, setScanPath] = useState('')
  const [discovered, setDiscovered] = useState<DiscoveredRepo[]>([])
  const [selectedDiscovered, setSelectedDiscovered] = useState<Set<number>>(new Set())
  const [scanning, setScanning] = useState(false)
  const [activeAgentTasks, setActiveAgentTasks] = useState<Set<number>>(new Set())
  const [folderAnalysis, setFolderAnalysis] = useState<FolderAnalysis | null>(null)
  const [selectedNested, setSelectedNested] = useState<Set<number>>(new Set())
  const [analyzing, setAnalyzing] = useState(false)

  useEffect(() => { if (id) { loadProject(); loadTasks(); loadActiveAgents() } }, [id])

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
      if (data.active) { data.active.forEach(a => activeIds.add(a.task_id)) }
      setActiveAgentTasks(activeIds)
    } catch {}
  }

  async function loadProject() {
    try { const data = await api.getProject(id!); setProject(data); setRepos(data.repos || []) }
    catch { toast.error('Failed to load project') }
  }

  async function loadTasks() {
    try { const data = await api.listTasks(id!); setTasks(data || []) }
    catch { toast.error('Failed to load tasks') }
  }

  async function handlePickDirectory(target: 'scan' | 'repo') {
    try {
      const result = await api.pickDirectory('Select Directory')
      if (result.cancelled || !result.path) return
      if (target === 'scan') setScanPath(result.path)
      else setRepoPath(result.path)
    } catch { toast.error('Failed to open directory picker') }
  }

  async function handleAnalyzeFolder(e: React.FormEvent) {
    e.preventDefault()
    if (!repoPath.trim()) return
    setAnalyzing(true); setFolderAnalysis(null)
    try {
      const analysis = await api.analyzeFolder(repoPath)
      setFolderAnalysis(analysis)
      const nestedCount = (analysis.submodules?.length || 0) + (analysis.nested_repos?.length || 0)
      setSelectedNested(new Set(Array.from({ length: nestedCount }, (_, i) => i)))
    } catch { toast.error('Failed to analyze folder') }
    finally { setAnalyzing(false) }
  }

  async function handleAddAnalyzed() {
    if (!folderAnalysis) return
    try {
      await api.addRepo(id!, folderAnalysis.path)
      toast.success('Folder added'); setRepoPath(''); setShowRepoForm(false); setFolderAnalysis(null); loadProject()
    } catch { toast.error('Failed to add folder') }
  }

  async function handleAddSelectedNested() {
    if (!folderAnalysis) return
    const items = [...(folderAnalysis.submodules || []), ...(folderAnalysis.nested_repos || [])]
    const selected = items.filter((_, i) => selectedNested.has(i))
    if (selected.length === 0) return
    let added = 0
    for (const item of selected) { try { await api.addRepo(id!, item.path); added++ } catch {} }
    toast.success(`Added ${added} folders`); setRepoPath(''); setShowRepoForm(false); setFolderAnalysis(null); setSelectedNested(new Set()); loadProject()
  }

  async function handleScanDiscover() {
    if (!scanPath.trim()) return
    setScanning(true)
    try {
      const repos = await api.discoverRepos(scanPath)
      setDiscovered(repos); setSelectedDiscovered(new Set(repos.map((_, i) => i)))
      if (repos.length === 0) toast.info('No git repositories found in that directory')
    } catch { toast.error('Failed to scan directory') }
    finally { setScanning(false) }
  }

  async function handleScanAdd() {
    const selected = discovered.filter((_, i) => selectedDiscovered.has(i))
    if (selected.length === 0) return
    let added = 0
    for (const repo of selected) { try { await api.addRepo(id!, repo.path); added++ } catch {} }
    toast.success(`Added ${added} repositories`); setShowScanForm(false); setDiscovered([]); setSelectedDiscovered(new Set()); setScanPath(''); loadProject()
  }

  async function handleCreateTask(e: React.FormEvent) {
    e.preventDefault()
    if (!taskTitle.trim()) return
    try {
      await api.createTask(id!, taskTitle, taskDesc)
      setTaskTitle(''); setTaskDesc(''); setShowTaskForm(false); toast.success('Task created'); loadTasks()
    } catch { toast.error('Failed to create task') }
  }

  async function handleDeleteRepo(repoId: number) {
    try { await api.deleteRepo(repoId); toast.success('Repo removed'); loadProject() }
    catch { toast.error('Failed to remove repo') }
  }

  async function handleDeleteTask(taskId: number) {
    try { await api.deleteTask(taskId); toast.success('Task deleted'); loadTasks() }
    catch { toast.error('Failed to delete task') }
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

      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-xl sm:text-2xl font-semibold tracking-tight">{project.name}</h1>
          {project.description && <p className="text-muted-foreground mt-1">{project.description}</p>}
        </div>
        <Link to={`/projects/${id}/settings`}>
          <Button variant="outline" size="xs">
            <Settings className="h-3 w-3 mr-1" />
            Settings
          </Button>
        </Link>
      </div>

      <Tabs value={activeTab} onValueChange={setActiveTab}>
        <TabsList variant="line">
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="codebase">Codebase</TabsTrigger>
          <TabsTrigger value="tasks">Tasks</TabsTrigger>
        </TabsList>

        {/* Overview Tab */}
        <TabsContent value="overview" className="pt-4">
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 mb-6">
            <Card>
              <CardContent className="pt-4 pb-3">
                <p className="text-2xl font-bold">{tasks.length}</p>
                <p className="text-xs text-muted-foreground">Total Tasks</p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="pt-4 pb-3">
                <p className="text-2xl font-bold">{tasks.filter(t => t.status === 'open').length}</p>
                <p className="text-xs text-muted-foreground">Open</p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="pt-4 pb-3">
                <p className="text-2xl font-bold">{tasks.filter(t => ['researching', 'clarifying', 'planning', 'executing'].includes(t.status)).length}</p>
                <p className="text-xs text-muted-foreground">In Progress</p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="pt-4 pb-3">
                <p className="text-2xl font-bold">{tasks.filter(t => t.status === 'done').length}</p>
                <p className="text-xs text-muted-foreground">Done</p>
              </CardContent>
            </Card>
          </div>

          <Card className="mb-4">
            <CardContent className="py-3">
              <p className="text-sm font-medium mb-1">Codebase</p>
              <p className="text-sm text-muted-foreground">{repos.length} {repos.length === 1 ? 'folder' : 'folders'} connected</p>
            </CardContent>
          </Card>

          {tasks.length > 0 && (
            <div>
              <h3 className="text-sm font-semibold text-muted-foreground uppercase tracking-wider mb-2">Recent Tasks</h3>
              <div className="space-y-1">
                {tasks.slice(0, 5).map(t => (
                  <Link key={t.id} to={`/tasks/${t.id}`} className="flex items-center gap-2 py-1.5 px-3 rounded-md hover:bg-accent/30 transition-colors">
                    <Badge variant={statusVariant[t.status] || 'secondary'} className="shrink-0">{t.status}</Badge>
                    <span className="text-sm truncate">{t.title}</span>
                    {activeAgentTasks.has(t.id) && <Loader2 className="h-3 w-3 animate-spin text-primary shrink-0" />}
                  </Link>
                ))}
              </div>
            </div>
          )}
        </TabsContent>

        {/* Codebase Tab */}
        <TabsContent value="codebase" className="pt-4">
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 mb-3">
          <div className="flex flex-wrap gap-2">
            <Button variant="outline" size="xs" onClick={() => { setShowScanForm(!showScanForm); setShowRepoForm(false); setFolderAnalysis(null) }}>
              <FolderSearch className="h-3 w-3 mr-1" />
              {showScanForm ? 'Cancel' : 'Scan Directory'}
            </Button>
            <Button variant="outline" size="xs" onClick={() => { setShowRepoForm(!showRepoForm); setShowScanForm(false); setFolderAnalysis(null) }}>
              <Plus className="h-3 w-3 mr-1" />
              {showRepoForm ? 'Cancel' : 'Add Folder'}
            </Button>
          </div>
        </div>

        {/* Scan directory form */}
        {showScanForm && (
          <Card className="mb-3">
            <CardContent className="pt-4 space-y-3">
              <p className="text-sm text-muted-foreground">Point to a parent directory and we'll find all git repos inside it.</p>
              <div className="flex flex-col sm:flex-row gap-2">
                <div className="flex gap-2 flex-1">
                  <Input placeholder="/path/to/your/projects" value={scanPath} onChange={e => setScanPath(e.target.value)} autoFocus />
                  <Button type="button" variant="outline" size="sm" onClick={() => handlePickDirectory('scan')} title="Browse..." className="shrink-0">
                    <FolderOpen className="h-4 w-4" />
                  </Button>
                </div>
                <Button onClick={handleScanDiscover} disabled={scanning || !scanPath.trim()} size="sm" className="shrink-0">
                  {scanning ? 'Scanning...' : 'Discover'}
                </Button>
              </div>
              {discovered.length > 0 && (
                <div className="space-y-2">
                  <div className="flex items-center justify-between">
                    <p className="text-sm font-medium">Found {discovered.length} repos:</p>
                    <button type="button" className="text-xs text-muted-foreground hover:text-foreground" onClick={() => setSelectedDiscovered(selectedDiscovered.size === discovered.length ? new Set() : new Set(discovered.map((_, i) => i)))}>
                      {selectedDiscovered.size === discovered.length ? 'Deselect all' : 'Select all'}
                    </button>
                  </div>
                  <div className="max-h-48 overflow-auto space-y-1">
                    {discovered.map((r, i) => (
                      <label key={i} className={`flex items-center gap-2 text-sm py-1.5 px-2 rounded cursor-pointer transition-colors ${selectedDiscovered.has(i) ? 'bg-secondary' : 'bg-secondary/40 opacity-60'}`}>
                        <input type="checkbox" className="accent-primary h-3.5 w-3.5 shrink-0" checked={selectedDiscovered.has(i)} onChange={() => { const next = new Set(selectedDiscovered); next.has(i) ? next.delete(i) : next.add(i); setSelectedDiscovered(next) }} />
                        <GitBranch className="h-3 w-3 text-muted-foreground shrink-0" />
                        <span className="font-medium shrink-0">{r.name}</span>
                        <span className="text-muted-foreground text-xs truncate hidden sm:inline">{r.path}</span>
                      </label>
                    ))}
                  </div>
                  <div className="flex gap-2">
                    <Button onClick={handleScanAdd} size="sm" disabled={selectedDiscovered.size === 0}>Add {selectedDiscovered.size} Selected</Button>
                    <Button variant="outline" size="sm" onClick={() => { setDiscovered([]); setSelectedDiscovered(new Set()); setScanPath('') }}>Cancel</Button>
                  </div>
                </div>
              )}
            </CardContent>
          </Card>
        )}

        {/* Add folder form with analysis */}
        {showRepoForm && (
          <Card className="mb-3">
            <CardContent className="pt-4 space-y-3">
              <form onSubmit={handleAnalyzeFolder} className="flex flex-col sm:flex-row gap-2">
                <div className="flex gap-2 flex-1">
                  <Input placeholder="Absolute path to any folder" value={repoPath} onChange={e => { setRepoPath(e.target.value); setFolderAnalysis(null) }} autoFocus />
                  <Button type="button" variant="outline" size="sm" onClick={() => handlePickDirectory('repo')} title="Browse..." className="shrink-0">
                    <FolderOpen className="h-4 w-4" />
                  </Button>
                </div>
                <Button type="submit" size="sm" className="shrink-0" disabled={analyzing || !repoPath.trim()}>
                  {analyzing ? <><Loader2 className="h-3 w-3 animate-spin mr-1" />Analyzing...</> : 'Analyze'}
                </Button>
              </form>

              {folderAnalysis && (
                <div className="space-y-3 border-t pt-3">
                  <div className="flex items-center gap-2">
                    <Info className="h-4 w-4 text-blue-500 shrink-0" />
                    <span className="text-sm font-medium">
                      {folderAnalysis.type === 'single-repo' && 'Single git repository'}
                      {folderAnalysis.type === 'monorepo' && `Monorepo with ${folderAnalysis.packages?.length || 0} packages`}
                      {folderAnalysis.type === 'multi-repo' && `Folder with ${(folderAnalysis.nested_repos?.length || 0) + (folderAnalysis.submodules?.length || 0)} repos`}
                      {folderAnalysis.type === 'plain-folder' && 'Plain folder (no git)'}
                    </span>
                    {folderAnalysis.is_git && <Badge variant="outline" className="text-xs"><GitBranch className="h-3 w-3 mr-1" />git</Badge>}
                  </div>

                  {(folderAnalysis.nested_repos?.length || folderAnalysis.submodules?.length) ? (() => {
                    const items = [...(folderAnalysis.submodules || []), ...(folderAnalysis.nested_repos || [])]
                    return (
                    <div className="space-y-2">
                      <div className="flex items-center justify-between">
                        <p className="text-xs text-muted-foreground">
                          {folderAnalysis.submodules?.length ? `${folderAnalysis.submodules.length} submodule(s)` : ''}
                          {folderAnalysis.submodules?.length && folderAnalysis.nested_repos?.length ? ' + ' : ''}
                          {folderAnalysis.nested_repos?.length ? `${folderAnalysis.nested_repos.length} nested repo(s)` : ''}
                        </p>
                        <button type="button" className="text-xs text-muted-foreground hover:text-foreground" onClick={() => setSelectedNested(selectedNested.size === items.length ? new Set() : new Set(items.map((_, i) => i)))}>
                          {selectedNested.size === items.length ? 'Deselect all' : 'Select all'}
                        </button>
                      </div>
                      <div className="max-h-36 overflow-auto space-y-1">
                        {items.map((r, i) => (
                          <label key={i} className={`flex items-center gap-2 text-sm py-1.5 px-2 rounded cursor-pointer transition-colors ${selectedNested.has(i) ? 'bg-secondary' : 'bg-secondary/40 opacity-60'}`}>
                            <input type="checkbox" className="accent-primary h-3.5 w-3.5 shrink-0" checked={selectedNested.has(i)} onChange={() => { const next = new Set(selectedNested); next.has(i) ? next.delete(i) : next.add(i); setSelectedNested(next) }} />
                            <GitBranch className="h-3 w-3 text-muted-foreground shrink-0" />
                            <span className="font-medium shrink-0">{r.name}</span>
                            <span className="text-muted-foreground text-xs truncate hidden sm:inline">{r.path}</span>
                          </label>
                        ))}
                      </div>
                      <div className="flex gap-2">
                        <Button size="sm" onClick={handleAddAnalyzed}>Add Root Folder</Button>
                        <Button size="sm" variant="outline" onClick={handleAddSelectedNested} disabled={selectedNested.size === 0}>Add {selectedNested.size} Selected</Button>
                      </div>
                    </div>
                    )
                  })() : null}

                  {folderAnalysis.packages?.length ? (
                    <div className="space-y-2">
                      <p className="text-xs text-muted-foreground">Detected workspace packages:</p>
                      <div className="max-h-36 overflow-auto space-y-1">
                        {folderAnalysis.packages.map((p, i) => (
                          <div key={i} className="flex items-center gap-2 text-sm py-1.5 px-2 rounded bg-secondary">
                            <Package className="h-3 w-3 text-muted-foreground shrink-0" />
                            <span className="font-medium shrink-0">{p.name}</span>
                            <Badge variant="outline" className="text-xs">{p.manager}</Badge>
                            <span className="text-muted-foreground text-xs truncate hidden sm:inline">{p.rel_path}</span>
                          </div>
                        ))}
                      </div>
                    </div>
                  ) : null}

                  {!folderAnalysis.nested_repos?.length && !folderAnalysis.submodules?.length && (
                    <Button size="sm" onClick={handleAddAnalyzed}>Add Folder</Button>
                  )}
                </div>
              )}
            </CardContent>
          </Card>
        )}

        {repos.length === 0 ? (
          <Card>
            <CardContent className="py-8 text-center text-muted-foreground text-sm">
              No folders added. Scan a directory or add folders manually.
            </CardContent>
          </Card>
        ) : (
          <div className="space-y-1">
            {repos.map(r => (
              <div key={r.id} className="flex items-center justify-between py-2 px-3 rounded-md border bg-card hover:bg-accent/30 transition-colors gap-2">
                <div className="flex items-center gap-2 min-w-0">
                  {r.type === 'git' ? <GitBranch className="h-4 w-4 text-muted-foreground shrink-0" /> : <Folder className="h-4 w-4 text-muted-foreground shrink-0" />}
                  <span className="font-medium text-sm shrink-0">{r.name}</span>
                  <Badge variant="outline" className="text-xs shrink-0">{r.type || 'git'}</Badge>
                  <span className="text-xs text-muted-foreground truncate hidden sm:inline">{r.path}</span>
                </div>
                <Button variant="ghost" size="icon" className="h-7 w-7 text-muted-foreground hover:text-destructive shrink-0" onClick={() => handleDeleteRepo(r.id)}>
                  <Trash2 className="h-3 w-3" />
                </Button>
              </div>
            ))}
          </div>
        )}
        </TabsContent>

        {/* Tasks Tab */}
        <TabsContent value="tasks" className="pt-4">
        <div className="flex items-center justify-between mb-3">
          <h2 className="text-sm font-semibold text-muted-foreground uppercase tracking-wider">Tasks</h2>
          <Button size="xs" onClick={() => setShowTaskForm(!showTaskForm)}>
            <Plus className="h-3 w-3 mr-1" />
            {showTaskForm ? 'Cancel' : 'New Task'}
          </Button>
        </div>

        {showTaskForm && (
          <Card className="mb-3">
            <CardContent className="pt-4">
              <form onSubmit={handleCreateTask} className="space-y-3">
                <Input placeholder="Task title" value={taskTitle} onChange={e => setTaskTitle(e.target.value)} autoFocus />
                <Textarea placeholder="Describe what you want to accomplish..." value={taskDesc} onChange={e => setTaskDesc(e.target.value)} rows={3} />
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
                    <Badge variant={statusVariant[t.status] || 'secondary'} className="shrink-0">{t.status}</Badge>
                    <span className="font-medium text-sm truncate">{t.title}</span>
                    {activeAgentTasks.has(t.id) && <Loader2 className="h-3 w-3 animate-spin text-primary shrink-0" />}
                  </Link>
                  <Button variant="ghost" size="icon" className="h-7 w-7 text-muted-foreground hover:text-destructive shrink-0" onClick={() => handleDeleteTask(t.id)}>
                    <Trash2 className="h-3 w-3" />
                  </Button>
                </CardContent>
              </Card>
            ))}
          </div>
        )}
        </TabsContent>
      </Tabs>
    </div>
  )
}
