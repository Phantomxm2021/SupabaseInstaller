import { useQuery } from '@tanstack/react-query'
import { ArrowRight, Box, Cpu, Database, HardDrive, MemoryStick, Plus } from 'lucide-react'
import { Link } from 'react-router-dom'
import { apiFetch } from '../../api/client'
import type { Project } from '../../api/types'

export function ProjectsPage() {
  const query = useQuery({ queryKey: ['projects'], queryFn: () => apiFetch<{ projects: Project[] }>('/api/projects') })
  return (
    <main className="page">
      <div className="page-heading">
        <div><p className="eyebrow">Runtime orchestration</p><h1>Projects</h1><p className="muted">Independent official Supabase stacks on this server.</p></div>
        <Link className="button primary" to="/projects/new"><Plus size={16} /> New project</Link>
      </div>
      <section className="host-grid" aria-label="Host resources">
        <Metric icon={<Cpu />} label="CPU" value="—" detail="Live metrics after deployment" />
        <Metric icon={<MemoryStick />} label="Memory" value="—" detail="Available memory" />
        <Metric icon={<HardDrive />} label="Disk" value="—" detail="Project data volume" />
      </section>
      <section className="panel project-panel">
        <div className="panel-heading"><div><h2>All projects</h2><p>{query.data?.projects.length ?? 0} configured runtimes</p></div></div>
        {query.isLoading && <div className="empty-state">Loading projects…</div>}
        {query.error && <div className="alert error">{query.error.message}</div>}
        {query.data?.projects.length === 0 && <div className="empty-state"><Box size={28} /><h3>No projects yet</h3><p>Create a Lightweight project with three fields.</p></div>}
        <div className="project-list">
          {query.data?.projects.map((project) => (
            <Link className="project-row" to={`/projects/${project.id}/overview`} key={project.id}>
              <span className="project-icon"><Database size={19} /></span>
              <span className="project-main"><strong>{project.name}</strong><small>https://{project.domain}</small></span>
              <span className={`badge ${project.health.toLowerCase()}`}>{labelHealth(project.health)}</span>
              <span className="version">{project.supabaseVersion}</span>
              <ArrowRight size={17} />
            </Link>
          ))}
        </div>
      </section>
    </main>
  )
}

function Metric({ icon, label, value, detail }: { icon: React.ReactNode; label: string; value: string; detail: string }) {
  return <div className="metric-card"><span className="metric-icon">{icon}</span><div><small>{label}</small><strong>{value}</strong><p>{detail}</p></div></div>
}

function labelHealth(health: Project['health']) {
  return health.charAt(0) + health.slice(1).toLowerCase()
}
