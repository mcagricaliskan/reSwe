import { useState, useEffect, useCallback, useRef } from 'react'
import * as api from './api'
import type { ProjectFile } from './api'

export interface FileMentionState {
  isActive: boolean
  query: string
  results: ProjectFile[]
  selectedIndex: number
  mentionStart: number
  loading: boolean
}

export function useFileMention(projectId: number | string | undefined) {
  const [isActive, setIsActive] = useState(false)
  const [query, setQuery] = useState('')
  const [results, setResults] = useState<ProjectFile[]>([])
  const [selectedIndex, setSelectedIndex] = useState(0)
  const [mentionStart, setMentionStart] = useState(0)
  const [loading, setLoading] = useState(false)
  const debounceRef = useRef<ReturnType<typeof setTimeout>>(null)

  useEffect(() => {
    if (!isActive || !projectId) {
      setResults([])
      return
    }
    if (!query) {
      // Show recent files when @ is typed with no query yet
      setLoading(true)
      api.searchProjectFiles(projectId, '').then(r => {
        setResults(r || [])
        setLoading(false)
      }).catch(() => setLoading(false))
      return
    }

    setLoading(true)
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => {
      api.searchProjectFiles(projectId, query).then(r => {
        setResults(r || [])
        setSelectedIndex(0)
        setLoading(false)
      }).catch(() => setLoading(false))
    }, 150)

    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current)
    }
  }, [isActive, query, projectId])

  const activate = useCallback((cursorPos: number) => {
    setIsActive(true)
    setMentionStart(cursorPos)
    setQuery('')
    setSelectedIndex(0)
    setResults([])
  }, [])

  const deactivate = useCallback(() => {
    setIsActive(false)
    setQuery('')
    setResults([])
    setSelectedIndex(0)
  }, [])

  return {
    isActive,
    query,
    results,
    selectedIndex,
    mentionStart,
    loading,
    setQuery,
    setSelectedIndex,
    activate,
    deactivate,
  }
}
