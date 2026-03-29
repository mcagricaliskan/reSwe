import { useState, useEffect, useRef, useCallback } from 'react'
import { useParams, Link } from 'react-router-dom'
import ReactMarkdown from 'react-markdown'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Separator } from '@/components/ui/separator'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Textarea } from '@/components/ui/textarea'
import {
  ChevronRight, Send, Play, Loader2, XCircle, Trash2, History,
  FileText, MessageSquareQuote, Calendar, Clock, ArrowLeft,
  MessageSquare, Zap, CheckCircle2
} from 'lucide-react'
import { toast } from 'sonner'
import type { Task, PlanMessage, ChatSession, AgentQuestion } from '@/lib/api'
import * as api from '@/lib/api'
import { wsClient } from '@/lib/ws'
import type { AgentStep } from '@/components/AgentSteps'
import { useFileMention } from '@/lib/useFileMention'
import { FileMention } from '@/components/FileMention'

function formatDate(d: string) {
  return new Date(d).toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric', hour: '2-digit', minute: '2-digit' })
}
function formatTime(d: string) {
  return new Date(d).toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' })
}

type ViewMode = 'overview' | 'planning'

export default function TaskPage() {
  const { id } = useParams<{ id: string }>()
  const [task, setTask] = useState<Task | null>(null)
  const [messages, setMessages] = useState<PlanMessage[]>([])
  const [input, setInput] = useState('')
  const [activePhase, setActivePhase] = useState<string | null>(null)
  const [agentSteps, setAgentSteps] = useState<AgentStep[]>([])
  const [currentChunk, setCurrentChunk] = useState('')
  const agentStartTimeRef = useRef<number | null>(null)
  const setAgentStartTime = (v: number | null) => { agentStartTimeRef.current = v }
  const [showSteps, setShowSteps] = useState(false)
  const [sessions, setSessions] = useState<ChatSession[]>([])
  const [showSessions, setShowSessions] = useState(false)
  const [viewMode, setViewMode] = useState<ViewMode>('overview')
  const [pendingQuestions, setPendingQuestions] = useState<AgentQuestion[]>([])
  const [questionAnswers, setQuestionAnswers] = useState<Record<number, string>>({})
  const [floatingBtn, setFloatingBtn] = useState<{ top: number; left: number; text: string } | null>(null)
  const chatEndRef = useRef<HTMLDivElement>(null)
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const planRef = useRef<HTMLDivElement>(null)
  const mention = useFileMention(task?.project_id)

  const loadTask = useCallback(async () => {
    try { setTask(await api.getTask(id!)) } catch { toast.error('Failed to load task') }
  }, [id])
  const loadMessages = useCallback(async () => {
    try { setMessages(await api.listPlanMessages(id!)) } catch {}
  }, [id])
  const loadSessions = useCallback(async () => {
    try { setSessions(await api.listSessions(id!)) } catch {}
  }, [id])

  async function handleRestoreSession(sessionId: number) {
    try {
      await api.restoreSession(sessionId)
      setShowSessions(false)
      loadMessages(); loadSessions(); loadTask()
      toast.success('Session restored')
    } catch { toast.error('Failed to restore session') }
  }

  // On mount
  useEffect(() => {
    if (!id) return
    loadTask(); loadMessages(); loadSessions()
    // Load pending questions (for paused state recovery)
    api.getPendingQuestions(id).then(qs => {
      if (qs && qs.length > 0) {
        setPendingQuestions(qs)
        setViewMode('planning')
      }
    }).catch(() => {})
    api.getAgentStatus(id).then(status => {
      if (status.active) {
        setActivePhase(status.phase || null)
        setAgentStartTime(Date.now())
        setViewMode('planning') // if agent running, go straight to planning view
      }
    }).catch(() => {})
    api.getLatestRun(id).then(run => {
      if (run?.steps?.length && run.status === 'running') {
        setAgentSteps(run.steps.map(s => ({
          step: s.step_number, think: s.think, action: s.action,
          action_arg: s.action_arg, observation: s.observation,
          is_final: s.is_final, phase: run.phase,
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
        setActivePhase(null); setCurrentChunk(''); setAgentStartTime(null)
        // Load the pending questions from DB
        api.getPendingQuestions(id!).then(qs => setPendingQuestions(qs || [])).catch(() => {})
      }
    }))
    unsubs.push(wsClient.on('agent_done', (msg) => {
      if (msg.task_id === taskId) {
        setActivePhase(null); setCurrentChunk(''); setAgentStartTime(null)
        setPendingQuestions([])
        loadTask(); loadMessages(); loadSessions()
      }
    }))
    unsubs.push(wsClient.on('agent_error', (msg) => {
      if (msg.task_id === taskId) {
        setActivePhase(null); setCurrentChunk(''); setAgentStartTime(null)
        toast.error(`Agent error: ${(msg.payload?.error as string) || 'Unknown'}`)
      }
    }))
    unsubs.push(wsClient.on('task_update', (msg) => { if (msg.task_id === taskId) loadTask() }))
    return () => unsubs.forEach(fn => fn())
  }, [id, loadTask, loadMessages])

  useEffect(() => {
    chatEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages, agentSteps.length, activePhase])

  async function handleSend() {
    const text = input.trim()
    if (!text || activePhase) return
    if (text.startsWith('/')) {
      const cmd = text.toLowerCase()
      if (cmd === '/execute' || cmd === '/run') { handleExecute(); setInput(''); return }
      if (cmd === '/clear') { handleClear(); setInput(''); return }
    }
    setInput(''); setAgentSteps([]); setCurrentChunk('')
    setActivePhase('plan'); setAgentStartTime(Date.now()); setShowSteps(true)
    try {
      if (messages.length === 0) await api.runPlan(id!)
      else await api.planChat(id!, text)
    } catch { setActivePhase(null); toast.error('Failed to send') }
  }

  function startPlanning(autoRun = false) {
    setViewMode('planning')
    if (autoRun && messages.length === 0) {
      // Kick off the agent immediately
      setAgentSteps([]); setCurrentChunk('')
      setActivePhase('plan'); setAgentStartTime(Date.now()); setShowSteps(true)
      api.runPlan(id!).catch(() => { setActivePhase(null); toast.error('Failed to start') })
    } else {
      setTimeout(() => textareaRef.current?.focus(), 100)
    }
  }

  async function handleExecute() {
    setViewMode('planning')
    setAgentSteps([]); setCurrentChunk('')
    setActivePhase('execute'); setAgentStartTime(Date.now()); setShowSteps(true)
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
      setActivePhase('plan'); setAgentStartTime(Date.now()); setShowSteps(true)
      await api.submitPlanAnswers(id!, answers)
    } catch {
      setActivePhase(null)
      toast.error('Failed to submit answers')
    }
  }

  async function handleClear() {
    try {
      await api.updateTask(id!, { title: task!.title, description: task!.description })
      setMessages([]); setAgentSteps([])
      toast.info('New chat started')
      loadTask(); loadSessions()
    } catch { toast.error('Failed to clear') }
  }

  async function handleCancel() {
    try {
      await api.cancelAgent(id!)
      setActivePhase(null); setCurrentChunk(''); setAgentStartTime(null)
      toast.info('Agent cancelled'); loadTask(); loadMessages()
    } catch { toast.error('Failed to cancel') }
  }

  function handleInputChange(e: React.ChangeEvent<HTMLTextAreaElement>) {
    const value = e.target.value
    const pos = e.target.selectionStart ?? value.length
    setInput(value)

    if (mention.isActive) {
      const q = value.slice(mention.mentionStart + 1, pos)
      if (q.includes(' ') || q.includes('\n') || pos <= mention.mentionStart) {
        mention.deactivate()
      } else {
        mention.setQuery(q)
      }
    } else {
      // Check if user just typed @
      if (pos >= 1 && value[pos - 1] === '@') {
        const charBefore = pos >= 2 ? value[pos - 2] : ' '
        if (charBefore === ' ' || charBefore === '\n' || pos === 1) {
          mention.activate(pos - 1)
        }
      }
    }
  }

  function selectMentionFile(file: api.ProjectFile) {
    const before = input.slice(0, mention.mentionStart)
    const cursorPos = textareaRef.current?.selectionStart ?? input.length
    const after = input.slice(cursorPos)
    setInput(before + '@' + file.rel_path + ' ' + after)
    mention.deactivate()
    setTimeout(() => textareaRef.current?.focus(), 0)
  }

  function completeMentionFolder(file: api.ProjectFile) {
    // Tab on a folder: fill path + "/" and keep dropdown open (like terminal tab-complete)
    const before = input.slice(0, mention.mentionStart)
    const cursorPos = textareaRef.current?.selectionStart ?? input.length
    const after = input.slice(cursorPos)
    const filled = file.rel_path + '/'
    setInput(before + '@' + filled + after)
    mention.setQuery(filled)
    mention.setSelectedIndex(0)
    setTimeout(() => {
      if (textareaRef.current) {
        const newPos = mention.mentionStart + 1 + filled.length
        textareaRef.current.selectionStart = newPos
        textareaRef.current.selectionEnd = newPos
        textareaRef.current.focus()
      }
    }, 0)
  }

  function handleKeyDown(e: React.KeyboardEvent) {
    if (mention.isActive && mention.results.length > 0) {
      if (e.key === 'ArrowDown') { e.preventDefault(); mention.setSelectedIndex(i => Math.min(i + 1, mention.results.length - 1)); return }
      if (e.key === 'ArrowUp') { e.preventDefault(); mention.setSelectedIndex(i => Math.max(i - 1, 0)); return }
      if (e.key === 'Tab') {
        e.preventDefault()
        const selected = mention.results[mention.selectedIndex]
        if (selected.is_dir) {
          completeMentionFolder(selected)
        } else {
          selectMentionFile(selected)
        }
        return
      }
      if (e.key === 'Enter') { e.preventDefault(); selectMentionFile(mention.results[mention.selectedIndex]); return }
      if (e.key === 'Escape') { e.preventDefault(); mention.deactivate(); return }
    }
    if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); handleSend() }
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
    textareaRef.current?.focus()
  }

  if (!task) return <p className="text-muted-foreground">Loading...</p>

  const hasPlan = !!task.implementation_plan
  const hasExecutions = task.executions && task.executions.length > 0
  const hasMessages = messages.length > 0

  // --- Breadcrumb (shared) ---
  const breadcrumb = (
    <div className="flex items-center gap-1 text-sm text-muted-foreground mb-2 overflow-hidden">
      <Link to="/" className="hover:text-foreground transition-colors shrink-0">Projects</Link>
      <ChevronRight className="h-3 w-3 shrink-0" />
      <Link to={`/projects/${task.project_id}`} className="hover:text-foreground transition-colors shrink-0">Project</Link>
      <ChevronRight className="h-3 w-3 shrink-0" />
      {viewMode === 'planning' ? (
        <button onClick={() => setViewMode('overview')} className="hover:text-foreground transition-colors truncate flex items-center gap-1">
          <ArrowLeft className="h-3 w-3" /> {task.title}
        </button>
      ) : (
        <span className="text-foreground truncate">{task.title}</span>
      )}
    </div>
  )

  // ===================== OVERVIEW MODE =====================
  if (viewMode === 'overview') {
    return (
      <div>
        {breadcrumb}

        {/* Task info */}
        <div className="mb-6">
          <div className="flex items-start sm:items-center justify-between gap-3 flex-wrap mb-2">
            <div className="flex items-center gap-3">
              <Badge variant="outline" className="shrink-0">{task.status}</Badge>
              <h1 className="text-xl font-semibold tracking-tight">{task.title}</h1>
            </div>
          </div>
          {task.description && <p className="text-muted-foreground text-sm mb-3">{task.description}</p>}
          <div className="flex items-center gap-4 text-xs text-muted-foreground">
            <span className="flex items-center gap-1"><Calendar className="h-3 w-3" /> Created {formatDate(task.created_at)}</span>
            {task.updated_at !== task.created_at && (
              <span className="flex items-center gap-1"><Clock className="h-3 w-3" /> Updated {formatDate(task.updated_at)}</span>
            )}
          </div>
        </div>

        <Separator className="mb-6" />

        {/* Status cards */}
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4 mb-6">
          {/* Plan status */}
          <Card className={hasPlan ? 'border-emerald-500/30' : ''}>
            <CardHeader className="pb-2">
              <CardTitle className="text-sm font-medium flex items-center gap-2">
                <FileText className="h-4 w-4" />
                Plan
              </CardTitle>
            </CardHeader>
            <CardContent>
              {hasPlan ? (
                <div>
                  <div className="flex items-center gap-1.5 text-emerald-400 text-sm mb-3">
                    <CheckCircle2 className="h-4 w-4" />
                    Plan ready
                  </div>
                  <div className="flex gap-2">
                    <Button size="sm" variant="outline" onClick={() => startPlanning()} className="flex-1">
                      View & Edit
                    </Button>
                    <Button size="sm" variant="success" onClick={handleExecute} className="flex-1 gap-1">
                      <Play className="h-3 w-3" /> Execute
                    </Button>
                  </div>
                </div>
              ) : (
                <div>
                  <p className="text-sm text-muted-foreground mb-3">No plan yet</p>
                  <Button size="sm" onClick={() => startPlanning(true)} className="w-full">
                    Start Planning
                  </Button>
                </div>
              )}
            </CardContent>
          </Card>

          {/* Chat status */}
          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-sm font-medium flex items-center gap-2">
                <MessageSquare className="h-4 w-4" />
                Chat
              </CardTitle>
            </CardHeader>
            <CardContent>
              {hasMessages ? (
                <div>
                  <p className="text-sm text-muted-foreground mb-3">{messages.length} messages</p>
                  <Button size="sm" variant="outline" onClick={() => startPlanning()} className="w-full">
                    Continue Chat
                  </Button>
                </div>
              ) : (
                <div>
                  <p className="text-sm text-muted-foreground mb-3">No conversation yet</p>
                  <Button size="sm" variant="outline" onClick={() => startPlanning()} className="w-full">
                    Start Chat
                  </Button>
                </div>
              )}
              {sessions.length > 1 && (
                <p className="text-xs text-muted-foreground mt-2">{sessions.length} sessions in history</p>
              )}
            </CardContent>
          </Card>

          {/* Execution status */}
          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-sm font-medium flex items-center gap-2">
                <Zap className="h-4 w-4" />
                Execution
              </CardTitle>
            </CardHeader>
            <CardContent>
              {hasExecutions ? (
                <div>
                  <p className="text-sm text-muted-foreground mb-1">{task.executions!.length} execution(s)</p>
                  <Badge variant={task.executions![0].status === 'completed' ? 'secondary' : 'outline'} className="text-xs">
                    Latest: {task.executions![0].status}
                  </Badge>
                </div>
              ) : (
                <p className="text-sm text-muted-foreground">Not executed yet</p>
              )}
            </CardContent>
          </Card>
        </div>

        {/* Plan preview (if exists) */}
        {hasPlan && (
          <Card className="mb-6">
            <CardHeader className="py-3 px-4 flex flex-row items-center justify-between">
              <CardTitle className="text-sm font-medium">Plan Preview</CardTitle>
              <Button size="xs" variant="outline" onClick={() => startPlanning()}>Open full view</Button>
            </CardHeader>
            <Separator />
            <CardContent className="pt-4">
              <div className="prose prose-sm max-w-none dark:prose-invert max-h-64 overflow-hidden relative">
                <ReactMarkdown>{task.implementation_plan}</ReactMarkdown>
                <div className="absolute bottom-0 left-0 right-0 h-16 bg-gradient-to-t from-card to-transparent" />
              </div>
            </CardContent>
          </Card>
        )}

        {/* Executions */}
        {hasExecutions && (
          <Card>
            <CardHeader className="py-2 px-4">
              <CardTitle className="text-xs font-medium text-muted-foreground">Execution Log</CardTitle>
            </CardHeader>
            <Separator />
            {task.executions!.map(ex => (
              <CardContent key={ex.id} className="pt-3">
                <div className="flex items-center gap-2 mb-1">
                  <Badge variant={ex.status === 'completed' ? 'secondary' : 'outline'} className="text-[10px]">{ex.status}</Badge>
                  <span className="text-[10px] text-muted-foreground">{ex.provider}/{ex.model}</span>
                </div>
                {ex.log && (
                  <ScrollArea className="h-40">
                    <pre className="text-[11px] whitespace-pre-wrap bg-secondary/30 rounded p-2">{ex.log}</pre>
                  </ScrollArea>
                )}
              </CardContent>
            ))}
          </Card>
        )}
      </div>
    )
  }

  // ===================== PLANNING MODE (chat + plan split) =====================

  const chatPanel = (
    <div className="flex flex-col h-full">
      <ScrollArea className="flex-1">
        <div className="space-y-2.5 p-2.5">
          {messages.length === 0 && !activePhase && (
            <div className="text-center py-6 text-muted-foreground">
              <p className="text-sm">Send a message to start planning.</p>
            </div>
          )}
          {messages.map(msg => (
            <div key={msg.id} className={`flex ${msg.role === 'user' ? 'justify-end' : 'justify-start'}`}>
              <div className={`max-w-[90%] rounded-lg px-3 py-2 text-sm ${
                msg.role === 'user' ? 'bg-primary text-primary-foreground' : 'bg-card border'
              }`}>
                {msg.role === 'user' ? (
                  <p className="whitespace-pre-wrap">{msg.content}</p>
                ) : (
                  <div className="prose prose-sm max-w-none dark:prose-invert">
                    <ReactMarkdown>{msg.content}</ReactMarkdown>
                  </div>
                )}
                <p className={`text-[10px] mt-1 ${msg.role === 'user' ? 'text-primary-foreground/50' : 'text-muted-foreground'}`}>
                  {formatTime(msg.created_at)}
                </p>
              </div>
            </div>
          ))}
          {activePhase && (
            <div className="flex justify-start">
              <div className="bg-card border rounded-lg px-3 py-2 text-sm max-w-[90%]">
                <div className="flex items-center gap-2 text-muted-foreground mb-1">
                  <Loader2 className="h-3 w-3 animate-spin" />
                  <span className="text-xs">Agent: {activePhase}</span>
                  <Button size="xs" variant="ghost" className="h-5 px-1 text-xs" onClick={() => setShowSteps(!showSteps)}>
                    {showSteps ? 'hide' : 'steps'}
                  </Button>
                </div>
                {showSteps && agentSteps.length > 0 && (
                  <div className="border rounded mt-1 max-h-48 overflow-auto text-xs">
                    {agentSteps.slice(-5).map((step, i) => (
                      <div key={i} className="px-2 py-1 border-b last:border-b-0">
                        {step.think && <p className="text-violet-400">{step.think.slice(0, 100)}{step.think.length > 100 ? '...' : ''}</p>}
                        {step.action && <p className="text-blue-400 font-mono">{step.action}({step.action_arg.slice(0, 50)})</p>}
                      </div>
                    ))}
                    {currentChunk && <div className="px-2 py-1 text-muted-foreground/50 animate-pulse">{currentChunk.slice(-80)}</div>}
                  </div>
                )}
              </div>
            </div>
          )}

          {/* Pending questions from agent */}
          {pendingQuestions.length > 0 && !activePhase && (
            <div className="mx-3 my-2 border border-amber-500/30 rounded-lg bg-card p-4 space-y-4">
              <div className="flex items-center gap-2 text-sm font-medium text-amber-400">
                <MessageSquare className="h-4 w-4" />
                Agent needs your input ({pendingQuestions.length} question{pendingQuestions.length > 1 ? 's' : ''})
              </div>
              {pendingQuestions.map(q => (
                <div key={q.id} className="space-y-2">
                  <p className="text-sm font-medium">{q.question}</p>
                  {q.options && q.options.length > 0 && (
                    <div className="flex flex-wrap gap-1.5">
                      {q.options.map((opt, i) => (
                        <Button
                          key={i}
                          size="xs"
                          variant={questionAnswers[q.id] === opt ? 'default' : 'outline'}
                          onClick={() => setQuestionAnswers(prev => ({ ...prev, [q.id]: opt }))}
                        >
                          {opt}
                        </Button>
                      ))}
                    </div>
                  )}
                  <Textarea
                    placeholder="Type your answer..."
                    value={questionAnswers[q.id] || ''}
                    onChange={e => setQuestionAnswers(prev => ({ ...prev, [q.id]: e.target.value }))}
                    rows={1}
                    className="min-h-[36px] max-h-[80px] resize-none text-sm"
                  />
                </div>
              ))}
              <Button size="sm" onClick={handleSubmitAnswers}>
                Submit Answers & Continue
              </Button>
            </div>
          )}

          <div ref={chatEndRef} />
        </div>
      </ScrollArea>

      <div className="shrink-0 border-t p-1.5">
        <div className="relative flex gap-2 items-end">
          {mention.isActive && (
            <FileMention
              results={mention.results}
              selectedIndex={mention.selectedIndex}
              loading={mention.loading}
              onSelect={selectMentionFile}
              onHover={mention.setSelectedIndex}
            />
          )}
          <Textarea
            ref={textareaRef}
            placeholder={activePhase ? 'Agent working...' : messages.length === 0 ? 'Describe what to do... (@ to reference files)' : 'Revise, ask, or /execute... (@ to reference files)'}
            value={input}
            onChange={handleInputChange}
            onKeyDown={handleKeyDown}
            disabled={!!activePhase}
            rows={1}
            className="min-h-[36px] max-h-[100px] resize-none text-sm"
          />
          {activePhase ? (
            <Button size="sm" variant="destructive" onClick={handleCancel} className="h-9 shrink-0"><XCircle className="h-3 w-3" /></Button>
          ) : (
            <Button size="sm" onClick={handleSend} disabled={!input.trim()} className="h-9 shrink-0"><Send className="h-3 w-3" /></Button>
          )}
        </div>
        <div className="flex items-center gap-2 mt-1.5 text-[10px] text-muted-foreground flex-wrap">
          {hasPlan && (
            <button onClick={handleExecute} className="hover:text-foreground transition-colors flex items-center gap-0.5" disabled={!!activePhase}>
              <Play className="h-2.5 w-2.5" /> Execute
            </button>
          )}
          <button onClick={handleClear} className="hover:text-foreground transition-colors flex items-center gap-0.5">
            <Trash2 className="h-2.5 w-2.5" /> New
          </button>
          {sessions.length > 1 && (
            <button onClick={() => setShowSessions(!showSessions)} className="hover:text-foreground transition-colors flex items-center gap-0.5">
              <History className="h-2.5 w-2.5" /> ({sessions.length})
            </button>
          )}
        </div>
        {showSessions && sessions.length > 0 && (
          <div className="mt-1.5 border rounded-lg bg-card p-1.5 space-y-0.5 max-h-32 overflow-auto">
            {sessions.map(sess => (
              <button
                key={sess.id}
                onClick={() => handleRestoreSession(sess.id)}
                className={`w-full text-left px-2 py-1.5 rounded text-xs hover:bg-accent transition-colors flex items-center justify-between ${
                  sess.status === 'active' ? 'bg-accent/50 text-foreground' : 'text-muted-foreground'
                }`}
              >
                <span className="truncate">{sess.messages?.[0]?.content || 'Empty'}</span>
                <span className="shrink-0 ml-2 text-[10px]">
                  {sess.status === 'active' ? 'now' : new Date(sess.created_at).toLocaleDateString()}
                </span>
              </button>
            ))}
          </div>
        )}
      </div>
    </div>
  )

  return (
    <div className="flex flex-col h-[calc(100vh-64px)]">
      {breadcrumb}

      <div className="flex-1 min-h-0">
        {hasPlan ? (
          /* Split: Plan left, Chat right */
          <div className="flex gap-0 h-full border rounded-lg overflow-hidden">
            <div className="flex-1 min-w-0 flex flex-col border-r">
              <div className="shrink-0 px-4 py-2 border-b bg-card flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <FileText className="h-3.5 w-3.5 text-muted-foreground" />
                  <span className="text-xs font-medium">Plan</span>
                </div>
                <Button size="xs" variant="success" onClick={handleExecute} disabled={!!activePhase} className="gap-1">
                  <Play className="h-3 w-3" /> Execute
                </Button>
              </div>
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
            </div>

            <div className="w-[300px] lg:w-[340px] shrink-0 flex flex-col bg-background">
              <div className="shrink-0 px-3 py-1.5 border-b bg-card">
                <span className="text-xs font-medium text-muted-foreground">Chat</span>
              </div>
              {chatPanel}
            </div>
          </div>
        ) : (
          /* Split: empty plan placeholder left, chat right */
          <div className="flex gap-0 h-full border rounded-lg overflow-hidden">
            <div className="flex-1 min-w-0 flex flex-col border-r">
              <div className="shrink-0 px-4 py-2 border-b bg-card flex items-center gap-2">
                <FileText className="h-3.5 w-3.5 text-muted-foreground" />
                <span className="text-xs font-medium">Plan</span>
                {activePhase === 'plan' && <Loader2 className="h-3 w-3 animate-spin text-primary" />}
              </div>
              <div className="flex-1 flex flex-col items-center justify-center text-muted-foreground">
                {activePhase === 'plan' ? (
                  <div className="text-center px-6">
                    <Loader2 className="h-8 w-8 mx-auto mb-3 animate-spin text-primary/40" />
                    <p className="text-sm">Agent is reading your codebase and creating a plan...</p>
                    {agentSteps.length > 0 && (
                      <div className="mt-4 text-left max-w-md border rounded p-3 text-xs space-y-1 max-h-48 overflow-auto">
                        {agentSteps.slice(-3).map((step, i) => (
                          <div key={i}>
                            {step.think && <p className="text-violet-400">{step.think.slice(0, 120)}</p>}
                            {step.action && <p className="text-blue-400 font-mono">{step.action}({step.action_arg.slice(0, 50)})</p>}
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                ) : (
                  <div className="text-center">
                    <FileText className="h-10 w-10 mx-auto mb-3 opacity-20" />
                    <p className="text-sm">Plan will appear here</p>
                    <p className="text-xs mt-1 text-muted-foreground/60">Chat with the agent on the right to create one</p>
                  </div>
                )}
              </div>
            </div>

            <div className="w-[300px] lg:w-[340px] shrink-0 flex flex-col bg-background">
              <div className="shrink-0 px-3 py-1.5 border-b bg-card">
                <span className="text-xs font-medium text-muted-foreground">Chat</span>
              </div>
              {chatPanel}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
