import type { UseFormReturn } from 'react-hook-form'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Field, FieldError, FieldLabel, FieldGroup } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
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
}: {
  form: UseFormReturn<ProjectForm>
  availability: ProjectIdentityAvailability
  onRetryAvailability?: () => void
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

  return (
    <Card>
      <CardHeader>
        <CardTitle>Project details</CardTitle>
        <CardDescription>Configure the identity, public address, Studio credentials, and runtime version.</CardDescription>
      </CardHeader>
      <CardContent>
        <FieldGroup className="space-y-6">
          <FormField
            control={form.control}
            name="name"
            render={({ field }) => (
              <FormItem>
                <FormLabel htmlFor="project-name">Project name</FormLabel>
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
                <FormLabel htmlFor="project-slug">Project slug</FormLabel>
                <Input id="project-slug" placeholder="production-api" {...field} aria-describedby={slugError ? 'slug-form-item-message' : undefined} aria-invalid={!!slugError || availability.slug.status === 'conflict'} />
                <FormMessage />
                <AvailabilityFeedback availability={availability.slug} retry={onRetryAvailability} />
              </FormItem>
            )}
          />

          <Field>
            <FieldLabel htmlFor="project-site-url-hostname">Site URL</FieldLabel>
            <div className="flex h-8 overflow-hidden rounded-lg border border-input bg-transparent focus-within:border-ring focus-within:ring-3 focus-within:ring-ring/50">
              <span aria-hidden="true" className="flex items-center border-r border-input px-2.5 text-sm text-muted-foreground">{httpsPrefix}</span>
              <Input
                id="project-site-url-hostname"
                aria-label="Site URL hostname"
                className="h-full rounded-none border-0 bg-transparent focus-visible:border-0 focus-visible:ring-0"
                placeholder="platform.example.com"
                aria-invalid={!!error('configuration.general.siteUrl')}
                value={hostnameFromSiteURL(siteURL)}
                onChange={(event) => form.setValue('configuration.general.siteUrl', siteURLFromHostname(event.target.value), { shouldDirty: true, shouldValidate: true })}
              />
            </div>
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
  )
}
