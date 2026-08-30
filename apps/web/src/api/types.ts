export type HealthStatus =
  "HEALTHY" | "DEGRADED" | "STARTING" | "STOPPED" | "UNHEALTHY" | "UNKNOWN";
export type ProjectStatus =
  | "DRAFT"
  | "INSTALLING"
  | "RUNNING"
  | "STOPPED"
  | "DEGRADED"
  | "FAILED"
  | "DELETING";

export interface Services {
  database: boolean;
  gateway: boolean;
  auth: boolean;
  rest: boolean;
  studio: boolean;
  postgresMeta: boolean;
  realtime: boolean;
  storage: boolean;
  imgproxy: boolean;
  functions: boolean;
  supavisor: boolean;
  logs: boolean;
  vector: boolean;
  directDb: boolean;
}

export type Preset = "LIGHTWEIGHT" | "STANDARD" | "FULL" | "CUSTOM";
export type SecretAction = "" | "retain" | "replace" | "remove";
/** Go's non-pointer secret wire shape always carries an action marker. */
export type UnsetSecretInput = { action: ""; value?: never };
export type RedactedSecretInput = UnsetSecretInput;
export type CreateSecretInput =
  UnsetSecretInput | { action: "replace"; value: string };
export type UpdateSecretInput =
  | UnsetSecretInput
  | { action: "retain" | "remove"; value?: never }
  | { action: "replace"; value: string };
export type SecretInput =
  RedactedSecretInput | CreateSecretInput | UpdateSecretInput;
