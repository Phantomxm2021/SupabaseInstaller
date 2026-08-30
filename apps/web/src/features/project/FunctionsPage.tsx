import { useEffect, useMemo, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, Archive, Check, ChevronDown, Code2, LoaderCircle, RotateCcw, ShieldAlert, Trash2, Upload } from 'lucide-react'
import { useParams } from 'react-router-dom'
import { toast } from 'sonner'
import { apiFetch } from '@/api/client'
import type { FunctionSummary, Operation } from '@/api/types'
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
import { useOperationEvents } from '../operations/useOperationEvents'

const functionStepLabels: Record<string, string> = {
  VALIDATING_ARCHIVE: 'Validating ZIP archive',
  STAGING_RELEASE: 'Staging function release',
  ACTIVATING_RELEASE: 'Activating function release',
  RESTARTING_FUNCTIONS: 'Restarting Functions service',
  DEPLOY_FUNCTION: 'Finalizing function deployment',
}

export function FunctionsPage() {
  const { projectId = '' } = useParams()
  const queryClient = useQueryClient()
  const [name, setName] = useState('')
  const [archive, setArchive] = useState<File | null>(null)
  const [deleteName, setDeleteName] = useState<string | null>(null)
  const [activeOperationId, setActiveOperationId] = useState<string | null>(null)
  const handledTerminalOperation = useRef<string | null>(null)
  const functions = useQuery({ queryKey: ['project-functions', projectId], queryFn: () => apiFetch<{ functions: FunctionSummary[]; enabled: boolean }>(`/api/projects/${projectId}/functions`), enabled: Boolean(projectId) })
  const operation = useQuery({
    queryKey: ['operation', activeOperationId],
    queryFn: () => apiFetch<Operation>(`/api/operations/${activeOperationId}`),
    enabled: Boolean(activeOperationId),
    refetchInterval: (query) => terminalOperation(query.state.data?.status) ? false : 2_000,
  })
  useOperationEvents(activeOperationId ?? '')
  useEffect(() => {
    const current = operation.data
    if (!activeOperationId || !current || !terminalOperation(current.status) || handledTerminalOperation.current === activeOperationId) return
    handledTerminalOperation.current = activeOperationId
    if (current.status === 'SUCCEEDED') void queryClient.invalidateQueries({ queryKey: ['project-functions', projectId] })
  }, [activeOperationId, operation.data, projectId, queryClient])
  const trackOperation = (operationId: string) => {
    handledTerminalOperation.current = null
    setActiveOperationId(operationId)
  }
  const upload = useMutation({
    mutationFn: async () => {
      if (!name.trim() || !archive) throw new Error('Enter a function name and choose a ZIP archive')
      const form = new FormData(); form.append('archive', archive)
      return apiFetch<{ operationId: string }>(`/api/projects/${projectId}/functions/${encodeURIComponent(name.trim())}/deploy`, { method: 'POST', body: form })
    },
    onSuccess: (result) => { trackOperation(result.operationId); toast.success(`Deployment queued (${result.operationId})`); setArchive(null); setName('') },
    onError: (error) => toast.error(error.message),
  })
  const rollback = useMutation({
    mutationFn: (functionName: string) => apiFetch<{ operationId: string }>(`/api/projects/${projectId}/functions/${encodeURIComponent(functionName)}/rollback`, { method: 'POST' }),
    onSuccess: (result) => { trackOperation(result.operationId); toast.success('Rollback queued') },
    onError: (error) => toast.error(error.message),
  })
  const remove = useMutation({
    mutationFn: (functionName: string) => apiFetch<{ operationId: string }>(`/api/projects/${projectId}/functions/${encodeURIComponent(functionName)}`, { method: 'DELETE', body: JSON.stringify({ confirmation: functionName }) }),
    onSuccess: (result) => { trackOperation(result.operationId); toast.success('Function deletion queued'); setDeleteName(null) },
    onError: (error) => toast.error(error.message),
  })
  const archiveLabel = useMemo(() => archive?.name ?? 'No ZIP selected', [archive])
  if (functions.isLoading) return <main className="page">Loading functions…</main>
  if (functions.error) return <main className="page"><Alert variant="destructive">{functions.error.message}</Alert></main>
  const items = functions.data?.functions ?? []
  const enabled = functions.data?.enabled ?? false
  const operationInProgress = Boolean(activeOperationId && !terminalOperation(operation.data?.status))
  return <main className="page functions-page" data-testid="functions-page">
    <PageHeader eyebrow="Edge Functions" title="Functions" description="Deploy a function ZIP and keep one previous release ready for rollback." />
    <Card className="functions-upload-card">
      <CardHeader className="functions-card-header"><CardTitle>Upload a function</CardTitle><CardDescription>The ZIP must contain index.ts at its root, inside a same-named folder, or under supabase/functions/function-name/. The filename can be function-name.zip.</CardDescription></CardHeader>
      {!enabled && <CardContent className="functions-service-alert"><Alert>Enable the Functions service in Server Settings before deploying code.</Alert></CardContent>}
      <CardContent className="functions-upload-content">
        <div className="functions-upload-grid">
          <div className="functions-field"><Label htmlFor="function-name">Function name</Label><Input id="function-name" placeholder="hello-world" value={name} onChange={(event) => setName(event.target.value)} /><span className="functions-field-help">Use lowercase letters, numbers, and hyphens.</span></div>
          <div className="functions-field"><Label htmlFor="function-archive">ZIP archive</Label><Input id="function-archive" type="file" accept=".zip,application/zip" onChange={(event) => { const selected = event.target.files?.[0] ?? null; setArchive(selected); if (selected && !name) setName(selected.name.replace(/\.zip$/i, '')) }} /><span className="functions-field-help">{archiveLabel}</span></div>
          <div className="functions-upload-action"><Button onClick={() => upload.mutate()} disabled={!enabled || upload.isPending || operationInProgress || !archive || !name.trim()}><Upload />{upload.isPending ? 'Uploading…' : operationInProgress ? 'Operation in progress…' : 'Deploy function'}</Button></div>
        </div>
      </CardContent>
      {upload.isPending && <CardContent className="functions-progress-content"><Progress value={55}><span className="sr-only">Uploading function</span></Progress></CardContent>}
    </Card>
    {activeOperationId && <FunctionOperationStatus operationId={activeOperationId} operation={operation.data} isLoading={operation.isLoading} />}
    <Card className="functions-list-card">
      <CardHeader className="functions-card-header"><CardTitle>Managed functions</CardTitle><CardDescription>Only Manager-owned releases appear here.</CardDescription></CardHeader>
      <CardContent className="functions-list-content">
        {items.length === 0 ? <div className="functions-empty-state"><Code2 /><p>No functions deployed yet.</p></div> : <Table><TableHeader><TableRow><TableHead>Function</TableHead><TableHead>Current release</TableHead><TableHead>Previous release</TableHead><TableHead className="text-right">Actions</TableHead></TableRow></TableHeader><TableBody>{items.map((item) => <TableRow key={item.name}><TableCell className="font-medium">{item.name}</TableCell><TableCell>{item.current ? <ReleaseBadge release={item.current} /> : <Badge variant="outline">None</Badge>}</TableCell><TableCell>{item.previous ? <ReleaseBadge release={item.previous} /> : <Badge variant="outline">Unavailable</Badge>}</TableCell><TableCell className="text-right"><DropdownMenu><DropdownMenuTrigger render={<Button variant="outline" size="sm" aria-label={`Actions for ${item.name}`} />}>Actions <ChevronDown /></DropdownMenuTrigger><DropdownMenuContent align="end"><DropdownMenuItem disabled={!enabled} onClick={() => { setName(item.name); document.getElementById('function-archive')?.focus() }}><Upload /> Deploy new version</DropdownMenuItem><DropdownMenuItem disabled={!enabled || !item.previous || rollback.isPending} onClick={() => rollback.mutate(item.name)}><RotateCcw /> Roll back</DropdownMenuItem><DropdownMenuItem variant="destructive" disabled={!enabled} onClick={() => setDeleteName(item.name)}><Trash2 /> Delete</DropdownMenuItem></DropdownMenuContent></DropdownMenu></TableCell></TableRow>)}</TableBody></Table>}
      </CardContent>
    </Card>
    <AlertDialog open={deleteName !== null} onOpenChange={(open) => { if (!open) setDeleteName(null) }}>
      <AlertDialogContent><AlertDialogHeader><AlertDialogTitle>Delete {deleteName}?</AlertDialogTitle><AlertDialogDescription>This removes the managed releases and restarts the Functions service. Unmanaged files are not touched.</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel>Cancel</AlertDialogCancel><AlertDialogAction variant="destructive" disabled={remove.isPending} onClick={() => { if (deleteName) remove.mutate(deleteName) }}>Delete function</AlertDialogAction></AlertDialogFooter></AlertDialogContent>
    </AlertDialog>
  </main>
}

