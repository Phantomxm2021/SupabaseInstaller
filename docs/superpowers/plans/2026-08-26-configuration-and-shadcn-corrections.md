# Configuration and shadcn Corrections Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver complete typed Supabase project configuration, dynamic runtime reconciliation, and a shadcn-based React interface while fixing Settings routing, duplicate navigation, post-install navigation, and stale project deletion.

**Architecture:** One versioned `ProjectConfiguration` aggregate is authoritative from browser to Manager to Provisioner. The Manager persists redacted typed configuration and encrypted secrets; the Provisioner translates a complete revision into official `.env`, `.env.functions`, and filtered Compose files, then reconciles only affected services with last-known-good rollback. The React application uses the same typed aggregate for creation and installed-project editing.

**Tech Stack:** Go 1.24, SQLite, AES-256-GCM, Docker Compose v2, React 19, TypeScript 5.9, Vite 7, TanStack Query 5, React Hook Form 7, Zod 4, Tailwind CSS 4, shadcn components, Vitest and Testing Library.

**Spec:** `docs/superpowers/specs/2026-08-26-configuration-and-shadcn-corrections-design.md`

## Global Constraints

- Luna High owns implementation; Sol owns audit, verification, and narrowly scoped corrections.
- Users edit validated typed fields, never raw `.env` or arbitrary Compose YAML.
- PostgreSQL is mandatory; Studio requires postgres-meta; imgproxy requires Storage; Logs manages Logflare and Vector.
- Secret plaintext is encrypted at rest and absent from normal GET responses, logs, operation events, and browser storage.
- Every update uses an expected configuration revision and preserves a last-known-good revision.
- Failed reconciliation restores the previous files and runtime without deleting volumes.
- The current pinned runtime remains `self-hosted/v0.8.0`; no `latest` image is allowed.
- Use actual shadcn generated components and theme tokens, not custom visual imitations.
- Follow test-driven development: each task starts with a failing targeted test and ends with relevant suites passing and a focused commit.

---

### Task 1: Authoritative Configuration Contracts and Validation

**Files:**
- Create: `internal/contracts/configuration.go`
- Create: `apps/manager/internal/project/configuration.go`
- Create: `apps/manager/internal/project/configuration_test.go`
- Modify: `internal/contracts/project.go`
- Modify: `internal/contracts/provisioner.go`
- Modify: `apps/manager/internal/project/validate.go`
- Modify: `apps/manager/internal/project/project_test.go`

**Interfaces:**
- Produces: `contracts.ProjectConfiguration`, `contracts.ConfigurationPatch`, `contracts.SecretInput`, `contracts.ReconcileProjectRequest`, `project.DefaultConfiguration`, `project.ApplyConfigurationPreset`, and `project.ValidateConfiguration`.
- Consumes: existing `contracts.Services`, `contracts.Preset`, and `contracts.ProjectDraft`.

- [ ] **Step 1: Write failing configuration default and dependency tests**

Add table tests asserting:

```go
func TestDefaultConfiguration(t *testing.T) {
    got := DefaultConfiguration(contracts.PresetLightweight)
    if !got.Services.Database || !got.Services.Auth || got.Services.Storage || got.Auth.SMTP.Enabled {
        t.Fatalf("unexpected Lightweight defaults: %#v", got)
    }
    if !got.Auth.Email.Enabled || got.Auth.Phone.Enabled || got.Auth.AnonymousSignIn {
        t.Fatalf("unexpected Auth defaults: %#v", got.Auth)
    }
}

func TestValidateConfigurationDependencies(t *testing.T) {
    cases := []struct {
        name string
        mutate func(*contracts.ProjectConfiguration)
        field string
    }{
        {"database mandatory", func(c *contracts.ProjectConfiguration) { c.Services.Database = false }, "services.database"},
        {"studio requires meta", func(c *contracts.ProjectConfiguration) { c.Services.PostgresMeta = false }, "services.postgresMeta"},
        {"imgproxy requires storage", func(c *contracts.ProjectConfiguration) { c.Services.Imgproxy = true }, "services.imgproxy"},
        {"vector follows logs", func(c *contracts.ProjectConfiguration) { c.Services.Vector = true }, "services.vector"},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            cfg := DefaultConfiguration(contracts.PresetLightweight)
            tc.mutate(&cfg)
            err := ValidateConfiguration(cfg)
            var validation *ValidationError
            if !errors.As(err, &validation) || validation.Fields[tc.field] == "" {
                t.Fatalf("expected field error for %s, got %v", tc.field, err)
            }
        })
    }
}
```

- [ ] **Step 2: Run the focused tests and verify failure**

Run: `GOCACHE=/tmp/supabase-installer-go-cache go test ./apps/manager/internal/project -run 'Test(DefaultConfiguration|ValidateConfigurationDependencies)'`

Expected: FAIL because the configuration types and functions do not exist.

- [ ] **Step 3: Define the complete typed contract**

Define these JSON-facing types in `internal/contracts/configuration.go`:

```go
type ProjectConfiguration struct {
    Revision  int64               `json:"revision"`
    General   GeneralConfig       `json:"general"`
    Services  Services            `json:"services"`
    Auth      AuthConfig          `json:"auth"`
    Storage   StorageConfig       `json:"storage"`
    Realtime  RealtimeConfig      `json:"realtime"`
    Functions FunctionsConfig     `json:"functions"`
    Database  DatabaseConfig      `json:"database"`
    Pooler    PoolerConfig        `json:"pooler"`
    Network   NetworkConfig       `json:"network"`
}

type SecretInput struct {
    Action string `json:"action"` // retain, replace, remove
    Value  string `json:"value,omitempty"`
}

type EmailAuthConfig struct {
    Enabled              bool `json:"enabled"`
    AllowSignup          bool `json:"allowSignup"`
    ConfirmEmail         bool `json:"confirmEmail"`
    SecureEmailChange    bool `json:"secureEmailChange"`
    DoubleConfirmChanges bool `json:"doubleConfirmChanges"`
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

type OAuthProviderConfig struct {
    Enabled   bool              `json:"enabled"`
    ClientID  string            `json:"clientId"`
    SecretSet bool              `json:"secretSet"`
    Secret    SecretInput       `json:"secret,omitempty"`
    Fields    map[string]string `json:"fields,omitempty"`
}
```

