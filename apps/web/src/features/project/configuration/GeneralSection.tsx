import { zodResolver } from '@hookform/resolvers/zod'
import { useEffect } from 'react'
import { useForm, type Resolver } from 'react-hook-form'
import type { GeneralConfig } from '../../../api/types'
import { generalSchema } from './schema'
import { SectionCard, TextField, ReadOnlyField, SectionSaveButton, useResetOnServerRevision, type SectionSave } from './fields'
import type { ConfigurationSection } from './types'

type Props = { initial: GeneralConfig; revision: number; onSave: SectionSave<GeneralConfig>; serverErrors?: Record<string, string> }
export function GeneralSection({ initial, revision, onSave, serverErrors = {} }: Props) {
  const form = useForm<GeneralConfig>({ resolver: zodResolver(generalSchema) as Resolver<GeneralConfig>, defaultValues: initial })
  useResetOnServerRevision(form, initial, revision)
  useEffect(() => { for (const [path, message] of Object.entries(serverErrors)) form.setError(path.replace(/^general\./, '') as 'domain' | 'siteUrl', { type: 'server', message }) }, [form, serverErrors])
  const value = form.watch()
  const callback = value.domain ? `https://${value.domain}/auth/v1/callback` : 'Set Domain to generate callback'
  return <form id="configuration-general-form" onSubmit={form.handleSubmit((value) => onSave({ value, dirty: form.formState.dirtyFields, setError: (name, message) => form.setError(name as 'domain', { type: 'server', message }) }))} className="space-y-5"><SectionCard title="General" description="Public identity and pinned runtime version."><div className="grid gap-4 md:grid-cols-2"><TextField form={form} name="domain" label="Domain" placeholder="bee.example.com" /><TextField form={form} name="siteUrl" label="Site URL" placeholder="https://example.com" /><ReadOnlyField label="Supabase version" value={initial.supabaseVersion} /><ReadOnlyField label="Project URL" value={value.domain ? `https://${value.domain}` : 'Set a domain to generate Project URL'} copy={Boolean(value.domain)} /><ReadOnlyField label="OAuth callback URL" value={callback} copy={Boolean(value.domain)} /></div></SectionCard><SectionSaveButton label="General" disabled={!form.formState.isDirty} /></form>
}
export type { ConfigurationSection }
