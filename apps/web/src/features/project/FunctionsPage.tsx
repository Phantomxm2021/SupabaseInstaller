import { useEffect, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, Archive, Check, ChevronDown, Code2, FileArchive, LoaderCircle, RotateCcw, ScrollText, ShieldAlert, Trash2, Upload, X } from 'lucide-react'
import { useNavigate, useParams } from 'react-router-dom'
import { toast } from 'sonner'
import { apiFetch } from '@/api/client'
import type { FunctionSummary, Operation } from '@/api/types'
import { PageHeader } from '@/components/app/PageHeader'
import { Alert } from '@/components/ui/alert'
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
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
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [name, setName] = useState('')
  const [archive, setArchive] = useState<File | null>(null)
  const [deployDialogOpen, setDeployDialogOpen] = useState(false)
  const [deleteName, setDeleteName] = useState<string | null>(null)
  const [activeOperationId, setActiveOperationId] = useState<string | null>(null)
  const [operationSurface, setOperationSurface] = useState<'dialog' | 'page' | null>(null)
  const handledTerminalOperation = useRef<string | null>(null)
  const archiveInputRef = useRef<HTMLInputElement>(null)
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
  const trackOperation = (operationId: string, surface: 'dialog' | 'page' = 'page') => {
    handledTerminalOperation.current = null
    setOperationSurface(surface)
    setActiveOperationId(operationId)
  }
  const openDeploymentDialog = (functionName?: string) => {
    if (operationSurface === 'dialog' && terminalOperation(operation.data?.status)) {
      setActiveOperationId(null)
      setOperationSurface(null)
    }
    if (functionName) setName(functionName)
    setDeployDialogOpen(true)
  }
  const upload = useMutation({
    mutationFn: async () => {
      if (!name.trim() || !archive) throw new Error('Enter a function name and choose a ZIP archive')
      const form = new FormData(); form.append('archive', archive)
      return apiFetch<{ operationId: string }>(`/api/projects/${projectId}/functions/${encodeURIComponent(name.trim())}/deploy`, { method: 'POST', body: form })
    },
    onSuccess: (result) => { trackOperation(result.operationId, 'dialog'); toast.success(`Deployment queued (${result.operationId})`); setArchive(null); setName('') },
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
  const clearArchive = () => {
    setArchive(null)
    if (archiveInputRef.current) archiveInputRef.current.value = ''
  }
  if (functions.isLoading) return <main className="page">Loading functions…</main>
  if (functions.error) return <main className="page"><Alert variant="destructive">{functions.error.message}</Alert></main>
  const items = functions.data?.functions ?? []
  const enabled = functions.data?.enabled ?? false
  const operationInProgress = Boolean(activeOperationId && !terminalOperation(operation.data?.status))
  const deploymentStatusVisible = upload.isPending || (activeOperationId !== null && operationSurface === 'dialog')
  return <main className="page functions-page" data-testid="functions-page" data-density="dashboard">
    <PageHeader eyebrow="Edge Functions" title="Edge Functions" description="Run server-side logic close to your users." actions={<Button onClick={() => openDeploymentDialog()} disabled={!enabled || operationInProgress}><Upload />Deploy a new function</Button>} />
    {!enabled && <Alert>Enable the Functions service in Project Settings before deploying code.</Alert>}
    {activeOperationId && operationSurface === 'page' && <FunctionOperationStatus operationId={activeOperationId} operation={operation.data} isLoading={operation.isLoading} />}
    <Card className="functions-list-card">
      <CardHeader className="functions-card-header"><CardTitle>Managed functions</CardTitle><CardDescription>Only Manager-owned releases appear here.</CardDescription></CardHeader>
      <CardContent className="functions-list-content">
        {items.length === 0 ? <div className="functions-empty-state"><Code2 /><p>No functions deployed yet.</p></div> : <Table><TableHeader><TableRow><TableHead>Function</TableHead><TableHead>Current release</TableHead><TableHead>Previous release</TableHead><TableHead className="text-right">Actions</TableHead></TableRow></TableHeader><TableBody>{items.map((item) => <TableRow key={item.name}><TableCell className="font-medium">{item.name}</TableCell><TableCell>{item.current ? <ReleaseBadge release={item.current} /> : <Badge variant="outline">None</Badge>}</TableCell><TableCell>{item.previous ? <ReleaseBadge release={item.previous} /> : <Badge variant="outline">Unavailable</Badge>}</TableCell><TableCell className="text-right"><DropdownMenu><DropdownMenuTrigger render={<Button variant="outline" size="sm" aria-label={`Actions for ${item.name}`} />}>Actions <ChevronDown /></DropdownMenuTrigger><DropdownMenuContent align="end"><DropdownMenuItem onClick={() => navigate('/projects/' + encodeURIComponent(projectId) + '/functions/' + encodeURIComponent(item.name) + '/logs')}><ScrollText /> View logs</DropdownMenuItem><DropdownMenuItem disabled={!enabled} onClick={() => openDeploymentDialog(item.name)}><Upload /> Deploy new version</DropdownMenuItem><DropdownMenuItem disabled={!enabled || !item.previous || rollback.isPending} onClick={() => rollback.mutate(item.name)}><RotateCcw /> Roll back</DropdownMenuItem><DropdownMenuItem variant="destructive" disabled={!enabled} onClick={() => setDeleteName(item.name)}><Trash2 /> Delete</DropdownMenuItem></DropdownMenuContent></DropdownMenu></TableCell></TableRow>)}</TableBody></Table>}
      </CardContent>
    </Card>
    <Dialog open={deployDialogOpen} onOpenChange={setDeployDialogOpen}>
      <DialogContent>
        {deploymentStatusVisible ? <>
          <DialogHeader><DialogTitle>{upload.isPending && !activeOperationId ? 'Uploading function archive' : 'Deployment status'}</DialogTitle><DialogDescription>The deployment continues if you close this dialog.</DialogDescription></DialogHeader>
          {upload.isPending && !activeOperationId ? <div className="grid gap-3" role="status"><div className="flex items-center gap-2 text-sm"><LoaderCircle className="size-4 animate-spin" />Uploading archive</div><Progress value={55}><span className="sr-only">Uploading function archive</span></Progress></div> : activeOperationId ? <FunctionDeploymentStatus operation={operation.data} isLoading={operation.isLoading} /> : null}
          <DialogFooter><Button variant="outline" onClick={() => setDeployDialogOpen(false)}>Close</Button></DialogFooter>
        </> : <>
          <DialogHeader><DialogTitle>Deploy a function</DialogTitle><DialogDescription>Upload a ZIP archive to deploy an Edge Function to this project.</DialogDescription></DialogHeader>
          <div className="grid gap-6">
            <div className="grid gap-2"><Label htmlFor="function-name">Function name</Label><Input id="function-name" placeholder="Give your function a name..." value={name} onChange={(event) => setName(event.target.value)} /><p className="text-xs text-muted-foreground">This name is used in the function URL.</p></div>
            <div className="grid gap-3"><div className="grid gap-1"><Label htmlFor="function-archive">Function ZIP file</Label><p className="text-xs text-muted-foreground">The archive must include <code>index.ts</code> at its root or in the requested function directory.</p></div><Input ref={archiveInputRef} id="function-archive" className="sr-only" type="file" accept=".zip,application/zip" onChange={(event) => { const selected = event.target.files?.[0] ?? null; setArchive(selected); if (selected && !name) setName(selected.name.replace(/\.zip$/i, '')) }} />{archive ? <Card className="border-border shadow-none"><CardContent className="flex items-center gap-3 px-3 py-3"><FileArchive className="size-5 text-muted-foreground" /><div className="min-w-0 flex-1"><p className="truncate text-sm">{archive.name}</p><p className="text-xs text-muted-foreground">ZIP archive ready to deploy</p></div><Button type="button" size="icon-sm" variant="ghost" aria-label="Remove ZIP file" onClick={clearArchive}><X /></Button></CardContent></Card> : <Button type="button" variant="outline" className="w-full justify-center border-dashed" onClick={() => archiveInputRef.current?.click()}><Upload />Choose ZIP file</Button>}</div>
          </div>
          <DialogFooter><Button variant="outline" onClick={() => setDeployDialogOpen(false)}>Cancel</Button><Button onClick={() => upload.mutate()} disabled={!enabled || upload.isPending || operationInProgress || !archive || !name.trim()}><Upload />Deploy function</Button></DialogFooter>
        </>}
      </DialogContent>
    </Dialog>
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

function FunctionDeploymentStatus({ operation, isLoading }: { operation?: Operation; isLoading: boolean }) {
  const status = operation?.status ?? 'QUEUED'
  const failed = status === 'FAILED' || status === 'ROLLED_BACK'
  const succeeded = status === 'SUCCEEDED'
  const title = succeeded ? 'Deployment complete' : failed ? 'Deployment failed' : isLoading ? 'Loading deployment status' : 'Deployment in progress'
  const step = functionStepLabels[operation?.currentStep ?? ''] ?? operation?.currentStep ?? 'Waiting for deployment to begin'
  return <div className="grid gap-4 rounded-lg border border-border bg-muted/20 p-4" role="status">
    <div className="flex items-start gap-3"><span className={`mt-0.5 grid size-7 place-items-center rounded-full ${failed ? 'bg-destructive/15 text-destructive' : succeeded ? 'bg-primary/15 text-primary' : 'bg-muted text-muted-foreground'}`}>{failed ? <AlertTriangle className="size-4" /> : succeeded ? <Check className="size-4" /> : <LoaderCircle className="size-4 animate-spin" />}</span><div className="min-w-0 flex-1"><p className="text-sm font-medium">{title}</p><p className="mt-1 text-xs text-muted-foreground">{step}</p></div><Badge variant={failed ? 'destructive' : succeeded ? 'default' : 'outline'}>{status}</Badge></div>
    {!succeeded && !failed && <Progress value={operation?.progress ?? 0}><span className="sr-only">Function deployment progress</span></Progress>}
    {operation?.errorMessage && <Alert variant="destructive"><ShieldAlert className="size-4" /><span>{operation.errorMessage}</span></Alert>}
  </div>
}

function ReleaseBadge({ release }: { release: { sha256: string; deployedAt: string } }) {
  return <span className="inline-flex items-center gap-2"><Archive className="size-4 text-muted-foreground" /><span className="font-mono text-xs">{release.sha256.slice(0, 12)}</span><span className="text-xs text-muted-foreground">{new Date(release.deployedAt).toLocaleString()}</span></span>
}

function terminalOperation(status?: Operation['status']) {
  return status ? ['SUCCEEDED', 'FAILED', 'ROLLED_BACK', 'CANCELLED'].includes(status) : false
}
