import { useEffect, useState } from 'react'
import type { UseFormReturn } from 'react-hook-form'
import { Card, CardContent } from '@/components/ui/card'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { OAuthProviderFields } from '../OAuthProviderFields'
import { type ProjectForm } from '../projectSchema'
import { disabledSmtpConfiguration, setServiceEnabled } from '../PresetStep'
import { AuthMethodDialog, type AuthenticationMethod } from './AuthMethodDialog'

export function SecurityIntegrationsStep({ form }: { form: UseFormReturn<ProjectForm> }) {
  const [dialogOpen, setDialogOpen] = useState(false)
  const [focusProvider, setFocusProvider] = useState<string>()
  const auth = form.watch('configuration.auth')
  const services = form.watch('configuration.services')
  const storage = form.watch('configuration.storage')
  const functions = form.watch('configuration.functions')
  const set = (name: string, value: unknown) => { form.setValue(name as any, value as any, { shouldDirty: true, shouldValidate: true }); form.setValue('preset', 'CUSTOM', { shouldDirty: true }) }
  const setSmtpEnabled = (enabled: boolean) => set('configuration.auth.smtp', enabled ? { ...form.getValues('configuration.auth.smtp'), enabled: true } : disabledSmtpConfiguration())
  const error = (name: string) => fieldError(form, name)
  const selectMethod = (method: AuthenticationMethod) => {
    set(`configuration.auth.oauth.${method.provider}`, { enabled: true, clientId: '', secretSet: false, secret: { action: '' }, fields: {} })
    setFocusProvider(method.provider)
  }
  useEffect(() => {
    if (!focusProvider) return
    const timer = window.setTimeout(() => document.getElementById(`oauth-provider-${focusProvider}`)?.focus(), 0)
    return () => window.clearTimeout(timer)
  }, [focusProvider, auth.oauth])
  useEffect(() => {
    const allowed = auth.phone.provider === 'twilio' ? ['accountSid', 'messageServiceSid', 'verifySid'] : auth.phone.provider === 'messagebird' ? ['originator'] : auth.phone.provider === 'textlocal' ? ['sender'] : []
    const fields = Object.fromEntries(Object.entries(auth.phone.fields).filter(([key]) => allowed.includes(key)))
    if (Object.keys(fields).length !== Object.keys(auth.phone.fields).length) set('configuration.auth.phone.fields', fields)
  }, [auth.phone.provider])
  const setBackend = (backend: string) => {
    const base = form.getValues('configuration.storage')
    const clean = backend === 'local' ? { ...base, backend, bucket: '', region: '', endpoint: '', accountId: '', accessKeyId: '', secretAccessKeySet: false, secretAccessKey: { action: '' as const }, forcePathStyle: false } : { ...base, backend, region: backend === 'r2' ? '' : base.region, endpoint: backend === 's3' ? base.endpoint : '', accountId: backend === 'r2' ? base.accountId : '' }
    set('configuration.storage', clean)
  }
  const variablesText = functions.variables.map((variable) => `${variable.name}=`).join('\n')
  const variableErrors = (form.formState.errors.configuration?.functions?.variables as any[] | undefined) ?? []
  const addedOAuth = Object.entries(auth.oauth).filter(([, config]) => config?.enabled).map(([provider]) => provider)

  return <div className="space-y-4">
    <Module title="Authentication" enabled={services.auth} onEnabledChange={(enabled) => setServiceEnabled(form, 'auth', enabled)} headerAction={services.auth && <AuthMethodDialog open={dialogOpen} onOpenChange={setDialogOpen} onSelect={selectMethod} addedOAuth={addedOAuth} />}>
      <div className="space-y-6"><FieldGroup className="grid gap-4 md:grid-cols-2"><Toggle label="Email Auth" checked={auth.email.enabled} onChange={(value) => set('configuration.auth.email.enabled', value)} /><Toggle label="Allow signup" checked={auth.email.allowSignup} onChange={(value) => { set('configuration.auth.email.allowSignup', value); set('configuration.auth.disableSignup', !value) }} error={error('configuration.auth.disableSignup')} /><Toggle label="Confirm email" checked={auth.email.confirmEmail} onChange={(value) => set('configuration.auth.email.confirmEmail', value)} /><Toggle label="Secure email change" checked={auth.email.secureEmailChange} onChange={(value) => { set('configuration.auth.email.secureEmailChange', value); set('configuration.auth.email.doubleConfirmChanges', value) }} error={error('configuration.auth.email.secureEmailChange')} /><NumberField label="Session JWT expiry (seconds)" name="configuration.auth.jwtExpiry" form={form} /></FieldGroup><Field><FieldLabel htmlFor="redirect-urls">Redirect URLs</FieldLabel><FieldDescription>One absolute http(s) URL per line.</FieldDescription><Textarea id="redirect-urls" value={auth.redirectUrls.join('\n')} onChange={(event) => set('configuration.auth.redirectUrls', event.target.value.split(/\n/).map((value) => value.trim()).filter(Boolean))} aria-invalid={!!error('configuration.auth.redirectUrls')} /><FieldError>{error('configuration.auth.redirectUrls')}</FieldError>{redirectErrors(form).map(([index, message]) => <FieldError key={index}>Redirect URL {index + 1}: {message}</FieldError>)}</Field>{auth.phone.enabled && <PhoneFields form={form} set={set} error={error} />}{auth.anonymousSignIn && <p className="text-sm text-muted-foreground">Anonymous sign-in is enabled.</p>}<OAuthProviderFields form={form} siteUrl={form.watch('configuration.general.siteUrl')} /></div>
    </Module>
    <Module title="Custom SMTP" enabled={auth.smtp.enabled} onEnabledChange={setSmtpEnabled}>
      <FieldGroup className="grid gap-4 md:grid-cols-2"><TextField label="Host" name="configuration.auth.smtp.host" form={form} /><NumberField label="Port" name="configuration.auth.smtp.port" form={form} /><TextField label="Username" name="configuration.auth.smtp.username" form={form} /><SecretField label="Password" path="configuration.auth.smtp.password" set={set} configured={auth.smtp.passwordSet} error={error('configuration.auth.smtp.password')} /><TextField label="Sender email" name="configuration.auth.smtp.senderEmail" form={form} /><TextField label="Sender name" name="configuration.auth.smtp.senderName" form={form} /></FieldGroup>
    </Module>
    <Module title="Storage & Image Transformation" enabled={services.storage} onEnabledChange={(enabled) => setServiceEnabled(form, 'storage', enabled)}>
      <div className="space-y-4"><Field><FieldLabel>Storage backend</FieldLabel><Select value={storage.backend} onValueChange={(value) => value && setBackend(value)}><SelectTrigger aria-label="Storage backend"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="local">Local filesystem</SelectItem><SelectItem value="aws-s3">AWS S3</SelectItem><SelectItem value="r2">Cloudflare R2</SelectItem><SelectItem value="s3">Custom S3-compatible</SelectItem></SelectContent></Select><FieldError>{error('configuration.storage.backend')}</FieldError></Field>{storage.backend !== 'local' && <><FieldGroup className="grid gap-4 md:grid-cols-2"><TextField label="Bucket" name="configuration.storage.bucket" form={form} /><TextField label="Region" name="configuration.storage.region" form={form} /><TextField label={storage.backend === 'r2' ? 'Account ID' : 'Endpoint'} name={storage.backend === 'r2' ? 'configuration.storage.accountId' : 'configuration.storage.endpoint'} form={form} /><TextField label="Access key ID" name="configuration.storage.accessKeyId" form={form} /><SecretField label="Secret access key" path="configuration.storage.secretAccessKey" set={set} configured={storage.secretAccessKeySet} error={error('configuration.storage.secretAccessKey')} /></FieldGroup><Toggle label="Force path style" checked={storage.forcePathStyle} onChange={(value) => set('configuration.storage.forcePathStyle', value)} /></>}<Toggle label="S3-compatible API" checked={storage.s3CompatibleApi} onChange={(value) => set('configuration.storage.s3CompatibleApi', value)} /><Toggle label="Image transformation" checked={services.imgproxy} onChange={(value) => setServiceEnabled(form, 'imgproxy', value)} /></div>
    </Module>
    <Module title="Edge Functions" enabled={services.functions} onEnabledChange={(enabled) => setServiceEnabled(form, 'functions', enabled)}>
      <div className="space-y-4"><Field><FieldLabel>Functions directory</FieldLabel><Input readOnly value={functions.directory} /></Field><Toggle label="Verify JWT by default" checked={functions.defaultJwtVerification} onChange={(value) => set('configuration.functions.defaultJwtVerification', value)} /><Field><FieldLabel htmlFor="function-variables">Environment variables</FieldLabel><FieldDescription>Enter NAME=value lines. Existing values remain write-only when left blank.</FieldDescription><Textarea id="function-variables" value={variablesText} onChange={(event) => { const variables = event.target.value.split(/\n/).filter(Boolean).map((line) => { const index = line.indexOf('='); const name = (index < 0 ? line : line.slice(0, index)).trim(); const value = index < 0 ? '' : line.slice(index + 1); const existing = functions.variables.find((variable) => variable.name === name); return { name, valueSet: value ? true : !!existing?.valueSet, value: value ? { action: 'replace' as const, value } : existing?.valueSet ? existing.value : { action: '' as const } } }); set('configuration.functions.variables', variables) }} aria-invalid={!!error('configuration.functions.variables')} /><FieldError>{error('configuration.functions.variables')}</FieldError>{variableErrors.map((item, index) => <FieldError key={index}>{item?.name?.message || item?.value?.message || item?.value?.value?.message}</FieldError>)}</Field></div>
    </Module>
  </div>
}

