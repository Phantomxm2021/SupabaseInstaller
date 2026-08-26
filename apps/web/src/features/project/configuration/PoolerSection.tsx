import { zodResolver } from '@hookform/resolvers/zod'
import { useEffect } from 'react'
import { useForm, type Resolver } from 'react-hook-form'
import type { PoolerConfig } from '../../../api/types'
import { poolerSchema } from './schema'
import { SectionCard, NumberField, ReadOnlyField, SectionSaveButton } from './fields'
export function PoolerSection({ initial, enabled, onSave }: { initial: PoolerConfig; enabled: boolean; onSave: (value: PoolerConfig, dirty: unknown) => void }) {
  const form = useForm<PoolerConfig>({ resolver: zodResolver(poolerSchema) as Resolver<PoolerConfig>, defaultValues: initial }); useEffect(() => { form.reset(initial) }, [initial, form]); const pooler = form.watch()
  return <form id="configuration-pooler-form" onSubmit={form.handleSubmit((value) => onSave(value, form.formState.dirtyFields))} className="space-y-5"><SectionCard title="Connection Pooler" description="Supavisor enablement is owned by Services. Ports are allocated by Manager."><p className="text-sm text-muted-foreground">Supavisor is currently <strong>{enabled ? 'enabled' : 'disabled'}</strong>.</p><div className="grid gap-4 md:grid-cols-2"><ReadOnlyField label="Transaction port" value={String(pooler.transactionPort || 'Pending allocation')} /><ReadOnlyField label="Session port" value={String(pooler.sessionPort || 'Pending allocation')} /><NumberField form={form} name="poolSize" label="Pool size" min={1} max={100000} /><NumberField form={form} name="maxClientConnections" label="Maximum client connections" min={1} max={100000} /></div></SectionCard><SectionSaveButton label="Connection Pooler" disabled={!form.formState.isDirty} /></form>
}
