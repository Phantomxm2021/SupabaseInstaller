import { zodResolver } from '@hookform/resolvers/zod'
import { useForm, type Resolver } from 'react-hook-form'
import type { Services } from '../../../api/types'
import { servicesSchema } from './schema'
import { SectionCard, Toggle, SectionSaveButton, useResetOnServerRevision, errorAt, type SectionSave } from './fields'

const labels: Record<keyof Services, string> = { database: 'PostgreSQL', gateway: 'Envoy Gateway', auth: 'Auth', rest: 'PostgREST', studio: 'Studio', postgresMeta: 'postgres-meta', realtime: 'Realtime', storage: 'Storage', imgproxy: 'Image Transformation', functions: 'Edge Functions', supavisor: 'Supavisor', logs: 'Logs / Logflare', vector: 'Vector', directDb: 'Direct PostgreSQL port' }
const locked: ReadonlySet<keyof Services> = new Set(['database'])

export function ServicesSection({ initial, revision, onSave }: { initial: Services; revision: number; onSave: SectionSave<Services> }) {
  const form = useForm<Services>({ resolver: zodResolver(servicesSchema) as Resolver<Services>, defaultValues: initial })
  useResetOnServerRevision(form, initial, revision)
  const change = (name: keyof Services, value: boolean) => {
    const next = { ...form.getValues(), [name]: value }
    if (value) {
      if (name === 'studio') next.postgresMeta = true
      if (name === 'imgproxy') { next.storage = true; next.rest = true; next.gateway = true; next.database = true }
      if (name === 'storage') { next.database = true; next.rest = true; next.gateway = true }
      if (name === 'realtime' || name === 'functions' || name === 'auth' || name === 'rest') next.gateway = true
      if (name === 'supavisor' || name === 'directDb') next.database = true
      if (name === 'logs') next.vector = true
    } else {
      if (name === 'studio') next.postgresMeta = false
      if (name === 'postgresMeta') next.studio = false
      if (name === 'logs') next.vector = false
      if (name === 'vector') next.logs = false
      if (name === 'storage') next.imgproxy = false
      if (name === 'rest') { next.storage = false; next.imgproxy = false }
      if (name === 'gateway') Object.assign(next, { auth: false, rest: false, studio: false, realtime: false, storage: false, imgproxy: false, functions: false })
    }
    form.reset(next, { keepDirty: true })
    for (const key of Object.keys(next) as Array<keyof Services>) if (next[key] !== initial[key]) form.setValue(key, next[key], { shouldDirty: true, shouldValidate: true })
  }
  const setError = (name: string, message: string) => form.setError(name as never, { type: 'server', message })
  return <form id="configuration-services-form" onSubmit={form.handleSubmit((value) => onSave({ value, dirty: form.formState.dirtyFields, setError }))} className="space-y-5"><SectionCard title="Services" description="This is the only section that owns service enablement. Dependencies are validated by Manager."><div className="grid gap-3 md:grid-cols-2">{(Object.keys(labels) as Array<keyof Services>).map((name) => <Toggle key={name} id={`service-${name}`} label={labels[name]} checked={Boolean(form.watch(name))} disabled={locked.has(name)} onChange={(value) => change(name, value)} error={errorAt(form.formState.errors, String(name))} description={locked.has(name) ? 'Required by the pinned runtime.' : name === 'logs' ? 'Logs and Vector are managed as one feature.' : name === 'imgproxy' ? 'Image Transformation requires Storage.' : undefined} />)}</div></SectionCard><SectionSaveButton label="Services" disabled={!form.formState.isDirty} /></form>
}
