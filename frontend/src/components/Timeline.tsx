import { CheckCircle2, FileText, Play, MessageSquare, AlertCircle, Clock, XCircle, Edit, ChevronDown, ChevronRight } from 'lucide-react'
import { useState } from 'react'
import type { TimelineEvent } from '@/lib/api'

const eventConfig: Record<string, { icon: typeof CheckCircle2; color: string }> = {
  plan_created:       { icon: FileText,     color: 'text-emerald-400' },
  plan_revised:       { icon: Edit,         color: 'text-blue-400' },
  plan_waiting:       { icon: Clock,        color: 'text-amber-400' },
  plan_error:         { icon: AlertCircle,  color: 'text-red-400' },
  execution_completed:{ icon: CheckCircle2, color: 'text-emerald-400' },
  execution_error:    { icon: AlertCircle,  color: 'text-red-400' },
  todo_executed:      { icon: Play,         color: 'text-blue-400' },
  todo_error:         { icon: XCircle,      color: 'text-red-400' },
  change_accepted:    { icon: CheckCircle2, color: 'text-emerald-400' },
  change_rejected:    { icon: XCircle,      color: 'text-red-400' },
  change_pending:     { icon: Clock,        color: 'text-amber-400' },
  chat:               { icon: MessageSquare, color: 'text-muted-foreground' },
}

function formatTimeAgo(dateStr: string): string {
  const now = Date.now()
  const then = new Date(dateStr).getTime()
  const diffMs = now - then
  const mins = Math.floor(diffMs / 60000)
  if (mins < 1) return 'just now'
  if (mins < 60) return `${mins}m ago`
  const hours = Math.floor(mins / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  return `${days}d ago`
}

function formatDuration(ms: unknown): string {
  if (typeof ms !== 'number' || ms <= 0) return ''
  const s = Math.floor(ms / 1000)
  if (s < 60) return `${s}s`
  const m = Math.floor(s / 60)
  return `${m}m ${s % 60}s`
}

interface TimelineProps {
  events: TimelineEvent[]
  onViewRun?: (runId: number) => void
  onViewChange?: (changeId: string) => void
}

export default function Timeline({ events, onViewRun, onViewChange }: TimelineProps) {
  const [expandedId, setExpandedId] = useState<string | null>(null)

  if (events.length === 0) {
    return (
      <div className="text-center py-8 text-muted-foreground">
        <p className="text-sm">No activity yet</p>
      </div>
    )
  }

  return (
    <div className="space-y-0">
      {events.map((event, i) => {
        const config = eventConfig[event.type] || { icon: MessageSquare, color: 'text-muted-foreground' }
        const Icon = config.icon
        const isExpanded = expandedId === event.id
        const duration = event.metadata?.duration_ms
        const steps = event.metadata?.step_count
        const file = event.metadata?.file as string | undefined
        const isLast = i === events.length - 1

        return (
          <div key={event.id} className="flex gap-3">
            {/* Vertical line + icon */}
            <div className="flex flex-col items-center shrink-0 w-6">
              <div className={`w-5 h-5 rounded-full flex items-center justify-center ${config.color} bg-secondary`}>
                <Icon className="h-3 w-3" />
              </div>
              {!isLast && <div className="w-px flex-1 bg-border mt-1" />}
            </div>

            {/* Content */}
            <div className="flex-1 min-w-0 pb-4">
              <button
                className="w-full text-left hover:bg-accent/30 rounded px-2 py-1 -mx-2 -my-1 transition-colors"
                onClick={() => setExpandedId(isExpanded ? null : event.id)}
              >
                <div className="flex items-center gap-2">
                  <span className="text-sm font-medium">{event.title}</span>
                  <span className="text-[10px] text-muted-foreground ml-auto shrink-0">{formatTimeAgo(event.created_at)}</span>
                  {isExpanded ? <ChevronDown className="h-3 w-3 text-muted-foreground" /> : <ChevronRight className="h-3 w-3 text-muted-foreground" />}
                </div>
                <div className="flex items-center gap-2 mt-0.5 text-[10px] text-muted-foreground">
                  {file && <span className="font-mono">{file}</span>}
                  {typeof steps === 'number' && steps > 0 && <span>{steps} steps</span>}
                  {duration != null && <span>{formatDuration(duration)}</span>}
                  {event.status && <span className="capitalize">{event.status}</span>}
                </div>
              </button>

              {isExpanded && (
                <div className="mt-2 px-2 space-y-2">
                  {event.description && (
                    <p className="text-xs text-muted-foreground whitespace-pre-wrap">{event.description}</p>
                  )}
                  <div className="flex gap-2">
                    {event.run_id && onViewRun && (
                      <button
                        className="text-[10px] text-primary hover:underline"
                        onClick={(e) => { e.stopPropagation(); onViewRun(event.run_id!) }}
                      >
                        View agent steps
                      </button>
                    )}
                    {event.change_id && onViewChange && (
                      <button
                        className="text-[10px] text-primary hover:underline"
                        onClick={(e) => { e.stopPropagation(); onViewChange(event.change_id!) }}
                      >
                        View diff
                      </button>
                    )}
                  </div>
                </div>
              )}
            </div>
          </div>
        )
      })}
    </div>
  )
}
