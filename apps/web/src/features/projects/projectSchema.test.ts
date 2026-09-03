import { applyPreset, defaultConfiguration, normalizeCreateConfiguration, projectConfigurationSchema, projectSchema, redactedSecretSchema, updateSecretSchema, type ProjectConfigurationForm } from './projectSchema'
import { toUpdateSecretInput } from '../../api/types'

it('defaults a new project to official JWT expiry', () => {
  expect(defaultConfiguration('LIGHTWEIGHT').auth.jwtExpiry).toBe(3600)
})

it.each([0, 604801])('rejects out-of-contract JWT expiry %s', (jwtExpiry) => {
  const project = validProject()
  project.configuration.auth.jwtExpiry = jwtExpiry
  expect(projectConfigurationSchema.safeParse(project.configuration).success).toBe(false)
})

it('accepts auth site URL and validates shared buffers and internal pool size', () => {
  const project = validProject()
  project.configuration.general.authSiteUrl = 'https://app.example.com'
  project.configuration.database.sharedBuffers = '256MB'
  project.configuration.pooler.internalDbPoolSize = 5
  expect(projectSchema.safeParse(project).success).toBe(true)
  project.configuration.database.sharedBuffers = 'not-a-memory-size'
  expect(projectSchema.safeParse(project).success).toBe(false)
})

function validProject() {
  return { name: 'Bee', slug: 'bee', preset: 'LIGHTWEIGHT' as const, configuration: { ...defaultConfiguration('LIGHTWEIGHT'), general: { domain: 'bee.example.com', siteUrl: 'https://example.com', supabaseVersion: 'self-hosted/v0.8.0' } } }
}

it.each(['LIGHTWEIGHT', 'STANDARD', 'FULL', 'CUSTOM'] as const)('default %s configuration matches create validation', (preset) => {
  const project = validProject()
  project.preset = preset as typeof project.preset
  project.configuration = { ...defaultConfiguration(preset), general: project.configuration.general }
  expect(projectSchema.safeParse(project).success).toBe(true)
  expect(project.configuration.auth.enabled).toBe(project.configuration.services.auth)
  expect(project.configuration.pooler.transactionPort).toBe(0)
  expect(project.configuration.pooler.sessionPort).toBe(0)
})

it('applies the complete service closure for presets', () => {
  expect(applyPreset('STANDARD')).toMatchObject({ database: true, gateway: true, storage: true, functions: true, supavisor: true })
  expect(applyPreset('FULL')).toMatchObject({ imgproxy: true, logs: true, vector: true })
  expect(applyPreset('CUSTOM')).toMatchObject({ database: true, gateway: true, auth: true })
})

it('normalizes update-only retain markers and never keeps plaintext for non-replace actions', () => {
  const configuration = validProject().configuration as any
  configuration.auth.smtp.password = { action: 'retain', value: 'must-not-cross-boundary' }
  configuration.functions.variables = [{ name: 'FOO', valueSet: false, value: { action: 'remove', value: 'discard-me' } }]
  const normalized = normalizeCreateConfiguration(configuration) as any
  expect(normalized.auth.smtp.password).toEqual({ action: '' })
  expect(normalized.functions.variables[0].value).toEqual({ action: '' })
})

it('rejects Go-invalid domain, renderer fields, duplicate ports and service drift', () => {
  const configuration = validProject().configuration as any
  configuration.general.domain = 'single-label'
  configuration.database.extensions = ['postgis']
  configuration.network.internalGatewayPort = 1234
  configuration.network.apiPort = 8080
  configuration.network.studioPort = 8080
  configuration.services.auth = false
  expect(projectSchema.safeParse({ ...validProject(), configuration }).success).toBe(false)
})

it('accepts IPv6/localhost parity and rejects Caddy without its gateway dependency', () => {
  const project = validProject()
  project.configuration.general.domain = '[::1]:8080'
  expect(projectSchema.safeParse(project).success).toBe(true)
  project.configuration.general.domain = '999.999.999.999'
  expect(projectSchema.safeParse(project).success).toBe(true)
  const caddy = validProject().configuration as any
  caddy.network.httpsMode = 'caddy'
  caddy.services.gateway = false
  caddy.services.auth = false
  caddy.auth.enabled = false
  caddy.services.rest = false
  caddy.services.studio = false
  expect(projectConfigurationSchema.safeParse(caddy).success).toBe(false)
  const supportedCaddy = validProject().configuration as any
  supportedCaddy.network.httpsMode = 'caddy'
  expect(projectConfigurationSchema.safeParse(supportedCaddy).success).toBe(false)
})

it('does not require hidden phone fields while disabled, but validates Twilio fields when enabled', () => {
  const disabled = validProject().configuration as any
  disabled.auth.phone = { enabled: false, provider: 'twilio', secretSet: false, secret: { action: '' }, fields: {} }
  expect(projectConfigurationSchema.safeParse(disabled).success).toBe(true)
  const enabled = { ...disabled, auth: { ...disabled.auth, phone: { ...disabled.auth.phone, enabled: true } } }
  expect(projectConfigurationSchema.safeParse(enabled).success).toBe(false)
  enabled.auth.phone.fields = { accountSid: 'AC', messageServiceSid: 'MG', verifySid: 'VE' }
  enabled.auth.phone.secret = { action: 'replace', value: 'secret' }
  expect(projectConfigurationSchema.safeParse(enabled).success).toBe(true)
})

