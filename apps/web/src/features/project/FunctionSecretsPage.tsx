import { useQuery } from '@tanstack/react-query'
import { Eye, EyeOff, Search, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useParams } from 'react-router-dom'
import { toast } from 'sonner'
import { APIError, apiFetch } from '@/api/client'
import type { FunctionVariable, FunctionsConfig, RedactedProjectConfiguration } from '@/api/types'
import { PageHeader } from '@/components/app/PageHeader'
import { Alert } from '@/components/ui/alert'
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Textarea } from '@/components/ui/textarea'
import { OperationPanel } from '../operations/OperationPanel'
import { functionsSchema } from './configuration/schema'
import { affectedServices, dirtyLabels, normalizeRedactedConfiguration, sectionImpact, type PendingConfigurationSave } from './configuration/types'
import { useConfigurationMutation } from './configuration/useConfigurationMutation'

type ConfigurationSnapshot = {
  projectId: string
  revision: number
  lastGoodRevision: number
  configuration: RedactedProjectConfiguration
}

type SaveInput = {
  value: unknown
  dirty: unknown
  setError: (name: string, message: string) => void
}

type SecretDraft = {
  id: number
  name: string
  value: string
  visible: boolean
}

const defaultSecrets = [
  ['SUPABASE_URL', 'The API gateway for your Supabase project.'],
  ['SUPABASE_DB_URL', 'The direct PostgreSQL connection URL. Should not be shared with anyone, only use it on the server.'],
  ['SUPABASE_PUBLISHABLE_KEYS', 'JSON dictionary of publishable API keys. Safe to use in a browser if RLS is enabled.'],
  ['SUPABASE_SECRET_KEYS', 'JSON dictionary of secret API keys. Should never be exposed to a browser.'],
  ['SUPABASE_ANON_KEY', 'Legacy anonymous key. Use SUPABASE_PUBLISHABLE_KEYS issued through JWT Signing Keys instead.'],
  ['SUPABASE_SERVICE_ROLE_KEY', 'Legacy service role key. Use SUPABASE_SECRET_KEYS issued through JWT Signing Keys instead.'],
  ['SUPABASE_JWKS', "JSON Web Key Set used to verify JWTs issued by your project's auth server."],
  ['SB_REGION', 'The region the function was invoked in. Set per request.'],
  ['SB_EXECUTION_ID', 'A unique identifier for each function instance. Set per request.'],
  ['DENO_DEPLOYMENT_ID', 'The version of the function code. Set when the function is deployed.'],
] as const

const newDraft = (id: number): SecretDraft => ({ id, name: '', value: '', visible: false })

export function FunctionSecretsPage() {
  const { projectId = '' } = useParams()
  const configuration = useQuery({
    queryKey: ['project-configuration', projectId],
    queryFn: () => apiFetch<ConfigurationSnapshot>(`/api/projects/${projectId}/configuration`),
    enabled: Boolean(projectId),
  })
  const [pending, setPending] = useState<PendingConfigurationSave>()
  const [operation, setOperation] = useState<{ projectId: string; operationId: string }>()
  const [conflict, setConflict] = useState(false)
  const config = useMemo(() => configuration.data ? normalizeRedactedConfiguration(configuration.data.configuration) : undefined, [configuration.data?.configuration, configuration.data?.revision])
  const update = useConfigurationMutation(projectId, configuration.data?.revision ?? 0, (result) => {
    setPending(undefined)
    setOperation(result)
    setConflict(false)
    toast.success('Functions secrets update queued')
  }, (error) => {
    if (error instanceof APIError && error.status === 409) {
      setConflict(true)
      setPending(undefined)
    }
    toast.error(error.message)
  })
  const submit = ({ value, dirty, setError }: SaveInput) => {
    if (!config) return
    const labels = dirtyLabels(dirty).map((name) => name.replaceAll('.', ' → '))
    if (!labels.length) return
    setPending({
      section: 'functions',
      value,
      labels,
      services: affectedServices('functions', dirty, value, config.services),
      impact: sectionImpact('functions', value, config.services),
      setError,
    })
  }
  const completed = async () => {
    await configuration.refetch()
    setOperation(undefined)
  }

  if (configuration.isLoading) return <main className="page">Loading function secrets…</main>
  if (configuration.error || !configuration.data || !config) return <main className="page"><Alert variant="destructive">Unable to load function secrets.</Alert></main>

  return <main className="page functions-secrets-page" data-density="dashboard">
    <PageHeader className="functions-secrets-heading" title="Edge Function Secrets" description="Manage encrypted values for your functions" />
    {operation ? <OperationPanel operationId={operation.operationId} projectId={projectId} projectName={projectId} onSucceeded={() => void completed()} /> : <>
      {conflict && <Alert variant="destructive" className="mb-4"><div className="flex items-center justify-between gap-3"><span>This configuration is stale. Your dirty fields are preserved.</span><Button size="sm" variant="outline" onClick={() => { setConflict(false); void configuration.refetch() }}>Reload</Button></div></Alert>}
      <FunctionSecretsEditor revision={configuration.data.revision} initial={config.functions as FunctionsConfig} onSave={submit} />
    </>}
    <AlertDialog open={Boolean(pending)} onOpenChange={(open) => !open && setPending(undefined)}>
      <AlertDialogContent>
        <AlertDialogHeader><AlertDialogTitle>Apply Functions secrets changes?</AlertDialogTitle><AlertDialogDescription>Only the changed environment variables and settings will be sent.</AlertDialogDescription></AlertDialogHeader>
        {pending && <div className="space-y-3 text-sm"><div><strong>Changed settings</strong><ul className="mt-1 list-disc pl-5">{pending.labels.map((label) => <li key={label}>{label}</li>)}</ul></div><div><strong>Affected services</strong><p className="mt-1 text-muted-foreground">{pending.services.join(', ') || 'Configuration metadata only'}</p></div><Badge variant="outline">{pending.impact === 'recreate' ? 'Runtime recreate required' : pending.impact === 'restart' ? 'Service restart required' : pending.impact === 'start' ? 'Service will be started' : pending.impact === 'stop' ? 'Service will be stopped' : 'No runtime restart expected'}</Badge></div>}
        <AlertDialogFooter><AlertDialogCancel>Keep editing</AlertDialogCancel><AlertDialogAction onClick={() => pending && update.mutate(pending)}>Confirm and apply</AlertDialogAction></AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </main>
}

