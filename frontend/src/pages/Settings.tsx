import { useState, useEffect } from 'react'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Plus, Trash2, Loader2 } from 'lucide-react'
import { toast } from 'sonner'
import type { ExcludeRule } from '@/lib/api'
import * as api from '@/lib/api'

export default function SettingsPage() {
  const [rules, setRules] = useState<ExcludeRule[]>([])
  const [loading, setLoading] = useState(true)
  const [showAddForm, setShowAddForm] = useState(false)
  const [newPattern, setNewPattern] = useState('')
  const [newEnabled, setNewEnabled] = useState(true)

  useEffect(() => { loadRules() }, [])

  async function loadRules() {
    setLoading(true)
    try {
      setRules(await api.listExcludeRules())
    } catch {
      toast.error('Failed to load rules')
    } finally {
      setLoading(false)
    }
  }

  async function handleAdd(e: React.FormEvent) {
    e.preventDefault()
    if (!newPattern.trim()) return
    try {
      await api.createExcludeRule(newPattern.trim(), newEnabled)
      setNewPattern('')
      setNewEnabled(true)
      setShowAddForm(false)
      toast.success('Rule added')
      loadRules()
    } catch {
      toast.error('Failed to add rule')
    }
  }

  async function handleToggle(rule: ExcludeRule) {
    // Optimistic update — flip immediately, revert on error
    setRules(prev => prev.map(r =>
      r.id === rule.id ? { ...r, enabled_by_default: !r.enabled_by_default } : r
    ))
    try {
      await api.updateExcludeRule(rule.id, rule.pattern, !rule.enabled_by_default)
    } catch {
      // Revert
      setRules(prev => prev.map(r =>
        r.id === rule.id ? { ...r, enabled_by_default: rule.enabled_by_default } : r
      ))
      toast.error('Failed to update rule')
    }
  }

  async function handleDelete(id: number) {
    // Optimistic
    const prev = rules
    setRules(rules.filter(r => r.id !== id))
    try {
      await api.deleteExcludeRule(id)
    } catch {
      setRules(prev)
      toast.error('Failed to delete rule')
    }
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    )
  }

  return (
    <div className="max-w-3xl mx-auto">
      <h1 className="text-xl font-bold mb-1">Exclude Patterns</h1>
      <p className="text-sm text-muted-foreground mb-6">
        Patterns that block the AI agent from reading files. Toggle on/off globally — projects inherit these defaults but can override individually.
      </p>

      <div className="flex justify-end mb-3">
        <Button variant="outline" size="xs" onClick={() => setShowAddForm(!showAddForm)}>
          <Plus className="h-3 w-3 mr-1" />
          {showAddForm ? 'Cancel' : 'Add Pattern'}
        </Button>
      </div>

      {showAddForm && (
        <Card className="mb-4">
          <CardContent className="pt-4">
            <form onSubmit={handleAdd} className="flex items-center gap-2">
              <Input
                placeholder="Pattern (e.g. *.env, *.key, secrets/)"
                value={newPattern}
                onChange={e => setNewPattern(e.target.value)}
                className="font-mono text-sm"
                autoFocus
              />
              <label className="flex items-center gap-1.5 text-sm shrink-0 cursor-pointer">
                <input
                  type="checkbox"
                  checked={newEnabled}
                  onChange={e => setNewEnabled(e.target.checked)}
                  className="rounded"
                />
                On by default
              </label>
              <Button type="submit" size="sm" className="shrink-0">Add</Button>
            </form>
          </CardContent>
        </Card>
      )}

      {rules.length === 0 ? (
        <Card>
          <CardContent className="py-8 text-center text-muted-foreground text-sm">
            No exclude patterns defined. Add patterns to protect sensitive files.
          </CardContent>
        </Card>
      ) : (
        <div className="space-y-1">
          {rules.map(rule => (
            <div
              key={rule.id}
              className="flex items-center justify-between py-2 px-3 rounded-md border bg-card gap-2"
            >
              <div className="flex items-center gap-3 min-w-0">
                <button
                  onClick={() => handleToggle(rule)}
                  className={`w-11 h-6 rounded-full transition-colors relative shrink-0 flex items-center px-1 ${
                    rule.enabled_by_default ? 'bg-emerald-600/80' : 'bg-zinc-600/50'
                  }`}
                >
                  <span className={`text-[9px] font-semibold uppercase absolute ${
                    rule.enabled_by_default ? 'left-1.5 text-emerald-100' : 'right-1 text-zinc-300'
                  }`}>
                    {rule.enabled_by_default ? 'on' : 'off'}
                  </span>
                  <span className={`w-4 h-4 rounded-full bg-zinc-200 transition-transform shadow-sm ${
                    rule.enabled_by_default ? 'translate-x-[22px]' : 'translate-x-0'
                  }`} />
                </button>
                <code className="text-sm font-mono">{rule.pattern}</code>
              </div>
              <Button
                variant="ghost"
                size="icon"
                className="h-7 w-7 text-muted-foreground hover:text-destructive shrink-0"
                onClick={() => handleDelete(rule.id)}
              >
                <Trash2 className="h-3 w-3" />
              </Button>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
