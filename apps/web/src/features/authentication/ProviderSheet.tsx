import { zodResolver } from '@hookform/resolvers/zod'
import { useEffect, useState } from 'react'
import { useForm, type Resolver } from 'react-hook-form'
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { Sheet, SheetContent, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import type { AuthConfig, GeneralConfig, OAuthProviderConfig } from '../../api/types'
import { OAUTH_PROVIDERS, specialFields } from '../projects/projectSchema'
import { NumberField, ReadOnlyField, SecretEditor, TextField, Toggle, errorAt } from '../project/configuration/fields'
import { authSchema, oauthProviderSchema } from '../project/configuration/schema'
import type { AuthenticationWorkspaceContext } from './AuthenticationWorkspace'

type Provider = 'email' | 'phone' | (typeof OAUTH_PROVIDERS)[number]
type Save = AuthenticationWorkspaceContext['requestSave']
const labels: Record<string, string> = { apple: 'Apple', azure: 'Azure', bitbucket: 'Bitbucket', discord: 'Discord', facebook: 'Facebook', figma: 'Figma', github: 'GitHub', gitlab: 'GitLab', google: 'Google', kakao: 'Kakao', keycloak: 'KeyCloak', linkedin_oidc: 'LinkedIn (OIDC)', notion: 'Notion', slack_oidc: 'Slack (OIDC)', snapchat: 'Snapchat', spotify: 'Spotify', twitch: 'Twitch', twitter: 'X / Twitter (OAuth 2.0)', workos: 'WorkOS', zoom: 'Zoom', email: 'Email', phone: 'Phone' }
export const providerLabel = (provider: string) => labels[provider] ?? provider
const emptyProvider = (): OAuthProviderConfig => ({ enabled: false, clientId: '', secretSet: false, secret: { action: '' }, fields: {} })
function oauthDefaults(provider: string, initial: OAuthProviderConfig): OAuthProviderConfig {
  const fields = { ...(initial.fields ?? {}) }
  const special = specialFields[provider]
  if (special && fields[special] == null) fields[special] = ''
  return {
    ...emptyProvider(),
    ...initial,
    enabled: Boolean(initial.enabled),
    clientId: initial.clientId ?? '',
    secretSet: Boolean(initial.secretSet),
    secret: initial.secret ?? { action: '' },
    fields,
  }
}
const emailDefaults = {
  securePasswordChange: false,
  requireCurrentPassword: false,
  preventLeakedPasswords: false,
  minimumPasswordLength: 6,
  passwordRequirements: '',
  emailOtpExpiration: 3600,
  emailOtpLength: 8,
} as const

export function ProviderSheet({ provider, auth, revision, general, onClose, onSave }: { provider?: Provider; auth: AuthConfig; revision: number; general: GeneralConfig; onClose: () => void; onSave: Save }) {
  const [discardOpen, setDiscardOpen] = useState(false)
  const [dirty, setDirty] = useState(false)
  useEffect(() => { setDirty(false); setDiscardOpen(false) }, [provider])
  const requestClose = (open: boolean) => { if (!open && dirty) setDiscardOpen(true); else if (!open) onClose() }
  if (!provider) return null
  return <><Sheet open onOpenChange={requestClose}><SheetContent aria-describedby={undefined} data-provider-drawer="true" className="authentication-provider-sheet w-full overflow-y-auto sm:max-w-[760px]"><SheetHeader><SheetTitle>{providerLabel(provider)}</SheetTitle></SheetHeader>{provider === 'email' ? <EmailForm initial={auth} onDirty={setDirty} onSave={onSave} onQueued={onClose} /> : provider === 'phone' ? <PhoneForm initial={auth} onDirty={setDirty} onSave={onSave} onQueued={onClose} /> : <OAuthForm provider={provider} initial={auth.oauth[provider] ?? emptyProvider()} callback={general.domain ? `https://${general.domain}/auth/v1/callback` : 'Set Domain to generate callback'} onDirty={setDirty} onSave={onSave} onQueued={onClose} />}</SheetContent></Sheet><AlertDialog open={discardOpen} onOpenChange={setDiscardOpen}><AlertDialogContent><AlertDialogHeader><AlertDialogTitle>Discard changes?</AlertDialogTitle><AlertDialogDescription>Your unsaved provider changes will be lost.</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel>Keep editing</AlertDialogCancel><AlertDialogAction onClick={() => { setDiscardOpen(false); onClose() }}>Discard changes</AlertDialogAction></AlertDialogFooter></AlertDialogContent></AlertDialog></>
}

function EmailForm({ initial, onDirty, onSave, onQueued }: { initial: AuthConfig; onDirty: (value: boolean) => void; onSave: Save; onQueued: () => void }) {
  const form = useForm<AuthConfig>({ resolver: zodResolver(authSchema) as Resolver<AuthConfig>, defaultValues: { ...initial, email: { ...emailDefaults, ...initial.email } } })
  const email = form.watch('email'); useDirty(form.formState.isDirty, onDirty)
  return <form className="authentication-provider-form space-y-0" onSubmit={form.handleSubmit((value) => onSave({ section: 'auth', value, dirty: form.formState.dirtyFields, onQueued }))}>
    <Toggle className="auth-setting-toggle auth-drawer-setting" id="email-auth" label="Enable email provider" description="Allow email-based sign up and log in for your application." checked={email.enabled} onChange={(value) => form.setValue('email.enabled', value, { shouldDirty: true, shouldValidate: true })} />
    <Toggle className="auth-setting-toggle auth-drawer-setting" id="secure-email-change" label="Secure email change" description="Users will be required to confirm any email change on both the old email address and new email address. If disabled, only the new email is required to confirm." checked={email.secureEmailChange} onChange={(value) => { form.setValue('email.secureEmailChange', value, { shouldDirty: true, shouldValidate: true }); form.setValue('email.doubleConfirmChanges', value, { shouldDirty: true, shouldValidate: true }) }} />
    <Toggle className="auth-setting-toggle auth-drawer-setting" id="secure-password-change" label="Secure password change" description="Users will need to be recently logged in to change their password without requiring reauthentication. A session is recent when it was created within the last 24 hours." checked={Boolean(email.securePasswordChange)} onChange={(value) => form.setValue('email.securePasswordChange', value, { shouldDirty: true, shouldValidate: true })} />
    <Toggle className="auth-setting-toggle auth-drawer-setting" id="require-current-password" label="Require current password when updating" description="Requires that the user supplies their current password when changing the password." checked={Boolean(email.requireCurrentPassword)} onChange={(value) => form.setValue('email.requireCurrentPassword', value, { shouldDirty: true, shouldValidate: true })} />
    <Toggle className="auth-setting-toggle auth-drawer-setting" id="prevent-leaked-passwords" label="Prevent use of leaked passwords" description="Rejects known or easy to guess passwords on sign up or password change. Powered by the HaveIBeenPwned.org Pwned Passwords API." checked={Boolean(email.preventLeakedPasswords)} onChange={(value) => form.setValue('email.preventLeakedPasswords', value, { shouldDirty: true, shouldValidate: true })} />
    <div className="auth-drawer-field-row"><NumberField form={form} name="email.minimumPasswordLength" label="Minimum password length" min={6} max={72} /><p className="text-xs text-muted-foreground">Passwords shorter than this value will be rejected as weak. Minimum 6 characters, though 8 or more is recommended.</p></div>
    <div className="auth-drawer-field-row"><TextField form={form} name="email.passwordRequirements" label="Password requirements" placeholder="lowercase:uppercase:number:symbol" /><p className="text-xs text-muted-foreground">Colon-separated character classes; a password must contain at least one character from each set.</p></div>
    <div className="auth-drawer-field-row"><NumberField form={form} name="email.emailOtpExpiration" label="Email OTP expiration" min={60} max={86400} /><p className="text-xs text-muted-foreground">Duration before an email OTP or link expires, in seconds.</p></div>
    <div className="auth-drawer-field-row"><NumberField form={form} name="email.emailOtpLength" label="Email OTP length" min={6} max={10} /><p className="text-xs text-muted-foreground">Number of digits in the email OTP.</p></div>
    <SaveButton disabled={!form.formState.isDirty} />
  </form>
}

function PhoneForm({ initial, onDirty, onSave, onQueued }: { initial: AuthConfig; onDirty: (value: boolean) => void; onSave: Save; onQueued: () => void }) {
  const form = useForm<AuthConfig>({ resolver: zodResolver(authSchema) as Resolver<AuthConfig>, defaultValues: initial })
  const phone = form.watch('phone'); useDirty(form.formState.isDirty, onDirty)
  const allowed = phone.provider === 'twilio' ? [['accountSid', 'Account SID'], ['messageServiceSid', 'Message Service SID'], ['verifySid', 'Verify SID']] : phone.provider === 'messagebird' ? [['originator', 'Originator']] : phone.provider === 'textlocal' ? [['sender', 'Sender']] : []
  return <form className="authentication-provider-form space-y-4" onSubmit={form.handleSubmit((value) => onSave({ section: 'auth', value, dirty: form.formState.dirtyFields, onQueued, setError: (name, message) => form.setError(name as never, { type: 'server', message }) }))}><Toggle className="auth-setting-toggle auth-drawer-setting" id="phone-auth" label="Enable Phone Auth" checked={phone.enabled} onChange={(value) => form.setValue('phone.enabled', value, { shouldDirty: true, shouldValidate: true })} /><div><label htmlFor="phone-provider" className="text-sm font-medium">Provider</label><Select value={phone.provider} onValueChange={(value) => form.setValue('phone.provider', value ?? '', { shouldDirty: true, shouldValidate: true })}><SelectTrigger id="phone-provider" aria-label="Phone provider" className="mt-1 w-full"><SelectValue placeholder="Choose provider" /></SelectTrigger><SelectContent><SelectItem value="twilio">Twilio</SelectItem><SelectItem value="messagebird">MessageBird</SelectItem><SelectItem value="textlocal">Textlocal</SelectItem></SelectContent></Select></div>{(phone.enabled || phone.secretSet) && <><div className="grid gap-4">{phone.enabled && allowed.map(([name, label]) => <TextField key={name} form={form} name={`phone.fields.${name}` as 'phone.fields'} label={label} />)}</div><SecretEditor label="Provider secret" configured={phone.secretSet} secret={phone.secret} error={errorAt(form.formState.errors, 'phone.secret')} onChange={(value) => form.setValue('phone.secret', value, { shouldDirty: true, shouldValidate: true })} /></>}<SaveButton disabled={!form.formState.isDirty} /></form>
}

function OAuthForm({ provider, initial, callback, onDirty, onSave, onQueued }: { provider: string; initial: OAuthProviderConfig; callback: string; onDirty: (value: boolean) => void; onSave: Save; onQueued: () => void }) {
  const form = useForm<OAuthProviderConfig>({ resolver: zodResolver(oauthProviderSchema(provider)) as Resolver<OAuthProviderConfig>, defaultValues: oauthDefaults(provider, initial) })
  const value = form.watch(); const fields = value.fields ?? {}; useDirty(form.formState.isDirty, onDirty); const special = specialFields[provider]
  const setFlag = (name: string, checked: boolean) => form.setValue(`fields.${name}` as `fields.${string}`, checked ? 'true' : 'false', { shouldDirty: true, shouldValidate: true })
  return <form className="authentication-provider-form space-y-4" onSubmit={form.handleSubmit((next) => onSave({ section: 'oauth', provider, value: next, dirty: form.formState.dirtyFields, onQueued, setError: (name, message) => form.setError(name as never, { type: 'server', message }) }))}>
    <Toggle className="auth-setting-toggle auth-drawer-setting" id={`oauth-${provider}-enabled`} label={`Enable ${providerLabel(provider)}`} description={provider === 'google' ? 'Enables Sign in with Google on the web using OAuth or One Tap, or in Android apps or Chrome extensions.' : undefined} checked={value.enabled} onChange={(enabled) => form.setValue('enabled', enabled, { shouldDirty: true, shouldValidate: true })} />
    <TextField form={form} name="clientId" label={provider === 'google' ? 'Client IDs' : 'Client ID'} placeholder={provider === 'google' ? 'Comma-separated client IDs' : undefined} />
    <SecretEditor label={provider === 'google' ? 'Client Secret (for OAuth)' : `${providerLabel(provider)} client secret`} configured={value.secretSet} secret={value.secret} error={errorAt(form.formState.errors, 'secret')} onChange={(secret) => form.setValue('secret', secret, { shouldDirty: true, shouldValidate: true })} />
    <Toggle className="auth-setting-toggle auth-drawer-setting" id={`${provider}-skip-nonce`} label="Skip nonce checks" description="Allows ID tokens with any nonce to be accepted, which is less secure. Useful when the nonce used to issue the ID token is not available, such as with iOS." checked={fields.skipNonceChecks === 'true'} onChange={(checked) => setFlag('skipNonceChecks', checked)} />
    <Toggle className="auth-setting-toggle auth-drawer-setting" id={`${provider}-allow-no-email`} label="Allow users without an email" description="Allows the user to successfully authenticate when the provider does not return an email address." checked={fields.allowUsersWithoutEmail === 'true'} onChange={(checked) => setFlag('allowUsersWithoutEmail', checked)} />
    {special && <TextField form={form} name={`fields.${special}` as 'fields'} label={special} />}
    {value.enabled && <ReadOnlyField label="Callback URL (for OAuth)" value={callback} copy={callback.startsWith('http')} />}
    <SaveButton disabled={!form.formState.isDirty} />
  </form>
}

function SaveButton({ disabled }: { disabled: boolean }) { return <div className="flex justify-end"><Button type="submit" disabled={disabled}>Save changes</Button></div> }
function useDirty(dirty: boolean, onDirty: (value: boolean) => void) { useEffect(() => onDirty(dirty), [dirty, onDirty]) }
