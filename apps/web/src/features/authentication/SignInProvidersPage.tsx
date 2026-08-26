import { zodResolver } from '@hookform/resolvers/zod'
import { useState } from 'react'
import { useForm, type Resolver } from 'react-hook-form'
import { ChevronRight, KeyRound, Mail, Smartphone } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import type { AuthConfig, OAuthProviderConfig } from '../../api/types'
import { OAUTH_PROVIDERS } from '../projects/projectSchema'
import { SectionCard, SectionSaveButton, Toggle } from '../project/configuration/fields'
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
  return <main className="page space-y-6"><div className="page-heading"><div><p className="eyebrow">Authentication</p><h1>Sign In / Providers</h1><p className="muted">Manage signup policy and configure each sign-in provider independently.</p></div></div><form onSubmit={form.handleSubmit((value) => requestSave({ section: 'auth', value, dirty: form.formState.dirtyFields, setError: (name, message) => form.setError(name as never, { type: 'server', message }) }))}><SectionCard title="User Signups" description="Controls that apply to all new users."><div className="grid gap-3 md:grid-cols-2"><Toggle id="allow-signup" label="Allow new users to sign up" checked={policy.email.allowSignup} onChange={(value) => { form.setValue('email.allowSignup', value, { shouldDirty: true, shouldValidate: true }); form.setValue('disableSignup', !value, { shouldDirty: true, shouldValidate: true }) }} /><Toggle id="anonymous-sign-in" label="Allow anonymous sign-ins" checked={policy.anonymousSignIn} onChange={(value) => form.setValue('anonymousSignIn', value, { shouldDirty: true, shouldValidate: true })} /><Toggle id="confirm-email" label="Confirm email" checked={policy.email.confirmEmail} onChange={(value) => form.setValue('email.confirmEmail', value, { shouldDirty: true, shouldValidate: true })} /><div className="rounded-lg border border-border p-3"><p className="text-sm font-medium">Manual linking</p><p className="mt-1 text-xs text-muted-foreground">Unavailable in this Manager version.</p></div></div><SectionSaveButton label="changes" disabled={!form.formState.isDirty} /></SectionCard></form><SectionCard title="Sign-in providers" description="Open a provider to change only its settings."><div className="grid gap-2 md:grid-cols-2">{(['email', 'phone', ...OAUTH_PROVIDERS] as Provider[]).map((item) => { const enabled = status(item); return <Button key={item} type="button" variant="outline" className="h-auto justify-between p-4" onClick={() => setProvider(item)}><span className="flex items-center gap-2">{item === 'email' ? <Mail aria-hidden="true" /> : item === 'phone' ? <Smartphone aria-hidden="true" /> : <KeyRound aria-hidden="true" />}{providerLabel(item)}</span><span className="flex items-center gap-2"><Badge variant={enabled ? 'default' : 'outline'}>{enabled ? 'Enabled' : 'Disabled'}</Badge><ChevronRight aria-hidden="true" /></span></Button> })}</div></SectionCard><ProviderSheet provider={provider} auth={auth} revision={revision} general={general} onClose={() => setProvider(undefined)} onSave={requestSave} /></main>
}
