import { useEffect, useState } from 'react'
import { BrowserRouter, Routes, Route, Link, useLocation } from 'react-router-dom'
import { Toaster } from '@/components/ui/sonner'
import { Button } from '@/components/ui/button'
import { wsClient } from '@/lib/ws'
import { FolderGit2, LayoutDashboard, Menu, X, Shield, Settings, ChevronDown, PanelLeftClose, PanelLeftOpen } from 'lucide-react'
import ProjectsPage from '@/pages/Projects'
import ProjectPage from '@/pages/Project'
import TaskPage from '@/pages/Task'
import SettingsPage from '@/pages/Settings'
import ProjectSettingsPage from '@/pages/ProjectSettings'
import { cn } from '@/lib/utils'

function Sidebar({ open, onClose, collapsed, onToggleCollapse }: { open: boolean; onClose: () => void; collapsed: boolean; onToggleCollapse: () => void }) {
  const location = useLocation()
  const [settingsOpen, setSettingsOpen] = useState(location.pathname.startsWith('/settings'))

  const navItems = [
    { to: '/', label: 'Projects', icon: FolderGit2 },
  ]

  const settingsItems = [
    { to: '/settings/exclude-patterns', label: 'Exclude Patterns', icon: Shield },
  ]

  // Auto-expand settings section when navigating to a settings page
  useEffect(() => {
    if (location.pathname.startsWith('/settings')) {
      setSettingsOpen(true)
    }
  }, [location.pathname])

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
        'fixed inset-y-0 left-0 z-50 border-r bg-card flex flex-col transition-all duration-200 ease-in-out md:relative md:translate-x-0',
        collapsed ? 'w-14' : 'w-60',
        open ? 'translate-x-0' : '-translate-x-full'
      )}>
        <div className={cn('flex items-center border-b', collapsed ? 'justify-center p-3' : 'justify-between p-4')}>
          <Link to="/" className="flex items-center gap-2" title="reSwe">
            <LayoutDashboard className="h-5 w-5 text-primary shrink-0" />
            {!collapsed && <span className="text-lg font-bold tracking-tight">reSwe</span>}
          </Link>
          <Button variant="ghost" size="icon" className="md:hidden h-8 w-8" onClick={onClose}>
            <X className="h-4 w-4" />
          </Button>
        </div>
        {!collapsed && <p className="text-xs text-muted-foreground px-4 pt-2 pb-1">SWE Agent Orchestrator</p>}
        <nav className="flex-1 p-2 space-y-1">
          {navItems.map(item => {
            const active = location.pathname === item.to ||
              (item.to !== '/' && location.pathname.startsWith(item.to))
            return (
              <Link
                key={item.to}
                to={item.to}
                title={collapsed ? item.label : undefined}
                className={cn(
                  'flex items-center gap-2 rounded-md text-sm font-medium transition-colors',
                  collapsed ? 'justify-center px-2 py-2' : 'px-3 py-2',
                  active
                    ? 'bg-accent text-accent-foreground'
                    : 'text-muted-foreground hover:bg-accent/50 hover:text-foreground'
                )}
              >
                <item.icon className="h-4 w-4 shrink-0" />
                {!collapsed && item.label}
              </Link>
            )
          })}

          {/* Settings — collapsible section */}
          <div className="pt-3">
            {collapsed ? (
              <Link
                to="/settings/exclude-patterns"
                title="Settings"
                className={cn(
                  'flex items-center justify-center px-2 py-2 rounded-md text-sm font-medium transition-colors',
                  location.pathname.startsWith('/settings')
                    ? 'bg-accent text-accent-foreground'
                    : 'text-muted-foreground hover:bg-accent/50 hover:text-foreground'
                )}
              >
                <Settings className="h-4 w-4 shrink-0" />
              </Link>
            ) : (
              <>
                <button
                  onClick={() => setSettingsOpen(!settingsOpen)}
                  className="flex items-center justify-between w-full px-3 py-2 rounded-md text-sm font-medium text-muted-foreground hover:bg-accent/50 hover:text-foreground transition-colors"
                >
                  <span className="flex items-center gap-2">
                    <Settings className="h-4 w-4" />
                    Settings
                  </span>
                  <ChevronDown className={cn(
                    'h-3.5 w-3.5 transition-transform',
                    settingsOpen && 'rotate-180'
                  )} />
                </button>

                {settingsOpen && (
                  <div className="ml-3 mt-0.5 space-y-0.5 border-l border-border/50 pl-2">
                    {settingsItems.map(item => {
                      const active = location.pathname === item.to
                      return (
                        <Link
                          key={item.to}
                          to={item.to}
                          className={cn(
                            'flex items-center gap-2 px-3 py-1.5 rounded-md text-sm transition-colors',
                            active
                              ? 'bg-accent text-accent-foreground font-medium'
                              : 'text-muted-foreground hover:bg-accent/50 hover:text-foreground'
                          )}
                        >
                          <item.icon className="h-3.5 w-3.5" />
                          {item.label}
                        </Link>
                      )
                    })}
                  </div>
                )}
              </>
            )}
          </div>
        </nav>

        {/* Collapse toggle — desktop only */}
        <div className="hidden md:flex border-t p-2">
          <Button
            variant="ghost"
            size="icon"
            className={cn('h-8 w-8', collapsed ? 'mx-auto' : 'ml-auto')}
            onClick={onToggleCollapse}
            title={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
          >
            {collapsed ? <PanelLeftOpen className="h-4 w-4" /> : <PanelLeftClose className="h-4 w-4" />}
          </Button>
        </div>
      </aside>
    </>
  )
}

export default function App() {
  const [sidebarOpen, setSidebarOpen] = useState(false)
  const [sidebarCollapsed, setSidebarCollapsed] = useState(() => {
    return localStorage.getItem('sidebar-collapsed') === 'true'
  })

  const toggleCollapse = () => {
    setSidebarCollapsed(prev => {
      localStorage.setItem('sidebar-collapsed', String(!prev))
      return !prev
    })
  }

  useEffect(() => {
    wsClient.connect()
    return () => wsClient.disconnect()
  }, [])

  return (
    <BrowserRouter>
      <div className="flex h-screen overflow-hidden bg-background">
        <Sidebar open={sidebarOpen} onClose={() => setSidebarOpen(false)} collapsed={sidebarCollapsed} onToggleCollapse={toggleCollapse} />

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
                <Route path="/projects/:id/settings" element={<ProjectSettingsPage />} />
                <Route path="/settings/exclude-patterns" element={<SettingsPage />} />
              </Routes>
            </div>
          </main>
        </div>
      </div>
      <Toaster />
    </BrowserRouter>
  )
}
