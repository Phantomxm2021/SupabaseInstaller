import { zodResolver } from '@hookform/resolvers/zod'
import { useForm, type Resolver } from 'react-hook-form'
import type { PoolerConfig } from '../../../api/types'
import { poolerSchema } from './schema'
import { SectionCard, NumberField, ReadOnlyField, SectionSaveButton, useResetOnServerRevision, type SectionSave } from './fields'

export function PoolerSection({ initial, revision, enabled, onSave }: { initial: PoolerConfig; revision: number; enabled: boolean; onSave: SectionSave<PoolerConfig> }) {
  const form = useForm<PoolerConfig>({ resolver: zodResolver(poolerSchema) as Resolver<PoolerConfig>, defaultValues: initial }); useResetOnServerRevision(form, initial, revision)
  const pooler = form.watch(); const setError = (name: string, message: string) => form.setError(name as never, { type: 'server', message })
  return <form id="configuration-pooler-form" onSubmit={form.handleSubmit((value) => onSave({ value, dirty: form.formState.dirtyFields, setError }))} className="space-y-5"><SectionCard title="Connection Pooler" description="Supavisor enablement is owned by Services. Ports are allocated by Manager."><p className="text-sm text-muted-foreground">Supavisor is currently <strong>{enabled ? 'enabled' : 'disabled'}.</strong></p><div className="grid gap-4 md:grid-cols-2"><ReadOnlyField label="Transaction port" value={String(pooler.transactionPort || 'Pending allocation')} /><ReadOnlyField label="Session port" value={String(pooler.sessionPort || 'Pending allocation')} /><NumberField form={form} name="poolSize" label="Pool size" min={1} max={100000} /><NumberField form={form} name="maxClientConnections" label="Maximum client connections" min={1} max={100000} /></div></SectionCard><SectionSaveButton label="Connection Pooler" disabled={!form.formState.isDirty} /></form>
}
