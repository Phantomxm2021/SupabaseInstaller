import { Activity, Braces, Database, FileClock, HardDrive, KeyRound, LayoutDashboard, LockKeyhole, Network, Radio, ScrollText, Settings, ShieldCheck, Waypoints } from 'lucide-react'
import { NavLink, Outlet, useLocation, useParams } from 'react-router-dom'
import { Card, CardContent } from '@/components/ui/card'

const navigation = [
  ['overview', 'Overview', LayoutDashboard], ['configuration', 'Configuration', Settings], ['configuration?section=services', 'Services', Activity], ['configuration?section=auth', 'Authentication', ShieldCheck],
  ['configuration?section=database', 'Database', Database], ['configuration?section=storage', 'Storage', HardDrive], ['configuration?section=realtime', 'Realtime', Radio], ['configuration?section=functions', 'Functions', Braces],
  ['configuration?section=pooler', 'Connection Pool', Waypoints], ['configuration?section=network', 'Network', Network], ['configuration?section=secrets', 'Secrets', KeyRound],
  ['logs', 'Logs', ScrollText], ['backups', 'Backups', FileClock], ['configuration?section=general', 'Project settings', Settings],
] as const

export function ProjectLayout() {
  const { projectId } = useParams()
  const location = useLocation()
  return <div className="project-shell"><aside className="project-nav"><div className="project-nav-title"><LockKeyhole size={14} /> Project</div>{navigation.map(([path, label, Icon]) => <NavLink key={`${path}-${label}`} to={`/projects/${projectId}/${path}`} className={() => { const [pathname, query] = path.split('?'); const expected = new URLSearchParams(query ?? '').get('section'); const active = location.pathname.endsWith(`/${pathname}`) && (expected ? new URLSearchParams(location.search).get('section') === expected : !location.search); return active ? 'active' : undefined }}><Icon size={15} />{label}</NavLink>)}</aside><div className="project-content"><Outlet /></div></div>
}

export function ComingSoonPage() {
  return <main className="page"><Card><CardContent className="empty-state"><h3>This project module is coming next</h3><p>The runtime is already managed from Overview.</p></CardContent></Card></main>
}
