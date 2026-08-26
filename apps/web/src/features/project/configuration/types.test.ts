import { describe, expect, it } from 'vitest'
import type { RedactedProjectConfiguration } from '../../../api/types'
import { normalizeRedactedConfiguration, sectionImpact, affectedServices } from './types'
import { normalizeConfigurationValue } from './useConfigurationMutation'
import { storageSchema, smtpSchema, oauthProviderSchema, authSchema, functionsSchema } from './schema'

function omittedConfiguration(): RedactedProjectConfiguration {
  return {
    revision: 7,
    general: { domain: 'bee.example.com', siteUrl: 'https://example.com', supabaseVersion: 'self-hosted/v0.8.0' },
    services: { database: true, gateway: true, auth: true, rest: true, studio: true, postgresMeta: true, realtime: false, storage: false, imgproxy: false, functions: false, supavisor: false, logs: false, vector: false, directDb: false },
    auth: {
      enabled: true, jwtExpiry: 3600, disableSignup: false,
      email: { enabled: true, allowSignup: true, confirmEmail: false, secureEmailChange: false, doubleConfirmChanges: false },
      phone: { enabled: false, secretSet: false, secret: { action: '' } },
      anonymousSignIn: false, smtp: { enabled: false, host: '', port: 0, username: '', passwordSet: false, password: { action: '' }, senderEmail: '', senderName: '' },
      oauth: { google: { enabled: false, clientId: '', secretSet: false, secret: { action: '' } } },
    },
    storage: { backend: 'local', s3CompatibleApi: false, bucket: '', region: '', endpoint: '', accountId: '', accessKeyId: '', secretAccessKeySet: false, secretAccessKey: { action: '' }, forcePathStyle: false, localPath: '/data' },
    realtime: { maxConnections: 100, databasePoolSize: 5, logLevel: 'info' },
    functions: { defaultJwtVerification: true, directory: '/functions' },
    database: { version: '17', directPort: false, directPortNumber: 0, maxConnections: 100, sharedBuffers: '128MB' },
    pooler: { transactionPort: 6543, sessionPort: 5432, poolSize: 20, maxClientConnections: 100 },
    network: { gateway: 'envoy', httpsMode: 'external', apiPort: 8000, studioPort: 3000, directDatabasePort: 5432, poolerPort: 6543 },
  }
}

describe('configuration projection normalization', () => {
  it('fills omitted phone provider and OAuth fields with schema-valid values', () => {
    const config = normalizeRedactedConfiguration(omittedConfiguration())
    expect(config.auth.phone.provider).toBe('')
    expect(config.auth.phone.fields).toEqual({})
    expect(config.auth.oauth?.google.fields).toEqual({})
    expect(config.functions.variables).toEqual([])
    expect(config.database.extensions).toEqual([])
  })

  it('labels rendered configuration changes as recreate', () => {
    expect(sectionImpact('smtp', {})).toBe('recreate')
    expect(sectionImpact('oauth', {})).toBe('recreate')
    expect(sectionImpact('auth', {})).toBe('recreate')
    expect(sectionImpact('storage', {})).toBe('recreate')
    expect(sectionImpact('realtime', {})).toBe('recreate')
    expect(sectionImpact('functions', {})).toBe('recreate')
  })

  it('calculates general changes including Auth, Studio, and gateway', () => {
    expect(affectedServices('general', { domain: 'new.example.com' })).toEqual(['Gateway', 'Auth', 'Studio'])
  })

  it('matches Manager truth tables for enabled configured secrets and endpoints', () => {
    const storage = { backend: 's3' as const, s3CompatibleApi: true, bucket: 'bucket', region: 'us-east-1', endpoint: 'not-a-url', accountId: '', accessKeyId: 'key', secretAccessKeySet: true, secretAccessKey: { action: 'remove' as const }, forcePathStyle: false, localPath: '' }
    expect(storageSchema.safeParse(storage).success).toBe(false)
    const smtp = { enabled: true, host: 'smtp.example.com', port: 587, username: 'u', passwordSet: true, password: { action: 'remove' as const }, senderEmail: 'a@example.com', senderName: 'A' }
    expect(smtpSchema.safeParse(smtp).success).toBe(false)
    const oauth = { enabled: true, clientId: 'client', secretSet: true, secret: { action: 'remove' as const }, fields: {} }
    expect(oauthProviderSchema('google').safeParse(oauth).success).toBe(false)
    const phone = { enabled: true, provider: 'twilio', secretSet: true, secret: { action: 'remove' as const }, fields: { accountSid: 'a', messageServiceSid: 'm' } }
    const auth = { enabled: true, jwtExpiry: 3600, disableSignup: false, email: { enabled: true, allowSignup: true, confirmEmail: false, secureEmailChange: false, doubleConfirmChanges: false }, phone, anonymousSignIn: false, redirectUrls: [], oauth: {}, smtp: { ...smtp, password: { action: 'retain' as const } } }
    expect(authSchema.safeParse(auth).success).toBe(false)
    expect(functionsSchema.safeParse({ defaultJwtVerification: true, directory: './functions', variables: [{ name: 'SUPABASE_URL', valueSet: false, value: { action: '' } }] }).success).toBe(false)
  })

  it('accepts redacted configured function values and sends retain at update boundary', () => {
    const initial = { defaultJwtVerification: true, directory: './functions', variables: [{ name: 'OPENAI_API_KEY', valueSet: true, value: { action: '' as const } }] }
    expect(functionsSchema.safeParse(initial).success).toBe(true)
    expect(normalizeConfigurationValue(initial)).toEqual({ defaultJwtVerification: true, directory: './functions', variables: [{ name: 'OPENAI_API_KEY', valueSet: true, value: { action: 'retain' } }] })
  })

  it('distinguishes service disable from removing a secret on an already disabled section', () => {
    const services = { auth: true } as import('../../../api/types').Services
    expect(sectionImpact('smtp', { enabled: false }, services, { enabled: true })).toBe('recreate')
    expect(sectionImpact('smtp', { enabled: false }, services, { enabled: false })).toBe('none')
  })
})
