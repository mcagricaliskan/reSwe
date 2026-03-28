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
  ChevronRight, Send, Play, Loader2, XCircle, Trash2, History
} from 'lucide-react'
import { toast } from 'sonner'
import type { Task, PlanMessage, ChatSession } from '@/lib/api'
import * as api from '@/lib/api'
import { wsClient } from '@/lib/ws'
import AgentSteps from '@/components/AgentSteps'
import type { AgentStep } from '@/components/AgentSteps'

export default function TaskPage() {
  const { id } = useParams<{ id: string }>()
  const [task, setTask] = useState<Task | null>(null)
  const [messages, setMessages] = useState<PlanMessage[]>([])
  const [input, setInput] = useState('')
  const [activePhase, setActivePhase] = useState<string | null>(null)
  const [agentSteps, setAgentSteps] = useState<AgentStep[]>([])
  const [currentChunk, setCurrentChunk] = useState('')
  const [agentStartTime, setAgentStartTime] = useState<number | null>(null)
  const [showSteps, setShowSteps] = useState(false)
  const [sessions, setSessions] = useState<ChatSession[]>([])
  const [showSessions, setShowSessions] = useState(false)
  const chatEndRef = useRef<HTMLDivElement>(null)
  const textareaRef = useRef<HTMLTextAreaElement>(null)

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
      loadMessages()
      loadSessions()
      loadTask()
      toast.success('Session restored')
    } catch { toast.error('Failed to restore session') }
  }

  // On mount
  useEffect(() => {
    if (!id) return
    loadTask()
    loadMessages()
    loadSessions()
    api.getAgentStatus(id).then(status => {
      if (status.active) {
        setActivePhase(status.phase || null)
        setAgentStartTime(Date.now())
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
    unsubs.push(wsClient.on('agent_done', (msg) => {
      if (msg.task_id === taskId) {
        setActivePhase(null); setCurrentChunk(''); setAgentStartTime(null)
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

  // Auto-scroll chat
  useEffect(() => {
    chatEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages, agentSteps.length, activePhase])

  async function handleSend() {
    const text = input.trim()
    if (!text || activePhase) return

    // Check for commands
    if (text.startsWith('/')) {
      const cmd = text.toLowerCase()
      if (cmd === '/execute' || cmd === '/run') {
        handleExecute()
        setInput('')
        return
      }
      if (cmd === '/clear') {
        handleClear()
        setInput('')
        return
      }
    }

    setInput('')
    setAgentSteps([]); setCurrentChunk('')
    setActivePhase('plan'); setAgentStartTime(Date.now()); setShowSteps(true)

    // If no messages yet, start a fresh plan. Otherwise, continue the conversation.
    try {
      if (messages.length === 0) {
        await api.runPlan(id!)
      } else {
        await api.planChat(id!, text)
      }
    } catch {
      setActivePhase(null); toast.error('Failed to send')
    }
  }

  async function handleExecute() {
    setAgentSteps([]); setCurrentChunk('')
    setActivePhase('execute'); setAgentStartTime(Date.now()); setShowSteps(true)
    try { await api.runExecute(id!) } catch { setActivePhase(null); toast.error('Failed to start') }
  }

  async function handleClear() {
    // Archive current session and start fresh
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

  function handleKeyDown(e: React.KeyboardEvent) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSend()
    }
  }

  if (!task) return <p className="text-muted-foreground">Loading...</p>

  const hasExecutions = task.executions && task.executions.length > 0

  return (
    <div className="flex flex-col h-[calc(100vh-100px)]">
      {/* Header */}
      <div className="shrink-0 mb-4">
        <div className="flex items-center gap-1 text-sm text-muted-foreground mb-2 overflow-hidden">
          <Link to="/" className="hover:text-foreground transition-colors shrink-0">Projects</Link>
          <ChevronRight className="h-3 w-3 shrink-0" />
          <Link to={`/projects/${task.project_id}`} className="hover:text-foreground transition-colors shrink-0">Project</Link>
          <ChevronRight className="h-3 w-3 shrink-0" />
          <span className="text-foreground truncate">{task.title}</span>
        </div>
        <div className="flex items-start sm:items-center gap-3 flex-wrap">
          <Badge variant="outline" className="shrink-0">{task.status}</Badge>
          <h1 className="text-lg font-semibold tracking-tight">{task.title}</h1>
        </div>
        {task.description && <p className="text-muted-foreground text-sm mt-1">{task.description}</p>}
      </div>

      {/* Chat area */}
      <div className="flex-1 min-h-0 flex gap-4">
        {/* Messages */}
        <div className="flex-1 flex flex-col min-w-0">
          <ScrollArea className="flex-1">
            <div className="space-y-4 p-4">
              {/* Empty state */}
              {messages.length === 0 && !activePhase && (
                <div className="text-center py-12 text-muted-foreground">
                  <p className="text-sm">Start by sending a message. The agent will read your codebase and create a plan.</p>
                  <p className="text-xs mt-2 text-muted-foreground/60">
                    Type anything to start planning. Use <code className="bg-secondary px-1 rounded">/execute</code> to run the plan, <code className="bg-secondary px-1 rounded">/clear</code> to start over.
                  </p>
                </div>
              )}

              {/* Messages */}
              {messages.map(msg => (
                <div key={msg.id} className={`flex ${msg.role === 'user' ? 'justify-end' : 'justify-start'}`}>
                  <div className={`max-w-[85%] rounded-lg px-4 py-3 text-sm ${
                    msg.role === 'user'
                      ? 'bg-primary text-primary-foreground'
                      : 'bg-card border'
                  }`}>
                    {msg.role === 'user' ? (
                      <p className="whitespace-pre-wrap">{msg.content}</p>
                    ) : (
                      <div className="prose prose-sm max-w-none dark:prose-invert">
                        <ReactMarkdown>{msg.content}</ReactMarkdown>
                      </div>
                    )}
                    <p className={`text-[10px] mt-2 ${msg.role === 'user' ? 'text-primary-foreground/50' : 'text-muted-foreground'}`}>
                      {new Date(msg.created_at).toLocaleTimeString()}
                    </p>
                  </div>
                </div>
              ))}

              {/* Active agent indicator */}
              {activePhase && (
                <div className="flex justify-start">
                  <div className="bg-card border rounded-lg px-4 py-3 text-sm max-w-[85%]">
                    <div className="flex items-center gap-2 text-muted-foreground mb-2">
                      <Loader2 className="h-3 w-3 animate-spin" />
                      <span className="text-xs">Agent working: {activePhase}</span>
                      <Button size="xs" variant="ghost" className="h-5 px-1 text-xs" onClick={() => setShowSteps(!showSteps)}>
                        {showSteps ? 'hide steps' : 'show steps'}
                      </Button>
                    </div>
                    {showSteps && agentSteps.length > 0 && (
                      <div className="border rounded mt-1 max-h-60 overflow-auto text-xs">
                        {agentSteps.slice(-5).map((step, i) => (
                          <div key={i} className="px-2 py-1 border-b last:border-b-0">
                            {step.think && <p className="text-violet-400">Think: {step.think.slice(0, 120)}{step.think.length > 120 ? '...' : ''}</p>}
                            {step.action && <p className="text-blue-400 font-mono">{step.action}({step.action_arg.slice(0, 60)})</p>}
                          </div>
                        ))}
                        {currentChunk && (
                          <div className="px-2 py-1 text-muted-foreground/50 animate-pulse">
                            {currentChunk.slice(-100)}
                          </div>
                        )}
                      </div>
                    )}
                  </div>
                </div>
              )}

              <div ref={chatEndRef} />
            </div>
          </ScrollArea>

          {/* Input */}
          <div className="shrink-0 border-t p-3">
            <div className="flex gap-2 items-end">
              <Textarea
                ref={textareaRef}
                placeholder={activePhase ? 'Agent is working...' : messages.length === 0 ? 'Describe what you want to do, or just say "plan this"...' : 'Ask to revise, or type /execute to run the plan...'}
                value={input}
                onChange={e => setInput(e.target.value)}
                onKeyDown={handleKeyDown}
                disabled={!!activePhase}
                rows={1}
                className="min-h-[40px] max-h-[120px] resize-none text-sm"
              />
              <div className="flex flex-col gap-1">
                {activePhase ? (
                  <Button size="sm" variant="destructive" onClick={handleCancel} className="h-9">
                    <XCircle className="h-3 w-3" />
                  </Button>
                ) : (
                  <Button size="sm" onClick={handleSend} disabled={!input.trim()} className="h-9">
                    <Send className="h-3 w-3" />
                  </Button>
                )}
              </div>
            </div>
            <div className="flex items-center gap-3 mt-2 text-[10px] text-muted-foreground">
              {task.implementation_plan && (
                <button onClick={handleExecute} className="hover:text-foreground transition-colors flex items-center gap-1" disabled={!!activePhase}>
                  <Play className="h-2.5 w-2.5" /> Execute plan
                </button>
              )}
              <button onClick={handleClear} className="hover:text-foreground transition-colors flex items-center gap-1">
                <Trash2 className="h-2.5 w-2.5" /> New chat
              </button>
              {sessions.length > 1 && (
                <button onClick={() => setShowSessions(!showSessions)} className="hover:text-foreground transition-colors flex items-center gap-1">
                  <History className="h-2.5 w-2.5" /> Sessions ({sessions.length})
                </button>
              )}
              <span>Shift+Enter for newline</span>
            </div>

            {/* Sessions panel */}
            {showSessions && sessions.length > 0 && (
              <div className="mt-2 border rounded-lg bg-card p-2 space-y-1 max-h-40 overflow-auto">
                {sessions.map(sess => (
                  <button
                    key={sess.id}
                    onClick={() => handleRestoreSession(sess.id)}
                    className={`w-full text-left px-3 py-2 rounded text-xs hover:bg-accent transition-colors flex items-center justify-between ${
                      sess.status === 'active' ? 'bg-accent/50 text-foreground' : 'text-muted-foreground'
                    }`}
                  >
                    <span className="truncate">
                      {sess.messages?.[0]?.content || 'Empty session'}
                    </span>
                    <span className="shrink-0 ml-2 text-[10px]">
                      {sess.status === 'active' ? 'current' : new Date(sess.created_at).toLocaleDateString()}
                    </span>
                  </button>
                ))}
              </div>
            )}
          </div>
        </div>

        {/* Steps sidebar — shows when agent is running and steps exist */}
        {agentSteps.length > 0 && (
          <div className="hidden lg:block w-80 shrink-0 border-l">
            <div className="p-2 border-b text-xs font-medium text-muted-foreground flex items-center justify-between">
              <span>Agent Steps</span>
              {activePhase && <Loader2 className="h-3 w-3 animate-spin" />}
            </div>
            <AgentSteps
              steps={agentSteps}
              streaming={!!activePhase}
              streamChunk={currentChunk}
              startTime={agentStartTime ?? undefined}
            />
          </div>
        )}
      </div>

      {/* Executions — collapsed at bottom */}
      {hasExecutions && (
        <div className="shrink-0 mt-4">
          <Card>
            <CardHeader className="py-2 px-4">
              <CardTitle className="text-xs font-medium text-muted-foreground">Executions</CardTitle>
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
        </div>
      )}
    </div>
  )
}
