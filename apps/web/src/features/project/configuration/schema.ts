import { z } from 'zod'
import { projectConfigurationSchema, SUPABASE_VERSION } from '../../projects/projectSchema'

const secretAction = z.discriminatedUnion('action', [
  z.object({ action: z.literal(''), value: z.never().optional() }),
  z.object({ action: z.literal('retain'), value: z.never().optional() }),
  z.object({ action: z.literal('remove'), value: z.never().optional() }),
  z.object({ action: z.literal('replace'), value: z.string().trim().min(1, 'A replacement value is required') }),
])
const requiredURL = z.string().url('Enter an http or https URL').refine((value) => /^https?:\/\//i.test(value), 'Enter an http or https URL')

export const generalSchema = z.object({ domain: z.string().min(1, 'Domain is required'), siteUrl: requiredURL, supabaseVersion: z.literal(SUPABASE_VERSION) })
export const servicesSchema = projectConfigurationSchema.shape.services
const emailAuthSchema = z.object({ enabled: z.boolean(), allowSignup: z.boolean(), confirmEmail: z.boolean(), secureEmailChange: z.boolean(), doubleConfirmChanges: z.boolean() })
const phoneUpdateSchema = z.object({ enabled: z.boolean(), provider: z.string(), secretSet: z.boolean(), secret: secretAction, fields: z.record(z.string(), z.string()) })
const providerUpdateSchema = z.object({ enabled: z.boolean(), clientId: z.string(), secretSet: z.boolean(), secret: secretAction, fields: z.record(z.string(), z.string()) })
const smtpUpdateSchema = z.object({ enabled: z.boolean(), host: z.string(), port: z.number().int().min(0).max(65535), username: z.string(), passwordSet: z.boolean(), password: secretAction, senderEmail: z.string(), senderName: z.string() })
export const authSchema = z.object({ enabled: z.boolean(), jwtExpiry: z.number().int().min(0).max(31536000), disableSignup: z.boolean(), email: emailAuthSchema, phone: phoneUpdateSchema, anonymousSignIn: z.boolean(), redirectUrls: z.array(requiredURL), oauth: z.record(z.string(), providerUpdateSchema), smtp: smtpUpdateSchema }).superRefine((value, context) => { if (value.disableSignup !== !value.email.allowSignup) context.addIssue({ code: 'custom', path: ['disableSignup'], message: 'Must match the email signup policy' }); if (value.email.secureEmailChange !== value.email.doubleConfirmChanges) context.addIssue({ code: 'custom', path: ['email', 'secureEmailChange'], message: 'Secure and double confirmation must match' }) })
export const smtpSchema = smtpUpdateSchema
export function oauthProviderSchema(provider: string) {
  void provider
  return z.object({ enabled: z.boolean(), clientId: z.string(), secretSet: z.boolean(), secret: secretAction, fields: z.record(z.string(), z.string()) }).superRefine((value, context) => {
    if (!value.enabled) return
    if (!value.clientId.trim()) context.addIssue({ code: 'custom', path: ['clientId'], message: 'Client ID is required when enabled' })
    if (!value.secretSet && value.secret.action !== 'replace') context.addIssue({ code: 'custom', path: ['secret'], message: 'Enter a client secret' })
  })
}
export const storageSchema = z.object({ backend: z.enum(['local', 's3', 'aws-s3', 'r2']), s3CompatibleApi: z.boolean(), bucket: z.string(), region: z.string(), endpoint: z.string(), accountId: z.string(), accessKeyId: z.string(), secretAccessKeySet: z.boolean(), secretAccessKey: secretAction, forcePathStyle: z.boolean(), localPath: z.string() }).superRefine((value, context) => { if (value.backend === 'local') { if (value.bucket || value.region || value.endpoint || value.accountId || value.accessKeyId || value.forcePathStyle || (value.secretAccessKeySet && value.secretAccessKey.action !== 'remove')) context.addIssue({ code: 'custom', path: ['backend'], message: 'Local storage cannot include object-storage settings' }); return }; if (!value.bucket.trim()) context.addIssue({ code: 'custom', path: ['bucket'], message: 'Bucket is required' }); if (value.backend !== 'r2' && !value.region.trim()) context.addIssue({ code: 'custom', path: ['region'], message: 'Region is required' }); if (!value.accessKeyId.trim()) context.addIssue({ code: 'custom', path: ['accessKeyId'], message: 'Access key ID is required' }); if (!value.secretAccessKeySet && value.secretAccessKey.action !== 'replace') context.addIssue({ code: 'custom', path: ['secretAccessKey'], message: 'Enter or retain a secret access key' }); if (value.backend === 's3' && !value.endpoint.trim()) context.addIssue({ code: 'custom', path: ['endpoint'], message: 'Endpoint is required for generic S3' }); if (value.backend === 'r2' && !value.accountId.trim()) context.addIssue({ code: 'custom', path: ['accountId'], message: 'Account ID is required for R2' }); if (value.backend === 'r2' && value.endpoint) context.addIssue({ code: 'custom', path: ['endpoint'], message: 'R2 endpoint is derived from account ID' }) })
export const realtimeSchema = projectConfigurationSchema.shape.realtime
export const functionsSchema = z.object({ defaultJwtVerification: z.boolean(), directory: z.string(), variables: z.array(z.object({ name: z.string(), valueSet: z.boolean(), value: secretAction })) })
export const databaseSchema = projectConfigurationSchema.shape.database
export const poolerSchema = projectConfigurationSchema.shape.pooler
export const networkSchema = projectConfigurationSchema.shape.network
export { secretAction }
