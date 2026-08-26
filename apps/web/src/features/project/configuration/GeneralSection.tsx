import { zodResolver } from '@hookform/resolvers/zod'
import { useEffect } from 'react'
import { useForm, type Resolver } from 'react-hook-form'
import type { GeneralConfig } from '../../../api/types'
import { generalSchema } from './schema'
import { SectionCard, TextField, ReadOnlyField, SectionSaveButton } from './fields'
import type { ConfigurationSection } from './types'

type Props = { initial: GeneralConfig; onSave: (value: GeneralConfig, dirty: unknown) => void }
export function GeneralSection({ initial, onSave }: Props) {
  const form = useForm<GeneralConfig>({ resolver: zodResolver(generalSchema) as Resolver<GeneralConfig>, defaultValues: initial })
  useEffect(() => { form.reset(initial) }, [initial, form])
  const value = form.watch()
  const callback = value.siteUrl ? `${value.siteUrl.replace(/\/$/, '')}/auth/v1/callback` : 'Set Site URL to generate callback'
  return <form id="configuration-general-form" onSubmit={form.handleSubmit((next) => onSave(next, form.formState.dirtyFields))} className="space-y-5"><SectionCard title="General" description="Public identity and pinned runtime version."><div className="grid gap-4 md:grid-cols-2"><TextField form={form} name="domain" label="Domain" placeholder="bee.example.com" /><TextField form={form} name="siteUrl" label="Site URL" placeholder="https://example.com" /><ReadOnlyField label="Supabase version" value={initial.supabaseVersion} /><ReadOnlyField label="Project URL" value={value.domain ? `https://${value.domain}` : 'Set a domain to generate Project URL'} copy={Boolean(value.domain)} /><ReadOnlyField label="OAuth callback URL" value={callback} copy={Boolean(value.siteUrl)} /></div></SectionCard><SectionSaveButton label="General" disabled={!form.formState.isDirty} /></form>
}
export type { ConfigurationSection }
