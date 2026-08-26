import type { RedactedProjectConfiguration } from './wire'

export const CONFIGURATION_SECTIONS = ['general', 'services', 'auth', 'smtp', 'oauth', 'storage', 'realtime', 'functions', 'database', 'pooler', 'network', 'secrets'] as const
export type ConfigurationSection = typeof CONFIGURATION_SECTIONS[number]

export type ConfigurationSnapshot = {
  projectId: string
  revision: number
  lastGoodRevision: number
  configuration: RedactedProjectConfiguration
}

export type PendingConfigurationSave = {
  section: ConfigurationSection
  provider?: string
  value: unknown
  labels: string[]
  services: string[]
  impact: 'none' | 'restart' | 'recreate' | 'start' | 'stop'
  setError?: (name: string, message: string) => void
}

export const SECTION_LABELS: Record<ConfigurationSection, string> = {
  general: 'General', services: 'Services', auth: 'Authentication', smtp: 'Email & SMTP', oauth: 'OAuth Providers',
  storage: 'Storage', realtime: 'Realtime', functions: 'Functions', database: 'Database', pooler: 'Connection Pooler',
  network: 'Gateway & Network', secrets: 'API & Secrets',
}

export function sectionLabel(section: ConfigurationSection) { return SECTION_LABELS[section] }

/** Go omitempty fields are intentionally optional on redacted responses. Keep
 * every section form on a total DTO before it calls watch/join/index methods. */
export function normalizeRedactedConfiguration(config: RedactedProjectConfiguration): RedactedProjectConfiguration {
  const oauth = Object.fromEntries(Object.entries(config.auth.oauth ?? {}).map(([provider, value]) => [provider, { ...value, fields: value.fields ?? {} }]))
  return {
    ...config,
    auth: { ...config.auth, redirectUrls: config.auth.redirectUrls ?? [], oauth, phone: { ...config.auth.phone, provider: config.auth.phone.provider ?? '', fields: config.auth.phone.fields ?? {} }, smtp: { ...config.auth.smtp } },
    database: { ...config.database, extensions: config.database.extensions ?? [] },
    functions: { ...config.functions, variables: config.functions.variables ?? [] },
  }
}

export function sectionEndpoint(section: ConfigurationSection, provider?: string) {
  return provider && section === 'oauth' ? `oauth/${encodeURIComponent(provider)}` : section
}

export function sectionImpact(section: ConfigurationSection, value: unknown): PendingConfigurationSave['impact'] {
  if (section === 'general' || section === 'network' || section === 'services' || section === 'database' || section === 'pooler') return 'recreate'
  if (section === 'smtp' || section === 'oauth' || section === 'auth' || section === 'storage' || section === 'realtime' || section === 'functions') return 'recreate'
  return 'none'
}

export function affectedServices(section: ConfigurationSection, value: unknown): string[] {
  if (section === 'services' && value && typeof value === 'object') {
    const names: Record<string, string> = { auth: 'Auth', rest: 'PostgREST', studio: 'Studio', postgresMeta: 'postgres-meta', realtime: 'Realtime', storage: 'Storage', imgproxy: 'imgproxy', functions: 'Edge Functions', supavisor: 'Supavisor', logs: 'Logflare', vector: 'Vector', directDb: 'PostgreSQL' }
    return Object.entries(value).filter(([, current]) => typeof current === 'boolean').map(([name]) => names[name] ?? name)
  }
  const defaults: Record<ConfigurationSection, string[]> = { general: ['Gateway', 'Auth', 'Studio'], services: [], auth: ['Auth'], smtp: ['Auth'], oauth: ['Auth'], storage: ['Storage'], realtime: ['Realtime'], functions: ['Edge Functions'], database: ['PostgreSQL'], pooler: ['Supavisor'], network: ['Gateway'], secrets: [] }
  return defaults[section]
}

export function dirtyLabels(value: unknown, path: string[] = []): string[] {
  if (value === true) return path.length ? [path.join('.')] : []
  if (!value || typeof value !== 'object') return []
  return Object.entries(value).flatMap(([key, child]) => dirtyLabels(child, [...path, key]))
}