function Module({ title, enabled, onEnabledChange, children, headerAction }: { title: string; enabled: boolean; onEnabledChange: (enabled: boolean) => void; children: React.ReactNode; headerAction?: React.ReactNode }) {
  const [open, setOpen] = useState(enabled)
  useEffect(() => setOpen(enabled), [enabled])
  return <Card><Collapsible open={open} onOpenChange={setOpen}><div className="flex items-center gap-3 px-4 py-3"><CollapsibleTrigger className="flex-1 text-left font-medium">{title}</CollapsibleTrigger>{headerAction}<Switch aria-label={title} checked={enabled} onClick={(event) => event.stopPropagation()} onCheckedChange={onEnabledChange} /></div>{enabled && <CollapsibleContent><CardContent className="border-t pt-4">{children}</CardContent></CollapsibleContent>}</Collapsible></Card>
}

function PhoneFields({ form, set, error }: { form: UseFormReturn<ProjectForm>; set: (name: string, value: unknown) => void; error: (name: string) => string | undefined }) {
  const phone = form.watch('configuration.auth.phone')
  return <section className="space-y-4"><h3 className="font-medium">Phone Auth</h3><Field><FieldLabel>Provider</FieldLabel><Select value={phone.provider || undefined} onValueChange={(value) => value && set('configuration.auth.phone.provider', value)}><SelectTrigger aria-label="Phone provider"><SelectValue placeholder="Choose provider" /></SelectTrigger><SelectContent><SelectItem value="twilio">Twilio</SelectItem><SelectItem value="messagebird">MessageBird</SelectItem><SelectItem value="textlocal">Textlocal</SelectItem></SelectContent></Select><FieldError>{error('configuration.auth.phone.provider')}</FieldError></Field><FieldGroup className="grid gap-4 md:grid-cols-2">{phone.provider === 'twilio' && <><TextField label="Account SID" name="configuration.auth.phone.fields.accountSid" form={form} /><TextField label="Message Service SID" name="configuration.auth.phone.fields.messageServiceSid" form={form} /><TextField label="Verify SID" name="configuration.auth.phone.fields.verifySid" form={form} /></>}{phone.provider === 'messagebird' && <TextField label="Originator" name="configuration.auth.phone.fields.originator" form={form} />}{phone.provider === 'textlocal' && <TextField label="Sender" name="configuration.auth.phone.fields.sender" form={form} />}<SecretField label="Provider secret" path="configuration.auth.phone.secret" set={set} configured={phone.secretSet} error={error('configuration.auth.phone.secret')} /></FieldGroup></section>
}

