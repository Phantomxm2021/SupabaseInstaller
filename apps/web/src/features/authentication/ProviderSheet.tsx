import { zodResolver } from '@hookform/resolvers/zod'
import { useEffect, useState } from 'react'
import { useForm, type Resolver } from 'react-hook-form'
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { Sheet, SheetContent, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import type { AuthConfig, GeneralConfig, OAuthProviderConfig } from '../../api/types'
import { OAUTH_PROVIDERS, specialFields } from '../projects/projectSchema'
import { ReadOnlyField, SecretEditor, TextField, Toggle, errorAt } from '../project/configuration/fields'
import { authSchema, oauthProviderSchema } from '../project/configuration/schema'
import type { AuthenticationWorkspaceContext } from './AuthenticationWorkspace'

type Provider = 'email' | 'phone' | (typeof OAUTH_PROVIDERS)[number]
type Save = AuthenticationWorkspaceContext['requestSave']
const labels: Record<string, string> = { apple: 'Apple', azure: 'Azure / Microsoft', bitbucket: 'Bitbucket', discord: 'Discord', facebook: 'Facebook', figma: 'Figma', github: 'GitHub', gitlab: 'GitLab', google: 'Google', kakao: 'Kakao', keycloak: 'Keycloak', linkedin_oidc: 'LinkedIn OIDC', notion: 'Notion', slack_oidc: 'Slack OIDC', snapchat: 'Snapchat', spotify: 'Spotify', twitch: 'Twitch', twitter: 'Twitter / X', workos: 'WorkOS', zoom: 'Zoom', email: 'Email', phone: 'Phone' }
export const providerLabel = (provider: string) => labels[provider] ?? provider
const emptyProvider = (): OAuthProviderConfig => ({ enabled: false, clientId: '', secretSet: false, secret: { action: '' }, fields: {} })

export function ProviderSheet({ provider, auth, revision, general, onClose, onSave }: { provider?: Provider; auth: AuthConfig; revision: number; general: GeneralConfig; onClose: () => void; onSave: Save }) {
  const [discardOpen, setDiscardOpen] = useState(false)
  const [dirty, setDirty] = useState(false)
  const requestClose = (open: boolean) => { if (!open && dirty) setDiscardOpen(true); else if (!open) onClose() }
  if (!provider) return null
  return <><Sheet open onOpenChange={requestClose}><SheetContent aria-describedby={undefined} className="w-full overflow-y-auto sm:max-w-xl"><SheetHeader><SheetTitle>{providerLabel(provider)}</SheetTitle></SheetHeader>{provider === 'email' ? <EmailForm initial={auth} onDirty={setDirty} onSave={onSave} onQueued={onClose} /> : provider === 'phone' ? <PhoneForm initial={auth} onDirty={setDirty} onSave={onSave} onQueued={onClose} /> : <OAuthForm provider={provider} initial={auth.oauth[provider] ?? emptyProvider()} callback={general.domain ? `https://${general.domain}/auth/v1/callback` : 'Set Domain to generate callback'} onDirty={setDirty} onSave={onSave} onQueued={onClose} />}</SheetContent></Sheet><AlertDialog open={discardOpen} onOpenChange={setDiscardOpen}><AlertDialogContent><AlertDialogHeader><AlertDialogTitle>Discard changes?</AlertDialogTitle><AlertDialogDescription>Your unsaved provider changes will be lost.</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel>Keep editing</AlertDialogCancel><AlertDialogAction onClick={() => { setDiscardOpen(false); onClose() }}>Discard changes</AlertDialogAction></AlertDialogFooter></AlertDialogContent></AlertDialog></>
}

function EmailForm({ initial, onDirty, onSave, onQueued }: { initial: AuthConfig; onDirty: (value: boolean) => void; onSave: Save; onQueued: () => void }) {
  const form = useForm<AuthConfig>({ resolver: zodResolver(authSchema) as Resolver<AuthConfig>, defaultValues: initial })
  const email = form.watch('email'); useDirty(form.formState.isDirty, onDirty)
  return <form className="space-y-4 p-4" onSubmit={form.handleSubmit((value) => onSave({ section: 'auth', value, dirty: form.formState.dirtyFields, onQueued }))}><Toggle id="email-auth" label="Email Auth" checked={email.enabled} onChange={(value) => form.setValue('email.enabled', value, { shouldDirty: true, shouldValidate: true })} /><Toggle id="secure-email-change" label="Confirm email changes at both addresses" description="Uses the pinned GoTrue secure-email-change setting to confirm with both the current and new address." checked={email.secureEmailChange} onChange={(value) => { form.setValue('email.secureEmailChange', value, { shouldDirty: true, shouldValidate: true }); form.setValue('email.doubleConfirmChanges', value, { shouldDirty: true, shouldValidate: true }) }} /><UnavailableSecurity /><SaveButton disabled={!form.formState.isDirty} /></form>
}

function PhoneForm({ initial, onDirty, onSave, onQueued }: { initial: AuthConfig; onDirty: (value: boolean) => void; onSave: Save; onQueued: () => void }) {
  const form = useForm<AuthConfig>({ resolver: zodResolver(authSchema) as Resolver<AuthConfig>, defaultValues: initial })
  const phone = form.watch('phone'); useDirty(form.formState.isDirty, onDirty)
  const allowed = phone.provider === 'twilio' ? [['accountSid', 'Account SID'], ['messageServiceSid', 'Message Service SID'], ['verifySid', 'Verify SID']] : phone.provider === 'messagebird' ? [['originator', 'Originator']] : phone.provider === 'textlocal' ? [['sender', 'Sender']] : []
  return <form className="space-y-4 p-4" onSubmit={form.handleSubmit((value) => onSave({ section: 'auth', value, dirty: form.formState.dirtyFields, onQueued, setError: (name, message) => form.setError(name as never, { type: 'server', message }) }))}><Toggle id="phone-auth" label="Enable Phone Auth" checked={phone.enabled} onChange={(value) => form.setValue('phone.enabled', value, { shouldDirty: true, shouldValidate: true })} /><div><label htmlFor="phone-provider" className="text-sm font-medium">Provider</label><Select value={phone.provider} onValueChange={(value) => form.setValue('phone.provider', value ?? '', { shouldDirty: true, shouldValidate: true })}><SelectTrigger id="phone-provider" aria-label="Phone provider" className="mt-1 w-full"><SelectValue placeholder="Choose provider" /></SelectTrigger><SelectContent><SelectItem value="twilio">Twilio</SelectItem><SelectItem value="messagebird">MessageBird</SelectItem><SelectItem value="textlocal">Textlocal</SelectItem></SelectContent></Select></div>{(phone.enabled || phone.secretSet) && <><div className="grid gap-4">{phone.enabled && allowed.map(([name, label]) => <TextField key={name} form={form} name={`phone.fields.${name}` as 'phone.fields'} label={label} />)}</div><SecretEditor label="Provider secret" configured={phone.secretSet} secret={phone.secret} error={errorAt(form.formState.errors, 'phone.secret')} onChange={(value) => form.setValue('phone.secret', value, { shouldDirty: true, shouldValidate: true })} /></>}<SaveButton disabled={!form.formState.isDirty} /></form>
}

function OAuthForm({ provider, initial, callback, onDirty, onSave, onQueued }: { provider: string; initial: OAuthProviderConfig; callback: string; onDirty: (value: boolean) => void; onSave: Save; onQueued: () => void }) {
  const form = useForm<OAuthProviderConfig>({ resolver: zodResolver(oauthProviderSchema(provider)) as Resolver<OAuthProviderConfig>, defaultValues: initial })
  const value = form.watch(); useDirty(form.formState.isDirty, onDirty); const special = specialFields[provider]
  return <form className="space-y-4 p-4" onSubmit={form.handleSubmit((next) => onSave({ section: 'oauth', provider, value: next, dirty: form.formState.dirtyFields, onQueued, setError: (name, message) => form.setError(name as never, { type: 'server', message }) }))}><Toggle id={`oauth-${provider}-enabled`} label={`Enable ${providerLabel(provider)}`} checked={value.enabled} onChange={(enabled) => form.setValue('enabled', enabled, { shouldDirty: true, shouldValidate: true })} /><TextField form={form} name="clientId" label="Client ID" /><SecretEditor label={`${providerLabel(provider)} client secret`} configured={value.secretSet} secret={value.secret} error={errorAt(form.formState.errors, 'secret')} onChange={(secret) => form.setValue('secret', secret, { shouldDirty: true, shouldValidate: true })} />{special && <TextField form={form} name={`fields.${special}` as 'fields'} label={special} />}{value.enabled && <ReadOnlyField label="Callback URL" value={callback} copy={callback.startsWith('http')} />}<SaveButton disabled={!form.formState.isDirty} /></form>
}

function UnavailableSecurity() { return <div className="rounded-lg border border-border p-3"><p className="text-sm font-medium">Password security controls</p><p className="mt-1 text-xs text-muted-foreground">Secure password change, current-password verification, and password length or rule controls are unavailable until the typed runtime schema lands.</p></div> }
function SaveButton({ disabled }: { disabled: boolean }) { return <div className="flex justify-end"><Button type="submit" disabled={disabled}>Save changes</Button></div> }
function useDirty(dirty: boolean, onDirty: (value: boolean) => void) { useEffect(() => onDirty(dirty), [dirty, onDirty]) }
