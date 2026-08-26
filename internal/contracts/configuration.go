package contracts

// ProjectConfiguration is the complete, versioned desired state for a project.
// Secrets are represented by SecretInput markers; plaintext values are never
// returned as part of a normal configuration projection.
type ProjectConfiguration struct {
	Revision  int64           `json:"revision"`
	General   GeneralConfig   `json:"general"`
	Services  Services        `json:"services"`
	Auth      AuthConfig      `json:"auth"`
	Storage   StorageConfig   `json:"storage"`
	Realtime  RealtimeConfig  `json:"realtime"`
	Functions FunctionsConfig `json:"functions"`
	Database  DatabaseConfig  `json:"database"`
	Pooler    PoolerConfig    `json:"pooler"`
	Network   NetworkConfig   `json:"network"`
}

type ConfigurationPatch struct {
	ExpectedRevision int64                 `json:"expectedRevision"`
	Configuration    *ProjectConfiguration `json:"configuration,omitempty"`
	General          *GeneralConfig        `json:"general,omitempty"`
	Services         *Services             `json:"services,omitempty"`
	Auth             *AuthConfig           `json:"auth,omitempty"`
	Storage          *StorageConfig        `json:"storage,omitempty"`
	Realtime         *RealtimeConfig       `json:"realtime,omitempty"`
	Functions        *FunctionsConfig      `json:"functions,omitempty"`
	Database         *DatabaseConfig       `json:"database,omitempty"`
	Pooler           *PoolerConfig         `json:"pooler,omitempty"`
	Network          *NetworkConfig        `json:"network,omitempty"`
}

type SecretInput struct {
	Action string `json:"action"` // retain, replace, remove
	Value  string `json:"value,omitempty"`
}

type GeneralConfig struct {
	Domain          string `json:"domain"`
	SiteURL         string `json:"siteUrl"`
	SupabaseVersion string `json:"supabaseVersion"`
}

type EmailAuthConfig struct {
	Enabled              bool `json:"enabled"`
	AllowSignup          bool `json:"allowSignup"`
	ConfirmEmail         bool `json:"confirmEmail"`
	SecureEmailChange    bool `json:"secureEmailChange"`
	DoubleConfirmChanges bool `json:"doubleConfirmChanges"`
}

type PhoneAuthConfig struct {
	Enabled   bool              `json:"enabled"`
	Provider  string            `json:"provider,omitempty"`
	SecretSet bool              `json:"secretSet"`
	Secret    SecretInput       `json:"secret,omitempty"`
	Fields    map[string]string `json:"fields,omitempty"` // provider-specific non-secret fields only
}

type SMTPConfig struct {
	Enabled     bool        `json:"enabled"`
	Host        string      `json:"host"`
	Port        int         `json:"port"`
	Username    string      `json:"username"`
	PasswordSet bool        `json:"passwordSet"`
	Password    SecretInput `json:"password,omitempty"`
	SenderEmail string      `json:"senderEmail"`
	SenderName  string      `json:"senderName"`
}

// EmailTemplateConfig is the subject and HTML source delivered to GoTrue through
// the project-local template service.
type EmailTemplateConfig struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

type EmailTemplatesConfig struct {
	Confirmation     EmailTemplateConfig `json:"confirmation"`
	Invite           EmailTemplateConfig `json:"invite"`
	MagicLink        EmailTemplateConfig `json:"magicLink"`
	EmailChange      EmailTemplateConfig `json:"emailChange"`
	Recovery         EmailTemplateConfig `json:"recovery"`
	Reauthentication EmailTemplateConfig `json:"reauthentication"`
}

type EmailNotificationConfig struct {
	Enabled  bool                `json:"enabled"`
	Template EmailTemplateConfig `json:"template"`
}

type EmailNotificationsConfig struct {
	PasswordChanged     EmailNotificationConfig `json:"passwordChanged"`
	EmailChanged        EmailNotificationConfig `json:"emailChanged"`
	PhoneChanged        EmailNotificationConfig `json:"phoneChanged"`
	IdentityLinked      EmailNotificationConfig `json:"identityLinked"`
	IdentityUnlinked    EmailNotificationConfig `json:"identityUnlinked"`
	MFAFactorEnrolled   EmailNotificationConfig `json:"mfaFactorEnrolled"`
	MFAFactorUnenrolled EmailNotificationConfig `json:"mfaFactorUnenrolled"`
}

type MailerConfig struct {
	Templates     EmailTemplatesConfig     `json:"templates"`
	Notifications EmailNotificationsConfig `json:"notifications"`
}

// RateLimitConfig configures the pinned GoTrue endpoint limits. Values are
// counts per GoTrue's built-in interval; the manager deliberately does not
// expose arbitrary environment variables.
type RateLimitConfig struct {
	EmailSent         int `json:"emailSent"`
	SMSSent           int `json:"smsSent"`
	TokenRefresh      int `json:"tokenRefresh"`
	TokenVerification int `json:"tokenVerification"`
	AnonymousUsers    int `json:"anonymousUsers"`
	SignupsAndSignins int `json:"signupsAndSignins"`
}

