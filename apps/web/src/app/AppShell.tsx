import { Boxes, Database, HardDrive, LogOut, Plus, ServerCog } from 'lucide-react'
import { NavLink, Outlet } from 'react-router-dom'

export function AppShell() {
  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand-row sidebar-brand"><span className="brand-mark"><Database size={20} /></span><span>Supabase Manager</span></div>
        <nav aria-label="Main navigation">
          <NavLink to="/projects"><Boxes size={17} /> Projects</NavLink>
          <NavLink to="/projects/new"><Plus size={17} /> New project</NavLink>
        </nav>
        <div className="sidebar-status">
          <span className="status-dot healthy" /><div><strong>Host online</strong><small>Docker connected</small></div>
        </div>
        <nav className="sidebar-bottom">
          <a href="/api/session"><ServerCog size={17} /> Manager settings</a>
          <button type="button"><LogOut size={17} /> Sign out</button>
        </nav>
      </aside>
      <div className="content-shell">
        <header className="topbar"><div><HardDrive size={16} /> Local Docker host</div><span className="badge neutral">Installer Core</span></header>
        <Outlet />
      </div>
    </div>
  )
}
