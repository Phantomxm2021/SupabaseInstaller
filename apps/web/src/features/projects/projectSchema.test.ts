import { applyPreset, defaultConfiguration, normalizeCreateConfiguration, projectConfigurationSchema, projectSchema } from './projectSchema'

function validProject() {
  return { name: 'Bee', slug: 'bee', preset: 'LIGHTWEIGHT' as const, configuration: { ...defaultConfiguration('LIGHTWEIGHT'), general: { domain: 'bee.example.com', siteUrl: 'https://example.com', supabaseVersion: 'self-hosted/v0.8.0' } } }
}

it.each(['LIGHTWEIGHT', 'STANDARD', 'FULL', 'CUSTOM'] as const)('default %s configuration matches create validation', (preset) => {
  const project = validProject()
  project.preset = preset as typeof project.preset
  project.configuration = { ...defaultConfiguration(preset), general: project.configuration.general }
  expect(projectSchema.safeParse(project).success).toBe(true)
  expect(project.configuration.auth.enabled).toBe(project.configuration.services.auth)
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
  expect(normalized.functions.variables[0].value).toEqual({ action: 'remove' })
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

it.each(['', 'replace', 'remove'] as const)('accepts create secret action %s', (action) => {
  const configuration = validProject().configuration as any
  configuration.auth.smtp.password = action === 'replace' ? { action, value: 'secret' } : { action }
  expect(projectConfigurationSchema.safeParse(configuration).success).toBe(true)
})
