import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Archive, ChevronDown, Code2, RotateCcw, Trash2, Upload } from 'lucide-react'
import { useParams } from 'react-router-dom'
import { toast } from 'sonner'
import { apiFetch } from '@/api/client'
import type { FunctionSummary } from '@/api/types'
import { PageHeader } from '@/components/app/PageHeader'
import { Alert } from '@/components/ui/alert'
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Progress } from '@/components/ui/progress'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'

export function FunctionsPage() {
  const { projectId = '' } = useParams()
  const queryClient = useQueryClient()
  const [name, setName] = useState('')
  const [archive, setArchive] = useState<File | null>(null)
  const [deleteName, setDeleteName] = useState<string | null>(null)
  const functions = useQuery({ queryKey: ['project-functions', projectId], queryFn: () => apiFetch<{ functions: FunctionSummary[]; enabled: boolean }>(`/api/projects/${projectId}/functions`), enabled: Boolean(projectId) })
  const upload = useMutation({
    mutationFn: async () => {
      if (!name.trim() || !archive) throw new Error('Enter a function name and choose a ZIP archive')
      const form = new FormData(); form.append('archive', archive)
      return apiFetch<{ operationId: string }>(`/api/projects/${projectId}/functions/${encodeURIComponent(name.trim())}/deploy`, { method: 'POST', body: form })
    },
    onSuccess: (result) => { toast.success(`Deployment queued (${result.operationId})`); setArchive(null); setName(''); void queryClient.invalidateQueries({ queryKey: ['project-functions', projectId] }) },
    onError: (error) => toast.error(error.message),
  })
  const rollback = useMutation({
    mutationFn: (functionName: string) => apiFetch<{ operationId: string }>(`/api/projects/${projectId}/functions/${encodeURIComponent(functionName)}/rollback`, { method: 'POST' }),
    onSuccess: () => { toast.success('Rollback queued'); void queryClient.invalidateQueries({ queryKey: ['project-functions', projectId] }) },
    onError: (error) => toast.error(error.message),
  })
  const remove = useMutation({
    mutationFn: (functionName: string) => apiFetch(`/api/projects/${projectId}/functions/${encodeURIComponent(functionName)}`, { method: 'DELETE', body: JSON.stringify({ confirmation: functionName }) }),
    onSuccess: () => { toast.success('Function deleted'); setDeleteName(null); void queryClient.invalidateQueries({ queryKey: ['project-functions', projectId] }) },
    onError: (error) => toast.error(error.message),
  })
  const archiveLabel = useMemo(() => archive?.name ?? 'No ZIP selected', [archive])
  if (functions.isLoading) return <main className="page">Loading functions…</main>
  if (functions.error) return <main className="page"><Alert variant="destructive">{functions.error.message}</Alert></main>
  const items = functions.data?.functions ?? []
  const enabled = functions.data?.enabled ?? false
  return <main className="page" data-testid="functions-page">
    <PageHeader eyebrow="Edge Functions" title="Functions" description="Deploy a function ZIP and keep one previous release ready for rollback." />
    <Card>
      <CardHeader><CardTitle>Upload a function</CardTitle><CardDescription>The ZIP must contain index.ts at its root. The filename can be function-name.zip.</CardDescription></CardHeader>
      {!enabled && <CardContent className="pt-0"><Alert>Enable the Functions service in Server Settings before deploying code.</Alert></CardContent>}
      <CardContent className="grid gap-4 md:grid-cols-[1fr_1fr_auto] md:items-end">
        <div className="grid gap-2"><Label htmlFor="function-name">Function name</Label><Input id="function-name" placeholder="hello-world" value={name} onChange={(event) => setName(event.target.value)} /></div>
        <div className="grid gap-2"><Label htmlFor="function-archive">ZIP archive</Label><Input id="function-archive" type="file" accept=".zip,application/zip" onChange={(event) => { const selected = event.target.files?.[0] ?? null; setArchive(selected); if (selected && !name) setName(selected.name.replace(/\.zip$/i, '')) }} /><span className="text-xs text-muted-foreground">{archiveLabel}</span></div>
        <Button onClick={() => upload.mutate()} disabled={!enabled || upload.isPending || !archive || !name.trim()}><Upload />{upload.isPending ? 'Uploading…' : 'Deploy function'}</Button>
      </CardContent>
      {upload.isPending && <CardContent className="pt-0"><Progress value={55}><span className="sr-only">Uploading function</span></Progress></CardContent>}
    </Card>
    <Card>
      <CardHeader><CardTitle>Managed functions</CardTitle><CardDescription>Only Manager-owned releases appear here.</CardDescription></CardHeader>
      <CardContent className="p-0">
        {items.length === 0 ? <div className="empty-state p-6"><Code2 /><p>No functions deployed yet.</p></div> : <Table><TableHeader><TableRow><TableHead>Function</TableHead><TableHead>Current release</TableHead><TableHead>Previous release</TableHead><TableHead className="text-right">Actions</TableHead></TableRow></TableHeader><TableBody>{items.map((item) => <TableRow key={item.name}><TableCell className="font-medium">{item.name}</TableCell><TableCell>{item.current ? <ReleaseBadge release={item.current} /> : <Badge variant="outline">None</Badge>}</TableCell><TableCell>{item.previous ? <ReleaseBadge release={item.previous} /> : <Badge variant="outline">Unavailable</Badge>}</TableCell><TableCell className="text-right"><DropdownMenu><DropdownMenuTrigger render={<Button variant="outline" size="sm" aria-label={`Actions for ${item.name}`} />}>Actions <ChevronDown /></DropdownMenuTrigger><DropdownMenuContent align="end"><DropdownMenuItem disabled={!enabled} onClick={() => { setName(item.name); document.getElementById('function-archive')?.focus() }}><Upload /> Deploy new version</DropdownMenuItem><DropdownMenuItem disabled={!enabled || !item.previous || rollback.isPending} onClick={() => rollback.mutate(item.name)}><RotateCcw /> Roll back</DropdownMenuItem><DropdownMenuItem variant="destructive" disabled={!enabled} onClick={() => setDeleteName(item.name)}><Trash2 /> Delete</DropdownMenuItem></DropdownMenuContent></DropdownMenu></TableCell></TableRow>)}</TableBody></Table>}
      </CardContent>
    </Card>
    <AlertDialog open={deleteName !== null} onOpenChange={(open) => { if (!open) setDeleteName(null) }}>
      <AlertDialogContent><AlertDialogHeader><AlertDialogTitle>Delete {deleteName}?</AlertDialogTitle><AlertDialogDescription>This removes the managed releases and restarts the Functions service. Unmanaged files are not touched.</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel>Cancel</AlertDialogCancel><AlertDialogAction variant="destructive" disabled={remove.isPending} onClick={() => { if (deleteName) remove.mutate(deleteName) }}>Delete function</AlertDialogAction></AlertDialogFooter></AlertDialogContent>
    </AlertDialog>
  </main>
}

function ReleaseBadge({ release }: { release: { sha256: string; deployedAt: string } }) {
  return <span className="inline-flex items-center gap-2"><Archive className="size-4 text-muted-foreground" /><span className="font-mono text-xs">{release.sha256.slice(0, 12)}</span><span className="text-xs text-muted-foreground">{new Date(release.deployedAt).toLocaleString()}</span></span>
}
