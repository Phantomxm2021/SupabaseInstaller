import type { RedactedProjectConfiguration } from './wire'
import type { Services } from '../../../api/types'

export const CONFIGURATION_SECTIONS = ['general', 'services', 'storage', 'realtime', 'functions', 'database', 'pooler', 'network', 'secrets'] as const
export type ConfigurationSection = typeof CONFIGURATION_SECTIONS[number]

/** Hardened public controls shared by the general and pooler forms. */
export type HardenedConfigurationControls = {
  authSiteUrl: string
  internalDbPoolSize: number
}

export type ConfigurationSnapshot = {
  projectId: string
  revision: number
  lastGoodRevision: number
  configuration: RedactedProjectConfiguration
}

type AuthenticationConfigurationSection = 'auth' | 'smtp' | 'oauth'

export type PendingConfigurationSave = {
  section: ConfigurationSection | AuthenticationConfigurationSection
  provider?: string
  value: unknown
  labels: string[]
  services: string[]
  impact: 'none' | 'restart' | 'recreate' | 'start' | 'stop'
  setError?: (name: string, message: string) => void
}

export const SECTION_LABELS: Record<ConfigurationSection | AuthenticationConfigurationSection, string> = {
  general: 'General', services: 'Services', auth: 'Authentication', smtp: 'Email & SMTP', oauth: 'OAuth Providers',
  storage: 'Storage', realtime: 'Realtime', functions: 'Functions', database: 'Database', pooler: 'Connection Pooler',
  network: 'Gateway & Network', secrets: 'API & Secrets',
}

export function sectionLabel(section: PendingConfigurationSave['section']) { return SECTION_LABELS[section] }

/** Go omitempty fields are intentionally optional on redacted responses. Keep
 * every section form on a total DTO before it calls watch/join/index methods. */
export function normalizeRedactedConfiguration(config: RedactedProjectConfiguration): RedactedProjectConfiguration {
  const oauth = Object.fromEntries(Object.entries(config.auth.oauth ?? {}).map(([provider, value]) => [provider, { ...value, fields: value.fields ?? {} }]))
  return {
    ...config,
    general: { ...config.general, authSiteUrl: config.general.authSiteUrl ?? '' },
    auth: { ...config.auth, redirectUrls: config.auth.redirectUrls ?? [], oauth, phone: { ...config.auth.phone, provider: config.auth.phone.provider ?? '', fields: config.auth.phone.fields ?? {} }, smtp: { ...config.auth.smtp } },
    database: { ...config.database, extensions: config.database.extensions ?? [] },
    pooler: { ...config.pooler, internalDbPoolSize: config.pooler.internalDbPoolSize || 5 },
    functions: { ...config.functions, variables: config.functions.variables ?? [] },
  }
}

export function sectionEndpoint(section: PendingConfigurationSave['section'], provider?: string) {
  return provider && section === 'oauth' ? `oauth/${encodeURIComponent(provider)}` : section
}

export function sectionImpact(section: PendingConfigurationSave['section'], value: unknown, services?: Services, previous?: unknown): PendingConfigurationSave['impact'] {
  if ((section === 'smtp' || section === 'oauth' || section === 'auth') && value && typeof value === 'object' && 'enabled' in value && (value as { enabled?: unknown }).enabled === false) {
    return previous && typeof previous === 'object' && (previous as { enabled?: unknown }).enabled === true ? 'recreate' : 'none'
  }
  if (services && ['auth', 'smtp', 'oauth', 'storage', 'realtime', 'functions'].includes(section)) {
    const owner = section === 'smtp' || section === 'oauth' || section === 'auth' ? 'auth' : section
    if (!(services as unknown as Record<string, boolean>)[owner]) return 'none'
  }
  if (section === 'general' || section === 'network' || section === 'pooler') {
    const owner = section === 'general' ? ['gateway', 'auth', 'studio'] : section === 'network' ? ['gateway'] : ['supavisor']
    if (services && !owner.some((name) => Boolean((services as unknown as Record<string, boolean>)[name]))) return 'none'
    return 'recreate'
  }
  if (section === 'services' || section === 'database') return 'recreate'
  if (section === 'smtp' || section === 'oauth' || section === 'auth' || section === 'storage' || section === 'realtime' || section === 'functions') return 'recreate'
  return 'none'
}

export function affectedServices(section: PendingConfigurationSave['section'], dirty: unknown, value?: unknown, services?: Services): string[] {
  if (section === 'services' && value && typeof value === 'object') {
    const names: Record<string, string> = { auth: 'Auth', rest: 'PostgREST', studio: 'Studio', postgresMeta: 'postgres-meta', realtime: 'Realtime', storage: 'Storage', imgproxy: 'imgproxy', functions: 'Edge Functions', supavisor: 'Supavisor', logs: 'Logflare', vector: 'Vector', directDb: 'PostgreSQL' }
    const before = (services ?? {}) as unknown as Record<string, boolean>
    return Object.entries(value).filter(([name, current]) => typeof current === 'boolean' && before[name] !== current).map(([name]) => names[name] ?? name)
  }
  const defaults: Record<PendingConfigurationSave['section'], string[]> = { general: ['Gateway', 'Auth', 'Studio'], services: [], auth: ['Auth'], smtp: ['Auth'], oauth: ['Auth'], storage: ['Storage'], realtime: ['Realtime'], functions: ['Edge Functions'], database: ['PostgreSQL'], pooler: ['Supavisor'], network: ['Gateway'], secrets: [] }
  const result = defaults[section]
  if (!services) return result
  return result.filter((name) => {
    const owner: Record<string, string> = { Auth: 'auth', Storage: 'storage', Realtime: 'realtime', 'Edge Functions': 'functions', PostgreSQL: 'database', Supavisor: 'supavisor', Gateway: 'gateway' }
    return (services as unknown as Record<string, boolean>)[owner[name] ?? name.toLowerCase()]
  })
}

export function dirtyLabels(value: unknown, path: string[] = []): string[] {
  if (value === true) return path.length ? [path.join('.')] : []
  if (!value || typeof value !== 'object') return []
  return Object.entries(value).flatMap(([key, child]) => dirtyLabels(child, [...path, key]))
}