/** Convert a redacted marker to an explicit update command at the client boundary. */
export function toUpdateSecretInput(
  input: RedactedSecretInput,
  configured: boolean,
  requested: "unchanged" | "remove" = "unchanged",
): UpdateSecretInput {
  if (input.action !== "")
    throw new Error("redacted secret markers must use an empty action");
  if (!configured) return { action: "" };
  return requested === "remove" ? { action: "remove" } : { action: "retain" };
}
export interface GeneralConfig {
  domain: string;
  siteUrl: string;
  supabaseVersion: string;
  studioUsername?: string;
  studioPasswordSet?: boolean;
  studioPassword?: SecretInput;
}
export interface EmailAuthConfig {
  enabled: boolean;
  allowSignup: boolean;
  confirmEmail: boolean;
  secureEmailChange: boolean;
  doubleConfirmChanges: boolean;
  securePasswordChange?: boolean;
  requireCurrentPassword?: boolean;
  preventLeakedPasswords?: boolean;
  minimumPasswordLength?: number;
  passwordRequirements?: string;
  emailOtpExpiration?: number;
  emailOtpLength?: number;
}
export interface PhoneAuthConfig {
  enabled: boolean;
  provider: string;
  secretSet: boolean;
  secret: SecretInput;
  fields: Record<string, string>;
}
export interface SMTPConfig {
  enabled: boolean;
  host: string;
  port: number;
  username: string;
  passwordSet: boolean;
  password: SecretInput;
  senderEmail: string;
  senderName: string;
}
export interface OAuthProviderConfig {
  enabled: boolean;
  clientId: string;
  secretSet: boolean;
  secret: SecretInput;
  fields: Record<string, string>;
}
export interface RateLimitConfig {
  emailSent: number;
  smsSent: number;
  tokenRefresh: number;
  tokenVerification: number;
  anonymousUsers: number;
  signupsAndSignins: number;
}
export interface MFAConfig {
  totpEnrollEnabled: boolean;
  totpVerifyEnabled: boolean;
  phoneEnrollEnabled: boolean;
  phoneVerifyEnabled: boolean;
  maxEnrolledFactors: number;
  phoneOtpLength: number;
}
export interface EmailTemplateConfig {
  subject: string;
  body: string;
}
export interface EmailNotificationConfig {
  enabled: boolean;
  template: EmailTemplateConfig;
}
export interface MailerConfig {
  templates: {
    confirmation: EmailTemplateConfig;
    invite: EmailTemplateConfig;
    magicLink: EmailTemplateConfig;
    emailChange: EmailTemplateConfig;
    recovery: EmailTemplateConfig;
    reauthentication: EmailTemplateConfig;
  };
  notifications: {
    passwordChanged: EmailNotificationConfig;
    emailChanged: EmailNotificationConfig;
    phoneChanged: EmailNotificationConfig;
    identityLinked: EmailNotificationConfig;
    identityUnlinked: EmailNotificationConfig;
    mfaFactorEnrolled: EmailNotificationConfig;
    mfaFactorUnenrolled: EmailNotificationConfig;
  };
}
export interface AuthConfig {
  enabled: boolean;
  jwtExpiry: number;
  disableSignup: boolean;
  email: EmailAuthConfig;
  phone: PhoneAuthConfig;
  anonymousSignIn: boolean;
  manualLinking?: boolean;
  redirectUrls: string[];
  oauth: Record<string, OAuthProviderConfig>;
  smtp: SMTPConfig;
  mailer: MailerConfig;
  rateLimits: RateLimitConfig;
  mfa: MFAConfig;
}
export interface AuthUser {
  id: string;
  email: string;
  phone: string;
  created_at: string;
  user_metadata: Record<string, unknown>;
  identities: { provider: string }[];
}
export interface OAuthApp {
  client_id: string;
  name: string;
  redirect_uris: string[];
  client_type: string;
  token_endpoint_auth_method: string;
  created_at: string;
}
export type StorageBackend = "local" | "s3" | "aws-s3" | "r2";
export interface StorageConfig {
  backend: StorageBackend;
  s3CompatibleApi: boolean;
  bucket: string;
  region: string;
  endpoint: string;
  accountId: string;
  accessKeyId: string;
  secretAccessKeySet: boolean;
  secretAccessKey: SecretInput;
  forcePathStyle: boolean;
  localPath: string;
}
export interface RealtimeConfig {
  maxConnections: number;
  databasePoolSize: number;
  logLevel: "debug" | "info" | "warn" | "error";
}
export interface FunctionVariable {
  name: string;
  valueSet: boolean;
  value: SecretInput;
}
export interface FunctionsConfig {
  defaultJwtVerification: boolean;
  directory: string;
  variables: FunctionVariable[];
}
export interface DatabaseConfig {
  version: string;
  directPort: boolean;
  directPortNumber: number;
  maxConnections: number;
  sharedBuffers: string;
  extensions: string[];
}
export interface PoolerConfig {
  transactionPort: number;
  sessionPort: number;
  poolSize: number;
  maxClientConnections: number;
}
export interface NetworkConfig {
  gateway: "envoy" | "kong";
  httpsMode: "external" | "caddy";
  managedTls?: {
    certificateName: string;
    certificateFile: string;
    privateKeyFile: string;
  };
  internalGatewayPort?: number;
  apiPort: number;
  studioPort: number;
  directDatabasePort: number;
  poolerPort: number;
}
/** Redacted aggregate returned by GET endpoints (never contains secret values). */
export interface RedactedPhoneAuthConfig {
  enabled: boolean;
  provider?: string;
  secretSet: boolean;
  secret: RedactedSecretInput;
  fields?: Record<string, string>;
}
export interface RedactedSMTPConfig {
  enabled: boolean;
  host: string;
  port: number;
  username: string;
  passwordSet: boolean;
  password: RedactedSecretInput;
  senderEmail: string;
  senderName: string;
}
export interface RedactedOAuthProviderConfig {
  enabled: boolean;
  clientId: string;
  secretSet: boolean;
  secret: RedactedSecretInput;
  fields?: Record<string, string>;
}
export interface RedactedAuthConfig {
  enabled: boolean;
  jwtExpiry: number;
  disableSignup: boolean;
  email: EmailAuthConfig;
  phone: RedactedPhoneAuthConfig;
  anonymousSignIn: boolean;
  manualLinking?: boolean;
  redirectUrls?: string[];
  oauth?: Record<string, RedactedOAuthProviderConfig>;
  smtp: RedactedSMTPConfig;
  mailer: MailerConfig;
  rateLimits: RateLimitConfig;
  mfa: MFAConfig;
}
export interface RedactedStorageConfig {
  backend: StorageBackend;
  s3CompatibleApi: boolean;
  bucket: string;
  region: string;
  endpoint: string;
  accountId: string;
  accessKeyId: string;
  secretAccessKeySet: boolean;
  secretAccessKey: RedactedSecretInput;
  forcePathStyle: boolean;
  localPath: string;
}
export interface RedactedFunctionVariable {
  name: string;
  valueSet: boolean;
  value: RedactedSecretInput;
}
export interface RedactedFunctionsConfig {
  defaultJwtVerification: boolean;
  directory: string;
  variables?: RedactedFunctionVariable[];
}
export interface RedactedDatabaseConfig {
  version: string;
  directPort: boolean;
  directPortNumber: number;
  maxConnections: number;
  sharedBuffers: string;
  extensions?: string[];
}
export interface RedactedNetworkConfig {
  gateway: "envoy" | "kong";
  httpsMode: "external" | "caddy";
  managedTls?: {
    certificateName: string;
    certificateFile: string;
    privateKeyFile: string;
  };
  internalGatewayPort?: number;
  apiPort: number;
  studioPort: number;
  directDatabasePort: number;
  poolerPort: number;
}
export interface RedactedProjectConfiguration {
  revision: number;
  general: GeneralConfig;
  services: Services;
  auth: RedactedAuthConfig;
  storage: RedactedStorageConfig;
  realtime: RealtimeConfig;
  functions: RedactedFunctionsConfig;
  database: RedactedDatabaseConfig;
  pooler: PoolerConfig;
  network: RedactedNetworkConfig;
}
/** Full editable aggregate used by the wizard before it is converted to a wire DTO. */
export interface ProjectConfiguration {
  revision: number;
  general: GeneralConfig;
  services: Services;
  auth: AuthConfig;
  storage: StorageConfig;
  realtime: RealtimeConfig;
  functions: FunctionsConfig;
  database: DatabaseConfig;
  pooler: PoolerConfig;
  network: NetworkConfig;
}
type WithSecret<T, S> = T extends SecretInput
  ? S
  : T extends Array<infer U>
    ? Array<WithSecret<U, S>>
    : T extends object
      ? { [K in keyof T]: WithSecret<T[K], S> }
      : T;