it.each(['', 'replace'] as const)('accepts create secret action %s', (action) => {
  const configuration = validProject().configuration as any
  configuration.auth.smtp.password = action === 'replace' ? { action, value: 'secret' } : { action }
  expect(projectConfigurationSchema.safeParse(configuration).success).toBe(true)
})

it('rejects update-only remove action in create configuration', () => {
  const configuration = validProject().configuration as any
  configuration.auth.smtp.password = { action: 'remove' }
  expect(projectConfigurationSchema.safeParse(configuration).success).toBe(false)
})

it('accepts canonical unset marker in update secret schema', () => {
  expect(updateSecretSchema.safeParse({ action: '' }).success).toBe(true)
})

it('does not expose unsupported manual TLS fields in defaults or create payload', () => {
  const configuration = validProject().configuration as ProjectConfigurationForm
  expect('certificate' in configuration.network).toBe(false)
  expect('manual').not.toBe(configuration.network.httpsMode)
  expect(JSON.stringify(normalizeCreateConfiguration(configuration))).not.toContain('certificate')
})

it('requires a lowercase 32-character hexadecimal R2 account ID', () => {
  const configuration = validProject().configuration as any
  configuration.services.storage = true
  configuration.storage = { ...configuration.storage, backend: 'r2', bucket: 'objects', accessKeyId: 'key', secretAccessKey: { action: 'replace', value: 'secret' } }
  configuration.storage.accountId = 'ABCDEF0123456789ABCDEF0123456789'
  expect(projectConfigurationSchema.safeParse(configuration).success).toBe(false)
  configuration.storage.accountId = 'abcdef0123456789abcdef0123456789'
  configuration.storage.forcePathStyle = true
  expect(projectConfigurationSchema.safeParse(configuration).success).toBe(true)
})

it('carries upload size limits as bytes and rejects values outside 1–5120 MiB', () => {
  const configuration = validProject().configuration as any
  expect(configuration.storage.uploadFileSizeLimit).toBe(50 * 1024 * 1024)
  configuration.storage.uploadFileSizeLimit = 5120 * 1024 * 1024
  expect(projectConfigurationSchema.safeParse(configuration).success).toBe(true)
  configuration.storage.uploadFileSizeLimit = 5120 * 1024 * 1024 + 1
  expect(projectConfigurationSchema.safeParse(configuration).success).toBe(false)
})

it('does not include the removed Functions directory in defaults or payloads', () => {
  const configuration = validProject().configuration as any
  expect('directory' in configuration.functions).toBe(false)
  expect(JSON.stringify(normalizeCreateConfiguration(configuration))).not.toContain('directory')
})

it('keeps redacted and update secret truth tables distinct', () => {
  expect(redactedSecretSchema.safeParse({ action: '' }).success).toBe(true)
  expect(redactedSecretSchema.safeParse({ action: 'retain' }).success).toBe(false)
  expect(updateSecretSchema.safeParse({ action: 'retain' }).success).toBe(true)
  expect(updateSecretSchema.safeParse({ action: 'remove', value: 'leak' }).success).toBe(false)
  expect(updateSecretSchema.safeParse({ action: 'replace' }).success).toBe(false)
  expect(updateSecretSchema.safeParse({ action: 'replace', value: 'secret' }).success).toBe(true)
  expect(toUpdateSecretInput({ action: '' }, true)).toEqual({ action: 'retain' })
  expect(toUpdateSecretInput({ action: '' }, false)).toEqual({ action: '' })
  expect(toUpdateSecretInput({ action: '' }, true, 'remove')).toEqual({ action: 'remove' })
})

it.each([
  ['SMTP', (configuration: any) => { configuration.auth.smtp = { ...configuration.auth.smtp, enabled: true, host: 'smtp.example.com', username: 'bee', senderEmail: 'bee@example.com', senderName: 'Bee', password: { action: 'replace', value: ' ' } } }, ['auth', 'smtp', 'password', 'value']],
  ['OAuth', (configuration: any) => { configuration.auth.oauth = { google: { enabled: true, clientId: 'client', secretSet: false, secret: { action: 'replace', value: ' ' }, fields: {} } } }, ['auth', 'oauth', 'google', 'secret', 'value']],
  ['Storage', (configuration: any) => { configuration.services.storage = true; configuration.storage = { ...configuration.storage, backend: 's3', bucket: 'bee', region: 'us-east-1', endpoint: 'https://s3.example.com', accessKeyId: 'access', secretAccessKey: { action: 'replace', value: ' ' } } }, ['storage', 'secretAccessKey', 'value']],
] as const)('reports %s whitespace secret at its nested value path', (_label, mutate, path) => {
  const configuration = validProject().configuration as any
  mutate(configuration)
  const result = projectConfigurationSchema.safeParse(configuration)
  expect(result.success).toBe(false)
  expect(result.success ? [] : result.error.issues.map((issue) => issue.path)).toContainEqual(path)
})

it('rejects SMTP display-name addresses like Manager validation', () => {
  const configuration = validProject().configuration as any
  configuration.auth.smtp = { ...configuration.auth.smtp, enabled: true, host: 'smtp.example.com', username: 'bee', senderEmail: 'Bee <bee@example.com>', senderName: 'Bee', password: { action: 'replace', value: 'secret' } }
  expect(projectConfigurationSchema.safeParse(configuration).success).toBe(false)
})
