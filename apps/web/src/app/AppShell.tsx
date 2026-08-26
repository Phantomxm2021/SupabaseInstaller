import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Boxes, Database, HardDrive, LogOut, ServerCog, UserCircle } from 'lucide-react'
import { NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom'
import { apiFetch, setCSRFToken } from '../api/client'
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
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

export function AppShell() {
  const navigate = useNavigate()
  const location = useLocation()
  const queryClient = useQueryClient()
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
