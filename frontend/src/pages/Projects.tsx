import { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Plus, Trash2, FolderOpen } from 'lucide-react'
import { toast } from 'sonner'
import type { Project } from '@/lib/api'
import * as api from '@/lib/api'

export default function ProjectsPage() {
  const [projects, setProjects] = useState<Project[]>([])
  const [showForm, setShowForm] = useState(false)
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [loading, setLoading] = useState(true)

  useEffect(() => { loadProjects() }, [])

  async function loadProjects() {
    try {
      const data = await api.listProjects()
      setProjects(data || [])
    } catch {
      toast.error('Failed to load projects')
    } finally {
      setLoading(false)
    }
  }

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault()
    if (!name.trim()) return
    try {
      await api.createProject(name, description)
      setName('')
      setDescription('')
      setShowForm(false)
      toast.success('Project created')
      loadProjects()
    } catch {
      toast.error('Failed to create project')
    }
  }

  async function handleDelete(id: number) {
    if (!confirm('Delete this project and all its tasks?')) return
    try {
      await api.deleteProject(id)
      toast.success('Project deleted')
      loadProjects()
    } catch {
      toast.error('Failed to delete project')
    }
  }

  if (loading) return <p className="text-muted-foreground">Loading...</p>

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-semibold tracking-tight">Projects</h1>
        <Button onClick={() => setShowForm(!showForm)} size="sm">
          <Plus className="h-4 w-4 mr-1" />
          {showForm ? 'Cancel' : 'New Project'}
        </Button>
      </div>

      {showForm && (
        <Card className="mb-6">
          <CardContent className="pt-6">
            <form onSubmit={handleCreate} className="space-y-3">
              <Input
                placeholder="Project name"
                value={name}
                onChange={e => setName(e.target.value)}
                autoFocus
              />
              <Textarea
                placeholder="Description (optional)"
                value={description}
                onChange={e => setDescription(e.target.value)}
                rows={2}
              />
              <Button type="submit" size="sm">Create Project</Button>
            </form>
          </CardContent>
        </Card>
      )}

      {projects.length === 0 ? (
        <Card>
          <CardContent className="flex flex-col items-center justify-center py-12 text-muted-foreground">
            <FolderOpen className="h-10 w-10 mb-3 opacity-40" />
            <p>No projects yet. Create one to get started.</p>
          </CardContent>
        </Card>
      ) : (
        <div className="space-y-2">
          {projects.map(p => (
            <Card key={p.id} className="hover:border-primary/30 transition-colors">
              <CardContent className="flex items-center justify-between py-4">
                <Link to={`/projects/${p.id}`} className="flex-1 min-w-0">
                  <div className="font-medium">{p.name}</div>
                  {p.description && (
                    <p className="text-sm text-muted-foreground truncate mt-0.5">{p.description}</p>
                  )}
                </Link>
                <div className="flex items-center gap-3 ml-4">
                  <span className="text-xs text-muted-foreground">
                    {new Date(p.updated_at).toLocaleDateString()}
                  </span>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-8 w-8 text-muted-foreground hover:text-destructive"
                    onClick={() => handleDelete(p.id)}
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      )}
    </div>
  )
}
