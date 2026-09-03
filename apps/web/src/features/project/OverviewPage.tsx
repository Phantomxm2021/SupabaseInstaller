import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Database, ExternalLink, Globe2, ServerCog, ShieldCheck } from 'lucide-react'
import { useState } from 'react'
import { useParams } from 'react-router-dom'
import { Alert } from '@/components/ui/alert'
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { apiFetch } from '../../api/client'
import type { Project } from '../../api/types'
import { LifecycleActions } from './LifecycleActions'
import { ServiceTable } from './ServiceTable'
import { CopyButton } from '@/components/app/CopyButton'
import { OperationPanel } from '../operations/OperationPanel'
export function OverviewPage() {
  const { projectId = '' } = useParams()
  const queryClient = useQueryClient()
  const [syncOpen, setSyncOpen] = useState(false)
  const [syncOperation, setSyncOperation] = useState<{ projectId: string; operationId: string }>()
  const project = useQuery({ queryKey: ['project', projectId], queryFn: () => apiFetch<Project>(`/api/projects/${projectId}`), enabled: Boolean(projectId) })
  const sync = useMutation({
    mutationFn: () => apiFetch<{ projectId: string; operationId: string }>(`/api/projects/${projectId}/runtime/sync`, { method: 'POST' }),
    onSuccess: (operation) => {
      setSyncOpen(false)
      setSyncOperation(operation)
    },
  })
  if (project.isLoading) return <main className="page">Loading project…</main>
  if (project.error) return <main className="page"><Alert variant="destructive">{project.error.message}</Alert></main>
  if (!project.data) return null
  const data = project.data
  if (syncOperation) return <main className="page narrow-page"><div className="page-heading"><p className="eyebrow">Official runtime update</p><h1>Synchronizing {data.name}</h1><p className="muted">Using the official Docker template bundled with this Manager release. Existing database and Storage volumes are retained.</p></div><OperationPanel operationId={syncOperation.operationId} projectId={syncOperation.projectId} projectName={data.name} onSucceeded={async () => { await queryClient.invalidateQueries({ queryKey: ['project', projectId] }); await queryClient.invalidateQueries({ queryKey: ['projects'] }); setSyncOperation(undefined) }} /></main>
  // Studio is served from the project's public Domain. Keep the Site URL
  // fallback for legacy records that do not have a Domain yet.
  const studioURL = data.domain ? `https://${data.domain}` : data.siteUrl
  const enabledServiceCount = Object.values(data.services).filter(Boolean).length
  return <main className="page project-overview-page" data-density="dashboard">
    <header className="project-overview-header">
      <div>
        <h1>{data.name}</h1>
        <div className="project-overview-origin">
          <span>{studioURL}</span>
          <CopyButton value={studioURL} label="Project URL" />
        </div>
      </div>
      <LifecycleActions project={data} onSyncOfficialRuntime={() => setSyncOpen(true)} />
    </header>
    <section className="project-overview-hero" data-testid="project-overview-hero">
      <div className="project-overview-facts">
        <OverviewFact icon={<ShieldCheck />} label="Status" value={humanize(data.health)} detail={humanize(data.status)} />
        <OverviewFact icon={<ServerCog />} label="Compute" value="Self-hosted" detail={`${humanize(data.preset)} stack`} />
        <OverviewFact icon={<Globe2 />} label="Supabase Studio" value={studioURL} detail={data.services.studio && data.status === 'RUNNING' ? <a href={studioURL} target="_blank" rel="noreferrer">Open Supabase Studio <ExternalLink /></a> : 'Studio unavailable while the project is stopped'} />
        <OverviewFact icon={<Database />} label="Services" value={`${enabledServiceCount} active services`} detail="Components enabled for this project" />
        <OverviewFact icon={<ServerCog />} label="Configuration" value={`Revision ${data.configurationRevision ?? 0}`} detail={`${data.slug} · ${humanize(data.preset)} preset`} />
      </div>
      <aside className="project-overview-runtime" aria-label="Runtime summary">
        <div className="project-overview-runtime-grid" aria-hidden="true" />
        <div className="project-overview-runtime-card"><span className="project-overview-runtime-icon"><Database /></span><div><strong>Primary Database</strong><p>Local Docker host</p><small>{data.domain} · {humanize(data.health)}</small></div></div>
      </aside>
    </section>
    <section className="project-overview-services">
      <div className="project-overview-section-heading"><div><h2>Services</h2><p>Components selected for this runtime.</p></div><span>{enabledServiceCount} active</span></div>
      <Card data-testid="overview-services-card"><CardHeader className="sr-only"><CardTitle>Services</CardTitle><CardDescription>Components selected for this runtime.</CardDescription></CardHeader><CardContent className="p-0"><ServiceTable services={data.services} /></CardContent></Card>
    </section>
    <AlertDialog open={syncOpen} onOpenChange={setSyncOpen}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Sync the official Supabase runtime?</AlertDialogTitle>
          <AlertDialogDescription>Manager will re-render this server with the official Docker template bundled with this Manager release, pull its pinned images, recreate the complete runtime, and verify its health.</AlertDialogDescription>
        </AlertDialogHeader>
        <p className="text-sm text-muted-foreground">Database and Storage volumes are retained. The server may be briefly unavailable while services restart.</p>
        {sync.error && <Alert variant="destructive">{sync.error.message}</Alert>}
        <AlertDialogFooter>
          <AlertDialogCancel disabled={sync.isPending}>Cancel</AlertDialogCancel>
          <AlertDialogAction disabled={sync.isPending} onClick={() => sync.mutate()}>{sync.isPending ? 'Queuing sync…' : 'Sync runtime'}</AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </main>
}

function OverviewFact({ icon, label, value, detail }: { icon: React.ReactNode; label: string; value: string; detail: React.ReactNode }) {
  return <article className="project-overview-fact"><span className="project-overview-fact-icon">{icon}</span><div><p>{label}</p><strong title={value}>{value}</strong><small>{detail}</small></div></article>
}

function humanize(value: string) {
  return value.toLocaleLowerCase().replaceAll('_', ' ').replace(/\b\w/g, (character) => character.toLocaleUpperCase())
}
