import { useState, useEffect, useRef, useCallback } from 'react'
import { useParams, Link } from 'react-router-dom'
import ReactMarkdown from 'react-markdown'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { ScrollArea } from '@/components/ui/scroll-area'
import {
  ChevronRight, Play, Loader2, FileText, Calendar,
  MessageSquareQuote, CheckCircle2, Circle, ArrowRight, XCircle, ChevronDown, ChevronUp, Clock
} from 'lucide-react'
import { toast } from 'sonner'
import type { Task, AgentQuestion, TaskMessage, PlanTodo, PendingChange, TimelineEvent } from '@/lib/api'
import * as api from '@/lib/api'
import { wsClient } from '@/lib/ws'
import type { AgentStep } from '@/components/AgentSteps'
import { ChatPanel } from '@/components/ChatPanel'
import DiffViewer from '@/components/DiffViewer'
import Timeline from '@/components/Timeline'

function formatDate(d: string) {
  return new Date(d).toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric', hour: '2-digit', minute: '2-digit' })
}

const todoStatusIcon: Record<string, typeof Circle> = {
  pending: Circle,
  in_progress: ArrowRight,
  done: CheckCircle2,
  failed: XCircle,
}

const todoStatusColor: Record<string, string> = {
  pending: 'text-muted-foreground/50',
  in_progress: 'text-blue-400',
  done: 'text-emerald-400',
  failed: 'text-red-400',
}

function TodoList({ todos, focused }: { todos: PlanTodo[]; focused?: boolean }) {
  const [expandedId, setExpandedId] = useState<number | null>(null)
  const currentTodo = todos.find(t => t.status === 'in_progress')

  // Auto-expand current TODO during execution
  useEffect(() => {
    if (currentTodo && focused) setExpandedId(currentTodo.id)
  }, [currentTodo?.id, focused])

  // Single unified list — parent controls the header/collapsing
  return (
    <div className="divide-y">
      {/* Status summary when focused */}
      {focused && currentTodo && (
        <div className="px-4 py-2 bg-blue-500/5 text-xs text-blue-400 flex items-center gap-1">
          <ArrowRight className="h-3 w-3" /> Running: {currentTodo.title}
        </div>
      )}
      {todos.map(todo => {
        const Icon = todoStatusIcon[todo.status] || Circle
        const color = todoStatusColor[todo.status] || 'text-muted-foreground'
        const isExpanded = expandedId === todo.id
        const isCurrent = todo.status === 'in_progress'

        return (
          <div key={todo.id} className={isCurrent ? 'bg-blue-500/5' : ''}>
            <button
              className={`w-full text-left flex items-center gap-2 hover:bg-accent/30 transition-colors ${focused ? 'px-4 py-2.5' : 'px-3 py-1.5 text-xs'}`}
              onClick={() => setExpandedId(isExpanded ? null : todo.id)}
            >
              <Icon className={`shrink-0 ${color} ${isCurrent ? 'animate-pulse' : ''} ${focused ? 'h-4 w-4' : 'h-3 w-3'}`} />
              <span className={`text-muted-foreground/40 shrink-0 font-mono ${focused ? 'text-xs w-4' : 'text-[10px] w-3'}`}>{todo.order_index}</span>
              <span className={`flex-1 min-w-0 ${focused ? 'text-sm' : 'text-xs truncate'} ${todo.status === 'done' ? 'line-through text-muted-foreground' : isCurrent && focused ? 'font-medium' : ''}`}>
                {todo.title}
              </span>
              {todo.depends_on.length > 0 && todo.status === 'pending' && (
                <span className="text-[10px] text-muted-foreground/40 shrink-0">
                  waits {todo.depends_on.join(', ')}
                </span>
              )}
              {isExpanded ? <ChevronUp className="h-3 w-3 shrink-0 text-muted-foreground" /> : <ChevronDown className="h-3 w-3 shrink-0 text-muted-foreground" />}
            </button>
            {isExpanded && (
              <div className={`pb-2 space-y-1.5 ${focused ? 'px-4 ml-6' : 'px-3 ml-5'}`}>
                {todo.description && (
                  <p className="text-xs text-muted-foreground whitespace-pre-wrap leading-relaxed">{todo.description}</p>
                )}
                {todo.result && (
                  <div className="border rounded p-2 bg-secondary/30">
                    <p className="text-[10px] font-semibold text-muted-foreground mb-1 uppercase tracking-wider">
                      {todo.status === 'done' ? 'Result' : todo.status === 'failed' ? 'Error' : 'Output'}
                    </p>
                    <pre className="text-xs whitespace-pre-wrap max-h-48 overflow-auto">{todo.result}</pre>
                  </div>
                )}
              </div>
            )}
          </div>
        )
      })}
    </div>
  )
}

