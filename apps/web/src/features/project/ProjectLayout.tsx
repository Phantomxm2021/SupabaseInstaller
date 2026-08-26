import { Activity, Braces, Database, FileClock, HardDrive, KeyRound, LayoutDashboard, LockKeyhole, Network, Radio, ScrollText, Settings, ShieldCheck, Waypoints } from 'lucide-react'
import { NavLink, Outlet, useLocation, useParams } from 'react-router-dom'
import { Card, CardContent } from '@/components/ui/card'
import { Sidebar, SidebarContent, SidebarGroup, SidebarGroupContent, SidebarGroupLabel, SidebarMenu, SidebarMenuButton, SidebarMenuItem, SidebarProvider, SidebarTrigger } from '../../components/ui/sidebar'

const navigation = [
  ['overview', 'Overview', LayoutDashboard], ['configuration', 'Configuration', Settings], ['configuration?section=services', 'Services', Activity], ['configuration?section=auth', 'Authentication', ShieldCheck],
  ['configuration?section=database', 'Database', Database], ['configuration?section=storage', 'Storage', HardDrive], ['configuration?section=realtime', 'Realtime', Radio], ['configuration?section=functions', 'Functions', Braces],
  ['configuration?section=pooler', 'Connection Pool', Waypoints], ['configuration?section=network', 'Network', Network], ['configuration?section=secrets', 'Secrets', KeyRound],
  ['logs', 'Logs', ScrollText], ['backups', 'Backups', FileClock], ['configuration?section=general', 'Project settings', Settings],
] as const

export function ProjectLayout() {
  const { projectId } = useParams()
  const location = useLocation()
  return <SidebarProvider defaultOpen={false}><div className="flex min-h-[calc(100vh-3.5rem)] w-full"><Sidebar collapsible="offcanvas"><SidebarContent><SidebarGroup><SidebarGroupLabel><LockKeyhole /> Project</SidebarGroupLabel><SidebarGroupContent><SidebarMenu>{navigation.map(([path, label, Icon]) => { const [pathname, query] = path.split('?'); const expected = new URLSearchParams(query ?? '').get('section'); const active = location.pathname.endsWith(`/${pathname}`) && (expected ? new URLSearchParams(location.search).get('section') === expected : !location.search); return <SidebarMenuItem key={`${path}-${label}`}><SidebarMenuButton isActive={active} tooltip={label} render={<NavLink to={`/projects/${projectId}/${path}`} />}><Icon /><span>{label}</span></SidebarMenuButton></SidebarMenuItem> })}</SidebarMenu></SidebarGroupContent></SidebarGroup></SidebarContent></Sidebar><div className="min-w-0 flex-1"><div className="flex h-10 items-center border-b px-4"><SidebarTrigger aria-label="Open project navigation" /></div><Outlet /></div></div></SidebarProvider>
}

export function ComingSoonPage() {
  return <main className="page"><Card><CardContent className="empty-state"><h3>This project module is coming next</h3><p>The runtime is already managed from Overview.</p></CardContent></Card></main>
}
