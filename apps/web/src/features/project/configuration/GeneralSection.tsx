import { zodResolver } from '@hookform/resolvers/zod'
import { useForm, type Resolver } from 'react-hook-form'
import type { GeneralConfig } from '../../../api/types'
import { generalSchema } from './schema'
import { SectionCard, TextField, ReadOnlyField, SectionSaveButton, useResetOnServerRevision } from './fields'
import type { ConfigurationSection } from './types'

type Props = { initial: GeneralConfig; revision: number; onSave: (value: GeneralConfig, dirty: unknown, setError: (name: string, message: string) => void) => void }
export function GeneralSection({ initial, revision, onSave }: Props) {
  const form = useForm<GeneralConfig>({ resolver: zodResolver(generalSchema) as Resolver<GeneralConfig>, defaultValues: initial })
  useResetOnServerRevision(form, initial, revision)
  const value = form.watch()
  const callback = value.domain ? `https://${value.domain}/auth/v1/callback` : 'Set Domain to generate callback'
  return <form id="configuration-general-form" onSubmit={form.handleSubmit((next) => onSave(next, form.formState.dirtyFields, (name, message) => form.setError(name as 'domain', { type: 'server', message })))} className="space-y-5"><SectionCard title="General" description="Public identity and pinned runtime version."><div className="grid gap-4 md:grid-cols-2"><TextField form={form} name="domain" label="Domain" placeholder="bee.example.com" /><TextField form={form} name="siteUrl" label="Site URL" placeholder="https://example.com" /><ReadOnlyField label="Supabase version" value={initial.supabaseVersion} /><ReadOnlyField label="Project URL" value={value.domain ? `https://${value.domain}` : 'Set a domain to generate Project URL'} copy={Boolean(value.domain)} /><ReadOnlyField label="OAuth callback URL" value={callback} copy={Boolean(value.domain)} /></div></SectionCard><SectionSaveButton label="General" disabled={!form.formState.isDirty} /></form>
}
export type { ConfigurationSection }
