import { useState, useRef, useEffect } from 'react'
import ReactMarkdown from 'react-markdown'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Textarea } from '@/components/ui/textarea'
import { Send, Loader2, XCircle, Trash2, MessageSquare, ChevronRight, ChevronDown, Brain, Wrench, Eye, Timer, CheckCircle2 } from 'lucide-react'
import type { AgentQuestion, ProjectFile } from '@/lib/api'
import type { AgentStep } from '@/components/AgentSteps'
import { useFileMention } from '@/lib/useFileMention'
import { FileMention } from '@/components/FileMention'

function formatTime(d: string) {
  return new Date(d).toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' })
}
function formatElapsed(ms: number) {
  const totalSeconds = Math.max(0, Math.floor(ms / 1000))
  if (totalSeconds < 60) return `${totalSeconds}s`
  const minutes = Math.floor(totalSeconds / 60)
  const seconds = totalSeconds % 60
  return `${minutes}m ${seconds}s`
}

export interface ChatMessage {
  id: number
  role: string
  content: string
  created_at: string
}

interface ChatPanelProps {
  messages: ChatMessage[]
  agentSteps: AgentStep[]
  currentChunk: string
  activePhase: string | null
  elapsedMs: number
  pendingQuestions: AgentQuestion[]
  questionAnswers: Record<number, string>
  onAnswerChange: (questionId: number, answer: string) => void
  onSubmitAnswers: () => void
  input: string
  onInputChange: (value: string) => void
  onSend: () => void
  onCancel: () => void
  onClear?: () => void
  placeholder?: string
  projectId?: number
  emptyMessage?: string
  extraActions?: React.ReactNode
}

