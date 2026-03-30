import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Check, X, FileText } from 'lucide-react'

interface DiffViewerProps {
  changeId: string
  filePath: string
  diff: string
  status: 'pending' | 'accepted' | 'rejected'
  onAccept: (changeId: string) => void
  onReject: (changeId: string, reason: string) => void
}

function parseDiffLines(diff: string) {
  return diff.split('\n').map((line, i) => {
    let type: 'add' | 'remove' | 'header' | 'context' = 'context'
    if (line.startsWith('+') && !line.startsWith('+++')) type = 'add'
    else if (line.startsWith('-') && !line.startsWith('---')) type = 'remove'
    else if (line.startsWith('@@') || line.startsWith('---') || line.startsWith('+++')) type = 'header'
    return { line, type, key: i }
  })
}

const lineStyles = {
  add: 'bg-emerald-500/15 text-emerald-300',
  remove: 'bg-red-500/15 text-red-300',
  header: 'text-muted-foreground/60 font-medium',
  context: 'text-foreground/70',
}

export default function DiffViewer({ changeId, filePath, diff, status, onAccept, onReject }: DiffViewerProps) {
  const [rejectReason, setRejectReason] = useState('')
  const [showRejectInput, setShowRejectInput] = useState(false)
  const lines = parseDiffLines(diff)

  return (
    <div className="border rounded-lg overflow-hidden my-2">
      {/* File header */}
      <div className="px-3 py-1.5 bg-card border-b flex items-center justify-between">
        <div className="flex items-center gap-2 text-xs">
          <FileText className="h-3 w-3 text-muted-foreground" />
          <span className="font-mono font-medium">{filePath}</span>
        </div>
        {status === 'accepted' && (
          <span className="text-[10px] text-emerald-400 font-medium flex items-center gap-1">
            <Check className="h-3 w-3" /> Accepted
          </span>
        )}
        {status === 'rejected' && (
          <span className="text-[10px] text-red-400 font-medium flex items-center gap-1">
            <X className="h-3 w-3" /> Rejected
          </span>
        )}
      </div>

      {/* Diff content */}
      <div className="overflow-x-auto max-h-80 overflow-y-auto">
        <pre className="text-xs leading-5">
          {lines.map(({ line, type, key }) => (
            <div key={key} className={`px-3 ${lineStyles[type]}`}>
              {line}
            </div>
          ))}
        </pre>
      </div>

      {/* Action buttons */}
      {status === 'pending' && (
        <div className="px-3 py-2 border-t bg-card space-y-2">
          <div className="flex items-center gap-2">
            <Button size="xs" variant="success" onClick={() => onAccept(changeId)} className="gap-1">
              <Check className="h-3 w-3" /> Accept
            </Button>
            <Button
              size="xs"
              variant="destructive"
              onClick={() => {
                if (showRejectInput && rejectReason.trim()) {
                  onReject(changeId, rejectReason)
                } else {
                  setShowRejectInput(true)
                }
              }}
              className="gap-1"
            >
              <X className="h-3 w-3" /> Reject
            </Button>
          </div>
          {showRejectInput && (
            <div className="flex gap-2">
              <Input
                placeholder="Why? (optional — helps the agent adjust)"
                value={rejectReason}
                onChange={e => setRejectReason(e.target.value)}
                onKeyDown={e => {
                  if (e.key === 'Enter') onReject(changeId, rejectReason)
                }}
                className="h-7 text-xs"
                autoFocus
              />
              <Button size="xs" variant="outline" onClick={() => onReject(changeId, rejectReason)}>
                Send
              </Button>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
