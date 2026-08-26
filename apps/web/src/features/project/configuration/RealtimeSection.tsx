import { zodResolver } from '@hookform/resolvers/zod'
import { useEffect } from 'react'
import { useForm, type Resolver } from 'react-hook-form'
import type { RealtimeConfig } from '../../../api/types'
import { realtimeSchema } from './schema'
import { SectionCard, NumberField, SectionSaveButton } from './fields'
export function RealtimeSection({ initial, enabled, onSave }: { initial: RealtimeConfig; enabled: boolean; onSave: (value: RealtimeConfig, dirty: unknown) => void }) {
  const form = useForm<RealtimeConfig>({ resolver: zodResolver(realtimeSchema) as Resolver<RealtimeConfig>, defaultValues: initial }); useEffect(() => { form.reset(initial) }, [initial, form])
  return <form id="configuration-realtime-form" onSubmit={form.handleSubmit((value) => onSave(value, form.formState.dirtyFields))} className="space-y-5"><SectionCard title="Realtime" description="Realtime enablement is owned by Services. Values are bounded and typed."><p className="text-sm text-muted-foreground">Realtime service is currently <strong>{enabled ? 'enabled' : 'disabled'}</strong>.</p><div className="grid gap-4 md:grid-cols-2"><NumberField form={form} name="maxConnections" label="Maximum connections" min={1} max={100000} /><NumberField form={form} name="databasePoolSize" label="Database pool size" min={1} max={10000} /><div><label htmlFor="realtime-log-level" className="text-sm font-medium">Log level</label><select id="realtime-log-level" className="mt-1 h-9 w-full rounded-md border bg-background px-3 text-sm" {...form.register('logLevel')}><option value="debug">debug</option><option value="info">info</option><option value="warn">warn</option><option value="error">error</option></select></div></div></SectionCard><SectionSaveButton label="Realtime" disabled={!form.formState.isDirty} /></form>
}
