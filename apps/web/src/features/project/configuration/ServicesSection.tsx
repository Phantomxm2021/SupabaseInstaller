import { zodResolver } from '@hookform/resolvers/zod'
import { useEffect } from 'react'
import { useForm, type Resolver } from 'react-hook-form'
import type { Services } from '../../../api/types'
import { servicesSchema } from './schema'
import { SectionCard, Toggle, SectionSaveButton } from './fields'

const labels: Record<keyof Services, string> = { database: 'PostgreSQL', gateway: 'Envoy Gateway', auth: 'Auth', rest: 'PostgREST', studio: 'Studio', postgresMeta: 'postgres-meta', realtime: 'Realtime', storage: 'Storage', imgproxy: 'Image Transformation', functions: 'Edge Functions', supavisor: 'Supavisor', logs: 'Logs / Logflare', vector: 'Vector', directDb: 'Direct PostgreSQL port' }
const locked: ReadonlySet<keyof Services> = new Set(['database', 'gateway', 'rest', 'studio', 'postgresMeta'])
export function ServicesSection({ initial, onSave }: { initial: Services; onSave: (value: Services, dirty: unknown) => void }) {
  const form = useForm<Services>({ resolver: zodResolver(servicesSchema) as Resolver<Services>, defaultValues: initial })
  useEffect(() => { form.reset(initial) }, [initial, form])
  const change = (name: keyof Services, value: boolean) => { const next = { ...form.getValues(), [name]: value }; if (name === 'studio') next.postgresMeta = value; if (name === 'logs') next.vector = value; if (name === 'storage' && !value) next.imgproxy = false; if (name === 'imgproxy' && value) next.storage = true; form.reset(next, { keepDirty: true }); for (const key of Object.keys(next) as Array<keyof Services>) if (next[key] !== initial[key]) form.setValue(key, next[key], { shouldDirty: true, shouldValidate: true }) }
  return <form id="configuration-services-form" onSubmit={form.handleSubmit((value) => onSave(value, form.formState.dirtyFields))} className="space-y-5"><SectionCard title="Services" description="This is the only section that owns service enablement. Dependencies are validated by Manager."><div className="grid gap-3 md:grid-cols-2">{(Object.keys(labels) as Array<keyof Services>).map((name) => <Toggle key={name} id={`service-${name}`} label={labels[name]} checked={Boolean(form.watch(name))} disabled={locked.has(name)} onChange={(value) => change(name, value)} description={locked.has(name) ? 'Required by the pinned runtime.' : name === 'logs' ? 'Logs and Vector are managed as one feature.' : name === 'imgproxy' ? 'Image Transformation requires Storage.' : undefined} />)}</div></SectionCard><SectionSaveButton label="Services" disabled={!form.formState.isDirty} /></form>
}
