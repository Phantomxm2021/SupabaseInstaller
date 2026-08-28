import type { UseFormReturn } from 'react-hook-form'
import type { ProjectForm } from './projectSchema'

export type ServiceName = keyof ProjectForm['configuration']['services']

export const serviceLabels: Record<ServiceName, string> = { database: 'PostgreSQL', gateway: 'API Gateway', auth: 'Authentication', rest: 'PostgREST', studio: 'Supabase Studio', postgresMeta: 'postgres-meta', realtime: 'Realtime', storage: 'Storage', imgproxy: 'Image Transformation', functions: 'Edge Functions', supavisor: 'Supavisor', logs: 'Logs & Analytics', vector: 'Vector', directDb: 'Direct PostgreSQL port' }

export const serviceGroups = [
  { label: 'Core services', names: ['database', 'gateway', 'rest', 'auth', 'studio', 'postgresMeta'] as const },
  { label: 'Extended services', names: ['realtime', 'storage', 'imgproxy', 'functions', 'supavisor', 'logs', 'vector', 'directDb'] as const },
] as const

export const servicePresets = [
  { name: 'LIGHTWEIGHT', label: 'Lightweight', description: 'Core database, gateway, Auth, REST and Studio.' },
  { name: 'STANDARD', label: 'Standard', description: 'Adds Realtime, Storage, Functions and Supavisor.' },
  { name: 'FULL', label: 'Full', description: 'All official optional services, including Logs and image transformation.' },
  { name: 'CUSTOM', label: 'Custom', description: 'Choose every service individually.' },
] as const

export function serviceControlState(services: ProjectForm['configuration']['services'], httpsMode: ProjectForm['configuration']['network']['httpsMode'], name: ServiceName) {
  const gatewayRequired = services.auth || services.rest || services.studio || services.realtime || services.storage || services.functions || httpsMode === 'caddy'
  const forced = name === 'database' || (name === 'gateway' && gatewayRequired) || (name === 'rest' && services.storage) || (name === 'postgresMeta' && services.studio) || (name === 'vector' && services.logs)
  const help = name === 'gateway' && gatewayRequired
    ? 'API Gateway is required by enabled services.'
    : name === 'storage'
      ? 'Storage requires PostgREST; disabling Storage also disables Image Transformation.'
      : name === 'imgproxy'
        ? 'Image Transformation requires Storage and enables it automatically.'
        : name === 'logs' || name === 'vector'
          ? 'Logs & Analytics and Vector are enabled or disabled together.'
          : undefined
  return { forced, help }
}

/** The only service mutation path used by every wizard step. */
export function setServiceEnabled(form: UseFormReturn<ProjectForm>, name: ServiceName, enabled: boolean) {
  const current = form.getValues('configuration.services'); const next = { ...current, [name]: enabled }
  if (enabled && name !== 'database') next.database = true
  if (enabled && ['auth', 'rest', 'studio', 'realtime', 'storage', 'functions', 'imgproxy'].includes(name)) next.gateway = true
  if (enabled && ['storage', 'imgproxy'].includes(name)) next.rest = true
  if (enabled && name === 'gateway') next.database = true
  if (name === 'rest' && !enabled && current.storage) next.rest = true
  if (name === 'gateway' && !enabled && (current.auth || current.rest || current.studio || current.realtime || current.storage || current.functions || form.getValues('configuration.network.httpsMode') === 'caddy')) next.gateway = true
  if (name === 'studio' && enabled) next.postgresMeta = true
  if (name === 'storage' && !enabled) { next.imgproxy = false; form.setValue('configuration.storage', { ...form.getValues('configuration.storage'), backend: 'local', bucket: '', region: '', endpoint: '', accountId: '', accessKeyId: '', secretAccessKeySet: false, secretAccessKey: { action: '' }, forcePathStyle: false }) }
  if (name === 'studio' && !enabled) next.postgresMeta = false
  if (name === 'logs') next.vector = enabled
  if (name === 'vector') next.logs = enabled
  if (name === 'directDb') { next.directDb = enabled; form.setValue('configuration.database.directPort', enabled, { shouldDirty: true }); form.setValue('configuration.database.directPortNumber', 0, { shouldDirty: true }); form.setValue('configuration.network.directDatabasePort', 0, { shouldDirty: true }) }
  if (name === 'auth') {
    const auth = form.getValues('configuration.auth')
    form.setValue('configuration.auth', enabled ? { ...auth, enabled } : {
      ...auth,
      enabled: false,
      anonymousSignIn: false,
      oauth: {},
      phone: { ...auth.phone, enabled: false, provider: '', secretSet: false, secret: { action: '' }, fields: {} },
      smtp: { enabled: false, host: '', port: 587, username: '', passwordSet: false, password: { action: '' }, senderEmail: '', senderName: '' },
    }, { shouldDirty: true })
  }
  if (name === 'imgproxy' && enabled) next.storage = true
  form.setValue('preset', 'CUSTOM', { shouldDirty: true }); form.setValue('configuration.services', next, { shouldDirty: true, shouldValidate: true })
}