// MFAConfig is the supported MFA subset of the pinned GoTrue runtime.
type MFAConfig struct {
	TOTPEnrollEnabled  bool `json:"totpEnrollEnabled"`
	TOTPVerifyEnabled  bool `json:"totpVerifyEnabled"`
	PhoneEnrollEnabled bool `json:"phoneEnrollEnabled"`
	PhoneVerifyEnabled bool `json:"phoneVerifyEnabled"`
	MaxEnrolledFactors int  `json:"maxEnrolledFactors"`
	PhoneOTPLength     int  `json:"phoneOtpLength"`
}

type OAuthProviderConfig struct {
	Enabled   bool              `json:"enabled"`
	ClientID  string            `json:"clientId"`
	SecretSet bool              `json:"secretSet"`
	Secret    SecretInput       `json:"secret,omitempty"`
	Fields    map[string]string `json:"fields,omitempty"`
}

type AuthConfig struct {
	Enabled         bool                           `json:"enabled"`
	JWTExpiry       int                            `json:"jwtExpiry"`
	DisableSignup   bool                           `json:"disableSignup"`
	Email           EmailAuthConfig                `json:"email"`
	Phone           PhoneAuthConfig                `json:"phone"`
	AnonymousSignIn bool                           `json:"anonymousSignIn"`
	RedirectURLs    []string                       `json:"redirectUrls,omitempty"`
	OAuth           map[string]OAuthProviderConfig `json:"oauth,omitempty"`
	SMTP            SMTPConfig                     `json:"smtp"`
	Mailer          MailerConfig                   `json:"mailer"`
	RateLimits      RateLimitConfig                `json:"rateLimits"`
	MFA             MFAConfig                      `json:"mfa"`
}

type StorageBackend string

const (
	StorageBackendLocal StorageBackend = "local"
	StorageBackendS3    StorageBackend = "s3"
	StorageBackendAWSS3 StorageBackend = "aws-s3"
	StorageBackendR2    StorageBackend = "r2"
)

type StorageConfig struct {
	Backend            StorageBackend `json:"backend"`
	S3CompatibleAPI    bool           `json:"s3CompatibleApi"`
	Bucket             string         `json:"bucket"`
	Region             string         `json:"region"`
	Endpoint           string         `json:"endpoint"`
	AccountID          string         `json:"accountId"`
	AccessKeyID        string         `json:"accessKeyId"`
	SecretAccessKeySet bool           `json:"secretAccessKeySet"`
	SecretAccessKey    SecretInput    `json:"secretAccessKey,omitempty"`
	ForcePathStyle     bool           `json:"forcePathStyle"`
	LocalPath          string         `json:"localPath"`
}

type LogLevel string

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
)

type RealtimeConfig struct {
	MaxConnections   int      `json:"maxConnections"`
	DatabasePoolSize int      `json:"databasePoolSize"`
	LogLevel         LogLevel `json:"logLevel"`
}

type FunctionVariable struct {
	Name     string      `json:"name"`
	ValueSet bool        `json:"valueSet"`
	Value    SecretInput `json:"value,omitempty"`
}

type FunctionsConfig struct {
	DefaultJWTVerification bool               `json:"defaultJwtVerification"`
	Directory              string             `json:"directory"`
	Variables              []FunctionVariable `json:"variables,omitempty"`
}

type DatabaseConfig struct {
	Version          string   `json:"version"`
	DirectPort       bool     `json:"directPort"`
	DirectPortNumber int      `json:"directPortNumber"`
	MaxConnections   int      `json:"maxConnections"`
	SharedBuffers    string   `json:"sharedBuffers"`
	Extensions       []string `json:"extensions,omitempty"`
}

type PoolerConfig struct {
	TransactionPort      int `json:"transactionPort"`
	SessionPort          int `json:"sessionPort"`
	PoolSize             int `json:"poolSize"`
	MaxClientConnections int `json:"maxClientConnections"`
}

type Gateway string

const (
	GatewayEnvoy Gateway = "envoy"
	GatewayKong  Gateway = "kong"
)

type HTTPSMode string

const (
	HTTPSModeExternal HTTPSMode = "external"
	HTTPSModeCaddy    HTTPSMode = "caddy"
)

type NetworkConfig struct {
	Gateway             Gateway   `json:"gateway"`
	HTTPSMode           HTTPSMode `json:"httpsMode"`
	InternalGatewayPort int       `json:"internalGatewayPort,omitempty"`
	APIPort             int       `json:"apiPort"`
	StudioPort          int       `json:"studioPort"`
	DirectDatabasePort  int       `json:"directDatabasePort"`
	PoolerPort          int       `json:"poolerPort"`
}

var OAuthProviderNames = []string{
	"apple", "azure", "bitbucket", "discord", "facebook", "figma",
	"github", "gitlab", "google", "kakao", "keycloak", "linkedin_oidc",
	"notion", "slack_oidc", "snapchat", "spotify", "twitch", "twitter",
	"workos", "zoom",
}
