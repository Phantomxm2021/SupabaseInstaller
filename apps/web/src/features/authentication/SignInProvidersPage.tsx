import { zodResolver } from '@hookform/resolvers/zod'
import { useState, type ReactNode } from 'react'
import { useForm, type Resolver } from 'react-hook-form'
import { ChevronRight, CircleCheck, KeyRound, Mail, Smartphone } from 'lucide-react'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { FieldError } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import type { AuthConfig, OAuthProviderConfig } from '../../api/types'
import { OAUTH_PROVIDERS } from '../projects/projectSchema'
import { errorAt, SectionSaveButton, Toggle } from '../project/configuration/fields'
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

  return <main className="page auth-page auth-providers-page">
    <header className="page-heading"><div><h1>Sign In / Providers</h1><p className="muted">Configure authentication providers and login methods for your users.</p></div></header>
    <Tabs defaultValue="supabase" className="auth-provider-tabs gap-10">
      <TabsList aria-label="Authentication provider source" variant="line" className="auth-tabs-list border-b border-border">
        <TabsTrigger value="supabase">Supabase Auth</TabsTrigger>
        <TabsTrigger value="third-party">Third-Party Auth</TabsTrigger>
      </TabsList>
      <TabsContent value="supabase" className="dashboard-stack">
        <form className="dashboard-section" onSubmit={form.handleSubmit((value) => requestSave({ section: 'auth', value, dirty: form.formState.dirtyFields, setError: (name, message) => form.setError(name as never, { type: 'server', message }) }))}>
          <h2>User Signups</h2>
          <section className="auth-settings-card">
            <SettingsToggleRow><Toggle className="auth-setting-toggle" id="allow-signup" label="Allow new users to sign up" description="If this is disabled, new users will not be able to sign up to your application." checked={policy.email.allowSignup} onChange={(value) => { form.setValue('email.allowSignup', value, { shouldDirty: true, shouldValidate: true }); form.setValue('disableSignup', !value, { shouldDirty: true, shouldValidate: true }) }} /></SettingsToggleRow>
            <SettingsToggleRow><Toggle className="auth-setting-toggle" id="manual-linking" label="Allow manual linking" description="Enable manual linking APIs for your project." checked={Boolean(policy.manualLinking)} onChange={(value) => form.setValue('manualLinking', value, { shouldDirty: true, shouldValidate: true })} /></SettingsToggleRow>
            <SettingsToggleRow><Toggle className="auth-setting-toggle" id="anonymous-sign-in" label="Allow anonymous sign-ins" description="Enable anonymous sign-ins for your project." checked={policy.anonymousSignIn} onChange={(value) => form.setValue('anonymousSignIn', value, { shouldDirty: true, shouldValidate: true })} /></SettingsToggleRow>
            <SettingsToggleRow><Toggle className="auth-setting-toggle" id="confirm-email" label="Confirm email" description="Users will need to confirm their email address before signing in for the first time." checked={policy.email.confirmEmail} onChange={(value) => form.setValue('email.confirmEmail', value, { shouldDirty: true, shouldValidate: true })} /></SettingsToggleRow>
            <div className="auth-settings-row flex items-center justify-between gap-6">
              <div><label htmlFor="jwt-expiry" className="text-sm font-medium">JWT expiry</label><p className="mt-1 text-xs text-muted-foreground">How long access tokens remain valid, in seconds (1–604800).</p></div>
              <div className="w-40"><Input id="jwt-expiry" aria-label="JWT expiry (seconds)" type="number" min={1} max={604800} {...form.register('jwtExpiry', { valueAsNumber: true })} aria-invalid={Boolean(errorAt(form.formState.errors, 'jwtExpiry'))} />{errorAt(form.formState.errors, 'jwtExpiry') && <FieldError className="mt-1">{errorAt(form.formState.errors, 'jwtExpiry')}</FieldError>}</div>
            </div>
            <SectionSaveButton label="changes" disabled={!form.formState.isDirty} />
          </section>
        </form>
        <section className="dashboard-section">
          <div><h2>Auth Providers</h2><p className="muted mt-1">Authenticate your users through a suite of providers and login methods.</p></div>
          <div className="auth-settings-card auth-provider-list">
            {(['email', 'phone', ...OAUTH_PROVIDERS] as Provider[]).map((item) => <ProviderRow key={item} item={item} enabled={status(item)} onClick={() => setProvider(item)} />)}
          </div>
        </section>
      </TabsContent>
      <TabsContent value="third-party" className="auth-third-party-empty">
        <h2>Third-Party Auth</h2><p className="muted">This Manager version currently exposes the GoTrue providers in Supabase Auth. No separate third-party provider configuration is available.</p>
      </TabsContent>
    </Tabs>
    <ProviderSheet key={provider ?? 'closed'} provider={provider} auth={auth} revision={revision} general={general} onClose={() => setProvider(undefined)} onSave={requestSave} />
  </main>
}

function ProviderRow({ item, enabled, onClick }: { item: Provider; enabled: boolean; onClick: () => void }) {
  const Icon = item === 'email' ? Mail : item === 'phone' ? Smartphone : KeyRound
  return <button type="button" className="auth-provider-row" data-density="dashboard-compact-row" onClick={onClick}>
    <span className="auth-provider-name"><Icon aria-hidden="true" />{providerLabel(item)}</span>
    <span className="auth-provider-action"><span className={enabled ? 'auth-provider-status is-enabled' : 'auth-provider-status'}>{enabled && <CircleCheck aria-hidden="true" />}{enabled ? 'Enabled' : 'Disabled'}</span><ChevronRight aria-hidden="true" /></span>
  </button>
}

function SettingsToggleRow({ children }: { children: ReactNode }) {
  return <div className="auth-settings-row">{children}</div>
}