function fieldError(form: UseFormReturn<ProjectForm>, name: string) { let value: any = form.formState.errors; for (const part of name.split('.')) value = value?.[part]; return (value?.message || value?.value?.message) as string | undefined }
function redirectErrors(form: UseFormReturn<ProjectForm>) { const errors = (form.formState.errors.configuration?.auth?.redirectUrls as any[] | undefined) ?? []; return errors.flatMap((item, index) => item?.message ? [[index, item.message] as [number, string]] : []) }
function Toggle({ label, checked, onChange, error }: { label: string; checked: boolean; onChange: (value: boolean) => void; error?: string }) { const id = `toggle-${label.toLowerCase().replace(/[^a-z0-9]+/g, '-')}`; return <Field className="rounded-lg border border-border px-3 py-2"><div className="flex items-center justify-between"><FieldLabel htmlFor={id}>{label}</FieldLabel><Switch id={id} checked={checked} onCheckedChange={onChange} aria-invalid={!!error} /></div><FieldError>{error}</FieldError></Field> }
function TextField({ label, name, form }: { label: string; name: string; form: UseFormReturn<ProjectForm> }) { const message = fieldError(form, name); return <Field><FieldLabel htmlFor={name}>{label}</FieldLabel><Input id={name} {...form.register(name as any)} aria-invalid={!!message} /><FieldError>{message}</FieldError></Field> }
function NumberField({ label, name, form }: { label: string; name: string; form: UseFormReturn<ProjectForm> }) { const message = fieldError(form, name); return <Field><FieldLabel htmlFor={name}>{label}</FieldLabel><Input id={name} type="number" {...form.register(name as any, { valueAsNumber: true })} aria-invalid={!!message} /><FieldError>{message}</FieldError></Field> }
function SecretField({ label, path, set, configured, error }: { label: string; path: string; set: (name: string, value: unknown) => void; configured: boolean; error?: string }) { return <Field><FieldLabel htmlFor={path}>{label}</FieldLabel><Input id={path} type="password" placeholder={configured ? 'Configured — enter to replace' : ''} onChange={(event) => set(path, event.target.value ? { action: 'replace', value: event.target.value } : { action: '' })} aria-invalid={!!error} /><FieldError>{error}</FieldError></Field> }