/** Create DTO uses only empty/replace actions; retain/remove are update-only. */
export type CreateProjectConfiguration = WithSecret<
  ProjectConfiguration,
  CreateSecretInput
>;
export type UpdateProjectConfiguration = WithSecret<
  ProjectConfiguration,
  UpdateSecretInput
>;
export interface CreateProjectRequest {
  name: string;
  slug: string;
  preset: Preset;
  configuration: CreateProjectConfiguration;
}
export interface ProjectDraft extends CreateProjectRequest {}

export interface Project {
  id: string;
  name: string;
  slug: string;
  domain: string;
  siteUrl: string;
  status: ProjectStatus;
  health: HealthStatus;
  supabaseVersion: string;
  preset: Preset;
  configurationRevision: number;
  services: Services;
  createdAt: string;
  updatedAt: string;
}

export interface HostResources {
  cpuPercent: number;
  cpuCores: number;
  memoryUsedBytes: number;
  memoryTotalBytes: number;
  diskUsedBytes: number;
  diskTotalBytes: number;
  collectedAt: string;
}

export interface Operation {
  id: string;
  projectId: string;
  type: string;
  status:
    | "QUEUED"
    | "RUNNING"
    | "SUCCEEDED"
    | "FAILED"
    | "ROLLING_BACK"
    | "ROLLED_BACK"
    | "CANCELLED";
  currentStep?: string;
  progress: number;
  errorCode?: string;
  errorMessage?: string;
}