Also define `GeneralConfig`, `PhoneAuthConfig`, `AuthConfig`, `StorageConfig`, `RealtimeConfig`, `FunctionsConfig`, `FunctionVariable`, `DatabaseConfig`, `PoolerConfig`, and `NetworkConfig` with the exact fields from the approved spec. Use enumerated string types for storage backend (`local`, `s3`, `aws-s3`, `r2`), gateway (`envoy`, `kong`), HTTPS mode (`external`, `caddy`, `manual`), and log level.

Add the 20-provider registry as stable constants:

```go
var OAuthProviderNames = []string{
    "apple", "azure", "bitbucket", "discord", "facebook", "figma",
    "github", "gitlab", "google", "kakao", "keycloak", "linkedin_oidc",
    "notion", "slack_oidc", "snapchat", "spotify", "twitch", "twitter",
    "workos", "zoom",
}
```

Extend `ProjectDraft` with `Configuration ProjectConfiguration` and keep `Services` only as a compatibility projection during migration. Extend `Project` with `ConfigurationRevision int64`.

Define Provisioner input:

```go
type ReconcileProjectRequest struct {
    OperationID      string               `json:"operationId"`
    IdempotencyKey   string               `json:"idempotencyKey"`
    ProjectID        string               `json:"projectId"`
    ProjectName      string               `json:"projectName"`
    Slug             string               `json:"slug"`
    ExpectedRevision int64                `json:"expectedRevision"`
    NextRevision     int64                `json:"nextRevision"`
    APIPort          int                  `json:"apiPort"`
    Configuration    ProjectConfiguration `json:"configuration"`
    Secrets          ProjectSecrets       `json:"secrets"`
    RuntimeSecrets   map[string]string    `json:"runtimeSecrets,omitempty"`
}

type ReconcileProjectResponse struct {
    OperationID       string   `json:"operationId"`
    ProjectID         string   `json:"projectId"`
    Revision          int64    `json:"revision"`
    EnabledServices   []string `json:"enabledServices"`
    RecreatedServices []string `json:"recreatedServices"`
    RolledBack        bool     `json:"rolledBack"`
}
```

- [ ] **Step 4: Implement defaults, preset closure, and authoritative validation**

Implement `DefaultConfiguration`, `ApplyConfigurationPreset`, and `ValidateConfiguration`. Validate URLs, email, redirect URL entries, port range, bounded positive connection values, environment variable names with `^[A-Z_][A-Z0-9_]*$`, reserved Functions names, provider names and provider-required fields. Return the existing field-aware `ValidationError` so the HTTP API can preserve field paths.

Required dependency behavior is deterministic: selecting Studio forces postgres-meta; selecting imgproxy forces Storage in preset application, but a direct invalid API payload is rejected; Logs sets both Logs and Vector; disabling Logs clears Vector.

- [ ] **Step 5: Run project and contract tests**

Run: `GOCACHE=/tmp/supabase-installer-go-cache go test ./apps/manager/internal/project ./internal/contracts`

Expected: PASS.

- [ ] **Step 6: Commit Task 1**

```bash
git add internal/contracts apps/manager/internal/project
git commit -m "feat: define typed project configuration"
```

---

### Task 2: Versioned Configuration Persistence and Encrypted Secret Patches

**Files:**
- Create: `apps/manager/internal/store/migrations/002_project_configuration.sql`
- Create: `apps/manager/internal/store/configuration.go`
- Create: `apps/manager/internal/project/configuration_service.go`
- Create: `apps/manager/internal/project/configuration_service_test.go`
- Modify: `apps/manager/internal/store/sqlite.go`
- Modify: `apps/manager/internal/store/store_test.go`
- Modify: `apps/manager/internal/store/projects.go`
- Modify: `apps/manager/internal/project/service.go`

**Interfaces:**
- Consumes: `contracts.ProjectConfiguration`, `contracts.SecretInput`, and `project.ValidateConfiguration` from Task 1; existing `secrets.Cipher` and `store.PutSecret`.
- Produces: `store.ConfigurationSnapshot`, `Store.GetConfiguration`, `Store.SaveConfiguration`, and `project.ConfigurationService`.

- [ ] **Step 1: Write failing migration, revision, and secret-patch tests**

Add tests that create a project, read revision 1, save expected revision 1 as revision 2, reject another expected revision 1 write with `project.ErrStaleConfiguration`, and confirm `config_json` contains no SMTP/OAuth/S3/Functions plaintext.

Use this secret patch table:

```go
cases := []struct {
    action string
    value string
    want  string
    exists bool
}{
    {"retain", "", "old-secret", true},
    {"replace", "new-secret", "new-secret", true},
    {"remove", "", "", false},
}
```

- [ ] **Step 2: Run store and service tests to verify failure**

Run: `GOCACHE=/tmp/supabase-installer-go-cache go test ./apps/manager/internal/store ./apps/manager/internal/project -run 'Test(Configuration|SecretPatch|Migration002)'`

Expected: FAIL because migration 002 and configuration persistence do not exist.

- [ ] **Step 3: Add ordered embedded migrations**

Change `sqlite.go` from one embedded migration string to `//go:embed migrations/*.sql` and apply numeric files transactionally through `schema_migrations`. Migration 002 adds `last_good_revision`, initializes a `general` or `aggregate` project config snapshot for existing projects from `projects.services_json`, and preserves existing installations.

Use immutable rows in `project_configs(project_id, section, revision, config_json, created_at)` and update `projects.config_revision` only in the same transaction that inserts a complete `aggregate` snapshot.

- [ ] **Step 4: Implement redacted snapshot persistence**

Define:

