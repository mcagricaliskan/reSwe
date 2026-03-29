import { useState, useEffect, useRef } from 'react'
import { Brain, Wrench, Eye, CheckCircle2, ChevronDown, ChevronRight, Clock, Timer } from 'lucide-react'
import { cn } from '@/lib/utils'

export interface AgentStep {
  step: number
  think: string
  action: string
  action_arg: string
  observation: string
  is_final: boolean
  phase: string
  started_at?: string
  completed_at?: string
  duration_ms?: number
}

interface AgentStepsProps {
  steps: AgentStep[]
  streaming: boolean
  streamChunk: string
  startTime?: number
}

function formatElapsed(ms: number): string {
  const s = Math.floor(ms / 1000)
  if (s < 60) return `${s}s`
  const m = Math.floor(s / 60)
  const rem = s % 60
  return `${m}m ${rem}s`
}

function truncateArg(arg: string, max = 60): string {
  if (!arg) return ''
  const oneline = arg.replace(/\n/g, ' ').trim()
  return oneline.length > max ? oneline.slice(0, max) + '...' : oneline
}

function CollapsibleStep({ step }: { step: AgentStep }) {
  const [expanded, setExpanded] = useState(false)
  const toggle = () => setExpanded(e => !e)

  if (step.is_final) {
    return (
      <div className="space-y-1">
        <button onClick={toggle} className="flex items-center gap-2 w-full text-left text-xs hover:bg-accent/50 rounded px-1.5 py-1 transition-colors">
          {expanded ? <ChevronDown className="h-3 w-3 shrink-0 text-emerald-400" /> : <ChevronRight className="h-3 w-3 shrink-0 text-emerald-400" />}
          <span className={cn("px-1.5 py-0.5 rounded text-[10px] font-bold", "bg-emerald-500/20 text-emerald-400")}>
            {step.step}
          </span>
          <CheckCircle2 className="h-3 w-3 text-emerald-400 shrink-0" />
          <span className="text-emerald-400 font-medium">done</span>
          {step.duration_ms != null && step.duration_ms > 0 && (
            <span className="ml-auto text-muted-foreground/60 flex items-center gap-0.5 shrink-0">
              <Timer className="h-2.5 w-2.5" />
              {formatElapsed(step.duration_ms)}
            </span>
          )}
        </button>
        {expanded && step.action_arg && (
          <div className="ml-6 pl-2 border-l-2 border-emerald-500/30">
            <pre className="text-sm text-foreground/80 whitespace-pre-wrap bg-secondary/30 rounded p-3 max-h-96 overflow-auto">
              {step.action_arg}
            </pre>
          </div>
        )}
      </div>
    )
  }

  return (
    <div className="space-y-1">
      {/* Collapsed row */}
      <button onClick={toggle} className="flex items-center gap-2 w-full text-left text-xs hover:bg-accent/50 rounded px-1.5 py-1 transition-colors">
        {expanded ? <ChevronDown className="h-3 w-3 shrink-0 text-muted-foreground" /> : <ChevronRight className="h-3 w-3 shrink-0 text-muted-foreground" />}
        <span className="px-1.5 py-0.5 rounded text-[10px] font-bold bg-secondary text-muted-foreground">
          {step.step}
        </span>
        {step.action ? (
          <span className="text-blue-400 font-mono truncate min-w-0">
            {step.action}(<span className="text-foreground/50">{truncateArg(step.action_arg)}</span>)
          </span>
        ) : (
          <span className="text-muted-foreground italic">thinking...</span>
        )}
        {step.duration_ms != null && step.duration_ms > 0 && (
          <span className="ml-auto text-muted-foreground/60 flex items-center gap-0.5 shrink-0">
            <Timer className="h-2.5 w-2.5" />
            {formatElapsed(step.duration_ms)}
          </span>
        )}
      </button>

      {/* Expanded details */}
      {expanded && (
        <div className="ml-6 space-y-1.5">
          {step.think && (
            <div className="flex gap-2 pl-2 border-l-2 border-violet-500/30">
              <Brain className="h-3.5 w-3.5 text-violet-400 shrink-0 mt-0.5" />
              <p className="text-sm text-foreground/80">{step.think}</p>
            </div>
          )}
          {step.action && (
            <div className="flex gap-2 pl-2 border-l-2 border-blue-500/30">
              <Wrench className="h-3.5 w-3.5 text-blue-400 shrink-0 mt-0.5" />
              <code className="text-xs font-mono text-blue-300 break-all whitespace-pre-wrap">
                {step.action}({step.action_arg})
              </code>
            </div>
          )}
          {step.observation && (
            <div className="flex gap-2 pl-2 border-l-2 border-amber-500/30">
              <Eye className="h-3.5 w-3.5 text-amber-400 shrink-0 mt-0.5" />
              <pre className={cn(
                "text-xs text-foreground/60 whitespace-pre-wrap bg-secondary/50 rounded p-2 overflow-auto flex-1 min-w-0",
                "max-h-64"
              )}>
                {step.observation}
              </pre>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

export default function AgentSteps({ steps, streaming, streamChunk, startTime }: AgentStepsProps) {
  const [elapsed, setElapsed] = useState(0)
  const [finalElapsed, setFinalElapsed] = useState(0)
  const bottomRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!streaming || !startTime) return
    setFinalElapsed(0)
    const interval = setInterval(() => {
      setElapsed(Date.now() - startTime)
    }, 1000)
    return () => {
      clearInterval(interval)
      if (startTime) setFinalElapsed(Date.now() - startTime)
    }
  }, [streaming, startTime])

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [steps.length, streamChunk])

  if (steps.length === 0 && !streaming) return null

  const lastStep = steps[steps.length - 1]

  return (
    <div className="flex flex-col h-[500px]">
      {/* Status bar */}
      <div className="flex items-center justify-between px-4 py-2 border-b bg-secondary/30 text-xs shrink-0">
        <div className="flex items-center gap-3">
          {streaming ? (
            <>
              <div className="flex items-center gap-1.5">
                <div className="h-2 w-2 rounded-full bg-emerald-400 animate-pulse" />
                <span className="text-emerald-400 font-medium">Running</span>
              </div>
              <span className="text-muted-foreground">Step {steps.length}{lastStep ? ` — ${lastStep.action || 'thinking'}` : ''}</span>
            </>
          ) : (
            <div className="flex items-center gap-1.5">
              <CheckCircle2 className="h-3 w-3 text-emerald-400" />
              <span className="text-muted-foreground">
                Completed — {steps.length} steps in {formatElapsed(finalElapsed || elapsed)}
              </span>
            </div>
          )}
        </div>
        <div className="flex items-center gap-1 text-muted-foreground">
          <Timer className="h-3 w-3" />
          {formatElapsed(streaming ? elapsed : (finalElapsed || elapsed))}
        </div>
      </div>

      {/* Steps list */}
      <div className="flex-1 overflow-auto p-2 space-y-0.5">
        {steps.map((step, i) => (
          <CollapsibleStep key={i} step={step} />
        ))}

        {/* Live streaming indicator */}
        {streaming && (
          <div className="space-y-1 px-1.5 py-1">
            <div className="flex items-center gap-2 text-xs text-muted-foreground font-medium">
              <Clock className="h-3 w-3 animate-spin shrink-0" />
              <span className="px-1.5 py-0.5 rounded text-[10px] font-bold bg-primary/20 text-primary animate-pulse">
                {steps.length + 1}
              </span>
              <span className="animate-pulse">thinking...</span>
            </div>

            {streamChunk && (
              <div className="ml-6 flex gap-2 pl-2 border-l-2 border-primary/30">
                <Brain className="h-3.5 w-3.5 text-primary shrink-0 mt-0.5 animate-pulse" />
                <pre className="text-xs text-foreground/50 font-mono whitespace-pre-wrap max-h-24 overflow-auto">
                  {streamChunk.slice(-500)}
                </pre>
              </div>
            )}
          </div>
        )}

        <div ref={bottomRef} />
      </div>
    </div>
  )
}
