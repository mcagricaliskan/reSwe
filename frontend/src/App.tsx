import { useEffect, useState } from 'react'
import { BrowserRouter, Routes, Route, Link, useLocation } from 'react-router-dom'
import { Toaster } from '@/components/ui/sonner'
import { Button } from '@/components/ui/button'
import { wsClient } from '@/lib/ws'
import { FolderGit2, LayoutDashboard, Menu, X } from 'lucide-react'
import ProjectsPage from '@/pages/Projects'
import ProjectPage from '@/pages/Project'
import TaskPage from '@/pages/Task'
import { cn } from '@/lib/utils'

function Sidebar({ open, onClose }: { open: boolean; onClose: () => void }) {
  const location = useLocation()

  const navItems = [
    { to: '/', label: 'Projects', icon: FolderGit2 },
  ]

  // Close sidebar on navigation (mobile)
  useEffect(() => {
    onClose()
  }, [location.pathname])

  return (
    <>
      {/* Overlay for mobile */}
      {open && (
        <div
          className="fixed inset-0 z-40 bg-black/50 md:hidden"
          onClick={onClose}
        />
      )}

      <aside className={cn(
        'fixed inset-y-0 left-0 z-50 w-60 border-r bg-card flex flex-col transition-transform duration-200 ease-in-out md:relative md:translate-x-0',
        open ? 'translate-x-0' : '-translate-x-full'
      )}>
        <div className="flex items-center justify-between p-4 border-b">
          <Link to="/" className="flex items-center gap-2">
            <LayoutDashboard className="h-5 w-5 text-primary" />
            <span className="text-lg font-bold tracking-tight">reSwe</span>
          </Link>
          <Button variant="ghost" size="icon" className="md:hidden h-8 w-8" onClick={onClose}>
            <X className="h-4 w-4" />
          </Button>
        </div>
        <p className="text-xs text-muted-foreground px-4 pt-2 pb-1">SWE Agent Orchestrator</p>
        <nav className="flex-1 p-2 space-y-1">
          {navItems.map(item => {
            const active = location.pathname === item.to ||
              (item.to !== '/' && location.pathname.startsWith(item.to))
            return (
              <Link
                key={item.to}
                to={item.to}
                className={cn(
                  'flex items-center gap-2 px-3 py-2 rounded-md text-sm font-medium transition-colors',
                  active
                    ? 'bg-accent text-accent-foreground'
                    : 'text-muted-foreground hover:bg-accent/50 hover:text-foreground'
                )}
              >
                <item.icon className="h-4 w-4" />
                {item.label}
              </Link>
            )
          })}
        </nav>
      </aside>
    </>
  )
}

export default function App() {
  const [sidebarOpen, setSidebarOpen] = useState(false)

  useEffect(() => {
    wsClient.connect()
    return () => wsClient.disconnect()
  }, [])

  return (
    <BrowserRouter>
      <div className="flex h-screen overflow-hidden bg-background">
        <Sidebar open={sidebarOpen} onClose={() => setSidebarOpen(false)} />

        <div className="flex-1 flex flex-col min-w-0 overflow-hidden">
          {/* Mobile header */}
          <header className="flex items-center gap-3 p-3 border-b md:hidden">
            <Button variant="ghost" size="icon" className="h-8 w-8" onClick={() => setSidebarOpen(true)}>
              <Menu className="h-5 w-5" />
            </Button>
            <span className="font-semibold text-sm">reSwe</span>
          </header>

          <main className="flex-1 overflow-auto">
            <div className="p-4 sm:p-6 max-w-5xl mx-auto w-full">
              <Routes>
                <Route path="/" element={<ProjectsPage />} />
                <Route path="/projects/:id" element={<ProjectPage />} />
                <Route path="/tasks/:id" element={<TaskPage />} />
              </Routes>
            </div>
          </main>
        </div>
      </div>
      <Toaster />
    </BrowserRouter>
  )
}
