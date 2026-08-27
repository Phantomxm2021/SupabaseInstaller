import type { UseFormReturn } from 'react-hook-form'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Field, FieldDescription, FieldError, FieldLabel, FieldGroup } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { FormControl, FormField, FormItem, FormLabel, FormMessage } from '@/components/ui/form'
import type { ProjectForm } from './projectSchema'

const httpsPrefix = 'https://'

function hostnameFromSiteURL(value: string) {
  return value.replace(/^https?:\/\//, '')
}

function siteURLFromHostname(value: string) {
  const hostname = value.trim().replace(/^https?:\/\//, '')
  return hostname ? `${httpsPrefix}${hostname}` : ''
}

export function BasicStep({ form }: { form: UseFormReturn<ProjectForm> }) {
  const error = (name: string) => { let current: any = form.formState.errors; for (const part of name.split('.')) current = current?.[part]; return current?.message as string | undefined }
  const siteURL = form.watch('configuration.general.siteUrl')
  return <Card><CardHeader><CardTitle>Project details</CardTitle><CardDescription>Basic identity and the pinned official Supabase runtime.</CardDescription></CardHeader><CardContent><FieldGroup className="grid gap-5 md:grid-cols-2">
    <FormField control={form.control} name="name" render={({ field }) => <FormItem><FormLabel>Project name</FormLabel><FormControl><Input autoFocus placeholder="Bee" {...field} /></FormControl><FormMessage /></FormItem>} />
    <Field><FieldLabel htmlFor="project-slug">Project slug</FieldLabel><Input id="project-slug" placeholder="bee" aria-invalid={!!error('slug')} {...form.register('slug')} /><FieldError>{error('slug')}</FieldError></Field>
    <Field><FieldLabel htmlFor="project-domain">Domain</FieldLabel><FieldDescription>Hostname only, without a scheme.</FieldDescription><Input id="project-domain" placeholder="bee.example.com" aria-invalid={!!error('configuration.general.domain')} {...form.register('configuration.general.domain')} /><FieldError>{error('configuration.general.domain')}</FieldError></Field>
    <Field><FieldLabel htmlFor="project-site-url-hostname">Site URL</FieldLabel><div className="flex h-8 overflow-hidden rounded-lg border border-input bg-transparent focus-within:border-ring focus-within:ring-3 focus-within:ring-ring/50"><span aria-hidden="true" className="flex items-center border-r border-input px-2.5 text-sm text-muted-foreground">{httpsPrefix}</span><Input id="project-site-url-hostname" aria-label="Site URL hostname" className="h-full rounded-none border-0 bg-transparent focus-visible:border-0 focus-visible:ring-0" placeholder="example.com" aria-invalid={!!error('configuration.general.siteUrl')} value={hostnameFromSiteURL(siteURL)} onChange={(event) => form.setValue('configuration.general.siteUrl', siteURLFromHostname(event.target.value), { shouldDirty: true, shouldValidate: true })} /></div><FieldError>{error('configuration.general.siteUrl')}</FieldError></Field>
    <Field><FieldLabel>Supabase version</FieldLabel><Select value={form.watch('configuration.general.supabaseVersion')} onValueChange={(value) => form.setValue('configuration.general.supabaseVersion', value as any, { shouldDirty: true, shouldValidate: true })}><SelectTrigger aria-label="Pinned Supabase version"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="self-hosted/v0.8.0">self-hosted/v0.8.0</SelectItem></SelectContent></Select></Field>
  </FieldGroup></CardContent></Card>
}
