import { useQuery } from '@tanstack/react-query'
import { ExternalLink, Globe2, Package } from 'lucide-react'
import { useParams } from 'react-router-dom'
import { Alert } from '@/components/ui/alert'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { apiFetch } from '../../api/client'
import type { Project } from '../../api/types'
import { LifecycleActions } from './LifecycleActions'
import { ServiceTable } from './ServiceTable'
export function OverviewPage() {
  const { projectId = '' } = useParams(); const project = useQuery({ queryKey: ['project', projectId], queryFn: () => apiFetch<Project>(`/api/projects/${projectId}`), enabled: Boolean(projectId) })
  if (project.isLoading) return <main className="page">Loading project…</main>
  if (project.error) return <main className="page"><Alert variant="destructive">{project.error.message}</Alert></main>
  if (!project.data) return null
  const data = project.data
  return <main className="page project-page"><div className="page-heading"><div><p className="eyebrow">Project overview</p><h1>{data.name}</h1><p className="muted">{data.slug} · {data.preset.toLowerCase()} preset</p></div><LifecycleActions project={data} /></div><section className="project-metrics"><Metric icon={<Globe2 />} label="API URL" value={`https://${data.domain}`} detail={<a href={`https://${data.domain}`} target="_blank" rel="noreferrer">Open endpoint <ExternalLink className="inline size-3" /></a>} /><Metric icon={<Package />} label="Version" value={data.supabaseVersion} detail="Pinned official template" /><Metric icon={<span className="inline-block size-2 rounded-full bg-primary" />} label="Runtime health" value={data.health} detail={data.status} /></section><Card data-testid="overview-services-card"><CardHeader><CardTitle>Services</CardTitle><CardDescription>Components selected for this runtime.</CardDescription></CardHeader><CardContent className="p-0"><ServiceTable services={data.services} /></CardContent></Card></main>
}
function Metric({ icon, label, value, detail }: { icon: React.ReactNode; label: string; value: string; detail: React.ReactNode }) { return <Card size="sm"><CardContent className="flex items-center gap-3"><span className="inline-flex size-9 items-center justify-center rounded-md bg-muted text-muted-foreground">{icon}</span><div className="min-w-0"><small className="block text-xs uppercase text-muted-foreground">{label}</small><strong className="block truncate text-sm">{value}</strong><p className="m-0 text-xs text-muted-foreground">{detail}</p></div></CardContent></Card> }
