import { useState, useEffect, useRef } from 'react'
import { Brain, Wrench, Eye, CheckCircle2, ChevronDown, ChevronRight, Clock, Timer } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

export interface AgentStep {
  step: number
  think: string
  action: string
  action_arg: string
  observation: string
  is_final: boolean
  phase: string
}

interface AgentStepsProps {
  steps: AgentStep[]
  streaming: boolean
  streamChunk: string // current raw LLM tokens flowing in
  startTime?: number  // Date.now() when agent started
}

function formatElapsed(ms: number): string {
  const s = Math.floor(ms / 1000)
  if (s < 60) return `${s}s`
  const m = Math.floor(s / 60)
  const rem = s % 60
  return `${m}m ${rem}s`
}

function ObservationBlock({ text }: { text: string }) {
  const [expanded, setExpanded] = useState(false)
  const isLong = text.length > 500

  return (
    <div className="flex gap-2 pl-2 border-l-2 border-amber-500/30">
      <Eye className="h-4 w-4 text-amber-400 shrink-0 mt-0.5" />
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <span className="text-xs font-semibold text-amber-400 uppercase">Observation</span>
          {isLong && (
            <Button
              variant="ghost"
              size="xs"
              className="h-5 px-1 text-xs text-muted-foreground"
              onClick={() => setExpanded(!expanded)}
            >
              {expanded ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />}
              {expanded ? 'collapse' : `${text.length} chars`}
            </Button>
          )}
        </div>
        <pre className={cn(
          "text-xs text-foreground/60 mt-0.5 whitespace-pre-wrap bg-secondary/50 rounded p-2 overflow-auto",
          expanded ? "max-h-96" : "max-h-32"
        )}>
          {!expanded && isLong
            ? text.slice(0, 500) + '\n... (click to expand)'
            : text}
        </pre>
      </div>
    </div>
  )
}

export default function AgentSteps({ steps, streaming, streamChunk, startTime }: AgentStepsProps) {
  const [elapsed, setElapsed] = useState(0)
  const [finalElapsed, setFinalElapsed] = useState(0)
  const bottomRef = useRef<HTMLDivElement>(null)

  // Timer — ticks while running, freezes final value when done
  useEffect(() => {
    if (!streaming || !startTime) return
    setFinalElapsed(0)
    const interval = setInterval(() => {
      setElapsed(Date.now() - startTime)
    }, 1000)
    return () => {
      clearInterval(interval)
      // Capture final elapsed when streaming stops
      if (startTime) {
        setFinalElapsed(Date.now() - startTime)
      }
    }
  }, [streaming, startTime])

  // Auto-scroll to bottom
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
      <div className="flex-1 overflow-auto p-4 space-y-3">
        {steps.map((step, i) => (
          <div key={i} className="space-y-1.5">
            {/* Step header */}
            <div className="flex items-center gap-2 text-xs text-muted-foreground font-medium">
              <span className={cn(
                "px-1.5 py-0.5 rounded text-[10px] font-bold",
                step.is_final ? "bg-emerald-500/20 text-emerald-400" : "bg-secondary text-muted-foreground"
              )}>
                {step.step}
              </span>
              {step.action && (
                <span className="text-blue-400 font-mono">{step.action}</span>
              )}
              {step.is_final && <span className="text-emerald-400">done</span>}
            </div>

            {/* Think */}
            {step.think && (
              <div className="flex gap-2 pl-2 border-l-2 border-violet-500/30">
                <Brain className="h-3.5 w-3.5 text-violet-400 shrink-0 mt-0.5" />
                <p className="text-sm text-foreground/80">{step.think}</p>
              </div>
            )}

            {/* Action */}
            {step.action && step.action !== 'done' && (
              <div className="flex gap-2 pl-2 border-l-2 border-blue-500/30">
                <Wrench className="h-3.5 w-3.5 text-blue-400 shrink-0 mt-0.5" />
                <code className="text-xs font-mono text-blue-300 break-all">
                  {step.action}({step.action_arg.length > 100 ? step.action_arg.slice(0, 100) + '...' : step.action_arg})
                </code>
              </div>
            )}

            {/* Observation */}
            {step.observation && !step.is_final && (
              <ObservationBlock text={step.observation} />
            )}

            {/* Final result */}
            {step.is_final && step.action_arg && (
              <div className="flex gap-2 pl-2 border-l-2 border-emerald-500/30">
                <CheckCircle2 className="h-3.5 w-3.5 text-emerald-400 shrink-0 mt-0.5" />
                <div className="min-w-0 flex-1">
                  <span className="text-xs font-semibold text-emerald-400 uppercase">Final Result</span>
                  <pre className="text-sm text-foreground/80 mt-1 whitespace-pre-wrap bg-secondary/30 rounded p-3 max-h-96 overflow-auto">
                    {step.action_arg}
                  </pre>
                </div>
              </div>
            )}
          </div>
        ))}

        {/* Live streaming indicator — shows actual tokens flowing */}
        {streaming && (
          <div className="space-y-1.5">
            <div className="flex items-center gap-2 text-xs text-muted-foreground font-medium">
              <span className="px-1.5 py-0.5 rounded text-[10px] font-bold bg-primary/20 text-primary animate-pulse">
                {steps.length + 1}
              </span>
              <span className="animate-pulse">thinking...</span>
              <Clock className="h-3 w-3 animate-spin" />
            </div>

            {streamChunk && (
              <div className="flex gap-2 pl-2 border-l-2 border-primary/30">
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
