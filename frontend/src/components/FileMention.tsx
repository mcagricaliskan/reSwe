import { useEffect, useRef } from 'react'
import type { ProjectFile } from '../lib/api'
import { File, Folder } from 'lucide-react'

interface Props {
  results: ProjectFile[]
  selectedIndex: number
  loading: boolean
  onSelect: (file: ProjectFile) => void
  onHover: (index: number) => void
}

export function FileMention({ results, selectedIndex, loading, onSelect, onHover }: Props) {
  const listRef = useRef<HTMLDivElement>(null)
  const selectedRef = useRef<HTMLButtonElement>(null)

  useEffect(() => {
    selectedRef.current?.scrollIntoView({ block: 'nearest' })
  }, [selectedIndex])

  if (!loading && results.length === 0) {
    return (
      <div className="absolute bottom-full mb-1 left-0 w-full bg-popover border rounded-lg shadow-lg z-50 p-2">
        <p className="text-xs text-muted-foreground text-center">No files found</p>
      </div>
    )
  }

  return (
    <div
      ref={listRef}
      className="absolute bottom-full mb-1 left-0 w-full max-h-52 overflow-auto bg-popover border rounded-lg shadow-lg z-50 py-1"
    >
      {loading && results.length === 0 && (
        <p className="text-xs text-muted-foreground text-center py-2">Searching...</p>
      )}
      {results.map((file, i) => (
        <button
          key={file.id}
          ref={i === selectedIndex ? selectedRef : undefined}
          className={`w-full text-left px-2 py-1 flex items-center gap-2 text-sm hover:bg-accent transition-colors ${
            i === selectedIndex ? 'bg-accent' : ''
          }`}
          onMouseEnter={() => onHover(i)}
          onMouseDown={e => {
            e.preventDefault()
            onSelect(file)
          }}
        >
          {file.is_dir ? (
            <Folder className="h-3 w-3 shrink-0 text-amber-500" />
          ) : (
            <File className="h-3 w-3 shrink-0 text-muted-foreground" />
          )}
          <span className="font-mono text-xs truncate">{file.rel_path}</span>
          <span className="text-[10px] text-muted-foreground shrink-0 ml-auto">
            {file.is_dir
              ? 'folder'
              : file.size < 1024
                ? `${file.size}B`
                : `${(file.size / 1024).toFixed(1)}KB`}
          </span>
        </button>
      ))}
    </div>
  )
}
