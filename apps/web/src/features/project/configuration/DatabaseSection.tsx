import { zodResolver } from '@hookform/resolvers/zod'
import { useForm, type Resolver } from 'react-hook-form'
import { Button } from '@/components/ui/button'
import type { DatabaseConfig } from '../../../api/types'
import { databaseSchema } from './schema'
import { SectionCard, NumberField, TextField, ReadOnlyField, SectionSaveButton, useResetOnServerRevision, type SectionSave } from './fields'

export function DatabaseSection({ initial, revision, directDb, onSave, onRotate }: { initial: DatabaseConfig; revision: number; directDb: boolean; onSave: SectionSave<DatabaseConfig>; onRotate: () => void }) {
  const form = useForm<DatabaseConfig>({ resolver: zodResolver(databaseSchema) as Resolver<DatabaseConfig>, defaultValues: initial })
  useResetOnServerRevision(form, initial, revision)
  const db = form.watch(); const setError = (name: string, message: string) => form.setError(name as never, { type: 'server', message })
  return <form id="configuration-database-form" onSubmit={form.handleSubmit((value) => onSave({ value, dirty: form.formState.dirtyFields, setError }))} className="space-y-5"><SectionCard title="Database" description="Postgres version and limits. Database service toggles are owned by Services."><div className="grid gap-4 md:grid-cols-2"><ReadOnlyField label="Postgres version" value={db.version} /><p className="rounded-lg border border-border p-3 text-sm text-muted-foreground">Direct PostgreSQL port is currently <strong>{directDb ? 'enabled' : 'disabled'}.</strong></p><NumberField form={form} name="maxConnections" label="Maximum connections" min={1} max={100000} /><TextField form={form} name="sharedBuffers" label="Shared buffers" /></div><ReadOnlyField label="Supported extensions" value={db.extensions.join(', ') || 'None supported by pinned runtime'} /><Button type="button" variant="destructive" onClick={onRotate}>Rotate database password</Button></SectionCard><SectionSaveButton label="Database" disabled={!form.formState.isDirty} /></form>
}
