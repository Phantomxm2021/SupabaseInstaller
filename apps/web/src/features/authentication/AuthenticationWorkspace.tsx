import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { Outlet, useOutletContext, useParams } from 'react-router-dom'
import { toast } from 'sonner'
import { Alert } from '@/components/ui/alert'
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { apiFetch } from '../../api/client'
import type { AuthConfig, GeneralConfig, RedactedProjectConfiguration, Services } from '../../api/types'
import { affectedServices, normalizeRedactedConfiguration, sectionImpact, sectionLabel, type PendingConfigurationSave } from '../project/configuration/types'
import { useConfigurationMutation } from '../project/configuration/useConfigurationMutation'
import { AuthenticationNavigation } from './navigation'
import { SignInProvidersPage } from './SignInProvidersPage'

type Snapshot = { projectId: string; revision: number; configuration: RedactedProjectConfiguration }
type SaveRequest = Omit<PendingConfigurationSave, 'labels' | 'services' | 'impact'> & { dirty: unknown; onQueued?: () => void }

export type AuthenticationWorkspaceContext = { projectId: string; revision: number; auth: AuthConfig; general: GeneralConfig; services: Services; requestSave: (request: SaveRequest) => void }
export function useAuthenticationWorkspace() { return useOutletContext<AuthenticationWorkspaceContext>() }

export function AuthenticationWorkspace() {
  const { projectId = '' } = useParams()
  const configuration = useQuery({ queryKey: ['project-configuration', projectId], queryFn: () => apiFetch<Snapshot>(`/api/projects/${projectId}/configuration`), enabled: Boolean(projectId) })
  const [pending, setPending] = useState<(PendingConfigurationSave & { onQueued?: () => void })>()
  const normalized = configuration.data ? normalizeRedactedConfiguration(configuration.data.configuration) : undefined
  const update = useConfigurationMutation(projectId, configuration.data?.revision ?? 0, () => { const next = pending; setPending(undefined); next?.onQueued?.(); toast.success('Configuration update queued') }, (error) => toast.error(error.message))
  const requestSave = (request: SaveRequest) => {
    if (!normalized) return
    const labels = dirtyLabels(request.dirty).map((label) => label.replaceAll('.', ' → '))
    if (!labels.length) return
    setPending({ ...request, labels, services: affectedServices(request.section, request.dirty, request.value, normalized.services), impact: sectionImpact(request.section, request.value, normalized.services) })
  }
  if (configuration.isLoading) return <main className="page"><div className="empty-state">Loading configuration…</div></main>
  if (configuration.error || !configuration.data || !normalized) return <main className="page"><Alert variant="destructive">Unable to load project configuration.</Alert></main>
  const context: AuthenticationWorkspaceContext = { projectId, revision: configuration.data.revision, auth: normalized.auth as unknown as AuthConfig, general: normalized.general, services: normalized.services, requestSave }
  return <section className="authentication-workspace"><AuthenticationNavigation /><div className="authentication-content"><Outlet context={context} /></div><AlertDialog open={Boolean(pending)} onOpenChange={(open) => !open && setPending(undefined)}><AlertDialogContent><AlertDialogHeader><AlertDialogTitle>Apply configuration changes?</AlertDialogTitle><AlertDialogDescription>Only the dirty fields in {pending && sectionLabel(pending.section)} will be sent.</AlertDialogDescription></AlertDialogHeader>{pending && <div className="space-y-3 text-sm"><div><strong>Changed settings</strong><ul className="mt-1 list-disc pl-5">{pending.labels.map((label) => <li key={label}>{label}</li>)}</ul></div><div><strong>Affected services</strong><p className="mt-1 text-muted-foreground">{pending.services.join(', ') || 'Configuration metadata only'}</p></div><Badge variant="outline">{pending.impact === 'recreate' ? 'Runtime recreate required' : 'No runtime restart expected'}</Badge></div>}<AlertDialogFooter><AlertDialogCancel>Keep editing</AlertDialogCancel><AlertDialogAction onClick={() => pending && update.mutate(pending)}>Confirm and apply</AlertDialogAction></AlertDialogFooter></AlertDialogContent></AlertDialog></section>
}

function dirtyLabels(value: unknown, path: string[] = []): string[] { if (value === true) return path.length ? [path.join('.')] : []; if (!value || typeof value !== 'object') return []; return Object.entries(value).flatMap(([key, child]) => dirtyLabels(child, [...path, key])) }

export function SignInProvidersRoute() {
  return <SignInProvidersPage />
}

export function EmailsRoute() {
  return <section className="page"><h1>Emails</h1></section>
}

export function URLConfigurationRoute() {
  return <section className="page"><h1>URL Configuration</h1></section>
}