export default function TaskPage() {
  const { id } = useParams<{ id: string }>()
  const [task, setTask] = useState<Task | null>(null)

  // Unified chat state
  const [messages, setMessages] = useState<TaskMessage[]>([])
  const [input, setInput] = useState('')
  const [activePhase, setActivePhase] = useState<string | null>(null)
  const [agentSteps, setAgentSteps] = useState<AgentStep[]>([])
  const [currentChunk, setCurrentChunk] = useState('')
  const [elapsedMs, setElapsedMs] = useState(0)
  const startTimeRef = useRef<number | null>(null)

  // Shared
  const [pendingQuestions, setPendingQuestions] = useState<AgentQuestion[]>([])
  const [questionAnswers, setQuestionAnswers] = useState<Record<number, string>>({})
  const [floatingBtn, setFloatingBtn] = useState<{ top: number; left: number; text: string } | null>(null)
  const [planOpen, setPlanOpen] = useState(true)
  const [todosOpen, setTodosOpen] = useState(false)
  const [pendingChanges, setPendingChanges] = useState<PendingChange[]>([])
  const [timeline, setTimeline] = useState<TimelineEvent[]>([])
  const [showTimeline, setShowTimeline] = useState(false)
  const planRef = useRef<HTMLDivElement>(null)

  // Data loading
  const loadTask = useCallback(async () => {
    try { setTask(await api.getTask(id!)) } catch { toast.error('Failed to load task') }
  }, [id])
  const loadMessages = useCallback(async () => {
    try { setMessages(await api.listTaskMessages(id!)) } catch {}
  }, [id])

  // On mount
  useEffect(() => {
    if (!id) return
    loadTask(); loadMessages()
    api.getTimeline(id).then(setTimeline).catch(() => {})
    api.getPendingQuestions(id).then(qs => {
      if (qs && qs.length > 0) setPendingQuestions(qs)
    }).catch(() => {})
    api.getAgentStatus(id).then(status => {
      if (status.active) {
        setActivePhase(status.phase || null)
        startTimeRef.current = Date.now()
      }
    }).catch(() => {})
    api.getLatestRun(id).then(run => {
      if (run?.steps?.length) {
        setAgentSteps(run.steps.map(s => ({
          step: s.step_number, think: s.think, action: s.action,
          action_arg: s.action_arg, observation: s.observation,
          is_final: s.is_final, phase: run.phase,
          started_at: s.started_at, completed_at: s.completed_at,
          duration_ms: s.duration_ms,
        })))
      }
    }).catch(() => {})
  }, [id, loadTask, loadMessages])

  // WebSocket
  useEffect(() => {
    const taskId = parseInt(id!)
    const unsubs: Array<() => void> = []

    unsubs.push(wsClient.on('agent_output', (msg) => {
      if (msg.task_id === taskId) setCurrentChunk(prev => prev + ((msg.payload?.chunk as string) || ''))
    }))
    unsubs.push(wsClient.on('agent_step', (msg) => {
      if (msg.task_id === taskId) {
        setAgentSteps(prev => [...prev, msg.payload as unknown as AgentStep])
        setCurrentChunk('')
      }
    }))
    unsubs.push(wsClient.on('agent_waiting', (msg) => {
      if (msg.task_id === taskId) {
        setActivePhase(null); setCurrentChunk(''); startTimeRef.current = null
        api.getPendingQuestions(id!).then(qs => setPendingQuestions(qs || [])).catch(() => {})
      }
    }))
    unsubs.push(wsClient.on('agent_done', (msg) => {
      if (msg.task_id === taskId) {
        setActivePhase(null); setCurrentChunk(''); startTimeRef.current = null
        setPendingQuestions([])
        loadTask(); loadMessages()
        api.getTimeline(id!).then(setTimeline).catch(() => {})
        // Reload steps from the completed run so they persist in UI
        const runId = (msg.payload as Record<string, unknown>)?.run_id as number
        if (runId) {
          api.getAgentRun(runId).then(run => {
            if (run?.steps?.length) {
              setAgentSteps(run.steps.map(s => ({
                step: s.step_number, think: s.think, action: s.action,
                action_arg: s.action_arg, observation: s.observation,
                is_final: s.is_final, phase: run.phase,
                started_at: s.started_at, completed_at: s.completed_at,
                duration_ms: s.duration_ms,
              })))
            }
          }).catch(() => {})
        }
      }
    }))
    unsubs.push(wsClient.on('agent_error', (msg) => {
      if (msg.task_id === taskId) {
        setActivePhase(null); setCurrentChunk(''); startTimeRef.current = null
        toast.error(`Agent error: ${(msg.payload?.error as string) || 'Unknown'}`)
      }
    }))
    unsubs.push(wsClient.on('task_update', (msg) => { if (msg.task_id === taskId) loadTask() }))
    // TODO updates
    unsubs.push(wsClient.on('todo_update', (msg) => { if (msg.task_id === taskId) loadTask() }))

    // Change approval events
    unsubs.push(wsClient.on('change_proposed', (msg) => {
      if (msg.task_id === taskId) {
        const p = msg.payload as Record<string, unknown>
        setPendingChanges(prev => [...prev, {
          id: p.change_id as string,
          run_id: 0, todo_id: p.todo_id as number, task_id: taskId,
          tool: p.tool as string, file_path: p.file_path as string,
          rel_path: p.file_path as string,
          old_content: '', new_content: '',
          diff: p.diff as string, status: 'pending',
          reject_reason: '', created_at: new Date().toISOString(),
        }])
        setActivePhase(null) // agent is paused
      }
    }))
    unsubs.push(wsClient.on('change_accepted', (msg) => {
      if (msg.task_id === taskId) {
        const cid = (msg.payload as Record<string, unknown>).change_id as string
        setPendingChanges(prev => prev.map(c => c.id === cid ? { ...c, status: 'accepted' } : c))
      }
    }))
    unsubs.push(wsClient.on('change_rejected', (msg) => {
      if (msg.task_id === taskId) {
        const cid = (msg.payload as Record<string, unknown>).change_id as string
        setPendingChanges(prev => prev.map(c => c.id === cid ? { ...c, status: 'rejected' } : c))
      }
    }))

    return () => unsubs.forEach(fn => fn())
  }, [id, loadTask, loadMessages])

  // Elapsed timer
  useEffect(() => {
    if (!activePhase || !startTimeRef.current) { setElapsedMs(0); return }
    setElapsedMs(Date.now() - startTimeRef.current)
    const interval = window.setInterval(() => {
      if (!startTimeRef.current) return
      setElapsedMs(Date.now() - startTimeRef.current)
    }, 1000)
    return () => window.clearInterval(interval)
  }, [activePhase])

  // --- Handlers ---
  async function handleSend() {
    const text = input.trim()
    if (!text || activePhase) return

    const normalized = text.toLowerCase()

    // /plan command — triggers planning via the same chat
    if (normalized === '/plan' || normalized.startsWith('/plan ')) {
      const details = text.slice(5).trim()
      setInput(''); setAgentSteps([]); setCurrentChunk('')
      setActivePhase('plan'); startTimeRef.current = Date.now()
      // Optimistic user message
      setMessages(prev => [...prev, {
        id: Date.now(), task_id: parseInt(id!), role: 'user', content: text,
        created_at: new Date().toISOString(),
      }])
      try {
        if (!details) await api.runPlan(id!)
        else await api.planChat(id!, details)
      } catch { setActivePhase(null); toast.error('Failed to start planning') }
      return
    }

    // /execute command
    if (normalized === '/execute' || normalized === '/run') { handleExecute(); setInput(''); return }

    // /clear command
    if (normalized === '/clear') { handleClear(); setInput(''); return }

    // Normal chat message
    setInput(''); setAgentSteps([]); setCurrentChunk('')
    setActivePhase('chat'); startTimeRef.current = Date.now()
    // Optimistic user message
    setMessages(prev => [...prev, {
      id: Date.now(), task_id: parseInt(id!), role: 'user', content: text,
      created_at: new Date().toISOString(),
    }])
    try { await api.taskChat(id!, text) } catch { setActivePhase(null); toast.error('Failed to send') }
  }

  async function handleExecute() {
    if (activePhase) return
    setAgentSteps([]); setCurrentChunk('')
    setActivePhase('execute'); startTimeRef.current = Date.now()
    // Collapse plan, open TODOs
    setPlanOpen(false)
    setTodosOpen(true)
    try { await api.runExecute(id!) } catch { setActivePhase(null); toast.error('Failed to start') }
  }

  async function handleAcceptChange(changeId: string) {
    try {
      setActivePhase('execute'); startTimeRef.current = Date.now()
      await api.acceptChange(changeId)
    } catch { toast.error('Failed to accept change') }
  }

  async function handleRejectChange(changeId: string, reason: string) {
    try {
      setActivePhase('execute'); startTimeRef.current = Date.now()
      await api.rejectChange(changeId, reason)
    } catch { toast.error('Failed to reject change') }
  }

  async function handleSubmitAnswers() {
    const answers = pendingQuestions
      .filter(q => questionAnswers[q.id]?.trim())
      .map(q => ({ question_id: q.id, answer: questionAnswers[q.id].trim() }))
    if (answers.length === 0) { toast.error('Please answer at least one question'); return }
    try {
      setPendingQuestions([])
      setQuestionAnswers({})
      setAgentSteps([]); setCurrentChunk('')
      setActivePhase('plan'); startTimeRef.current = Date.now()
      await api.submitPlanAnswers(id!, answers)
    } catch {
      setActivePhase(null)
      toast.error('Failed to submit answers')
    }
  }

  async function handleClear() {
    try {
      await api.clearTaskMessages(id!)
      setMessages([]); setAgentSteps([])
      toast.info('Chat cleared')
    } catch { toast.error('Failed to clear') }
  }

  async function handleCancel() {
    try {
      await api.cancelAgent(id!)
      setActivePhase(null); setCurrentChunk(''); startTimeRef.current = null
      toast.info('Agent cancelled'); loadMessages()
    } catch { toast.error('Failed to cancel') }
  }

  function handlePlanMouseUp() {
    const sel = window.getSelection()
    const text = sel?.toString().trim()
    if (!text || text.length < 5) { setFloatingBtn(null); return }
    const range = sel!.getRangeAt(0)
    const rect = range.getBoundingClientRect()
    const containerRect = planRef.current?.getBoundingClientRect()
    if (!containerRect) return
    setFloatingBtn({
      top: rect.top - containerRect.top - 36,
      left: Math.max(0, rect.left - containerRect.left + rect.width / 2 - 60),
      text,
    })
  }

  function handleQuoteSelection() {
    if (!floatingBtn) return
    const quoted = floatingBtn.text.split('\n').map(l => `> ${l}`).join('\n')
    setInput(prev => prev ? `${prev}\n\n${quoted}\n\n` : `${quoted}\n\n`)
    setFloatingBtn(null)
  }

  if (!task) return <p className="text-muted-foreground">Loading...</p>

  const hasPlan = !!task.implementation_plan
  const showPlanPanel = hasPlan || activePhase === 'plan'

  return (
    <div className="flex flex-col h-[calc(100vh-64px)]">
      {/* Breadcrumb */}
      <div className="flex items-center gap-1 text-sm text-muted-foreground mb-2 overflow-hidden shrink-0">
        <Link to="/" className="hover:text-foreground transition-colors shrink-0">Projects</Link>
        <ChevronRight className="h-3 w-3 shrink-0" />
        <Link to={`/projects/${task.project_id}`} className="hover:text-foreground transition-colors shrink-0">Project</Link>
        <ChevronRight className="h-3 w-3 shrink-0" />
        <span className="text-foreground truncate">{task.title}</span>
      </div>

      {/* Task info bar */}
      <div className="shrink-0 mb-3 flex items-center gap-3 flex-wrap">
        <Badge variant="outline" className="shrink-0">{task.status}</Badge>
        <h1 className="text-lg font-semibold tracking-tight">{task.title}</h1>
        {task.description && <span className="text-muted-foreground text-sm hidden sm:inline">— {task.description}</span>}
        <div className="ml-auto flex items-center gap-2 text-xs text-muted-foreground">
          <span className="flex items-center gap-1"><Calendar className="h-3 w-3" /> {formatDate(task.created_at)}</span>
          {hasPlan && (
            <Button size="xs" variant="success" onClick={handleExecute} disabled={!!activePhase} className="gap-1">
              <Play className="h-3 w-3" /> Execute
            </Button>
          )}
        </div>
      </div>

      {/* Main area: optional plan panel + chat */}
      <div className="flex-1 min-h-0 flex gap-0 border rounded-lg overflow-hidden">
        {/* Plan panel (left) — shown when plan exists or planning is active */}
        {showPlanPanel && (
          <div className="flex-1 min-w-0 flex flex-col border-r overflow-hidden">
            {(() => {
              const hasTodos = task.todos && task.todos.length > 0
              const isExecPhase = activePhase === 'execute-todo' || activePhase === 'execute'

              // Not planned yet — show loading
              if (!hasPlan && activePhase === 'plan') {
                return (
                  <div className="flex-1 flex flex-col items-center justify-center text-muted-foreground">
                    <Loader2 className="h-8 w-8 mx-auto mb-3 animate-spin text-primary/40" />
                    <p className="text-sm">Agent is creating a plan...</p>
                    <p className="text-xs mt-1 text-muted-foreground/60">Live progress in chat</p>
                  </div>
                )
              }

              return (
                <ScrollArea className="flex-1 min-h-0">
                  {/* ── Activity Section (collapsible) ── */}
                  {timeline.length > 0 && (
                    <div className="border-b">
                      <button
                        className="w-full px-4 py-2 flex items-center justify-between bg-card hover:bg-accent/30 transition-colors"
                        onClick={() => setShowTimeline(!showTimeline)}
                      >
                        <div className="flex items-center gap-2">
                          {showTimeline ? <ChevronDown className="h-3 w-3 text-muted-foreground" /> : <ChevronRight className="h-3 w-3 text-muted-foreground" />}
                          <Clock className="h-3.5 w-3.5 text-muted-foreground" />
                          <span className="text-xs font-medium">Activity</span>
                          <span className="text-[10px] text-muted-foreground">{timeline.length} events</span>
                        </div>
                      </button>
                      {showTimeline && (
                        <div className="px-4 py-3">
                          <Timeline events={timeline} />
                        </div>
                      )}
                    </div>
                  )}

                  {/* ── Plan Section (collapsible) ── */}
                  {hasPlan && (
                    <div className="border-b">
                      <button
                        className="w-full px-4 py-2 flex items-center justify-between bg-card hover:bg-accent/30 transition-colors"
                        onClick={() => setPlanOpen(!planOpen)}
                      >
                        <div className="flex items-center gap-2">
                          {planOpen ? <ChevronDown className="h-3 w-3 text-muted-foreground" /> : <ChevronRight className="h-3 w-3 text-muted-foreground" />}
                          <FileText className="h-3.5 w-3.5 text-muted-foreground" />
                          <span className="text-xs font-medium">Plan</span>
                          {activePhase === 'plan' && <Loader2 className="h-3 w-3 animate-spin text-primary" />}
                        </div>
                        {!activePhase && (
                          <Button size="xs" variant="success" onClick={(e: React.MouseEvent) => { e.stopPropagation(); handleExecute() }} className="gap-1">
                            <Play className="h-3 w-3" /> Execute
                          </Button>
                        )}
                      </button>
                      {planOpen && (
                        <div ref={planRef} className="relative">
                          <div className="p-4 prose prose-sm max-w-none dark:prose-invert" onMouseUp={handlePlanMouseUp}>
                            <ReactMarkdown>{task.implementation_plan}</ReactMarkdown>
                          </div>
                          {floatingBtn && (
                            <Button size="xs" className="absolute z-50 shadow-lg gap-1"
                              style={{ top: floatingBtn.top, left: Math.max(4, floatingBtn.left) }}
                              onClick={handleQuoteSelection}
                            >
                              <MessageSquareQuote className="h-3 w-3" /> Ask about this
                            </Button>
                          )}
                        </div>
                      )}
                    </div>
                  )}

                  {/* ── TODOs Section (collapsible) ── */}
                  {hasTodos && (
                    <div>
                      <button
                        className="w-full px-4 py-2 flex items-center justify-between bg-card hover:bg-accent/30 transition-colors border-b"
                        onClick={() => setTodosOpen(!todosOpen)}
                      >
                        <div className="flex items-center gap-2">
                          {todosOpen ? <ChevronDown className="h-3 w-3 text-muted-foreground" /> : <ChevronRight className="h-3 w-3 text-muted-foreground" />}
                          <CheckCircle2 className="h-3.5 w-3.5 text-muted-foreground" />
                          <span className="text-xs font-medium">Steps</span>
                          <span className="text-[10px] text-muted-foreground">
                            {task.todos!.filter(t => t.status === 'done').length}/{task.todos!.length}
                          </span>
                          {isExecPhase && <Loader2 className="h-3 w-3 animate-spin text-primary" />}
                          {task.todos!.every(t => t.status === 'done') && (
                            <span className="text-[10px] text-emerald-400 font-medium">All done</span>
                          )}
                        </div>
                        {/* Mini progress bar in header */}
                        <div className="w-20 h-1.5 bg-secondary rounded-full overflow-hidden">
                          <div
                            className="h-full bg-emerald-400 rounded-full transition-all duration-500"
                            style={{ width: `${task.todos!.length > 0 ? Math.round((task.todos!.filter(t => t.status === 'done').length / task.todos!.length) * 100) : 0}%` }}
                          />
                        </div>
                      </button>
                      {todosOpen && (
                        <TodoList todos={task.todos!} focused={isExecPhase || task.todos!.some(t => t.status !== 'pending')} />
                      )}
                    </div>
                  )}
                </ScrollArea>
              )
            })()}
          </div>
        )}

        {/* Chat panel (right, or full width if no plan) */}
        <div className={`${showPlanPanel ? 'w-[375px] lg:w-[425px] shrink-0' : 'flex-1'} flex flex-col bg-background overflow-hidden`}>
          {showPlanPanel && (
            <div className="shrink-0 px-3 py-1.5 border-b bg-card">
              <span className="text-xs font-medium text-muted-foreground">Chat</span>
            </div>
          )}
          <ChatPanel
            messages={messages}
            agentSteps={agentSteps}
            currentChunk={currentChunk}
            activePhase={activePhase}
            elapsedMs={elapsedMs}
            pendingQuestions={pendingQuestions}
            questionAnswers={questionAnswers}
            onAnswerChange={(qId, answer) => setQuestionAnswers(prev => ({ ...prev, [qId]: answer }))}
            onSubmitAnswers={handleSubmitAnswers}
            input={input}
            onInputChange={setInput}
            onSend={handleSend}
            onCancel={handleCancel}
            onClear={messages.length > 0 ? handleClear : undefined}
            placeholder={messages.length === 0
              ? 'Ask a question, or /plan to create a plan... (@ to reference files)'
              : 'Continue chatting, /plan, /execute... (@ to reference files)'}
            projectId={task.project_id}
            emptyMessage="Ask questions about your codebase, discuss approaches, or type /plan to create an implementation plan."
            renderAfterSteps={pendingChanges.length > 0 ? () => (
              <div className="px-3 space-y-2">
                {pendingChanges.map(change => (
                  <DiffViewer
                    key={change.id}
                    changeId={change.id}
                    filePath={change.rel_path || change.file_path}
                    diff={change.diff}
                    status={change.status as 'pending' | 'accepted' | 'rejected'}
                    onAccept={handleAcceptChange}
                    onReject={handleRejectChange}
                  />
                ))}
              </div>
            ) : undefined}
          />
        </div>
      </div>
    </div>
  )
}