function FunctionOperationStatus({ operationId, operation, isLoading }: { operationId: string; operation?: Operation; isLoading: boolean }) {
  const status = operation?.status ?? 'QUEUED'
  const failed = status === 'FAILED'
  const rolledBack = status === 'ROLLED_BACK'
  const succeeded = status === 'SUCCEEDED'
  const title = succeeded ? 'Function operation complete' : failed ? 'Function operation failed' : rolledBack ? 'Function operation rolled back' : isLoading ? 'Loading function operation' : 'Function operation in progress'
  const step = functionStepLabels[operation?.currentStep ?? ''] ?? operation?.currentStep ?? 'Waiting for worker'
  const badgeVariant = failed || rolledBack ? 'destructive' : succeeded ? 'default' : 'outline'
  return <Card className="functions-operation-card" aria-live="polite">
    <CardContent className="functions-operation-content">
      <span className={`functions-operation-icon ${failed || rolledBack ? 'failed' : succeeded ? 'done' : ''}`}>{failed || rolledBack ? <AlertTriangle /> : succeeded ? <Check /> : <LoaderCircle className="spin" />}</span>
      <div className="functions-operation-copy"><p className="functions-operation-kicker">Operation {operationId}</p><h2>{title}</h2><p>{step}</p></div>
      <Badge variant={badgeVariant}>{status}</Badge>
    </CardContent>
    {!succeeded && <CardContent className="functions-operation-progress"><Progress value={operation?.progress ?? 0}><span className="sr-only">Function operation progress</span></Progress></CardContent>}
    {operation?.errorMessage && <CardContent className="functions-operation-error"><Alert variant="destructive"><ShieldAlert className="size-4" /><span>{operation.errorMessage}</span></Alert></CardContent>}
  </Card>
}

function ReleaseBadge({ release }: { release: { sha256: string; deployedAt: string } }) {
  return <span className="inline-flex items-center gap-2"><Archive className="size-4 text-muted-foreground" /><span className="font-mono text-xs">{release.sha256.slice(0, 12)}</span><span className="text-xs text-muted-foreground">{new Date(release.deployedAt).toLocaleString()}</span></span>
}

function terminalOperation(status?: Operation['status']) {
  return status ? ['SUCCEEDED', 'FAILED', 'ROLLED_BACK', 'CANCELLED'].includes(status) : false
}
