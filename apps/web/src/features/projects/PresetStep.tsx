import type { UseFormReturn } from 'react-hook-form'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldError, FieldLabel, FieldGroup } from '@/components/ui/field'
import { Switch } from '@/components/ui/switch'
import type { ProjectForm, PresetName } from './projectSchema'
import { applyPreset, defaultConfiguration } from './projectSchema'

const names: Record<keyof ProjectForm['configuration']['services'], string> = { database: 'PostgreSQL', gateway: 'API Gateway', auth: 'Authentication', rest: 'PostgREST', studio: 'Supabase Studio', postgresMeta: 'postgres-meta', realtime: 'Realtime', storage: 'Storage', imgproxy: 'Image Transformation', functions: 'Edge Functions', supavisor: 'Supavisor', logs: 'Logs & Analytics', vector: 'Vector', directDb: 'Direct PostgreSQL port' }
const presets: Array<[PresetName, string]> = [['LIGHTWEIGHT', 'Core database, gateway, Auth, REST and Studio.'], ['STANDARD', 'Adds Realtime, Storage, Functions and Supavisor.'], ['FULL', 'All official optional services, including Logs and image transformation.'], ['CUSTOM', 'Choose every service individually.']]

/** The only service mutation path used by every wizard step. */
export function setServiceEnabled(form: UseFormReturn<ProjectForm>, name: keyof ProjectForm['configuration']['services'], enabled: boolean) {
  const current = form.getValues('configuration.services'); const next = { ...current, [name]: enabled }
  if (name === 'studio' && enabled) next.postgresMeta = true
  if (name === 'storage' && !enabled) { next.imgproxy = false; form.setValue('configuration.storage', { ...form.getValues('configuration.storage'), backend: 'local', bucket: '', region: '', endpoint: '', accountId: '', accessKeyId: '', secretAccessKeySet: false, secretAccessKey: { action: '' }, forcePathStyle: false }) }
  if (name === 'logs') next.vector = enabled
  if (name === 'vector') next.logs = enabled
  if (name === 'directDb') { next.directDb = enabled; form.setValue('configuration.database.directPort', false, { shouldDirty: true }); form.setValue('configuration.database.directPortNumber', 0, { shouldDirty: true }); form.setValue('configuration.network.directDatabasePort', 0, { shouldDirty: true }) }
  if (name === 'auth') form.setValue('configuration.auth.enabled', enabled, { shouldDirty: true })
  if (name === 'imgproxy' && enabled) next.storage = true
  form.setValue('preset', 'CUSTOM', { shouldDirty: true }); form.setValue('configuration.services', next, { shouldDirty: true, shouldValidate: true })
}

export function PresetStep({ form }: { form: UseFormReturn<ProjectForm> }) {
  const preset = form.watch('preset'); const services = form.watch('configuration.services')
  const serviceError = (name: keyof ProjectForm['configuration']['services']) => (form.formState.errors.configuration?.services as any)?.[name]?.message as string | undefined
  const setPreset = (next: PresetName) => {
    const current = form.getValues('configuration')
    const reset = defaultConfiguration(next)
    // Presets are aggregate defaults; preserve only identity fields entered on step 1.
    reset.general = current.general
    form.setValue('preset', next, { shouldDirty: true })
    form.setValue('configuration', reset as ProjectForm['configuration'], { shouldDirty: true, shouldValidate: true })
  }
  return <Card><CardHeader><CardTitle>Preset & services</CardTitle><CardDescription>Every switch is typed and dependency-aware. Editing a preset changes it to Custom.</CardDescription></CardHeader><CardContent><FieldGroup className="grid gap-3 md:grid-cols-2">
    {presets.map(([key, description]) => <Button type="button" variant={preset === key ? 'default' : 'outline'} aria-label={key[0] + key.slice(1).toLowerCase()} key={key} className="h-auto justify-start whitespace-normal p-4 text-left" onClick={() => setPreset(key)}><strong>{key[0] + key.slice(1).toLowerCase()}</strong><FieldDescription>{description}</FieldDescription></Button>)}
  </FieldGroup><FieldGroup className="mt-6 grid gap-3 md:grid-cols-2">{(Object.keys(names) as Array<keyof typeof names>).map((name) => { const gatewayRequired = services.auth || services.rest || services.studio || services.realtime || services.storage || services.functions; const forced = name === 'database' || (name === 'gateway' && gatewayRequired) || (name === 'postgresMeta' && services.studio) || (name === 'vector' && services.logs); return <Field key={name} className="rounded-lg border border-border px-3 py-2"><div className="flex items-center justify-between"><FieldLabel htmlFor={`service-${name}`}>{names[name]}{forced ? ' (required)' : ''}</FieldLabel><Switch id={`service-${name}`} checked={services[name]} disabled={forced} onCheckedChange={(checked) => setServiceEnabled(form, name, checked)} /></div><FieldError>{serviceError(name)}</FieldError></Field> })}</FieldGroup></CardContent></Card>
}