export function ChatPanel({
  messages,
  agentSteps,
  currentChunk,
  activePhase,
  elapsedMs,
  pendingQuestions,
  questionAnswers,
  onAnswerChange,
  onSubmitAnswers,
  input,
  onInputChange,
  onSend,
  onCancel,
  onClear,
  placeholder,
  projectId,
  emptyMessage = 'Send a message to start chatting.',
  extraActions,
}: ChatPanelProps) {
  const chatEndRef = useRef<HTMLDivElement>(null)
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const mention = useFileMention(projectId)

  const elapsedLabel = activePhase ? formatElapsed(elapsedMs) : null

  useEffect(() => {
    chatEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages, agentSteps.length, activePhase])

  function handleInputChange(e: React.ChangeEvent<HTMLTextAreaElement>) {
    const value = e.target.value
    const pos = e.target.selectionStart ?? value.length
    onInputChange(value)

    if (mention.isActive) {
      const q = value.slice(mention.mentionStart + 1, pos)
      if (q.includes(' ') || q.includes('\n') || pos <= mention.mentionStart) {
        mention.deactivate()
      } else {
        mention.setQuery(q)
      }
    } else {
      if (pos >= 1 && value[pos - 1] === '@') {
        const charBefore = pos >= 2 ? value[pos - 2] : ' '
        if (charBefore === ' ' || charBefore === '\n' || pos === 1) {
          mention.activate(pos - 1)
        }
      }
    }
  }

  function selectMentionFile(file: ProjectFile) {
    const before = input.slice(0, mention.mentionStart)
    const cursorPos = textareaRef.current?.selectionStart ?? input.length
    const after = input.slice(cursorPos)
    onInputChange(before + '@' + file.rel_path + ' ' + after)
    mention.deactivate()
    setTimeout(() => textareaRef.current?.focus(), 0)
  }

  function completeMentionFolder(file: ProjectFile) {
    const before = input.slice(0, mention.mentionStart)
    const cursorPos = textareaRef.current?.selectionStart ?? input.length
    const after = input.slice(cursorPos)
    const filled = file.rel_path + '/'
    onInputChange(before + '@' + filled + after)
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
        if (selected.is_dir) completeMentionFolder(selected)
        else selectMentionFile(selected)
        return
      }
      if (e.key === 'Enter') { e.preventDefault(); selectMentionFile(mention.results[mention.selectedIndex]); return }
      if (e.key === 'Escape') { e.preventDefault(); mention.deactivate(); return }
    }
    if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); onSend() }
  }

  function truncateArg(arg: string, max = 50): string {
    if (!arg) return ''
    const oneline = arg.replace(/\n/g, ' ').trim()
    return oneline.length > max ? oneline.slice(0, max) + '...' : oneline
  }

  function CollapsibleStepMessage({ step, index }: { step: AgentStep; index: number }) {
    const [expanded, setExpanded] = useState(false)

    if (step.is_final) {
      return (
        <div key={`agent-step-${index}`} className="flex justify-start">
          <div className="max-w-[90%] rounded-lg text-sm bg-card border overflow-hidden">
            <button onClick={() => setExpanded(e => !e)} className="flex items-center gap-2 w-full text-left px-3 py-1.5 hover:bg-accent/30 transition-colors text-xs">
              {expanded ? <ChevronDown className="h-3 w-3 text-emerald-400 shrink-0" /> : <ChevronRight className="h-3 w-3 text-emerald-400 shrink-0" />}
              <CheckCircle2 className="h-3 w-3 text-emerald-400 shrink-0" />
              <span className="text-emerald-400 font-medium">Plan complete</span>
              {step.duration_ms != null && step.duration_ms > 0 && (
                <span className="ml-auto text-muted-foreground/60 flex items-center gap-0.5">
                  <Timer className="h-2.5 w-2.5" />{formatElapsed(step.duration_ms)}
                </span>
              )}
            </button>
            {expanded && step.action_arg && (
              <div className="px-3 pb-2 border-t">
                <div className="prose prose-sm max-w-none dark:prose-invert mt-2">
                  <ReactMarkdown>{step.action_arg}</ReactMarkdown>
                </div>
              </div>
            )}
          </div>
        </div>
      )
    }

    return (
      <div key={`agent-step-${index}`} className="flex justify-start">
        <div className="max-w-[90%] rounded-lg text-sm bg-card border overflow-hidden">
          <button onClick={() => setExpanded(e => !e)} className="flex items-center gap-2 w-full text-left px-3 py-1.5 hover:bg-accent/30 transition-colors text-xs">
            {expanded ? <ChevronDown className="h-3 w-3 text-muted-foreground shrink-0" /> : <ChevronRight className="h-3 w-3 text-muted-foreground shrink-0" />}
            <span className="text-[10px] font-bold text-muted-foreground bg-secondary px-1 py-0.5 rounded">{step.step}</span>
            {step.action ? (
              <span className="text-blue-400 font-mono truncate min-w-0">
                {step.action}(<span className="text-foreground/40">{truncateArg(step.action_arg)}</span>)
              </span>
            ) : (
              <span className="text-muted-foreground italic">thinking...</span>
            )}
            {step.duration_ms != null && step.duration_ms > 0 && (
              <span className="ml-auto text-muted-foreground/60 flex items-center gap-0.5 shrink-0">
                <Timer className="h-2.5 w-2.5" />{formatElapsed(step.duration_ms)}
              </span>
            )}
          </button>
          {expanded && (
            <div className="px-3 pb-2 border-t space-y-1.5 pt-1.5">
              {step.think && (
                <div className="flex gap-2 pl-2 border-l-2 border-violet-500/30">
                  <Brain className="h-3 w-3 text-violet-400 shrink-0 mt-0.5" />
                  <p className="text-xs text-foreground/80">{step.think}</p>
                </div>
              )}
              {step.action && (
                <div className="flex gap-2 pl-2 border-l-2 border-blue-500/30">
                  <Wrench className="h-3 w-3 text-blue-400 shrink-0 mt-0.5" />
                  <code className="text-[11px] font-mono text-blue-300 break-all whitespace-pre-wrap">
                    {step.action}({step.action_arg})
                  </code>
                </div>
              )}
              {step.observation && (
                <div className="flex gap-2 pl-2 border-l-2 border-amber-500/30">
                  <Eye className="h-3 w-3 text-amber-400 shrink-0 mt-0.5" />
                  <pre className="text-[11px] text-foreground/60 whitespace-pre-wrap bg-secondary/50 rounded p-1.5 overflow-auto max-h-48 flex-1 min-w-0">
                    {step.observation}
                  </pre>
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    )
  }

  return (
    <div className="flex flex-col h-full">
      <ScrollArea className="flex-1">
        <div className="space-y-2.5 p-2.5">
          {messages.length === 0 && !activePhase && (
            <div className="text-center py-6 text-muted-foreground">
              <p className="text-sm">{emptyMessage}</p>
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
          {agentSteps.map((step, index) => (
            <CollapsibleStepMessage key={`step-${index}`} step={step} index={index} />
          ))}
          {activePhase && currentChunk && (
            <div className="flex justify-start">
              <div className="bg-card border rounded-lg px-3 py-2 text-sm max-w-[90%]">
                <div className="flex items-center gap-2 text-muted-foreground mb-2">
                  <Loader2 className="h-3 w-3 animate-spin" />
                  <span className="text-xs">Agent is thinking...</span>
                  {elapsedLabel && <span className="text-[10px]">{elapsedLabel}</span>}
                </div>
                <pre className="max-h-32 overflow-auto whitespace-pre-wrap text-xs text-foreground/60">
                  {currentChunk}
                </pre>
                <p className="text-[10px] mt-2 text-muted-foreground">Streaming</p>
              </div>
            </div>
          )}
          {activePhase && !currentChunk && agentSteps.length === 0 && (
            <div className="flex justify-start">
              <div className="bg-card border rounded-lg px-3 py-2 text-sm max-w-[90%]">
                <div className="flex items-center gap-2 text-muted-foreground">
                  <Loader2 className="h-3 w-3 animate-spin" />
                  <span className="text-xs">Agent is reading the codebase...</span>
                  {elapsedLabel && <span className="text-[10px]">{elapsedLabel}</span>}
                </div>
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
                          onClick={() => onAnswerChange(q.id, opt)}
                        >
                          {opt}
                        </Button>
                      ))}
                    </div>
                  )}
                  <Textarea
                    placeholder="Type your answer..."
                    value={questionAnswers[q.id] || ''}
                    onChange={e => onAnswerChange(q.id, e.target.value)}
                    rows={1}
                    className="min-h-[36px] max-h-[80px] resize-none text-sm"
                  />
                </div>
              ))}
              <Button size="sm" onClick={onSubmitAnswers}>
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
            placeholder={activePhase ? 'Agent working...' : (placeholder || 'Type a message... (@ to reference files)')}
            value={input}
            onChange={handleInputChange}
            onKeyDown={handleKeyDown}
            disabled={!!activePhase}
            rows={1}
            className="min-h-[36px] max-h-[100px] resize-none text-sm"
          />
          {activePhase ? (
            <Button size="sm" variant="destructive" onClick={onCancel} className="h-9 shrink-0"><XCircle className="h-3 w-3" /></Button>
          ) : (
            <Button size="sm" onClick={onSend} disabled={!input.trim()} className="h-9 shrink-0"><Send className="h-3 w-3" /></Button>
          )}
        </div>
        {(onClear || extraActions) && (
          <div className="flex items-center gap-2 mt-1.5 text-[10px] text-muted-foreground flex-wrap">
            {extraActions}
            {onClear && (
              <button onClick={onClear} className="hover:text-foreground transition-colors flex items-center gap-0.5">
                <Trash2 className="h-2.5 w-2.5" /> New
              </button>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
