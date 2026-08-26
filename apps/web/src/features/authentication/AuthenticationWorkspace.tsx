import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { Outlet, useOutletContext, useParams } from 'react-router-dom'
import { toast } from 'sonner'
import { Alert } from '@/components/ui/alert'
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { apiFetch } from '../../api/client'
import type { AuthConfig, GeneralConfig, RedactedProjectConfiguration, Services } from '../../api/types'
import { affectedServices, normalizeRedactedConfiguration, sectionImpact, sectionLabel, type PendingConfigurationSave } from '../project/configuration/types'
import { useConfigurationMutation } from '../project/configuration/useConfigurationMutation'
import { AuthenticationNavigation } from './navigation'
import { SignInProvidersPage } from './SignInProvidersPage'
import { EmailsPage } from './EmailsPage'
import { EmailTemplateEditorPage } from './EmailTemplateEditorPage'
import { MultiFactorPage } from './MultiFactorPage'
import { RateLimitsPage } from './RateLimitsPage'

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
  return <EmailsPage />
}

export function EmailTemplateEditorRoute() {
  return <EmailTemplateEditorPage />
}

export function RateLimitsRoute() {
  return <RateLimitsPage />
}

export function MultiFactorRoute() {
  return <MultiFactorPage />
}

export function URLConfigurationRoute() {
  const { auth, general, requestSave } = useAuthenticationWorkspace()
  const [siteUrl, setSiteUrl] = useState(general.siteUrl)
  const [redirectUrls, setRedirectUrls] = useState(auth.redirectUrls)
  const siteDirty = siteUrl !== general.siteUrl
  const redirectsDirty = JSON.stringify(redirectUrls) !== JSON.stringify(auth.redirectUrls)
  return <main className="page auth-page space-y-16"><header className="page-heading"><div><h1>URL Configuration</h1><p className="muted">Configure site URL and redirect URLs for authentication.</p></div></header><section className="space-y-4"><h2>Site URL</h2><form className="auth-settings-card auth-url-card" onSubmit={(event) => { event.preventDefault(); requestSave({ section: 'general', value: { ...general, siteUrl }, dirty: { siteUrl: true } }) }}><div className="grid gap-8 p-7 lg:grid-cols-[minmax(18rem,1fr)_minmax(20rem,1fr)]"><div><h3 className="text-base font-medium">Site URL</h3><p className="mt-1 text-sm text-muted-foreground">The default redirect URL when a valid redirect is not specified.</p></div><Input aria-label="Site URL" value={siteUrl} onChange={(event) => setSiteUrl(event.target.value)} placeholder="https://app.example.com" /></div><div className="flex justify-end border-t border-border p-5"><Button type="submit" disabled={!siteDirty}>Save changes</Button></div></form></section><section className="space-y-4"><div><h2>Redirect URLs</h2><p className="muted mt-1">URLs authentication providers may redirect to after sign-in.</p></div><form className="auth-settings-card auth-url-card" onSubmit={(event) => { event.preventDefault(); requestSave({ section: 'auth', value: { ...auth, redirectUrls }, dirty: { redirectUrls: true } }) }}><div className="space-y-3 p-7">{redirectUrls.map((value, index) => <div className="flex gap-3" key={`${index}-${value}`}><Input aria-label={`Redirect URL ${index + 1}`} value={value} onChange={(event) => setRedirectUrls((current) => current.map((item, position) => position === index ? event.target.value : item))} placeholder="https://app.example.com/auth/callback" /><Button type="button" variant="outline" onClick={() => setRedirectUrls((current) => current.filter((_, position) => position !== index))}>Remove</Button></div>)}{redirectUrls.length === 0 && <p className="py-16 text-center text-sm text-muted-foreground">No Redirect URLs</p>}<Button type="button" variant="outline" onClick={() => setRedirectUrls((current) => [...current, ''])}>Add URL</Button></div><div className="flex justify-end border-t border-border p-5"><Button type="submit" disabled={!redirectsDirty}>Save changes</Button></div></form></section></main>
}
