import { useState, useEffect, useRef, useCallback } from 'react'
import { useParams, Link } from 'react-router-dom'
import ReactMarkdown from 'react-markdown'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { ScrollArea } from '@/components/ui/scroll-area'
import {
  ChevronRight, Play, Loader2, FileText, Calendar,
  MessageSquareQuote
} from 'lucide-react'
import { toast } from 'sonner'
import type { Task, AgentQuestion, TaskMessage } from '@/lib/api'
import * as api from '@/lib/api'
import { wsClient } from '@/lib/ws'
import type { AgentStep } from '@/components/AgentSteps'
import { ChatPanel } from '@/components/ChatPanel'

function formatDate(d: string) {
  return new Date(d).toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric', hour: '2-digit', minute: '2-digit' })
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
      if (run?.steps?.length && run.status === 'running') {
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
    try { await api.runExecute(id!) } catch { setActivePhase(null); toast.error('Failed to start') }
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
          <div className="flex-1 min-w-0 flex flex-col border-r">
            <div className="shrink-0 px-4 py-2 border-b bg-card flex items-center justify-between">
              <div className="flex items-center gap-2">
                <FileText className="h-3.5 w-3.5 text-muted-foreground" />
                <span className="text-xs font-medium">Plan</span>
                {activePhase === 'plan' && <Loader2 className="h-3 w-3 animate-spin text-primary" />}
              </div>
              {hasPlan && (
                <Button size="xs" variant="success" onClick={handleExecute} disabled={!!activePhase} className="gap-1">
                  <Play className="h-3 w-3" /> Execute
                </Button>
              )}
            </div>
            {hasPlan ? (
              <div ref={planRef} className="relative flex-1 min-h-0">
                <ScrollArea className="h-full">
                  <div className="p-4 prose prose-sm max-w-none dark:prose-invert" onMouseUp={handlePlanMouseUp}>
                    <ReactMarkdown>{task.implementation_plan}</ReactMarkdown>
                  </div>
                </ScrollArea>
                {floatingBtn && (
                  <Button size="xs" className="absolute z-50 shadow-lg gap-1"
                    style={{ top: floatingBtn.top, left: Math.max(4, floatingBtn.left) }}
                    onClick={handleQuoteSelection}
                  >
                    <MessageSquareQuote className="h-3 w-3" /> Ask about this
                  </Button>
                )}
              </div>
            ) : (
              <div className="flex-1 flex flex-col items-center justify-center text-muted-foreground">
                <Loader2 className="h-8 w-8 mx-auto mb-3 animate-spin text-primary/40" />
                <p className="text-sm">Agent is creating a plan...</p>
                <p className="text-xs mt-1 text-muted-foreground/60">Live progress in chat</p>
              </div>
            )}
          </div>
        )}

        {/* Chat panel (right, or full width if no plan) */}
        <div className={`${showPlanPanel ? 'w-[375px] lg:w-[425px] shrink-0' : 'flex-1'} flex flex-col bg-background`}>
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
            extraActions={
              hasPlan ? (
                <button onClick={handleExecute} className="hover:text-foreground transition-colors flex items-center gap-0.5" disabled={!!activePhase}>
                  <Play className="h-2.5 w-2.5" /> Execute
                </button>
              ) : undefined
            }
          />
        </div>
      </div>
    </div>
  )
}
