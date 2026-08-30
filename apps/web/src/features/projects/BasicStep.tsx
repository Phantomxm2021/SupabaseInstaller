import type { UseFormReturn } from 'react-hook-form'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Field, FieldError, FieldLabel, FieldGroup } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { InputGroup, InputGroupAddon, InputGroupInput } from '@/components/ui/input-group'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { FormControl, FormField, FormItem, FormLabel, FormMessage } from '@/components/ui/form'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import type { ProjectForm } from './projectSchema'
import type { ProjectIdentityAvailability } from './wizard/useProjectIdentityAvailability'
import type { Availability } from './wizard/types'

const httpsPrefix = 'https://'

function hostnameFromSiteURL(value: string) {
  return value.replace(/^https?:\/\//, '')
}

function siteURLFromHostname(value: string) {
  const hostname = value.trim().replace(/^https?:\/\//, '')
  return hostname ? `${httpsPrefix}${hostname}` : ''
}

function AvailabilityFeedback({ availability, retry }: { availability: Availability; retry?: () => void }) {
  if (availability.status === 'idle') return null
  const message = availability.message ?? 'Checking availability…'
  return <div role="status" aria-live="polite" className="flex items-center gap-2 text-sm text-muted-foreground">
    <span>{message}</span>
    {availability.status === 'unavailable' && retry && <button type="button" className="underline" onClick={retry}>Retry</button>}
  </div>
}

export function BasicStep({
  form,
  availability,
  onRetryAvailability,
  tls,
  onTLSChange,
}: {
  form: UseFormReturn<ProjectForm>
  availability: ProjectIdentityAvailability
  onRetryAvailability?: () => void
  tls: { enabled: boolean; certificate?: File; privateKey?: File }
  onTLSChange: (next: { enabled: boolean; certificate?: File; privateKey?: File }) => void
}) {
  const error = (name: string) => {
    let current: any = form.formState.errors
    for (const part of name.split('.')) current = current?.[part]
    return current?.message as string | undefined
  }
  const siteURL = form.watch('configuration.general.siteUrl')
  const nameError = error('name')
  const slugError = error('slug')
  const studioPasswordError = error('configuration.general.studioPassword')

  const customTLS = tls.enabled

  return (
    <div className="space-y-5">
    <Card>
      <CardHeader>
        <CardTitle>Server details</CardTitle>
        <CardDescription>Configure the identity, public address, Studio credentials, and runtime version.</CardDescription>
      </CardHeader>
      <CardContent>
        <FieldGroup className="space-y-6">
          <FormField
            control={form.control}
            name="name"
            render={({ field }) => (
              <FormItem>
                <FormLabel htmlFor="project-name">Server name</FormLabel>
                <Input id="project-name" autoFocus placeholder="Production API" {...field} aria-describedby={nameError ? 'name-form-item-message' : undefined} aria-invalid={!!nameError || availability.name.status === 'conflict'} />
                <FormMessage />
                <AvailabilityFeedback availability={availability.name} retry={onRetryAvailability} />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name="slug"
            render={({ field }) => (
              <FormItem>
                <FormLabel htmlFor="project-slug">Server slug</FormLabel>
                <Input id="project-slug" placeholder="production-api" {...field} aria-describedby={slugError ? 'slug-form-item-message' : undefined} aria-invalid={!!slugError || availability.slug.status === 'conflict'} />
                <FormMessage />
                <AvailabilityFeedback availability={availability.slug} retry={onRetryAvailability} />
              </FormItem>
            )}
          />

          <Field>
            <FieldLabel htmlFor="project-site-url-hostname">Site URL</FieldLabel>
            <InputGroup>
              <InputGroupAddon aria-hidden="true">{httpsPrefix}</InputGroupAddon>
              <InputGroupInput
                id="project-site-url-hostname"
                aria-label="Site URL hostname"
                placeholder="platform.example.com"
                aria-invalid={!!error('configuration.general.siteUrl')}
                value={hostnameFromSiteURL(siteURL)}
                onChange={(event) => form.setValue('configuration.general.siteUrl', siteURLFromHostname(event.target.value), { shouldDirty: true, shouldValidate: true })}
              />
            </InputGroup>
            <FieldError>{error('configuration.general.siteUrl')}</FieldError>
          </Field>

          <Field>
            <FieldLabel htmlFor="studio-username">Studio username</FieldLabel>
            <Input id="studio-username" {...form.register('configuration.general.studioUsername')} aria-invalid={!!error('configuration.general.studioUsername')} />
            <FieldError>{error('configuration.general.studioUsername')}</FieldError>
          </Field>

          <Field>
            <FieldLabel htmlFor="studio-password">Studio password</FieldLabel>
            <Input
              id="studio-password"
              type="password"
              onChange={(event) => form.setValue('configuration.general.studioPassword', event.target.value ? { action: 'replace', value: event.target.value } : { action: '' }, { shouldDirty: true, shouldValidate: true })}
              aria-invalid={!!studioPasswordError}
            />
            <FieldError>{studioPasswordError}</FieldError>
          </Field>

          <Collapsible>
            <CollapsibleTrigger className="font-medium">Runtime settings</CollapsibleTrigger>
            <CollapsibleContent className="mt-4">
              <Field>
                <FieldLabel>Supabase version</FieldLabel>
                <Select value={form.watch('configuration.general.supabaseVersion')} onValueChange={(value) => form.setValue('configuration.general.supabaseVersion', value as any, { shouldDirty: true, shouldValidate: true })}>
                  <SelectTrigger aria-label="Pinned Supabase version"><SelectValue /></SelectTrigger>
                  <SelectContent><SelectItem value="self-hosted/v0.8.0">self-hosted/v0.8.0</SelectItem></SelectContent>
                </Select>
              </Field>
            </CollapsibleContent>
          </Collapsible>
        </FieldGroup>
      </CardContent>
    </Card>
    <Card>
      <CardHeader>
        <CardTitle>TLS certificate</CardTitle>
        <CardDescription>Use the installed default certificate, or upload a certificate and matching private key for this server.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-5">
        <label className="flex cursor-pointer items-start gap-3 rounded-md border border-border px-4 py-3">
          <input type="radio" name="tls-source" checked={!customTLS} onChange={() => onTLSChange({ enabled: false })} className="mt-1" />
          <span className="space-y-1"><span className="block font-medium">Use default certificate</span><span className="block text-sm text-muted-foreground">Uses the host certificate named <code>cloudflare-origin</code>.</span></span>
        </label>
        <label className="flex cursor-pointer items-start gap-3 rounded-md border border-border px-4 py-3">
          <input type="radio" name="tls-source" checked={customTLS} onChange={() => onTLSChange({ ...tls, enabled: true })} className="mt-1" />
          <span className="space-y-1"><span className="block font-medium">Upload custom certificate</span><span className="block text-sm text-muted-foreground">Files are validated by the host and stored as <code>cloudflare-origin-{'{base-domain}'}.pem</code> and <code>.key</code>.</span></span>
        </label>
        {customTLS && <div className="grid gap-5 sm:grid-cols-2">
          <Field><FieldLabel htmlFor="tls-certificate">Certificate (.pem or .crt)</FieldLabel><Input id="tls-certificate" type="file" accept=".pem,.crt,application/x-pem-file" onChange={(event) => onTLSChange({ ...tls, certificate: event.target.files?.[0] })} /><p className="text-xs text-muted-foreground">{tls.certificate?.name ?? 'No certificate selected'}</p></Field>
          <Field><FieldLabel htmlFor="tls-private-key">Private key (.key or .pem)</FieldLabel><Input id="tls-private-key" type="file" accept=".key,.pem,application/x-pem-file" onChange={(event) => onTLSChange({ ...tls, privateKey: event.target.files?.[0] })} /><p className="text-xs text-muted-foreground">{tls.privateKey?.name ?? 'No private key selected'}</p></Field>
        </div>}
      </CardContent>
    </Card>
    </div>
  )
}
