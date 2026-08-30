import { useQuery } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useParams } from 'react-router-dom'
import { toast } from 'sonner'
import { APIError, apiFetch } from '@/api/client'
import type { FunctionsConfig, RedactedProjectConfiguration } from '@/api/types'
import { PageHeader } from '@/components/app/PageHeader'
import { Alert } from '@/components/ui/alert'
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { OperationPanel } from '../operations/OperationPanel'
import { FunctionsSection } from './configuration/FunctionsSection'
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

  return <main className="page functions-secrets-page">
    <PageHeader eyebrow="Edge Functions" title="Functions" description="Configure encrypted environment variables for your deployed functions." />
    {operation ? <OperationPanel operationId={operation.operationId} projectId={projectId} projectName={projectId} onSucceeded={() => void completed()} /> : <>
      {conflict && <Alert variant="destructive" className="mb-4"><div className="flex items-center justify-between gap-3"><span>This configuration is stale. Your dirty fields are preserved.</span><Button size="sm" variant="outline" onClick={() => { setConflict(false); void configuration.refetch() }}>Reload</Button></div></Alert>}
      <FunctionsSection revision={configuration.data.revision} initial={config.functions as FunctionsConfig} enabled={config.services.functions} onSave={submit} />
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
