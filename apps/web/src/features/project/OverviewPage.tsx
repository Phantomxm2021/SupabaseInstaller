import { useQuery } from '@tanstack/react-query'
import { ExternalLink, Globe2, Package } from 'lucide-react'
import { useParams } from 'react-router-dom'
import { apiFetch } from '../../api/client'
import type { Project } from '../../api/types'
import { LifecycleActions } from './LifecycleActions'
import { ServiceTable } from './ServiceTable'

export function OverviewPage() {
  const { projectId = '' } = useParams()
  const project = useQuery({ queryKey: ['project', projectId], queryFn: () => apiFetch<Project>(`/api/projects/${projectId}`), enabled: Boolean(projectId) })
  if (project.isLoading) return <main className="page">Loading project…</main>
  if (project.error) return <main className="page"><div className="alert error">{project.error.message}</div></main>
  if (!project.data) return null
  const data = project.data
  return (
    <main className="page project-page">
      <div className="page-heading">
        <div><p className="eyebrow">Project overview</p><h1>{data.name}</h1><p className="muted">{data.slug} · {data.preset.toLowerCase()} preset</p></div>
        <LifecycleActions project={data} />
      </div>
      <section className="project-metrics">
        <div className="metric-card"><span className="metric-icon"><Globe2 /></span><div><small>API URL</small><strong className="metric-value">https://{data.domain}</strong><p><a href={`https://${data.domain}`} target="_blank" rel="noreferrer">Open endpoint <ExternalLink size={11} /></a></p></div></div>
        <div className="metric-card"><span className="metric-icon"><Package /></span><div><small>Version</small><strong className="metric-value">{data.supabaseVersion}</strong><p>Pinned official template</p></div></div>
        <div className="metric-card"><span className={`status-dot ${data.health.toLowerCase()}`} /><div><small>Runtime health</small><strong className="metric-value">{data.health}</strong><p>{data.status}</p></div></div>
      </section>
      <section className="panel">
        <div className="panel-heading"><h2>Services</h2><p>Components selected for this runtime.</p></div>
        <ServiceTable services={data.services} />
      </section>
    </main>
  )
}