```go
type ConfigurationSnapshot struct {
    ProjectID string
    Revision  int64
    LastGoodRevision int64
    Configuration contracts.ProjectConfiguration
}

func (s *Store) GetConfiguration(ctx context.Context, projectID string) (ConfigurationSnapshot, error)
func (s *Store) SaveConfiguration(ctx context.Context, projectID string, expected int64, cfg contracts.ProjectConfiguration, now time.Time) (ConfigurationSnapshot, error)
func (s *Store) MarkConfigurationGoodOwned(ctx context.Context, projectID string, revision int64, owner string, fence int64, phase string, now time.Time) error
```

Before JSON encoding, clear every `SecretInput.Value`; persist only `PasswordSet`/`SecretSet` flags in configuration JSON. Use a transaction and `UPDATE ... WHERE config_revision = ?` to implement optimistic concurrency.

- [ ] **Step 5: Implement encrypted retain/replace/remove semantics**

`ConfigurationService` receives the store, cipher, clock, and operation coordinator. It validates the proposed aggregate, applies secret patches to named kinds such as `smtp.password`, `oauth.google.secret`, `storage.secretAccessKey`, and `functions.OPENAI_API_KEY`, and returns a redacted snapshot.

Add transactional store methods for secret upsert/delete. Do not delete an existing secret for `retain`. Reject `replace` with an empty value and reject unknown actions.

- [ ] **Step 6: Run store, project, and race tests**

Run: `GOCACHE=/tmp/supabase-installer-go-cache go test -race ./apps/manager/internal/store ./apps/manager/internal/project`

Expected: PASS and no race report.

- [ ] **Step 7: Commit Task 2**

```bash
git add apps/manager/internal/store apps/manager/internal/project
git commit -m "feat: persist versioned encrypted project configuration"
```

---

### Task 3: Version-Aware Supabase Environment and Compose Renderer

**Files:**
- Create: `apps/provisioner/internal/render/provider_registry.go`
- Create: `apps/provisioner/internal/render/environment.go`
- Create: `apps/provisioner/internal/render/services.go`
- Create: `apps/provisioner/internal/render/testdata/standard.golden.yml`
- Create: `apps/provisioner/internal/render/testdata/full.golden.yml`
- Create: `apps/provisioner/internal/render/testdata/custom-auth.env.golden`
- Modify: `apps/provisioner/internal/render/render.go`
- Modify: `apps/provisioner/internal/render/render_test.go`
- Modify: `apps/provisioner/internal/projectfs/root.go`
- Modify: `apps/provisioner/internal/projectfs/root_test.go`

**Interfaces:**
- Consumes: `contracts.ProjectConfiguration` and decrypted `RuntimeSecrets` from Tasks 1–2.
- Produces: `render.Project`, `render.OutputFiles`, `projectfs.RuntimeFiles`, and complete service/environment mapping.

- [ ] **Step 1: Write failing renderer selection and Auth/SMTP mapping tests**

Assert these representative behaviors:

```go
func TestRenderCustomAuthAndSMTP(t *testing.T) {
    cfg := testConfiguration()
    cfg.Auth.Email.ConfirmEmail = true
    cfg.Auth.SMTP = contracts.SMTPConfig{Enabled: true, Host: "smtp.example.com", Port: 587, Username: "mailer", SenderEmail: "noreply@example.com", SenderName: "Example"}
    cfg.Auth.OAuth["google"] = contracts.OAuthProviderConfig{Enabled: true, ClientID: "google-client", SecretSet: true}
    out, err := Project(Input{Configuration: cfg, RuntimeSecrets: map[string]string{"smtp.password": "smtp-secret", "oauth.google.secret": "oauth-secret"}})
    if err != nil { t.Fatal(err) }
    for _, line := range []string{"ENABLE_EMAIL_AUTOCONFIRM=false", "SMTP_HOST=smtp.example.com", "SMTP_PORT=587", "GOOGLE_ENABLED=true", "GOOGLE_CLIENT_ID=google-client", "GOOGLE_SECRET=oauth-secret"} {
        if !strings.Contains(out.Env, line) { t.Errorf("missing %q", line) }
    }
    if !strings.Contains(out.Compose, "GOTRUE_EXTERNAL_GOOGLE_ENABLED") { t.Fatal("Google mapping missing from Auth service") }
}
```

Add tests for Lightweight, Standard, Full and a Custom combination; verify disabled services are absent, dependencies are pruned only when safe, images remain pinned, API port binds to `127.0.0.1`, and `.env.functions` contains Function variables but not reserved overrides.

- [ ] **Step 2: Run renderer tests and verify failure**

Run: `GOCACHE=/tmp/supabase-installer-go-cache go test ./apps/provisioner/internal/render ./apps/provisioner/internal/projectfs`

Expected: FAIL because only `Lightweight` and two runtime files exist.

- [ ] **Step 3: Replace the fixed Lightweight renderer with one complete renderer**

Define:

```go
type Input struct {
    ProjectID string
    Slug string
    APIPort int
    Configuration contracts.ProjectConfiguration
    Secrets provisionersecrets.ProjectSecrets
    RuntimeSecrets map[string]string
    TemplateCompose []byte
}

type OutputFiles struct {
    Env string
    FunctionsEnv string
    Compose string
    ComposeProjectName string
    EnabledComposeServices []string
}

func Project(input Input) (OutputFiles, error)
```

Map product service names to Compose services using explicit slices. Include `db`, gateway, `auth`, `rest`, `realtime`, `storage`, `imgproxy`, `meta`, `studio`, `functions`, `supavisor`, `analytics`, and `vector` according to configuration. Keep template-only dependency containers only when their selected feature requires them.

- [ ] **Step 4: Implement official variable mappings**

Start with the embedded `.env.example`, replace values without duplicating keys, and escape newline/control characters. Required Auth/Email mappings include:

