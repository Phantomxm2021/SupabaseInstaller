import { zodResolver } from '@hookform/resolvers/zod'
import { useState, type ReactNode } from 'react'
import { useForm, type Resolver } from 'react-hook-form'
import { ChevronRight, KeyRound, Mail, Smartphone } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import type { AuthConfig, OAuthProviderConfig } from '../../api/types'
import { OAUTH_PROVIDERS } from '../projects/projectSchema'
import { SectionSaveButton, Toggle } from '../project/configuration/fields'
import { authSchema } from '../project/configuration/schema'
import { ProviderSheet, providerLabel } from './ProviderSheet'
import { useAuthenticationWorkspace } from './AuthenticationWorkspace'

type Provider = 'email' | 'phone' | (typeof OAUTH_PROVIDERS)[number]
const emptyProvider = (): OAuthProviderConfig => ({ enabled: false, clientId: '', secretSet: false, secret: { action: '' }, fields: {} })

export function SignInProvidersPage() {
  const { auth, revision, general, requestSave } = useAuthenticationWorkspace()
  const [provider, setProvider] = useState<Provider>()
  const form = useForm<AuthConfig>({ resolver: zodResolver(authSchema) as Resolver<AuthConfig>, defaultValues: auth })
  const policy = form.watch()
  const status = (item: Provider) => item === 'email' ? policy.email.enabled : item === 'phone' ? policy.phone.enabled : (auth.oauth[item] ?? emptyProvider()).enabled
  return <main className="page space-y-12"><header className="page-heading"><div><h1>Sign In / Providers</h1><p className="muted">Configure authentication providers and login methods for your users.</p></div></header><form className="space-y-4" onSubmit={form.handleSubmit((value) => requestSave({ section: 'auth', value, dirty: form.formState.dirtyFields, setError: (name, message) => form.setError(name as never, { type: 'server', message }) }))}><h2 className="text-xl font-semibold">User Signups</h2><section className="overflow-hidden rounded-xl border border-border bg-card"><SettingsToggleRow><Toggle id="allow-signup" label="Allow new users to sign up" description="If this is disabled, new users will not be able to sign up to your application." checked={policy.email.allowSignup} onChange={(value) => { form.setValue('email.allowSignup', value, { shouldDirty: true, shouldValidate: true }); form.setValue('disableSignup', !value, { shouldDirty: true, shouldValidate: true }) }} /></SettingsToggleRow><SettingsToggleRow><Toggle id="anonymous-sign-in" label="Allow anonymous sign-ins" description="Allow users to sign in without providing an email address or phone number." checked={policy.anonymousSignIn} onChange={(value) => form.setValue('anonymousSignIn', value, { shouldDirty: true, shouldValidate: true })} /></SettingsToggleRow><SettingsToggleRow><Toggle id="confirm-email" label="Confirm email" description="Users must confirm their email address before signing in for the first time." checked={policy.email.confirmEmail} onChange={(value) => form.setValue('email.confirmEmail', value, { shouldDirty: true, shouldValidate: true })} /></SettingsToggleRow><SectionSaveButton label="changes" disabled={!form.formState.isDirty} /></section></form><section className="space-y-4"><div><h2 className="text-xl font-semibold">Auth Providers</h2><p className="muted mt-1">Authenticate users through a suite of providers and login methods.</p></div><div className="overflow-hidden rounded-xl border border-border bg-card">{(['email', 'phone', ...OAUTH_PROVIDERS] as Provider[]).map((item) => { const enabled = status(item); return <button key={item} type="button" className="flex w-full items-center justify-between gap-4 border-b border-border px-5 py-5 text-left last:border-0 hover:bg-muted/45 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" onClick={() => setProvider(item)}><span className="flex items-center gap-3 text-sm font-medium">{item === 'email' ? <Mail className="size-5 text-muted-foreground" aria-hidden="true" /> : item === 'phone' ? <Smartphone className="size-5 text-muted-foreground" aria-hidden="true" /> : <KeyRound className="size-5 text-muted-foreground" aria-hidden="true" />}{providerLabel(item)}</span><span className="flex items-center gap-4"><Badge variant={enabled ? 'default' : 'outline'}>{enabled ? 'Enabled' : 'Disabled'}</Badge><ChevronRight className="size-5 text-muted-foreground" aria-hidden="true" /></span></button> })}</div></section><ProviderSheet provider={provider} auth={auth} revision={revision} general={general} onClose={() => setProvider(undefined)} onSave={requestSave} /></main>
}

function SettingsToggleRow({ children }: { children: ReactNode }) {
  return <div className="border-b border-border px-5 py-4 last:border-0 [&>div]:rounded-none [&>div]:border-0 [&>div]:p-0 [&_p]:text-sm">{children}</div>
}
