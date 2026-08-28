import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { Boxes, ChevronsUpDown, Database, LayoutDashboard, LogOut, Plus, Search, ServerCog, Settings, ShieldCheck, UserCircle } from 'lucide-react'
import { Link, NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom'
import { apiFetch, setCSRFToken } from '../api/client'
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarInset,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  SidebarTrigger,
  useSidebar,
} from '../components/ui/sidebar'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '../components/ui/dropdown-menu'
import { Badge } from '../components/ui/badge'
import type { Project } from '../api/types'

export function AppShell() {
  const navigate = useNavigate()
  const location = useLocation()
  const queryClient = useQueryClient()
  const projectMatch = location.pathname.match(/^\/projects\/([^/]+)(?:\/|$)/)
  const projectId = projectMatch?.[1]
  const isProjectRoute = Boolean(projectId) && projectId !== 'new'
  const isProjectsLanding = location.pathname === '/projects'
  const isProjectCreation = location.pathname === '/projects/new'
  const showSidebar = !isProjectsLanding && !isProjectCreation
  const projects = useQuery({
    queryKey: ['projects'],
    queryFn: () => apiFetch<{ projects: Project[] }>('/api/projects'),
    enabled: isProjectRoute,
    staleTime: 30_000,
  })
  const [projectSearch, setProjectSearch] = useState('')
  const activeProject = projects.data?.projects.find((project) => project.id === projectId)
  const visibleProjects = projects.data?.projects.filter((project) => project.name.toLocaleLowerCase().includes(projectSearch.trim().toLocaleLowerCase())) ?? []
  const logout = useMutation({
    mutationFn: () => apiFetch('/api/session', { method: 'DELETE' }),
    onSuccess: () => {
      setCSRFToken('')
      queryClient.clear()
      navigate('/login', { replace: true })
    },
  })
  return (
      <SidebarProvider defaultOpen={false}>
      {showSidebar && <Sidebar collapsible="icon" className="primary-sidebar">
          <SidebarContent>
            {!isProjectRoute && <nav aria-label="Main navigation">
              <SidebarMenu>
                <SidebarMenuItem>
                  <SidebarMenuButton className="primary-sidebar-menu-button" collapsibleIcon={false} isActive={location.pathname === '/projects'} render={<NavLink to="/projects" end />}>
                    <Boxes /> <span>Projects</span>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              </SidebarMenu>
            </nav>}
            {isProjectRoute && projectId && <nav aria-label="Project navigation" className="primary-project-navigation">
              <SidebarMenu>
                <SidebarMenuItem><SidebarMenuButton className="primary-sidebar-menu-button" collapsibleIcon={false} isActive={location.pathname === `/projects/${projectId}` || location.pathname.endsWith('/overview')} render={<Link to={`/projects/${projectId}/overview`} aria-current={location.pathname === `/projects/${projectId}` || location.pathname.endsWith('/overview') ? 'page' : undefined} />}><LayoutDashboard /><span>Project Overview</span></SidebarMenuButton></SidebarMenuItem>
                <SidebarMenuItem><SidebarMenuButton className="primary-sidebar-menu-button" collapsibleIcon={false} isActive={location.pathname.includes('/authentication')} render={<Link to={`/projects/${projectId}/authentication`} aria-current={location.pathname.includes('/authentication') ? 'page' : undefined} />}><ShieldCheck /><span>Authentication</span></SidebarMenuButton></SidebarMenuItem>
                <SidebarMenuItem><SidebarMenuButton className="primary-sidebar-menu-button" collapsibleIcon={false} isActive={location.pathname.endsWith('/configuration')} render={<Link to={`/projects/${projectId}/configuration`} aria-current={location.pathname.endsWith('/configuration') ? 'page' : undefined} />}><Settings /><span>Project Settings</span></SidebarMenuButton></SidebarMenuItem>
              </SidebarMenu>
            </nav>}
          </SidebarContent>
          <div className="sidebar-status">
            <span className="status-dot healthy" /><div><strong>Host online</strong><small>Docker connected</small></div>
          </div>
          <SidebarFooter>
            <DropdownMenu>
              <DropdownMenuTrigger render={<SidebarMenuButton className="primary-sidebar-menu-button" collapsibleIcon={false} />}>
                <UserCircle /><span>Account</span>
              </DropdownMenuTrigger>
              <DropdownMenuContent side="right" align="end">
                <DropdownMenuItem render={<NavLink to="/settings" />}><ServerCog /> Manager settings</DropdownMenuItem>
                <DropdownMenuItem variant="destructive" disabled={logout.isPending} onClick={() => logout.mutate()}><LogOut /> Sign out</DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </SidebarFooter>
      </Sidebar>}
      <SidebarInset>
        <header className="topbar" aria-label="Dashboard header">
          {showSidebar && <ResponsiveSidebarTrigger />}
          <div className="topbar-left">
            <span className="topbar-logo" aria-hidden="true"><Database size={18} /></span>
            {isProjectRoute && <><span className="topbar-slash" aria-hidden="true" /><DropdownMenu onOpenChange={(open) => { if (!open) setProjectSearch('') }}><DropdownMenuTrigger className="topbar-project-trigger" aria-label="Show projects"><Database /><span>{activeProject?.name ?? projectId}</span><ChevronsUpDown /></DropdownMenuTrigger><DropdownMenuContent className="topbar-menu topbar-project-menu" sideOffset={8} align="start"><label className="topbar-project-search"><Search /><input aria-label="Find project" autoFocus value={projectSearch} onChange={(event) => setProjectSearch(event.target.value)} placeholder="Find project..." /></label>{visibleProjects.map((project) => <DropdownMenuItem key={project.id} render={<Link to={`/projects/${project.id}/overview`} />} className="topbar-menu-item" data-current={project.id === projectId || undefined}><span>{project.name}</span>{project.id === projectId && <span className="topbar-menu-current">✓</span>}</DropdownMenuItem>)}<DropdownMenuSeparator /><DropdownMenuItem render={<Link to="/projects/new" />} className="topbar-menu-item topbar-menu-create"><Plus /><span>New project</span></DropdownMenuItem></DropdownMenuContent></DropdownMenu><span className="topbar-slash" aria-hidden="true" /><DropdownMenu><DropdownMenuTrigger className="topbar-project-trigger topbar-branch-trigger" aria-label="Show branches"><span>main</span><Badge variant="outline">Local</Badge><ChevronsUpDown /></DropdownMenuTrigger><DropdownMenuContent className="topbar-menu topbar-branch-menu" sideOffset={8} align="start"><DropdownMenuRadioGroup value="main"><DropdownMenuRadioItem value="main" className="topbar-menu-item"><span>main</span><Badge variant="outline">Local</Badge></DropdownMenuRadioItem></DropdownMenuRadioGroup></DropdownMenuContent></DropdownMenu></>}
          </div>
        </header>
        <div className="content-shell"><Outlet /></div>
      </SidebarInset>
    </SidebarProvider>
  )
}

function ResponsiveSidebarTrigger() {
  const { isMobile, openMobile, state } = useSidebar()
  const isOpen = isMobile ? openMobile : state === 'expanded'
  return <SidebarTrigger className="desktop-sidebar-trigger" aria-label={isOpen ? 'Close sidebar' : 'Open sidebar'} />
}
