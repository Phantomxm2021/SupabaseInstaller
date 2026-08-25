import { Activity, Braces, Database, FileClock, HardDrive, KeyRound, LayoutDashboard, LockKeyhole, Network, Radio, ScrollText, Settings, ShieldCheck, Waypoints } from 'lucide-react'
import { NavLink, Outlet, useParams } from 'react-router-dom'

const navigation = [
  ['overview', 'Overview', LayoutDashboard], ['services', 'Services', Activity], ['authentication', 'Authentication', ShieldCheck],
  ['database', 'Database', Database], ['storage', 'Storage', HardDrive], ['realtime', 'Realtime', Radio], ['functions', 'Functions', Braces],
  ['pooler', 'Connection Pool', Waypoints], ['logs', 'Logs', ScrollText], ['network', 'Network', Network], ['secrets', 'Secrets', KeyRound],
  ['backups', 'Backups', FileClock], ['settings', 'Settings', Settings],
] as const

export function ProjectLayout() {
  const { projectId } = useParams()
  return <div className="project-shell"><aside className="project-nav"><div className="project-nav-title"><LockKeyhole size={14} /> Project</div>{navigation.map(([path, label, Icon]) => <NavLink key={path} to={`/projects/${projectId}/${path}`}><Icon size={15} />{label}</NavLink>)}</aside><div className="project-content"><Outlet /></div></div>
}

export function ComingSoonPage() {
  return <main className="page"><div className="empty-state panel"><h3>This project module is coming next</h3><p>The runtime is already managed from Overview.</p></div></main>
}
