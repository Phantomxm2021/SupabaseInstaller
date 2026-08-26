import { zodResolver } from '@hookform/resolvers/zod'
import { useForm, type Resolver } from 'react-hook-form'
import type { StorageConfig } from '../../../api/types'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { storageSchema } from './schema'
import { SectionCard, Toggle, TextField, SecretEditor, SectionSaveButton, ReadOnlyField, useResetOnServerRevision } from './fields'

export function StorageSection({ initial, revision, storageEnabled, onSave }: { initial: StorageConfig; revision: number; storageEnabled: boolean; onSave: (value: StorageConfig, dirty: unknown) => void }) {
  const form = useForm<StorageConfig>({ resolver: zodResolver(storageSchema) as Resolver<StorageConfig>, defaultValues: initial })
  useResetOnServerRevision(form, initial, revision)
  const storage = form.watch()
  const objectStorage = storage.backend !== 'local'
  const setBackend = (backend: StorageConfig['backend']) => {
    const current = form.getValues()
    const next = { ...current, backend, endpoint: backend === 's3' ? current.endpoint : '', accountId: backend === 'r2' ? current.accountId : '', region: backend === 'r2' || backend === 'local' ? '' : current.region }
    if (backend === 'local') {
      Object.assign(next, { bucket: '', region: '', endpoint: '', accountId: '', accessKeyId: '', forcePathStyle: false })
      if (current.secretAccessKeySet) { next.secretAccessKeySet = true; next.secretAccessKey = { action: 'remove' } }
      else next.secretAccessKey = { action: '' }
    } else if (current.backend !== backend && current.secretAccessKeySet) {
      // Keep the existing key when moving between S3-compatible backends unless the user explicitly removes it.
      next.secretAccessKey = { action: 'retain' }
    }
    form.reset(next, { keepDirty: true })
    form.setValue('backend', backend, { shouldDirty: true, shouldValidate: true })
  }
  return <form id="configuration-storage-form" onSubmit={form.handleSubmit((value) => onSave(value, form.formState.dirtyFields))} className="space-y-5"><SectionCard title="Storage" description="Storage enablement is owned by Services. Backend changes retain or explicitly remove credentials and normalize derived fields."><div className="rounded-lg border border-border p-3 text-sm text-muted-foreground">Storage service is currently <strong>{storageEnabled ? 'enabled' : 'disabled'}</strong>. Use the Services section to change it.</div><div><label htmlFor="storage-backend" className="text-sm font-medium">Backend</label><Select value={storage.backend} onValueChange={(value) => setBackend(value as StorageConfig['backend'])}><SelectTrigger id="storage-backend" aria-label="Storage backend" className="mt-1 w-full"><SelectValue placeholder="Choose backend" /></SelectTrigger><SelectContent><SelectItem value="local">Local filesystem</SelectItem><SelectItem value="s3">Generic S3</SelectItem><SelectItem value="aws-s3">AWS S3</SelectItem><SelectItem value="r2">Cloudflare R2</SelectItem></SelectContent></Select></div><Toggle id="s3-compatible-api" label="Enable S3-compatible API" checked={storage.s3CompatibleApi} onChange={(value) => form.setValue('s3CompatibleApi', value, { shouldDirty: true, shouldValidate: true })} />{objectStorage ? <div className="grid gap-4 md:grid-cols-2"><TextField form={form} name="bucket" label="Bucket" /><TextField form={form} name="region" label="Region" /><TextField form={form} name="endpoint" label="Endpoint" /><TextField form={form} name="accountId" label="Account ID (R2)" /><TextField form={form} name="accessKeyId" label="Access key ID" /><SecretEditor label="Secret access key" configured={storage.secretAccessKeySet} secret={storage.secretAccessKey} onChange={(value) => form.setValue('secretAccessKey', value, { shouldDirty: true, shouldValidate: true })} /><Toggle id="force-path-style" label="Force path style" checked={storage.forcePathStyle} onChange={(value) => form.setValue('forcePathStyle', value, { shouldDirty: true, shouldValidate: true })} /></div> : <ReadOnlyField label="Managed local path" value={storage.localPath || 'Project-scoped runtime path'} />}</SectionCard><SectionSaveButton label="Storage" disabled={!form.formState.isDirty} /></form>
}
