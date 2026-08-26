import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Activity, Boxes, Braces, Database, FileClock, Globe2, HardDrive, KeyRound, LayoutDashboard, LockKeyhole, LogOut, Mail, Network, Radio, ScrollText, ServerCog, Settings, ShieldCheck, UserCircle, Waypoints } from 'lucide-react'
import { NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom'
import { apiFetch, setCSRFToken } from '../api/client'
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
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
  DropdownMenuTrigger,
} from '../components/ui/dropdown-menu'
import { Badge } from '../components/ui/badge'

const projectNavigation = [
  ['configuration?section=general', 'General', Settings], ['configuration?section=services', 'Services', Activity], ['configuration?section=auth', 'Authentication', ShieldCheck], ['configuration?section=smtp', 'Email & SMTP', Mail],
  ['configuration?section=oauth', 'OAuth Providers', Globe2], ['configuration?section=storage', 'Storage', HardDrive], ['configuration?section=realtime', 'Realtime', Radio], ['configuration?section=functions', 'Functions', Braces],
  ['configuration?section=database', 'Database', Database], ['configuration?section=pooler', 'Connection Pool', Waypoints], ['configuration?section=network', 'Gateway & Network', Network], ['configuration?section=secrets', 'API & Secrets', KeyRound],
] as const

const runtimeNavigation = [['logs', 'Logs', ScrollText], ['backups', 'Backups', FileClock]] as const

export function AppShell() {
  const navigate = useNavigate()
  const location = useLocation()
  const queryClient = useQueryClient()
  const projectMatch = location.pathname.match(/^\/projects\/([^/]+)(?:\/|$)/)
  const projectId = projectMatch?.[1]
  const isProjectRoute = Boolean(projectId) && projectId !== 'new'
  const logout = useMutation({
    mutationFn: () => apiFetch('/api/session', { method: 'DELETE' }),
    onSuccess: () => {
      setCSRFToken('')
      queryClient.clear()
      navigate('/login', { replace: true })
    },
  })
  return (
    <SidebarProvider defaultOpen>
      <Sidebar collapsible="icon">
          <SidebarHeader>
            <div className="brand-row sidebar-brand"><span className="brand-mark"><Database size={20} /></span><span>Supabase Manager</span></div>
          </SidebarHeader>
          <SidebarContent>
            <nav aria-label="Main navigation">
              <SidebarMenu>
                <SidebarMenuItem>
                  <SidebarMenuButton isActive={location.pathname === '/projects' || location.pathname.startsWith('/projects/')} tooltip="Projects" render={<NavLink to="/projects" />}>
                    <Boxes /> <span>Projects</span>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              </SidebarMenu>
            </nav>
            {isProjectRoute && projectId && <nav aria-label="Project navigation">
              <SidebarGroup>
                <SidebarGroupLabel><LockKeyhole /> Project</SidebarGroupLabel>
                <SidebarGroupContent>
                  <SidebarMenu>
                    <SidebarMenuItem><SidebarMenuButton isActive={location.pathname.endsWith('/overview')} tooltip="Overview" render={<NavLink to={`/projects/${projectId}/overview`} />}><LayoutDashboard /><span>Overview</span></SidebarMenuButton></SidebarMenuItem>
                    {projectNavigation.map(([path, label, Icon]) => {
                      const [pathname, query] = path.split('?')
                      const expected = new URLSearchParams(query ?? '').get('section')
                      const active = location.pathname.endsWith(`/${pathname}`) && (expected ? new URLSearchParams(location.search).get('section') === expected : !location.search)
                      return <SidebarMenuItem key={`${path}-${label}`}><SidebarMenuButton isActive={active} tooltip={label} render={<NavLink to={`/projects/${projectId}/${path}`} />}><Icon /><span>{label}</span></SidebarMenuButton></SidebarMenuItem>
                    })}
                  </SidebarMenu>
                </SidebarGroupContent>
              </SidebarGroup>
              <SidebarGroup>
                <SidebarGroupLabel>Runtime</SidebarGroupLabel>
                <SidebarGroupContent>
                  <SidebarMenu>
                    {runtimeNavigation.map(([path, label, Icon]) => <SidebarMenuItem key={path}><SidebarMenuButton isActive={location.pathname.endsWith(`/${path}`)} tooltip={label} render={<NavLink to={`/projects/${projectId}/${path}`} />}><Icon /><span>{label}</span></SidebarMenuButton></SidebarMenuItem>)}
                  </SidebarMenu>
                </SidebarGroupContent>
              </SidebarGroup>
            </nav>}
          </SidebarContent>
          <div className="sidebar-status">
            <span className="status-dot healthy" /><div><strong>Host online</strong><small>Docker connected</small></div>
          </div>
          <SidebarFooter>
            <DropdownMenu>
              <DropdownMenuTrigger render={<SidebarMenuButton tooltip="Account" />}>
                <UserCircle /><span>Account</span>
              </DropdownMenuTrigger>
              <DropdownMenuContent side="right" align="end">
                <DropdownMenuItem render={<NavLink to="/settings" />}><ServerCog /> Manager settings</DropdownMenuItem>
                <DropdownMenuItem variant="destructive" disabled={logout.isPending} onClick={() => logout.mutate()}><LogOut /> Sign out</DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </SidebarFooter>
      </Sidebar>
      <SidebarInset>
        <header className="topbar">
          <ResponsiveSidebarTrigger />
          <div><HardDrive size={16} /> Local Docker host</div><Badge variant="outline">Installer Core</Badge>
        </header>
        <div className="content-shell"><Outlet /></div>
      </SidebarInset>
    </SidebarProvider>
  )
}

function ResponsiveSidebarTrigger() {
  const { isMobile, openMobile, state } = useSidebar()
  const isOpen = isMobile ? openMobile : state === 'expanded'
  return <SidebarTrigger aria-label={isOpen ? 'Close sidebar' : 'Open sidebar'} />
}
