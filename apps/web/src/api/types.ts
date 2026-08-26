export type HealthStatus = 'HEALTHY' | 'DEGRADED' | 'STARTING' | 'STOPPED' | 'UNHEALTHY' | 'UNKNOWN'
export type ProjectStatus = 'DRAFT' | 'INSTALLING' | 'RUNNING' | 'STOPPED' | 'DEGRADED' | 'FAILED' | 'DELETING'

export interface Services {
  database: boolean
  gateway: boolean
  auth: boolean
  rest: boolean
  studio: boolean
  postgresMeta: boolean
  realtime: boolean
  storage: boolean
  imgproxy: boolean
  functions: boolean
  supavisor: boolean
  logs: boolean
  vector: boolean
  directDb: boolean
}

export type Preset = 'LIGHTWEIGHT' | 'STANDARD' | 'FULL' | 'CUSTOM'
export type SecretAction = '' | 'retain' | 'replace' | 'remove'
/** A secret mutation is always explicit on create/update requests. */
export type RedactedSecretInput = { action: ''; value?: never }
export type CreateSecretInput = { action: '' | 'remove'; value?: never } | { action: 'replace'; value: string }
export type UpdateSecretInput = { action: 'retain' | 'remove'; value?: never } | { action: 'replace'; value: string }
export type SecretInput = RedactedSecretInput | CreateSecretInput | UpdateSecretInput
export interface GeneralConfig { domain: string; siteUrl: string; supabaseVersion: string }
export interface EmailAuthConfig { enabled: boolean; allowSignup: boolean; confirmEmail: boolean; secureEmailChange: boolean; doubleConfirmChanges: boolean }
export interface PhoneAuthConfig { enabled: boolean; provider: string; secretSet: boolean; secret: SecretInput; fields: Record<string, string> }
export interface SMTPConfig { enabled: boolean; host: string; port: number; username: string; passwordSet: boolean; password: SecretInput; senderEmail: string; senderName: string }
export interface OAuthProviderConfig { enabled: boolean; clientId: string; secretSet: boolean; secret: SecretInput; fields: Record<string, string> }
export interface AuthConfig { enabled: boolean; jwtExpiry: number; disableSignup: boolean; email: EmailAuthConfig; phone: PhoneAuthConfig; anonymousSignIn: boolean; redirectUrls: string[]; oauth: Record<string, OAuthProviderConfig>; smtp: SMTPConfig }
export type StorageBackend = 'local' | 's3' | 'aws-s3' | 'r2'
export interface StorageConfig { backend: StorageBackend; s3CompatibleApi: boolean; bucket: string; region: string; endpoint: string; accountId: string; accessKeyId: string; secretAccessKeySet: boolean; secretAccessKey: SecretInput; forcePathStyle: boolean; localPath: string }
export interface RealtimeConfig { maxConnections: number; databasePoolSize: number; logLevel: 'debug' | 'info' | 'warn' | 'error' }
export interface FunctionVariable { name: string; valueSet: boolean; value: SecretInput }
export interface FunctionsConfig { defaultJwtVerification: boolean; directory: string; variables: FunctionVariable[] }
export interface DatabaseConfig { version: string; directPort: boolean; directPortNumber: number; maxConnections: number; sharedBuffers: string; extensions: string[] }
export interface PoolerConfig { transactionPort: number; sessionPort: number; poolSize: number; maxClientConnections: number }
export interface NetworkConfig { gateway: 'envoy' | 'kong'; httpsMode: 'external' | 'caddy' | 'manual'; internalGatewayPort?: number; apiPort: number; studioPort: number; directDatabasePort: number; poolerPort: number; certificate?: string }
/** Redacted aggregate returned by GET endpoints (never contains secret values). */
export interface RedactedPhoneAuthConfig { enabled: boolean; provider?: string; secretSet: boolean; secret: RedactedSecretInput; fields?: Record<string, string> }
export interface RedactedSMTPConfig { enabled: boolean; host: string; port: number; username: string; passwordSet: boolean; password: RedactedSecretInput; senderEmail: string; senderName: string }
export interface RedactedOAuthProviderConfig { enabled: boolean; clientId: string; secretSet: boolean; secret: RedactedSecretInput; fields?: Record<string, string> }
export interface RedactedAuthConfig { enabled: boolean; jwtExpiry: number; disableSignup: boolean; email: EmailAuthConfig; phone: RedactedPhoneAuthConfig; anonymousSignIn: boolean; redirectUrls?: string[]; oauth?: Record<string, RedactedOAuthProviderConfig>; smtp: RedactedSMTPConfig }
export interface RedactedStorageConfig { backend: StorageBackend; s3CompatibleApi: boolean; bucket: string; region: string; endpoint: string; accountId: string; accessKeyId: string; secretAccessKeySet: boolean; secretAccessKey: RedactedSecretInput; forcePathStyle: boolean; localPath: string }
export interface RedactedFunctionVariable { name: string; valueSet: boolean; value: RedactedSecretInput }
export interface RedactedFunctionsConfig { defaultJwtVerification: boolean; directory: string; variables?: RedactedFunctionVariable[] }
export interface RedactedDatabaseConfig { version: string; directPort: boolean; directPortNumber: number; maxConnections: number; sharedBuffers: string; extensions?: string[] }
export interface RedactedNetworkConfig { gateway: 'envoy' | 'kong'; httpsMode: 'external' | 'caddy' | 'manual'; internalGatewayPort?: number; apiPort: number; studioPort: number; directDatabasePort: number; poolerPort: number; certificate?: string }
export interface RedactedProjectConfiguration { revision: number; general: GeneralConfig; services: Services; auth: RedactedAuthConfig; storage: RedactedStorageConfig; realtime: RealtimeConfig; functions: RedactedFunctionsConfig; database: RedactedDatabaseConfig; pooler: PoolerConfig; network: RedactedNetworkConfig }
/** Full editable aggregate used by the wizard before it is converted to a wire DTO. */
export interface ProjectConfiguration { revision: number; general: GeneralConfig; services: Services; auth: AuthConfig; storage: StorageConfig; realtime: RealtimeConfig; functions: FunctionsConfig; database: DatabaseConfig; pooler: PoolerConfig; network: NetworkConfig }
type WithSecret<T, S> = T extends SecretInput ? S : T extends Array<infer U> ? Array<WithSecret<U, S>> : T extends object ? { [K in keyof T]: WithSecret<T[K], S> } : T
/** Create DTO uses only empty/remove/replace actions; retain is update-only. */
export type CreateProjectConfiguration = WithSecret<ProjectConfiguration, CreateSecretInput>
export type UpdateProjectConfiguration = WithSecret<ProjectConfiguration, UpdateSecretInput>
export interface CreateProjectRequest { name: string; slug: string; supabaseVersion: string; preset: Preset; configuration: CreateProjectConfiguration }
export interface ProjectDraft extends CreateProjectRequest {}

export interface Project {
  id: string
  name: string
  slug: string
  domain: string
  siteUrl: string
  status: ProjectStatus
  health: HealthStatus
  supabaseVersion: string
  preset: Preset
  configurationRevision: number
  services: Services
  createdAt: string
  updatedAt: string
}

export interface Operation {
  id: string
  projectId: string
  type: string
  status: 'QUEUED' | 'RUNNING' | 'SUCCEEDED' | 'FAILED' | 'ROLLING_BACK' | 'ROLLED_BACK' | 'CANCELLED'
  currentStep?: string
  progress: number
  errorCode?: string
  errorMessage?: string
}
