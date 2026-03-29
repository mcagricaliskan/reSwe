import { useState, useEffect } from 'react'
import { useParams, Link } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'
import { ChevronRight, Shield, X, Plus, Loader2 } from 'lucide-react'
import { toast } from 'sonner'
import type { Project, ProjectExcludeConfig } from '@/lib/api'
import * as api from '@/lib/api'

export default function ProjectSettingsPage() {
  const { id } = useParams<{ id: string }>()
  const [project, setProject] = useState<Project | null>(null)
  const [excludeConfig, setExcludeConfig] = useState<ProjectExcludeConfig | null>(null)
  const [newCustomPattern, setNewCustomPattern] = useState('')
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    if (id) loadData()
  }, [id])

  async function loadData() {
    setLoading(true)
    try {
      const [proj, config] = await Promise.all([
        api.getProject(id!),
        api.getProjectExcludeConfig(id!),
      ])
      setProject(proj)
      setExcludeConfig(config)
    } catch {
      toast.error('Failed to load project settings')
    } finally {
      setLoading(false)
    }
  }

  async function loadExcludeConfig() {
    try {
      setExcludeConfig(await api.getProjectExcludeConfig(id!))
    } catch { /* non-critical */ }
  }

  async function handleToggleRule(ruleId: number, currentEnabled: boolean) {
    if (!excludeConfig) return
    // Optimistic
    setExcludeConfig(prev => prev ? {
      ...prev,
      rules: prev.rules.map(r =>
        r.id === ruleId ? { ...r, enabled: !r.enabled, overridden: true } : r
      ),
    } : prev)
    try {
      await api.setProjectExcludeOverride(id!, ruleId, !currentEnabled)
      loadExcludeConfig()
    } catch {
      loadExcludeConfig()
      toast.error('Failed to update rule')
    }
  }

  async function handleResetRule(ruleId: number) {
    try {
      await api.deleteProjectExcludeOverride(id!, ruleId)
      loadExcludeConfig()
    } catch {
      toast.error('Failed to reset rule')
    }
  }

  async function handleAddCustomPattern(e: React.FormEvent) {
    e.preventDefault()
    if (!newCustomPattern.trim()) return
    try {
      await api.addProjectCustomPattern(id!, newCustomPattern.trim())
      setNewCustomPattern('')
      loadExcludeConfig()
    } catch {
      toast.error('Failed to add pattern')
    }
  }

  async function handleDeleteCustomPattern(patternId: number) {
    if (!excludeConfig) return
    // Optimistic
    setExcludeConfig(prev => prev ? {
      ...prev,
      custom_patterns: prev.custom_patterns.filter(p => p.id !== patternId),
    } : prev)
    try {
      await api.deleteProjectCustomPattern(patternId)
      loadExcludeConfig()
    } catch {
      loadExcludeConfig()
      toast.error('Failed to delete pattern')
    }
  }

  if (loading || !project) {
    return (
      <div className="flex items-center justify-center h-64">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    )
  }

  return (
    <div>
      {/* Breadcrumb */}
      <div className="flex items-center gap-1 text-sm text-muted-foreground mb-4">
        <Link to="/" className="hover:text-foreground transition-colors">Projects</Link>
        <ChevronRight className="h-3 w-3" />
        <Link to={`/projects/${id}`} className="hover:text-foreground transition-colors truncate">{project.name}</Link>
        <ChevronRight className="h-3 w-3" />
        <span className="text-foreground">Settings</span>
      </div>

      <h1 className="text-xl font-semibold tracking-tight mb-1">{project.name}</h1>
      <p className="text-sm text-muted-foreground mb-6">Project settings</p>

      <Tabs defaultValue="exclude-patterns">
        <TabsList variant="line">
          <TabsTrigger value="exclude-patterns">
            <Shield className="h-3.5 w-3.5" />
            Exclude Patterns
          </TabsTrigger>
          {/* Future tabs go here */}
        </TabsList>

        <TabsContent value="exclude-patterns" className="pt-4">
          {excludeConfig ? (
            <div className="space-y-6">
              {/* Global rules with per-project on/off */}
              <div>
                <div className="flex items-center justify-between mb-2">
                  <h3 className="text-sm font-medium">Global Rules</h3>
                  <Link to="/settings/exclude-patterns" className="text-xs text-muted-foreground hover:text-foreground underline">
                    Manage global rules
                  </Link>
                </div>
                <p className="text-xs text-muted-foreground mb-3">
                  Toggle rules on/off for this project. Overrides are highlighted — click the x to reset to global default.
                </p>
                {excludeConfig.rules.length > 0 ? (
                  <div className="flex flex-wrap gap-1.5">
                    {excludeConfig.rules.map(rule => (
                      <Badge
                        key={rule.id}
                        variant={rule.enabled ? 'default' : 'outline'}
                        className={`cursor-pointer text-xs font-mono gap-1 ${rule.overridden ? 'ring-1 ring-primary/50' : ''}`}
                        onClick={() => handleToggleRule(rule.id, rule.enabled)}
                      >
                        {rule.pattern}
                        {rule.overridden && (
                          <button
                            onClick={(e) => { e.stopPropagation(); handleResetRule(rule.id) }}
                            className="ml-0.5 opacity-60 hover:opacity-100"
                            title="Reset to global default"
                          >
                            <X className="h-3 w-3" />
                          </button>
                        )}
                      </Badge>
                    ))}
                  </div>
                ) : (
                  <p className="text-xs text-muted-foreground">
                    No global rules defined. <Link to="/settings/exclude-patterns" className="underline">Add some in Settings</Link>
                  </p>
                )}
              </div>

              {/* Custom patterns */}
              <div>
                <h3 className="text-sm font-medium mb-2">Custom Patterns</h3>
                <p className="text-xs text-muted-foreground mb-3">
                  Additional patterns specific to this project.
                </p>
                <div className="flex flex-wrap gap-1.5 mb-3">
                  {excludeConfig.custom_patterns.map(cp => (
                    <Badge key={cp.id} variant="secondary" className="text-xs font-mono gap-1">
                      {cp.pattern}
                      <button onClick={() => handleDeleteCustomPattern(cp.id)} className="hover:text-destructive">
                        <X className="h-3 w-3" />
                      </button>
                    </Badge>
                  ))}
                  {excludeConfig.custom_patterns.length === 0 && (
                    <p className="text-xs text-muted-foreground">No custom patterns added.</p>
                  )}
                </div>
                <form onSubmit={handleAddCustomPattern} className="flex gap-2 max-w-md">
                  <Input
                    placeholder="Add pattern (e.g. *.log, secrets/)"
                    value={newCustomPattern}
                    onChange={e => setNewCustomPattern(e.target.value)}
                    className="text-sm h-8 font-mono"
                  />
                  <Button type="submit" size="sm" variant="outline" className="h-8 shrink-0" disabled={!newCustomPattern.trim()}>
                    <Plus className="h-3 w-3 mr-1" />
                    Add
                  </Button>
                </form>
              </div>

              {/* Effective summary */}
              {excludeConfig.effective.length > 0 && (
                <div>
                  <h3 className="text-sm font-medium mb-2">Effective Patterns ({excludeConfig.effective.length})</h3>
                  <p className="text-xs text-muted-foreground mb-2">All active patterns merged — these files will be blocked from the AI agent.</p>
                  <div className="flex flex-wrap gap-1">
                    {excludeConfig.effective.map((pat, i) => (
                      <code key={i} className="text-xs bg-secondary px-1.5 py-0.5 rounded">{pat}</code>
                    ))}
                  </div>
                </div>
              )}
            </div>
          ) : (
            <Card>
              <CardContent className="py-8 text-center text-muted-foreground text-sm">
                Failed to load exclude configuration.
              </CardContent>
            </Card>
          )}
        </TabsContent>
      </Tabs>
    </div>
  )
}