function FunctionSecretsEditor({ initial, revision, onSave }: { initial: FunctionsConfig; revision: number; onSave: (input: SaveInput) => void }) {
  const [drafts, setDrafts] = useState<SecretDraft[]>(() => [newDraft(1)])
  const [removed, setRemoved] = useState<string[]>([])
  const [search, setSearch] = useState('')
  const [errors, setErrors] = useState<Record<string, string>>({})

  useEffect(() => {
    setDrafts([newDraft(1)])
    setRemoved([])
    setErrors({})
  }, [revision])

  const configured = useMemo(() => initial.variables.filter((variable) => variable.valueSet && !removed.includes(variable.name)), [initial.variables, removed])
  const filtered = configured.filter((variable) => variable.name.toLowerCase().includes(search.trim().toLowerCase()))
  const changed = drafts.some((draft) => draft.name.trim() || draft.value) || removed.length > 0

  const updateDraft = (id: number, patch: Partial<SecretDraft>) => setDrafts((current) => current.map((draft) => draft.id === id ? { ...draft, ...patch } : draft))
  const removeDraft = (id: number) => setDrafts((current) => current.length === 1 ? current : current.filter((draft) => draft.id !== id))
  const addDraft = () => setDrafts((current) => [...current, newDraft(Math.max(...current.map((draft) => draft.id), 0) + 1)])

  const save = () => {
    const variables = mergeVariables(initial.variables, drafts, removed)
    const value = { ...initial, variables }
    const parsed = functionsSchema.safeParse(value)
    if (!parsed.success) {
      const nextErrors: Record<string, string> = {}
      for (const issue of parsed.error.issues) nextErrors[issue.path.join('.')] = issue.message
      setErrors(nextErrors)
      return
    }
    setErrors({})
    onSave({ value, dirty: { variables: true }, setError: (name, message) => setErrors((current) => ({ ...current, [name]: message })) })
  }

  return <div className="function-secrets-workspace">
    <form className="function-secrets-editor" onSubmit={(event) => { event.preventDefault(); save() }}>
      <header className="function-secrets-editor-title"><h2>Add or replace secrets</h2></header>
      <div className="function-secrets-editor-body">
        {drafts.map((draft, index) => <div className="function-secrets-editor-fields" key={draft.id}>
          <div className="function-secrets-editor-row"><div className="function-secrets-editor-copy"><label htmlFor={`function-secret-name-${draft.id}`}>Name</label><p>A unique name for your secret.</p></div>
          <div className="function-secrets-editor-control">
            <div className="function-secrets-name-control"><Input id={`function-secret-name-${draft.id}`} placeholder="e.g. CLIENT_KEY" value={draft.name} onChange={(event) => updateDraft(draft.id, { name: event.target.value.toUpperCase() })} aria-invalid={Boolean(errors[`variables.${index}.name`])} /><Button type="button" variant="outline" size="icon" aria-label="Remove secret" disabled={drafts.length === 1} onClick={() => removeDraft(draft.id)}><Trash2 /></Button></div>
            {errors[`variables.${index}.name`] && <p className="function-secret-error">{errors[`variables.${index}.name`]}</p>}
          </div></div>
          <div className="function-secrets-editor-row"><div className="function-secrets-editor-copy"><label htmlFor={`function-secret-value-${draft.id}`}>Value</label><p>Supports multi-line values such as PEM keys, JSON, or functions.</p></div><div className="function-secrets-editor-control"><div className="function-secrets-value-control"><Textarea id={`function-secret-value-${draft.id}`} aria-label={`Value for ${draft.name || 'new secret'}`} className={draft.visible ? undefined : 'function-secret-value-concealed'} value={draft.value} onChange={(event) => updateDraft(draft.id, { value: event.target.value })} autoComplete="new-password" /><Button type="button" variant="ghost" size="icon" aria-label={draft.visible ? 'Hide secret value' : 'Show secret value'} onClick={() => updateDraft(draft.id, { visible: !draft.visible })}>{draft.visible ? <EyeOff /> : <Eye />}</Button></div>{errors[`variables.${index}.value`] && <p className="function-secret-error">{errors[`variables.${index}.value`]}</p>}</div></div>
        </div>)}
      </div>
      <footer className="function-secrets-editor-footer"><p>Insert or update multiple secrets at once by pasting key-value pairs</p><div><Button type="button" variant="outline" size="sm" onClick={addDraft}>Add another</Button><Button type="submit" size="sm" disabled={!changed}>Save</Button></div></footer>
    </form>

    <section className="function-secrets-listing" aria-labelledby="custom-secrets-title">
      <div className="function-secrets-listing-header"><div><h2 id="custom-secrets-title">Custom secrets</h2><p>Secrets you have defined for this project</p></div><div className="function-secrets-search"><Search /><Input aria-label="Search for a secret" placeholder="Search for a secret" value={search} onChange={(event) => setSearch(event.target.value)} /></div></div>
      <div className="function-secrets-table"><Table><TableHeader><TableRow><TableHead>Name</TableHead><TableHead>Digest <Badge variant="outline">SHA256</Badge></TableHead><TableHead>Updated</TableHead><TableHead><span className="sr-only">Actions</span></TableHead></TableRow></TableHeader><TableBody>{filtered.length ? filtered.map((variable) => <TableRow key={variable.name}><TableCell><Button type="button" variant="ghost" size="xs" className="function-secret-name-button">{variable.name}</Button></TableCell><TableCell className="text-muted-foreground">Unavailable</TableCell><TableCell className="text-muted-foreground">—</TableCell><TableCell className="text-right"><Button type="button" variant="ghost" size="icon-sm" aria-label={`Remove ${variable.name}`} onClick={() => setRemoved((current) => [...current, variable.name])}><Trash2 /></Button></TableCell></TableRow>) : <TableRow><TableCell colSpan={4} className="function-secrets-empty"><strong>{search ? 'No matching custom secrets' : 'No custom secrets created'}</strong><span>{search ? 'Try another secret name.' : 'This project has no custom secrets yet.'}</span></TableCell></TableRow>}</TableBody></Table></div>
    </section>

    <section className="function-secrets-listing" aria-labelledby="default-secrets-title">
      <div className="function-secrets-listing-header"><div><h2 id="default-secrets-title">Default secrets</h2><p>Reserved secrets available in every project</p></div><a className="function-secrets-docs-link" href="https://supabase.com/docs/guides/functions/secrets#default-secrets" target="_blank" rel="noreferrer">Docs</a></div>
      <div className="function-secrets-table"><Table><TableHeader><TableRow><TableHead>Name</TableHead><TableHead>Description</TableHead></TableRow></TableHeader><TableBody>{defaultSecrets.map(([name, description]) => <TableRow key={name}><TableCell><Button type="button" variant="ghost" size="xs" className="function-secret-name-button">{name}</Button>{name === 'SUPABASE_ANON_KEY' || name === 'SUPABASE_SERVICE_ROLE_KEY' ? <Badge variant="outline" className="ml-2">Deprecated</Badge> : null}</TableCell><TableCell className="whitespace-normal text-muted-foreground">{description}</TableCell></TableRow>)}</TableBody></Table></div>
    </section>
  </div>
}

function mergeVariables(initial: FunctionVariable[], drafts: SecretDraft[], removed: string[]): FunctionVariable[] {
  const existing: FunctionVariable[] = initial.map((variable) => ({ ...variable, value: variable.valueSet ? { action: removed.includes(variable.name) ? 'remove' : 'retain' } : { action: '' } }))
  for (const draft of drafts) {
    const name = draft.name.trim()
    if (!name && !draft.value) continue
    const index = existing.findIndex((variable) => variable.name === name)
    const replacement: FunctionVariable = { name, valueSet: true, value: { action: 'replace', value: draft.value } }
    if (index >= 0) existing[index] = replacement
    else existing.push(replacement)
  }
  return existing
}