```text
SITE_URL
ADDITIONAL_REDIRECT_URLS
JWT_EXPIRY
DISABLE_SIGNUP
ENABLE_EMAIL_SIGNUP
ENABLE_EMAIL_AUTOCONFIRM
ENABLE_ANONYMOUS_USERS
ENABLE_PHONE_SIGNUP
ENABLE_PHONE_AUTOCONFIRM
SMTP_ADMIN_EMAIL
SMTP_HOST
SMTP_PORT
SMTP_USER
SMTP_PASS
SMTP_SENDER_NAME
```

Inject missing official Auth service environment entries for secure email change and every provider. Provider registry entries generate:

```text
GOTRUE_EXTERNAL_<PROVIDER>_ENABLED
GOTRUE_EXTERNAL_<PROVIDER>_CLIENT_ID
GOTRUE_EXTERNAL_<PROVIDER>_SECRET
GOTRUE_EXTERNAL_<PROVIDER>_REDIRECT_URI
```

and versioned special keys for Azure Tenant URL, GitHub Enterprise URL, GitLab URL, and Keycloak Realm URL. Storage maps `STORAGE_BACKEND`, `GLOBAL_S3_BUCKET`, `GLOBAL_S3_ENDPOINT`, `GLOBAL_S3_FORCE_PATH_STYLE`, `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, and `REGION`. Functions maps `FUNCTIONS_VERIFY_JWT` and writes secret variables only to `.env.functions`. Pooler maps the four `POOLER_*` controls. Direct database exposure modifies only the DB port binding.

- [ ] **Step 5: Make runtime file replacement atomic as a set**

Replace `WriteRuntimeFiles(slug, compose, environment)` with:

```go
type RuntimeFiles struct { Compose, Env, FunctionsEnv []byte }
func (r *Root) StageRuntimeFiles(slug string, files RuntimeFiles) (restore func() error, commit func() error, err error)
```

Stage all candidate files, fsync them, retain `.manager-last-good/` copies, and make `restore` reinstall all prior files. Set `.env` and `.env.functions` to `0600` and Compose to `0600`.

- [ ] **Step 6: Update golden files and run Compose validation**

Run renderer tests, then validate generated representative files with:

```bash
docker compose --file /tmp/supabase-manager-render/lightweight/docker-compose.yml --project-directory /tmp/supabase-manager-render/lightweight config --quiet
docker compose --file /tmp/supabase-manager-render/standard/docker-compose.yml --project-directory /tmp/supabase-manager-render/standard config --quiet
docker compose --file /tmp/supabase-manager-render/full/docker-compose.yml --project-directory /tmp/supabase-manager-render/full config --quiet
```

Expected: all commands exit 0 and no rendered image uses `latest`.

- [ ] **Step 7: Commit Task 3**

```bash
git add apps/provisioner/internal/render apps/provisioner/internal/projectfs
git commit -m "feat: render typed Supabase runtime configuration"
```

---

### Task 4: Provisioner Reconciliation and Last-Known-Good Rollback

**Files:**
- Modify: `apps/provisioner/internal/compose/runner.go`
- Modify: `apps/provisioner/internal/compose/runner_test.go`
- Create: `apps/provisioner/internal/runtime/reconcile.go`
- Create: `apps/provisioner/internal/runtime/reconcile_test.go`
- Modify: `apps/provisioner/internal/runtime/backend.go`
- Modify: `apps/provisioner/internal/runtime/backend_test.go`
- Modify: `apps/provisioner/internal/server/server.go`
- Modify: `apps/provisioner/internal/server/server_test.go`
- Modify: `internal/contracts/provisioner.go`

**Interfaces:**
- Consumes: `render.Project`, `projectfs.StageRuntimeFiles`, and `contracts.ReconcileProjectRequest`.
- Produces: `POST /internal/v1/projects/reconcile`, `runtime.Backend.Reconcile`, and `compose.Runner.Reconcile`.

- [ ] **Step 1: Write failing affected-service and rollback tests**

Use a fake runner and inspector to assert:

```go
cases := []struct {
    name string
    changed []string
    wantRecreate []string
}{
    {"smtp", []string{"auth.smtp"}, []string{"auth"}},
    {"google oauth", []string{"auth.oauth.google"}, []string{"auth"}},
    {"functions env", []string{"functions.environment"}, []string{"functions"}},
    {"storage backend", []string{"storage.backend"}, []string{"storage"}},
    {"site URL", []string{"general.siteUrl"}, []string{"auth", "studio", "api-gw"}},
}
```

Add a failure test where the first health check fails, prior files are restored, prior services are recreated, recovery health succeeds, and volumes are never removed.

- [ ] **Step 2: Run runtime/server tests and verify failure**

Run: `GOCACHE=/tmp/supabase-installer-go-cache go test ./apps/provisioner/internal/runtime ./apps/provisioner/internal/server ./apps/provisioner/internal/compose`

Expected: FAIL because reconcile does not exist.

- [ ] **Step 3: Add fixed-argument Compose reconcile operations**

Add runner methods:

```go
func (r *Runner) UpSelected(ctx context.Context, project ProjectRef, services ...string) error
func (r *Runner) Recreate(ctx context.Context, project ProjectRef, services ...string) error
func (r *Runner) RemoveStopped(ctx context.Context, project ProjectRef, services ...string) error
func (r *Runner) Validate(ctx context.Context, project ProjectRef) error
```

Commands are fixed argument arrays: `docker compose config --quiet`, `up -d --remove-orphans <selected>`, and `rm -s -f <disabled>`. Do not use `down -v`, shell interpolation, or user-provided Compose service names.

- [ ] **Step 4: Implement reconciliation**

`Backend.Reconcile` checks the metadata expected revision, renders candidate files, validates Compose, commits files, removes newly disabled containers, recreates only affected services, and inspects all enabled services. On success it advances metadata revision. On failure it invokes the restore closure, recreates the prior affected services, verifies recovery, leaves metadata revision unchanged, and returns a typed result stating rollback success.

Initial installation calls the same reconcile path with expected revision 0, then starts DB before selected dependent services.

- [ ] **Step 5: Register the private endpoint and preserve idempotency**

Register `POST /internal/v1/projects/reconcile`. Cache the typed response under the request idempotency key in project metadata. Return `409 STALE_CONFIG_REVISION` for mismatched revisions and a redacted `422 RECONCILE_FAILED` response for runtime failure.

- [ ] **Step 6: Run Provisioner tests and race tests**

Run: `GOCACHE=/tmp/supabase-installer-go-cache go test -race ./apps/provisioner/...`

Expected: PASS and no race report.

- [ ] **Step 7: Commit Task 4**

```bash
git add apps/provisioner internal/contracts/provisioner.go
git commit -m "feat: reconcile project configuration with rollback"
```

---

### Task 5: Manager Configuration API and UPDATE_CONFIG Operations

**Files:**
- Create: `apps/manager/internal/configuration/orchestrator.go`
- Create: `apps/manager/internal/configuration/orchestrator_test.go`
- Create: `apps/manager/internal/httpapi/configuration.go`
- Create: `apps/manager/internal/httpapi/configuration_test.go`
- Modify: `apps/manager/internal/provisioner/client.go`
- Modify: `apps/manager/internal/provisioner/client_test.go`
- Modify: `apps/manager/internal/httpapi/router.go`
- Modify: `apps/manager/cmd/manager/main.go`
- Modify: `apps/manager/internal/operation/model.go`
- Modify: `apps/manager/internal/install/orchestrator.go`
- Modify: `apps/manager/internal/install/orchestrator_test.go`
- Modify: `apps/manager/internal/project/service.go`

**Interfaces:**
- Consumes: Tasks 1–4 configuration service and Provisioner reconcile contract.
- Produces: authenticated configuration GET/PATCH endpoints and durable `UPDATE_CONFIG` operations.

- [ ] **Step 1: Write failing API contract tests**

Test these behaviors:

```text
GET /api/projects/{id}/configuration -> 200 redacted aggregate
PATCH /api/projects/{id}/configuration/auth with matching revision -> 202 {projectId, operationId, revision}
PATCH with stale revision -> 409 CONFIGURATION_STALE
PATCH invalid SMTP -> 422 field errors
PATCH secret retain -> response never includes existing plaintext
unauthenticated GET/PATCH -> 401
PATCH without CSRF -> 403
```

Also test creation forwards the complete selected configuration and its enabled service set to Provisioner.

- [ ] **Step 2: Run API/orchestrator tests and verify failure**

Run: `GOCACHE=/tmp/supabase-installer-go-cache go test ./apps/manager/internal/httpapi ./apps/manager/internal/configuration ./apps/manager/internal/install`

Expected: FAIL because configuration handlers and orchestration do not exist.

- [ ] **Step 3: Add Provisioner client reconciliation**

Add:

```go
func (client *Client) Reconcile(ctx context.Context, input contracts.ReconcileProjectRequest) (contracts.ReconcileProjectResponse, error)
```

POST JSON to `/internal/v1/projects/reconcile`, preserve manager bearer authorization, decode stale revision separately, and never include response bodies containing secrets in errors.

- [ ] **Step 4: Implement UPDATE_CONFIG orchestration**

Create an operation of type `UPDATE_CONFIG`, decrypt only the secret kinds required for rendering, save the desired revision, call Provisioner reconcile, and mark the revision good on success. On Provisioner failure, keep the desired revision for audit but return the normal redacted GET projection from the last-good configuration and finish the operation as rolled back or failed according to the Provisioner result.

Operation steps are `VALIDATE_CONFIGURATION`, `SAVE_CONFIGURATION`, `RENDER_RUNTIME`, `RECONCILE_SERVICES`, `VERIFY_SERVICES`, and `MARK_CONFIGURATION_GOOD`.

- [ ] **Step 5: Implement section PATCH handlers over the aggregate**

Each route decodes `{expectedRevision, value}` with unknown-field rejection, merges only its section into the current aggregate, applies `SecretInput` actions, validates the complete result, queues the operation, and returns 202. Implement the exact endpoints from the design spec. GET responses set every secret value to empty and expose only `passwordSet`/`secretSet` booleans.

Add `POST /api/projects/{id}/secrets/{kind}/reveal` for `anonKey`, `serviceRoleKey`, `jwtSecret`, and `databasePassword`. It accepts the administrator password, enforces recent authentication, sets `Cache-Control: no-store`, and returns one plaintext value without writing it to operation history. Add `POST /api/projects/{id}/secrets/databasePassword/rotate` as a durable sensitive operation that updates PostgreSQL and dependent services before replacing the encrypted stored password.

- [ ] **Step 6: Make installation use the same aggregate and renderer**

During project creation, normalize default configuration when omitted for backwards-compatible tests, persist revision 1, and pass it to Provisioner. Health inspection uses `EnabledComposeServices` derived from the configuration rather than `lightweightServiceNames()`.

- [ ] **Step 7: Wire dependencies and run Manager suites**

Run:

```bash
GOCACHE=/tmp/supabase-installer-go-cache go test -race ./apps/manager/...
GOCACHE=/tmp/supabase-installer-go-cache go vet ./apps/manager/... ./internal/...
```

Expected: PASS and no vet or race findings.

- [ ] **Step 8: Commit Task 5**

```bash
git add apps/manager internal/contracts
git commit -m "feat: expose durable project configuration API"
```

---

### Task 6: shadcn Foundation, Global Navigation, Settings, and Cache Fixes

**Files:**
- Create: `apps/web/components.json`
- Create: `apps/web/src/lib/utils.ts`
- Create: `apps/web/src/components/ui/button.tsx`
- Create: `apps/web/src/components/ui/card.tsx`
- Create: `apps/web/src/components/ui/sidebar.tsx`
- Create: `apps/web/src/components/ui/dropdown-menu.tsx`
- Create: `apps/web/src/components/ui/alert-dialog.tsx`
- Create: `apps/web/src/components/ui/badge.tsx`
- Create: `apps/web/src/components/ui/progress.tsx`
- Create: `apps/web/src/components/ui/sonner.tsx`
- Create: `apps/web/src/features/settings/ManagerSettingsPage.tsx`
- Create: `apps/web/src/features/settings/ManagerSettingsPage.test.tsx`
- Modify: `apps/web/package.json`
- Modify: `package-lock.json`
- Modify: `apps/web/vite.config.ts`
- Modify: `apps/web/tsconfig.app.json`
- Modify: `apps/web/src/styles.css`
- Modify: `apps/web/src/main.tsx`
- Modify: `apps/web/src/app/router.tsx`
- Modify: `apps/web/src/app/AppShell.tsx`
- Modify: `apps/web/src/app/AppShell.test.tsx`
- Modify: `apps/web/src/features/project/DeleteProjectDialog.tsx`
- Modify: `apps/web/src/features/project/DeleteProjectDialog.test.tsx`
- Modify: `apps/web/src/features/project/OverviewPage.tsx`

**Interfaces:**
- Consumes: existing session response and TanStack Query keys.
- Produces: shadcn theme foundation, `/settings`, corrected global navigation, AlertDialog deletion, and immediate project-list refresh.

- [ ] **Step 1: Write failing navigation, settings, and deletion-cache tests**

Assert global navigation has Projects but no New Project; Manager Settings is a React Router link to `/settings`; the Settings page displays username and never displays `csrfToken` or its value; and successful delete executes:

```ts
await queryClient.cancelQueries({ queryKey: ['project', project.id] })
queryClient.removeQueries({ queryKey: ['project', project.id] })
await queryClient.invalidateQueries({ queryKey: ['projects'] })
navigate('/projects', { replace: true })
```

- [ ] **Step 2: Run focused web tests and verify failure**

Run: `npm run test --workspace apps/web -- --run src/app/AppShell.test.tsx src/features/settings/ManagerSettingsPage.test.tsx src/features/project/DeleteProjectDialog.test.tsx`

Expected: FAIL because `/settings` and shadcn components do not exist and the current link targets `/api/session`.

- [ ] **Step 3: Install and initialize the official shadcn Vite dependencies**

Run:

```bash
npm install --workspace apps/web tailwindcss @tailwindcss/vite class-variance-authority clsx tailwind-merge tw-animate-css sonner radix-ui
npx shadcn@latest init -d -c apps/web
npx shadcn@latest add -c apps/web button card sidebar dropdown-menu alert-dialog badge progress sonner tabs form input textarea select checkbox switch collapsible table skeleton separator scroll-area field
```

The resulting `package-lock.json` pins resolved package versions. Configure the `@/*` alias and `components.json`. Add Tailwind import and product theme tokens to `styles.css`.

Generate or copy the official shadcn component source into `src/components/ui`; preserve its accessibility primitives and do not wrap it in old `.button`, `.panel`, or `.dialog` classes.

- [ ] **Step 4: Rebuild AppShell and Settings with shadcn composition**

Use `SidebarProvider`, `Sidebar`, `SidebarHeader`, `SidebarContent`, `SidebarFooter`, `SidebarMenu`, and `SidebarMenuButton`. Keep Projects in global navigation; remove New Project. Put Settings and Sign out in the footer account menu. Add `/settings` under `AuthenticatedShell` and render safe session fields (`username`, `mustChangePassword`) plus control-plane status; never pass `csrfToken` to a visible component.

- [ ] **Step 5: Convert deletion to AlertDialog and fix query invalidation**

Preserve runtime-only versus runtime-and-data selection and exact-name confirmation. On successful deletion, invalidate `['projects']`, remove project detail/configuration queries, then replace-navigate. Display success/failure through Sonner.

- [ ] **Step 6: Run focused tests, full web tests, and build**

Run:

```bash
npm run test --workspace apps/web -- --run
npm run build --workspace apps/web
```

Expected: PASS; the production bundle contains the Settings route and no TypeScript errors.

- [ ] **Step 7: Commit Task 6**

```bash
git add apps/web package-lock.json
git commit -m "feat: adopt shadcn shell and fix navigation flows"
```

---

### Task 7: Configurable New Project Wizard and Success Navigation

**Files:**
- Create: `apps/web/src/features/projects/ServicesStep.tsx`
- Create: `apps/web/src/features/projects/AuthStep.tsx`
- Create: `apps/web/src/features/projects/StorageFunctionsStep.tsx`
- Create: `apps/web/src/features/projects/DatabaseNetworkStep.tsx`
- Create: `apps/web/src/features/projects/OAuthProviderFields.tsx`
- Modify: `apps/web/src/api/types.ts`
- Modify: `apps/web/src/features/projects/projectSchema.ts`
- Modify: `apps/web/src/features/projects/NewProjectPage.tsx`
- Modify: `apps/web/src/features/projects/NewProjectPage.test.tsx`
- Modify: `apps/web/src/features/projects/BasicStep.tsx`
- Modify: `apps/web/src/features/projects/PresetStep.tsx`
- Modify: `apps/web/src/features/projects/ReviewStep.tsx`
- Modify: `apps/web/src/features/operations/OperationPanel.tsx`
- Modify: `apps/web/src/features/operations/OperationPanel.test.tsx`

**Interfaces:**
- Consumes: backend `ProjectConfiguration` JSON contract and shadcn components.
- Produces: a six-step typed wizard and automatic `/projects/:id/overview` navigation after create success.

- [ ] **Step 1: Write failing preset, SMTP, payload, and navigation tests**

Test selecting Standard enables Realtime, Storage, Functions, and Supavisor; manual service change sets preset Custom; imgproxy cannot remain on when Storage is off; SMTP enabled requires the six SMTP fields; OAuth callback is read-only; create POST contains `configuration`; and a `SUCCEEDED` operation invokes:

```ts
navigate(`/projects/${projectId}/overview`, { replace: true })
```

- [ ] **Step 2: Run wizard/operation tests and verify failure**

Run: `npm run test --workspace apps/web -- --run src/features/projects/NewProjectPage.test.tsx src/features/operations/OperationPanel.test.tsx`

Expected: FAIL because only Basic and a disabled preset display exist.

- [ ] **Step 3: Mirror backend types and Zod validation**

Add TypeScript types with the exact backend JSON property names. Build `projectConfigurationSchema` with discriminated storage backend validation, conditional SMTP/OAuth fields, redirect URL validation, Function environment-name validation, numeric bounds, and service dependency refinements. Export `defaultConfiguration(preset)` and `applyPreset(preset)` matching Go defaults.

- [ ] **Step 4: Build the six wizard steps with shadcn fields**

Use Tabs/step navigation, Card, Field/Form, Switch, Select, Collapsible, Input, Password Input, and Alert. All official services appear. Required and auto-managed dependencies explain why their controls are disabled. Provider cards use the registry and show provider-specific fields. Secret inputs submit `replace` only when the user entered a new value.

- [ ] **Step 5: Build a redacted Review step and submit complete configuration**

Review lists actual enabled/disabled services and summaries of Auth, SMTP, enabled OAuth providers, Storage, Functions, database, and network. It displays “Configured” for entered secrets, never their value. Submit the selected pinned version, preset, and complete aggregate.

- [ ] **Step 6: Navigate after success and refresh project queries**

Retain both IDs from the create response. Pass `projectId` and `onSucceeded` into `OperationPanel`. Trigger navigation once when the operation becomes `SUCCEEDED`, invalidate `['projects']`, and do not navigate on FAILED/ROLLED_BACK/CANCELLED.

- [ ] **Step 7: Run wizard tests, all web tests, and build**

Run:

```bash
npm run test --workspace apps/web -- --run
npm run build --workspace apps/web
```

Expected: PASS.

- [ ] **Step 8: Commit Task 7**

```bash
git add apps/web/src/api apps/web/src/features/projects apps/web/src/features/operations
git commit -m "feat: add complete configurable project wizard"
```

---

### Task 8: Installed Project Configuration UI

**Files:**
- Create: `apps/web/src/features/configuration/ConfigurationPage.tsx`
- Create: `apps/web/src/features/configuration/ConfigurationPage.test.tsx`
- Create: `apps/web/src/features/configuration/GeneralSection.tsx`
- Create: `apps/web/src/features/configuration/ServicesSection.tsx`
- Create: `apps/web/src/features/configuration/AuthSection.tsx`
- Create: `apps/web/src/features/configuration/SMTPSection.tsx`
- Create: `apps/web/src/features/configuration/OAuthSection.tsx`
- Create: `apps/web/src/features/configuration/StorageSection.tsx`
- Create: `apps/web/src/features/configuration/RealtimeSection.tsx`
- Create: `apps/web/src/features/configuration/FunctionsSection.tsx`
- Create: `apps/web/src/features/configuration/DatabaseSection.tsx`
- Create: `apps/web/src/features/configuration/NetworkSection.tsx`
- Create: `apps/web/src/features/configuration/SecretsSection.tsx`
- Create: `apps/web/src/features/configuration/useConfigurationMutation.ts`
- Create: `apps/web/src/components/ui/tabs.tsx`
- Create: `apps/web/src/components/ui/form.tsx`
- Create: `apps/web/src/components/ui/input.tsx`
- Create: `apps/web/src/components/ui/select.tsx`
- Create: `apps/web/src/components/ui/switch.tsx`
- Create: `apps/web/src/components/ui/collapsible.tsx`
- Create: `apps/web/src/components/ui/table.tsx`
- Create: `apps/web/src/components/ui/skeleton.tsx`
- Modify: `apps/web/src/app/router.tsx`
- Modify: `apps/web/src/features/project/ProjectLayout.tsx`
- Modify: `apps/web/src/api/types.ts`

**Interfaces:**
- Consumes: configuration GET/PATCH endpoints and `OperationPanel` from Tasks 5 and 7.
- Produces: `/projects/:projectId/configuration` and all installed-project typed forms.

- [ ] **Step 1: Write failing redaction, dirty-state, affected-service, and conflict tests**

Assert initial secret fields are empty with “Configured” badges; saving an unchanged SMTP password sends `{action: "retain"}`; replacing it sends `{action: "replace", value: "new-value"}`; update preview says “Auth will be recreated”; 409 shows a reload action and does not overwrite form state; success opens the returned operation and refetches configuration after it finishes.

- [ ] **Step 2: Run focused configuration-page tests and verify failure**

Run: `npm run test --workspace apps/web -- --run src/features/configuration/ConfigurationPage.test.tsx`

Expected: FAIL because the page does not exist.

- [ ] **Step 3: Build the configuration shell and section query model**

Use query key `['project-configuration', projectId]`. Route the existing Authentication, Database, Storage, Realtime, Functions, Pooler, Network, Secrets, and Settings project navigation items into the relevant section or redirect them to `/configuration?section=<name>`; add a first-class Configuration item. Avoid duplicate forms with different persistence behavior.

- [ ] **Step 4: Implement typed sections and secret controls**

Each section owns one React Hook Form/Zod schema, displays Advanced fields with Collapsible, and calls one section PATCH endpoint with `expectedRevision`. SMTP, OAuth, S3, and Functions secret controls explicitly support retain/replace/remove. OAuth callback URLs are derived and copyable read-only fields.

`SecretsSection` displays Project URL and Anon Key, keeps Service Role Key, JWT secret, and database password hidden, and requires password re-entry before reveal/copy. Revealed values stay in component memory only, clear on section change or timeout, and are never written to local/session storage or TanStack Query cache. Database password rotation uses a separate AlertDialog with the PRD warning.

- [ ] **Step 5: Add update preview and operation flow**

Before mutation, show an AlertDialog listing changed field labels, affected services, and restart/recreate behavior. On 202, open `OperationPanel`; on success invalidate configuration and project queries; on failure show the rollback result. Use Sonner only for concise summaries while the operation panel retains details.

- [ ] **Step 6: Run configuration tests, full web tests, and build**

Run:

```bash
npm run test --workspace apps/web -- --run
npm run build --workspace apps/web
```

Expected: PASS with no accessibility-role query failures.

- [ ] **Step 7: Commit Task 8**

```bash
git add apps/web/src/features/configuration apps/web/src/components/ui apps/web/src/app/router.tsx apps/web/src/features/project/ProjectLayout.tsx apps/web/src/api/types.ts
git commit -m "feat: add project configuration workspace"
```

---

### Task 9: Complete shadcn Migration of Existing Screens

**Files:**
- Modify: `apps/web/src/features/auth/LoginPage.tsx`
- Modify: `apps/web/src/features/auth/SetupPage.tsx`
- Modify: `apps/web/src/features/projects/ProjectsPage.tsx`
- Modify: `apps/web/src/features/project/OverviewPage.tsx`
- Modify: `apps/web/src/features/project/LifecycleActions.tsx`
- Modify: `apps/web/src/features/project/ServiceTable.tsx`
- Modify: `apps/web/src/features/operations/OperationPanel.tsx`
- Modify: `apps/web/src/styles.css`
- Modify: existing corresponding `*.test.tsx` files

**Interfaces:**
- Consumes: Task 6 shadcn foundation.
- Produces: one coherent shadcn visual and interaction language across all existing screens.

- [ ] **Step 1: Add failing semantic component regression tests**

Assert setup/login fields have labels and descriptions; lifecycle buttons expose disabled/busy state; Projects uses Card/Table/Badge semantics without duplicate New Project controls; OperationPanel uses Progress and live status; and every destructive action is an AlertDialog.

- [ ] **Step 2: Run all web tests to capture the pre-migration failures**

Run: `npm run test --workspace apps/web -- --run`

Expected: the new semantic assertions fail against the legacy class-based components.

- [ ] **Step 3: Convert existing screens component by component**

Replace legacy `.panel`, `.button`, `.badge`, raw dialog, raw progress bar, and ad-hoc form markup with the generated shadcn components. Keep feature behavior and test-visible copy stable unless the approved spec changes it. Remove CSS rules only after their last consumer is migrated.

- [ ] **Step 4: Verify theme, responsive behavior, and accessibility**

At 1440px and 768px widths verify sidebar collapse, wizard/configuration overflow, visible focus rings, keyboard dialog operation, non-color status text, and reduced motion. Fix token usage in `styles.css`; do not add feature-specific hard-coded colors when a semantic token exists.

- [ ] **Step 5: Run all web tests and production build**

Run:

```bash
npm run test --workspace apps/web -- --run
npm run lint --workspace apps/web
npm run build --workspace apps/web
```

Expected: PASS.

- [ ] **Step 6: Commit Task 9**

```bash
git add apps/web
git commit -m "refactor: complete shadcn interface migration"
```

---

### Task 10: End-to-End Acceptance, Security Audit, and Operations Documentation

**Files:**
- Create: `tests/integration/configuration_reconcile_test.go`
- Create: `docs/operations/project-configuration.md`
- Create: `README.md`
- Modify: `tests/integration/installer_test.go`
- Modify: `deploy/.env.example`

**Interfaces:**
- Consumes: the complete release from Tasks 1–9.
- Produces: reproducible proof that browser configuration changes affect the real pinned Supabase runtime safely.

- [ ] **Step 1: Add a failing real-runtime acceptance test**

The real-runtime test creates a Custom project with Auth and custom SMTP, waits for healthy, verifies rendered configuration without printing secrets, enables Google OAuth, confirms only Auth is recreated, reads `/auth/v1/settings`, changes Functions environment, and confirms only Functions is recreated. A separate deterministic integration test injects an inspector failure after candidate recreation, verifies prior files are restored, verifies the prior Auth is recreated and healthy, and asserts the update operation reports rollback. Do not use an unreachable SMTP host as the rollback trigger because SMTP reachability is not an Auth liveness guarantee.

- [ ] **Step 2: Run focused acceptance and verify the test detects an unmet behavior**

Run: `GOCACHE=/tmp/supabase-installer-go-cache go test ./tests/integration -run TestConfigurationReconcile -v`

Expected before final fixes: FAIL on the first unmet integration assertion; after implementation: PASS.

- [ ] **Step 3: Document safe configuration behavior**

Document every UI section, secret retain/replace/remove semantics, which service is restarted or recreated per setting, rollback behavior, external reverse proxy expectations, and how to inspect an operation without exposing `.env` contents. State that raw `.env` and Compose editing are intentionally unavailable.

- [ ] **Step 4: Run the full verification matrix**

Run:

```bash
GOCACHE=/tmp/supabase-installer-go-cache go test ./...
GOCACHE=/tmp/supabase-installer-go-cache go test -race ./apps/manager/... ./apps/provisioner/... ./internal/...
GOCACHE=/tmp/supabase-installer-go-cache go vet ./...
npm run test --workspace apps/web -- --run
npm run lint --workspace apps/web
npm run build --workspace apps/web
docker compose -f deploy/docker-compose.yml --env-file deploy/.env config --quiet
```

Expected: every command exits 0.

- [ ] **Step 5: Run browser acceptance through the single public URL**

Start both product containers using the existing top-level Compose deployment. Through the browser, verify Manager Settings, absence of the sidebar New Project link, Custom wizard configuration, install-success navigation, installed-project configuration changes, rollback feedback, and delete-success list refresh.

- [ ] **Step 6: Audit for secret exposure and fixed Lightweight remnants**

Run:

```bash
rg -n 'csrfToken|SMTP_PASS|AWS_SECRET_ACCESS_KEY|GOTRUE_EXTERNAL_.*_SECRET|SERVICE_ROLE_KEY' apps/web apps/manager apps/provisioner
rg -n 'Lightweight\(|lightweightServiceNames|preset-card disabled|href="/api/session"' apps internal
```

Expected: CSRF exists only in the API client/session bootstrap, secret names appear only in typed mapping/redaction tests, and no fixed Lightweight renderer or raw session link remains.

- [ ] **Step 7: Commit Task 10**

```bash
git add tests docs README.md deploy/.env.example
git commit -m "test: verify configurable Supabase runtime management"
```
