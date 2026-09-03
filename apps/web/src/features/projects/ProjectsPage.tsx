import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Cpu, Database, HardDrive, MemoryStick, Plus, RotateCw, Search } from 'lucide-react'
import { useMemo, useState, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { AsyncState } from '@/components/app/AsyncState'
import { PageHeader } from '@/components/app/PageHeader'
import { Badge } from '@/components/ui/badge'
import { Button, buttonVariants } from '@/components/ui/button'
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { apiFetch } from '../../api/client'
import type { HostResources, Project } from '../../api/types'

export function ProjectsPage() {
  const [search, setSearch] = useState('')
  const query = useQuery({ queryKey: ['projects'], queryFn: () => apiFetch<{ projects: Project[] }>('/api/projects') })
  const resources = useQuery({ queryKey: ['host-resources'], queryFn: () => apiFetch<HostResources>('/api/host/resources'), staleTime: 30_000, retry: false })
  const projects = query.data?.projects ?? []
  const filteredProjects = useMemo(() => {
    const term = search.trim().toLowerCase()
    return term ? projects.filter((project) => `${project.name} ${project.domain}`.toLowerCase().includes(term)) : projects
  }, [projects, search])

  return (
    <main className="page space-y-5" data-density="dashboard">
      <div className="page-heading mb-0">
        <PageHeader
          eyebrow="Runtime orchestration"
          title="Projects"
          description="Independent official Supabase stacks on this host."
          actions={<Link className={buttonVariants()} to="/projects/new"><Plus data-icon="inline-start" />New project</Link>}
        />
      </div>

      <section className="host-grid mb-0" aria-label="Host resources">
        <Metric icon={<Cpu />} label="CPU" loading={resources.isLoading} value={resources.data ? `${Math.round(resources.data.cpuPercent)}%` : 'Unavailable'} detail={resources.data ? `${resources.data.cpuCores} cores` : 'Live metrics unavailable'} />
        <Metric icon={<MemoryStick />} label="Memory" loading={resources.isLoading} value={resources.data ? `${formatBytes(resources.data.memoryUsedBytes)} / ${formatBytes(resources.data.memoryTotalBytes)}` : 'Unavailable'} detail={resources.data ? `${usagePercent(resources.data.memoryUsedBytes, resources.data.memoryTotalBytes)}% used` : 'Available memory'} />
        <Metric icon={<HardDrive />} label="Disk" loading={resources.isLoading} value={resources.data ? `${formatBytes(resources.data.diskUsedBytes)} / ${formatBytes(resources.data.diskTotalBytes)}` : 'Unavailable'} detail={resources.data ? `${usagePercent(resources.data.diskUsedBytes, resources.data.diskTotalBytes)}% used` : 'Server data volume'} />
      </section>

      <Card data-testid="projects-card">
        <CardHeader className="border-b">
          <CardTitle>All projects</CardTitle>
          <CardDescription>{projects.length} configured projects</CardDescription>
          <CardAction className="w-full sm:w-64">
            <div className="relative">
              <Search aria-hidden="true" className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input aria-label="Search projects" value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Search by project or domain" className="pl-8" />
            </div>
          </CardAction>
        </CardHeader>
        <CardContent className="p-0">
          {query.isLoading ? <AsyncState variant="loading" className="h-52 p-4" /> : null}
          {query.error ? <AsyncState variant="error" title="Projects unavailable" description={query.error.message} onRetry={() => { void query.refetch() }} className="m-4" /> : null}
          {!query.isLoading && !query.error && projects.length === 0 ? (
            <AsyncState
              variant="empty"
              title="No projects yet"
              description="Create a complete project from the guided configuration wizard."
              action={<Link className={buttonVariants()} to="/projects/new"><Plus data-icon="inline-start" />Create project</Link>}
              className="items-center px-6 text-center"
            />
          ) : null}
          {!query.isLoading && !query.error && projects.length > 0 ? <ProjectsTable projects={filteredProjects} /> : null}
        </CardContent>
      </Card>
    </main>
  )
}

function ProjectsTable({ projects }: { projects: Project[] }) {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead className="pl-5">Project</TableHead>
          <TableHead>Health</TableHead>
          <TableHead className="pr-5 text-right">Actions</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {projects.length === 0 ? <TableRow><TableCell colSpan={3} className="h-24 text-center text-muted-foreground">No projects match your search.</TableCell></TableRow> : null}
        {projects.map((project) => (
          <TableRow key={project.id}>
            <TableCell className="pl-5">
              <Link className="flex items-center gap-3" to={`/projects/${project.id}/overview`}>
                <span className="inline-flex size-9 items-center justify-center rounded-lg border bg-muted/40 text-primary"><Database className="size-4" /></span>
                <span className="min-w-0">
                  <strong className="block truncate font-medium">{project.name}</strong>
                  <span className="block truncate text-xs text-muted-foreground">https://{project.domain}</span>
                </span>
              </Link>
            </TableCell>
            <TableCell><div className="flex items-center gap-2"><span className={`size-1.5 rounded-full ${healthDotClass(project.health)}`} aria-hidden="true" /><Badge variant={healthBadgeVariant(project.health)}>{labelHealth(project.health)}</Badge></div></TableCell>
            <TableCell className="pr-5"><div className="flex items-center justify-end gap-2"><RetryProjectButton project={project} /><Link className={buttonVariants({ variant: 'secondary', size: 'sm' })} to={`/projects/${project.id}/overview`}>View Details</Link></div></TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}

function RetryProjectButton({ project }: { project: Project }) {
  const queryClient = useQueryClient()
  const [message, setMessage] = useState('')
  const retry = useMutation({
    mutationFn: () => apiFetch<{ projectId: string; operationId: string }>(`/api/projects/${project.id}/retry`, { method: 'POST' }),
    onMutate: () => setMessage('Retrying project…'),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['projects'] })
      setMessage('Retry queued')
    },
    onError: () => setMessage(''),
  })
  if (project.status !== 'FAILED') return null
  return <div className="flex items-center gap-2"><Button variant="secondary" size="sm" aria-label={`Retry ${project.name}`} disabled={retry.isPending} onClick={() => retry.mutate()}>{retry.isPending ? <RotateCw className="animate-spin" /> : <RotateCw />}{retry.isPending ? 'Retrying…' : 'Retry'}</Button>{message && <span className="sr-only" role="status">{message}</span>}</div>
}

function Metric({ icon, label, loading, value, detail }: { icon: ReactNode; label: string; loading: boolean; value: string; detail: string }) {
  return <Card size="sm"><CardContent className="flex items-center gap-3"><span className="inline-flex size-9 items-center justify-center rounded-md bg-muted text-muted-foreground">{icon}</span><div className="min-w-0"><span className="block text-xs font-medium tracking-wide text-muted-foreground uppercase">{label}</span>{loading ? <><Skeleton data-testid="resource-skeleton" className="mt-1 h-5 w-24" /><Skeleton className="mt-1 h-3 w-16" /></> : <><strong className="block truncate text-base">{value}</strong><p className="m-0 text-xs text-muted-foreground">{detail}</p></>}</div></CardContent></Card>
}

function formatBytes(bytes: number) { if (!Number.isFinite(bytes) || bytes < 0) return '—'; const units = ['B', 'KB', 'MB', 'GB', 'TB']; let value = bytes; let unit = 0; while (value >= 1024 && unit < units.length - 1) { value /= 1024; unit += 1 } return `${unit === 0 ? Math.round(value) : value.toFixed(1)} ${units[unit]}` }
function usagePercent(used: number, total: number) { if (!Number.isFinite(used) || !Number.isFinite(total) || total <= 0) return '—'; return Math.min(100, Math.max(0, (used / total) * 100)).toFixed(0) }
function labelHealth(health: Project['health']) { return health.charAt(0) + health.slice(1).toLowerCase() }
function healthBadgeVariant(health: Project['health']) { return health === 'HEALTHY' ? 'default' : health === 'UNHEALTHY' ? 'destructive' : 'outline' }
function healthDotClass(health: Project['health']) { return health === 'HEALTHY' ? 'bg-primary' : health === 'UNHEALTHY' ? 'bg-destructive' : 'bg-muted-foreground' }
