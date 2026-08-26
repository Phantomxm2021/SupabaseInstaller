import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Boxes, Database, HardDrive, LogOut, ServerCog, UserCircle } from 'lucide-react'
import { NavLink, Outlet, useNavigate } from 'react-router-dom'
import { apiFetch, setCSRFToken } from '../api/client'
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
} from '../components/ui/sidebar'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '../components/ui/dropdown-menu'

export function AppShell() {
  const navigate = useNavigate()
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
      <div className="app-shell">
        <Sidebar collapsible="icon">
          <SidebarHeader>
            <div className="brand-row sidebar-brand"><span className="brand-mark"><Database size={20} /></span><span>Supabase Manager</span></div>
          </SidebarHeader>
          <SidebarContent>
            <SidebarMenu aria-label="Main navigation">
              <SidebarMenuItem>
                <SidebarMenuButton tooltip="Projects" render={<NavLink to="/projects" />}>
                  <Boxes /> <span>Projects</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
            </SidebarMenu>
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
      <div className="content-shell">
        <header className="topbar"><div><HardDrive size={16} /> Local Docker host</div><span className="badge neutral">Installer Core</span></header>
        <Outlet />
      </div>
      </div>
    </SidebarProvider>
  )
}
