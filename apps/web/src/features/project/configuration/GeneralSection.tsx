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
  useEffect(() => { for (const [path, message] of Object.entries(serverErrors)) form.setError(path.replace(/^general\./, '') as 'siteUrl', { type: 'server', message }) }, [form, serverErrors])
  const domain = initial.domain
  const callback = domain ? `https://${domain}/auth/v1/callback` : 'Save a base domain to generate callback'
  return <form id="configuration-general-form" onSubmit={form.handleSubmit((value) => onSave({ value, dirty: form.formState.dirtyFields, setError: (name, message) => form.setError(name as 'siteUrl', { type: 'server', message }) }))} className="space-y-5"><SectionCard title="General" description="The public project hostname is generated from the slug and server base domain."><div className="space-y-4"><TextField form={form} name="siteUrl" label="Server base domain" placeholder="https://platform.example.com" /><TextField form={form} name="authSiteUrl" label="Auth site URL" placeholder="https://app.example.com" /><ReadOnlyField label="Public project hostname" value={domain || 'Save a base domain to generate a public project hostname'} /><ReadOnlyField label="Supabase version" value={initial.supabaseVersion} /><ReadOnlyField label="Project URL" value={domain ? `https://${domain}` : 'Save a base domain to generate Project URL'} copy={Boolean(domain)} /><ReadOnlyField label="OAuth callback URL" value={callback} copy={Boolean(domain)} /></div></SectionCard><SectionSaveButton label="General" disabled={!form.formState.isDirty} /></form>
}

export type { ConfigurationSection }
