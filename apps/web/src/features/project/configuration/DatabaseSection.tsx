import { zodResolver } from '@hookform/resolvers/zod'
import { useEffect } from 'react'
import { useForm, type Resolver } from 'react-hook-form'
import type { DatabaseConfig } from '../../../api/types'
import { databaseSchema } from './schema'
import { SectionCard, NumberField, TextField, ReadOnlyField, SectionSaveButton } from './fields'
export function DatabaseSection({ initial, directDb, onSave, onRotate }: { initial: DatabaseConfig; directDb: boolean; onSave: (value: DatabaseConfig, dirty: unknown) => void; onRotate: () => void }) {
  const form = useForm<DatabaseConfig>({ resolver: zodResolver(databaseSchema) as Resolver<DatabaseConfig>, defaultValues: initial }); useEffect(() => { form.reset(initial) }, [initial, form]); const db = form.watch()
  return <form id="configuration-database-form" onSubmit={form.handleSubmit((value) => onSave(value, form.formState.dirtyFields))} className="space-y-5"><SectionCard title="Database" description="Postgres version and limits. Database service toggles are owned by Services."><div className="grid gap-4 md:grid-cols-2"><ReadOnlyField label="Postgres version" value={db.version} /><p className="rounded-lg border border-border p-3 text-sm text-muted-foreground">Direct PostgreSQL port is currently <strong>{directDb ? 'enabled' : 'disabled'}.</strong></p><NumberField form={form} name="maxConnections" label="Maximum connections" min={1} max={100000} /><TextField form={form} name="sharedBuffers" label="Shared buffers" /></div><ReadOnlyField label="Supported extensions" value={db.extensions.join(', ') || 'None supported by pinned runtime'} /><button type="button" className="rounded-md border border-destructive px-3 py-2 text-sm text-destructive" onClick={onRotate}>Rotate database password</button></SectionCard><SectionSaveButton label="Database" disabled={!form.formState.isDirty} /></form>
}
