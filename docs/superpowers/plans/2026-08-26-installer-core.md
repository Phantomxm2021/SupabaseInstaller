# Supabase Manager Installer Core Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver a deployable two-container Manager that boots from one Compose command, serves a React UI at one URL, creates isolated Lightweight Supabase projects, streams installation progress, and supports project start, stop, restart, delete, and health inspection.

**Architecture:** A Go Manager embeds the compiled React application, stores metadata in SQLite, encrypts project secrets, and exposes the public REST/SSE API. A private Go Provisioner owns the Docker Socket and controlled project filesystem, accepts typed authenticated commands, and runs pinned official Supabase Compose assets. Browser, Manager, and Provisioner share generated contracts; only Manager publishes a host port.

**Tech Stack:** Go 1.24+, React 19, TypeScript 5.9, Vite 7, React Router 7, TanStack Query 5, Zod 4, SQLite, Docker Engine API, Docker Compose v2, Vitest, Testing Library, Playwright.

**Spec:** `docs/superpowers/specs/2026-08-26-supabase-manager-v1-design.md`

## Global Constraints

- Production supports Linux `amd64` and `arm64`, Docker Engine 27+, and Docker Compose v2.
- macOS Apple Silicon Docker Desktop is a development and integration-test environment only.
- The browser and Manager container must never receive the Docker Socket.
- The Provisioner must have no published host port and no general shell endpoint.
- Each project must have unique Compose identity, network, data, Auth data, secrets, and ports.
- PostgreSQL, Envoy, Auth, PostgREST, Studio, and postgres-meta are enabled in Lightweight.
- Realtime, Storage, imgproxy, Functions, Supavisor, Logs/Logflare, Vector, and direct database exposure are disabled in Lightweight.
- All runtime images and the official Self-hosted template are pinned; `latest` and GitHub `master` are forbidden.
- Secret plaintext must not enter SQLite, logs, browser storage, operation events, or unencrypted backups.
- Official Supabase initialization SQL and role/schema creation logic must not be copied or modified.
- Production startup is one `docker compose up -d`; only one Manager URL is public.

## Planned File Structure

```text
go.mod                                      shared Go module
Makefile                                    repeatable build/test targets
package.json                                npm workspace entry point
apps/manager/cmd/manager/main.go            Manager process composition root
apps/manager/internal/...                   Manager domain, DB, HTTP, operations
apps/provisioner/cmd/provisioner/main.go    Provisioner process composition root
apps/provisioner/internal/...               private RPC, files, Compose, health
apps/web/src/...                            React routes and feature modules
internal/contracts/...                      shared Go request/response contracts
internal/templates/self-hosted-v0.8.0/...   pinned unmodified official assets
deploy/docker-compose.yml                   two-container production deployment
deploy/Dockerfile.manager                   React build + Go Manager image
deploy/Dockerfile.provisioner               minimal Provisioner image
tests/integration/...                       process and Docker acceptance tests
```

---

### Task 1: Reproducible Monorepo and Process Configuration

**Files:**
- Create: `go.mod`
- Create: `package.json`
- Create: `Makefile`
- Create: `.gitignore`
- Create: `apps/manager/internal/config/config.go`
- Create: `apps/manager/internal/config/config_test.go`
- Create: `apps/manager/cmd/manager/main.go`
- Create: `apps/provisioner/internal/config/config.go`
- Create: `apps/provisioner/internal/config/config_test.go`
- Create: `apps/provisioner/cmd/provisioner/main.go`

**Interfaces:**
- Produces: `managerconfig.Load() (Config, error)` with `ListenAddr`, `DatabasePath`, `MasterEncryptionKey`, `ProvisionerURL`, `ProvisionerToken`, and `WebDistPath`.
- Produces: `provisionerconfig.Load() (Config, error)` with `ListenAddr`, `ProjectRoot`, `DockerHost`, and `ManagerToken`.

- [ ] **Step 1: Write failing configuration tests**

```go
func TestLoadRejectsMissingManagerSecrets(t *testing.T) {
    t.Setenv("MASTER_ENCRYPTION_KEY", "")
    t.Setenv("PROVISIONER_TOKEN", "")
    _, err := Load()
    assert.ErrorContains(t, err, "MASTER_ENCRYPTION_KEY")
}

func TestProvisionerDefaultsToPrivateAddressAndProjectRoot(t *testing.T) {
    t.Setenv("MANAGER_TOKEN", strings.Repeat("a", 32))
    cfg, err := Load()
    require.NoError(t, err)
    assert.Equal(t, "0.0.0.0:9090", cfg.ListenAddr)
    assert.Equal(t, "/opt/supabase-manager/projects", cfg.ProjectRoot)
}
```

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./apps/manager/internal/config ./apps/provisioner/internal/config`
Expected: FAIL because both configuration packages are absent.

- [ ] **Step 3: Add the minimal module, workspace, configuration loaders, and process entry points**

Use `supabase-manager` as the Go module. Decode the encryption key from base64 and require exactly 32 bytes. Require a Provisioner token of at least 32 bytes. Keep both `main.go` files limited to loading configuration, constructing dependencies, starting an `http.Server`, and handling SIGTERM with a 15-second graceful shutdown.

- [ ] **Step 4: Add stable developer commands**

`Makefile` targets must be `test`, `test-go`, `test-web`, `build`, `lint`, and `integration`. `package.json` must define an npm workspace for `apps/web` and pin the package manager version.

- [ ] **Step 5: Run verification**

Run: `go test ./apps/manager/internal/config ./apps/provisioner/internal/config && go vet ./...`
Expected: PASS with no warnings.

- [ ] **Step 6: Commit**

```sh
git add go.mod package.json Makefile .gitignore apps/manager apps/provisioner
git commit -m "chore: scaffold manager and provisioner services"
```

---

### Task 2: Project Domain, Slugs, Presets, and Dependency Rules

**Files:**
- Create: `apps/manager/internal/project/model.go`
- Create: `apps/manager/internal/project/slug.go`
- Create: `apps/manager/internal/project/preset.go`
- Create: `apps/manager/internal/project/validate.go`
- Create: `apps/manager/internal/project/project_test.go`
- Create: `internal/contracts/project.go`

**Interfaces:**
- Produces: `project.NormalizeSlug(name string) string`.
- Produces: `project.ApplyPreset(preset Preset) Services`.
- Produces: `project.ValidateDraft(Draft) error`.
- Produces: contract types `Project`, `ProjectDraft`, `Services`, `ProjectStatus`, and `HealthStatus` with JSON field names consumed by the web app.

- [ ] **Step 1: Write failing domain tests**

```go
func TestNormalizeSlug(t *testing.T) {
    assert.Equal(t, "my-bee-2", NormalizeSlug("  My Bee 2!  "))
}

func TestLightweightPresetMatchesPRD(t *testing.T) {
    got := ApplyPreset(PresetLightweight)
    assert.True(t, got.Database && got.Gateway && got.Auth && got.REST && got.Studio && got.PostgresMeta)
    assert.False(t, got.Realtime || got.Storage || got.Imgproxy || got.Functions || got.Supavisor || got.Logs || got.Vector || got.DirectDB)
}

func TestStudioRequiresPostgresMeta(t *testing.T) {
    draft := validDraft()
    draft.Services.Studio = true
    draft.Services.PostgresMeta = false
    assert.ErrorContains(t, ValidateDraft(draft), "postgres-meta")
}

func TestDraftRequiresAbsoluteSiteURLAndDomain(t *testing.T) {
    draft := validDraft()
    draft.SiteURL = "localhost:3000"
    assert.ErrorContains(t, ValidateDraft(draft), "siteUrl")
}
```

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./apps/manager/internal/project`
Expected: FAIL because domain types and functions do not exist.

- [ ] **Step 3: Implement the minimal pure domain layer**

Statuses must include `DRAFT`, `INSTALLING`, `RUNNING`, `STOPPED`, `DEGRADED`, `FAILED`, and `DELETING`. Validate names, `[a-z0-9-]` slugs, DNS-style domains or localhost development hosts, absolute `http`/`https` Site URLs, pinned non-`latest` versions, and all Lightweight dependency invariants.

- [ ] **Step 4: Run domain tests and full Go tests**

Run: `go test ./apps/manager/internal/project && go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```sh
git add apps/manager/internal/project internal/contracts/project.go
git commit -m "feat: define project domain and lightweight preset"
```

---

### Task 3: SQLite Store, Migrations, and Encrypted Secrets

**Files:**
- Create: `apps/manager/internal/store/migrations/001_initial.sql`
- Create: `apps/manager/internal/store/sqlite.go`
- Create: `apps/manager/internal/store/projects.go`
- Create: `apps/manager/internal/store/operations.go`
- Create: `apps/manager/internal/store/store_test.go`
- Create: `apps/manager/internal/secrets/cipher.go`
- Create: `apps/manager/internal/secrets/cipher_test.go`

**Interfaces:**
- Produces: `secrets.NewCipher(key []byte) (*Cipher, error)`.
- Produces: `Cipher.Encrypt(projectID, kind string, plaintext []byte) (Envelope, error)` and `Decrypt(...)`.
- Produces: `store.Open(path string) (*Store, error)`, `CreateProject`, `GetProject`, `ListProjects`, `UpdateProjectStatus`, and transaction-scoped operation methods.

- [ ] **Step 1: Write failing encryption tests**

```go
func TestCipherRoundTripBindsProjectAndKind(t *testing.T) {
    cipher, _ := NewCipher(bytes.Repeat([]byte{7}, 32))
    env, err := cipher.Encrypt("project-a", "postgres-password", []byte("secret"))
    require.NoError(t, err)
    got, err := cipher.Decrypt("project-a", "postgres-password", env)
    require.NoError(t, err)
    assert.Equal(t, []byte("secret"), got)
    _, err = cipher.Decrypt("project-b", "postgres-password", env)
    assert.Error(t, err)
}
```

- [ ] **Step 2: Write failing persistence tests**

```go
func TestStorePersistsProjectWithoutPlaintextSecret(t *testing.T) {
    dbPath := filepath.Join(t.TempDir(), "manager.db")
    s := openTestStore(t, dbPath)
    require.NoError(t, s.CreateProject(ctx, projectFixture()))
    raw, err := os.ReadFile(dbPath)
    require.NoError(t, err)
    assert.NotContains(t, string(raw), "plain-postgres-password")
}
```

- [ ] **Step 3: Run tests and verify RED**

Run: `go test ./apps/manager/internal/secrets ./apps/manager/internal/store`
Expected: FAIL because cipher and store packages are absent.

- [ ] **Step 4: Implement AES-256-GCM and the initial migration**

Use a random 12-byte nonce per encryption and authenticated data `v1:<project-id>:<kind>`. Store envelope version, nonce, and ciphertext separately. The migration must create `admins`, `sessions`, `projects`, `project_services`, `project_configs`, `project_secrets`, `operations`, `operation_steps`, `operation_events`, `port_allocations`, and `schema_migrations` with foreign keys enabled.

- [ ] **Step 5: Implement transaction-bound store methods**

Use `modernc.org/sqlite` so Manager remains CGO-free. Enable WAL, foreign keys, a 5-second busy timeout, and a single writer connection. Never accept secret plaintext in generic project JSON.

- [ ] **Step 6: Run verification**

Run: `go test -race ./apps/manager/internal/secrets ./apps/manager/internal/store`
Expected: PASS and the plaintext assertion remains green.

- [ ] **Step 7: Commit**

```sh
git add apps/manager/internal/store apps/manager/internal/secrets
git commit -m "feat: persist projects and encrypt secrets"
```

---

### Task 4: Administrator Bootstrap and Secure Sessions

**Files:**
- Create: `apps/manager/internal/auth/service.go`
- Create: `apps/manager/internal/auth/password.go`
- Create: `apps/manager/internal/auth/service_test.go`
- Create: `apps/manager/internal/httpapi/auth_handlers.go`
- Create: `apps/manager/internal/httpapi/auth_middleware.go`
- Create: `apps/manager/internal/httpapi/auth_handlers_test.go`
- Create: `internal/contracts/auth.go`

**Interfaces:**
- Produces: `auth.Service.Bootstrap`, `Login`, `Authenticate`, `Logout`, and `ChangePassword`.
- Produces: `GET /api/setup/status`, `POST /api/setup`, `POST /api/session`, `DELETE /api/session`, and `GET /api/session`.

- [ ] **Step 1: Write failing service tests**

```go
func TestBootstrapCanOnlyCreateFirstAdmin(t *testing.T) {
    svc := newAuthService(t)
    first, err := svc.Bootstrap(ctx, "admin", "correct horse battery staple")
    require.NoError(t, err)
    assert.NotEmpty(t, first.RecoveryCodes)
    _, err = svc.Bootstrap(ctx, "other", "another secure password")
    assert.ErrorIs(t, err, ErrAlreadyBootstrapped)
}

func TestSessionStoresHashNotCookieValue(t *testing.T) {
    svc, store := newAuthServiceWithStore(t)
    session, _ := svc.Login(ctx, "admin", "correct horse battery staple")
    assert.NotContains(t, store.RawSessionRow(t), session.Token)
}
```

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./apps/manager/internal/auth ./apps/manager/internal/httpapi`
Expected: FAIL because auth behavior is missing.

- [ ] **Step 3: Implement Argon2id credentials and opaque sessions**

Use OWASP-compatible Argon2id parameters, constant-time verification, 32-byte random session tokens, SHA-256 session hashes, 12-hour idle expiry, and 7-day absolute expiry. Recovery codes are displayed once and stored as Argon2id hashes.

- [ ] **Step 4: Implement same-origin session handlers**

Use a `supabase_manager_session` HttpOnly cookie with `SameSite=Strict`, path `/`, and configurable Secure behavior. Require `Origin` to match the request origin on mutations and issue a synchronizer CSRF token for authenticated browser requests.

- [ ] **Step 5: Run verification**

Run: `go test -race ./apps/manager/internal/auth ./apps/manager/internal/httpapi`
Expected: PASS.

- [ ] **Step 6: Commit**

```sh
git add apps/manager/internal/auth apps/manager/internal/httpapi internal/contracts/auth.go
git commit -m "feat: add first-run admin and secure sessions"
```

---

### Task 5: Durable Operations and Collision-Free Port Allocation

**Files:**
- Create: `apps/manager/internal/operation/model.go`
- Create: `apps/manager/internal/operation/service.go`
- Create: `apps/manager/internal/operation/service_test.go`
- Create: `apps/manager/internal/ports/allocator.go`
- Create: `apps/manager/internal/ports/allocator_test.go`
- Create: `internal/contracts/operation.go`

**Interfaces:**
- Produces: `ports.Allocator.Reserve(ctx, projectID string, kind Kind) (int, error)` and `ReleaseProject`.
- Produces: `operation.Service.Create`, `StartStep`, `CompleteStep`, `Fail`, `BeginRollback`, `CompleteRollback`, `EventsAfter`, and `RecoverIncomplete`.

- [ ] **Step 1: Write failing allocator and state-machine tests**

```go
func TestAllocatorNeverReturnsReservedOrListeningPort(t *testing.T) {
    allocator := newAllocator(t, 18001, 18003, fakeListener{busy: map[int]bool{18002: true}})
    first, _ := allocator.Reserve(ctx, "bee", ports.API)
    second, _ := allocator.Reserve(ctx, "nomo", ports.API)
    assert.Equal(t, 18001, first)
    assert.Equal(t, 18003, second)
}

func TestFailedInstallCanEnterRollbackButCannotSucceed(t *testing.T) {
    svc := newOperationService(t)
    op := svc.Create(ctx, "bee", operation.Create)
    svc.Start(ctx, op.ID)
    svc.Fail(ctx, op.ID, "START_AUTH", errors.New("unhealthy"))
    assert.NoError(t, svc.BeginRollback(ctx, op.ID))
    assert.Error(t, svc.Succeed(ctx, op.ID))
}
```

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./apps/manager/internal/ports ./apps/manager/internal/operation`
Expected: FAIL because packages are absent.

- [ ] **Step 3: Implement transactional reservations and operation transitions**

Reserve ports inside an immediate SQLite transaction, probe host availability through an injected listener, and keep reservations for stopped projects. Persist monotonically increasing operation event sequence numbers. Enforce the transition graph from the approved design.

- [ ] **Step 4: Implement recovery classification**

`RecoverIncomplete` must return resumable read/check steps separately from ambiguous destructive steps. It must never automatically repeat data deletion or restore.

- [ ] **Step 5: Run verification**

Run: `go test -race ./apps/manager/internal/ports ./apps/manager/internal/operation`
Expected: PASS.

- [ ] **Step 6: Commit**

```sh
git add apps/manager/internal/operation apps/manager/internal/ports internal/contracts/operation.go
git commit -m "feat: add durable operations and port allocation"
```

---

### Task 6: Typed Provisioner Protocol and Filesystem Sandbox

**Files:**
- Create: `internal/contracts/provisioner.go`
- Create: `apps/provisioner/internal/auth/middleware.go`
- Create: `apps/provisioner/internal/projectfs/root.go`
- Create: `apps/provisioner/internal/projectfs/root_test.go`
- Create: `apps/provisioner/internal/server/server.go`
- Create: `apps/provisioner/internal/server/server_test.go`
- Create: `apps/manager/internal/provisioner/client.go`
- Create: `apps/manager/internal/provisioner/client_test.go`

**Interfaces:**
- Produces: typed `PrepareProjectRequest`, `LifecycleRequest`, `InspectProjectResponse`, `OperationEvent`, and error envelopes.
- Produces: private endpoints `/internal/v1/projects/prepare`, `/lifecycle`, `/inspect`, and `/operations/:id/events`.

- [ ] **Step 1: Write failing path-containment tests**

```go
func TestProjectPathRejectsTraversalAndAbsoluteInput(t *testing.T) {
    root := New(t.TempDir())
    for _, slug := range []string{"../escape", "/tmp/escape", "bee/../../escape", "Bee"} {
        _, err := root.ProjectPath(slug)
        assert.Error(t, err, slug)
    }
}
```

- [ ] **Step 2: Write failing private-auth and stale-revision tests**

```go
func TestProvisionerRejectsMissingServiceToken(t *testing.T) {
    response := serveRequest(t, http.MethodPost, "/internal/v1/projects/prepare", nil, "")
    assert.Equal(t, http.StatusUnauthorized, response.Code)
}

func TestProvisionerRejectsStaleConfigRevision(t *testing.T) {
    response := prepareWithRevision(t, 3, existingRevision(4))
    assert.Equal(t, http.StatusConflict, response.Code)
}
```

- [ ] **Step 3: Run tests and verify RED**

Run: `go test ./apps/provisioner/internal/... ./apps/manager/internal/provisioner`
Expected: FAIL because protocol and sandbox do not exist.

- [ ] **Step 4: Implement explicit contracts and containment**

Accept only lowercase validated slugs and resolve paths using `filepath.Clean`, `filepath.Rel`, and a root-prefix check. Create staging directories with mode `0700`, secret files with `0600`, and atomically rename completed configurations.

- [ ] **Step 5: Implement private authentication and idempotency**

Use constant-time comparison of a 32-byte bearer token, require operation and idempotency IDs, persist the latest project revision in `project.json`, and return the prior response for an already completed idempotency key.

- [ ] **Step 6: Run verification**

Run: `go test -race ./apps/provisioner/internal/... ./apps/manager/internal/provisioner`
Expected: PASS.

- [ ] **Step 7: Commit**

```sh
git add internal/contracts/provisioner.go apps/provisioner/internal apps/manager/internal/provisioner
git commit -m "feat: add private typed provisioner protocol"
```

---

### Task 7: Pinned Official Template and Lightweight Configuration Renderer

**Files:**
- Create: `internal/templates/manifest.json`
- Create: `internal/templates/embed.go`
- Vendor unchanged: `internal/templates/self-hosted-v0.8.0/`
- Create: `apps/provisioner/internal/render/render.go`
- Create: `apps/provisioner/internal/render/render_test.go`
- Create: `apps/provisioner/internal/render/testdata/lightweight.golden.yml`
- Create: `apps/provisioner/internal/secrets/generate.go`
- Create: `apps/provisioner/internal/secrets/generate_test.go`
- Create: `scripts/verify-template.sh`

**Interfaces:**
- Produces: `render.Lightweight(Input) (OutputFiles, error)`.
- Produces: `secrets.Generate(io.Reader) (ProjectSecrets, error)` with independent database, JWT, anon, service-role, dashboard, and internal secrets.

- [ ] **Step 1: Pin and verify the official source**

Copy the unmodified `docker/` directory from the official `self-hosted/v0.8.0` tag. Record the tag, upstream commit, file SHA-256 hashes, and supported architectures in `manifest.json`. `scripts/verify-template.sh` must fail if a vendored official file differs from the manifest.

- [ ] **Step 2: Write failing renderer and secret tests**

```go
func TestLightweightRenderUsesPinnedImagesAndUniqueComposeName(t *testing.T) {
    out, err := Lightweight(validInput("bee", 18001))
    require.NoError(t, err)
    assert.Contains(t, out.Env, "SUPABASE_PUBLIC_URL=https://bee.example.com")
    assert.NotContains(t, out.Compose, ":latest")
    assert.NotContains(t, out.Compose, "realtime:")
    assert.NotContains(t, out.Compose, "storage:")
    assert.Equal(t, "supabase-manager-bee", out.ComposeProjectName)
}

func TestGenerateProducesDifferentSecretsPerProject(t *testing.T) {
    a, _ := Generate(rand.Reader)
    b, _ := Generate(rand.Reader)
    assert.NotEqual(t, a.JWTSecret, b.JWTSecret)
    assert.NotEqual(t, a.ServiceRoleKey, b.ServiceRoleKey)
}
```

- [ ] **Step 3: Run tests and verify RED**

Run: `go test ./apps/provisioner/internal/render ./apps/provisioner/internal/secrets`
Expected: FAIL because renderer and generator are absent.

- [ ] **Step 4: Implement deterministic layered rendering**

Never alter upstream SQL or initialization assets. Copy the pinned template, generate `.env`, `.env.functions`, `project.json`, and a Manager-owned override that binds the project gateway to `127.0.0.1:<allocated-port>`. Remove optional services and their `depends_on` edges for Lightweight while preserving PostgreSQL, Envoy, Auth, REST, Studio, and postgres-meta.

- [ ] **Step 5: Implement cryptographic project secret generation**

Use `crypto/rand`, generate at least 256 bits for database/JWT/internal secrets, create valid anon/service-role keys with distinct claims, and redact all string formatting. Do not use upstream placeholder values.

- [ ] **Step 6: Verify output**

Run: `sh scripts/verify-template.sh && go test ./apps/provisioner/internal/render ./apps/provisioner/internal/secrets && docker compose -f apps/provisioner/internal/render/testdata/lightweight.golden.yml config --quiet`
Expected: all commands PASS; Compose emits no interpolation or schema error.

- [ ] **Step 7: Commit**

```sh
git add internal/templates apps/provisioner/internal/render apps/provisioner/internal/secrets scripts/verify-template.sh
git commit -m "feat: render pinned lightweight Supabase runtime"
```

---

### Task 8: Controlled Compose Lifecycle, Health, Logs, and Rollback

**Files:**
- Create: `apps/provisioner/internal/compose/runner.go`
- Create: `apps/provisioner/internal/compose/runner_test.go`
- Create: `apps/provisioner/internal/health/inspect.go`
- Create: `apps/provisioner/internal/health/inspect_test.go`
- Create: `apps/provisioner/internal/redact/redact.go`
- Create: `apps/provisioner/internal/redact/redact_test.go`
- Modify: `apps/provisioner/internal/server/server.go`

**Interfaces:**
- Produces: `compose.Runner.Pull`, `UpDatabase`, `UpServices`, `Stop`, `Restart`, `DownRuntime`, and `RemoveTemporary`.
- Produces: `health.Inspector.Project(ctx, ProjectRef) (HealthReport, error)`.

- [ ] **Step 1: Write failing command-construction tests**

```go
func TestRunnerUsesArgumentVectorAndFixedProjectDirectory(t *testing.T) {
    exec := &fakeExecutor{}
    runner := NewRunner(exec)
    _ = runner.UpDatabase(ctx, ProjectRef{Slug: "bee", Dir: "/projects/bee"})
    assert.Equal(t, "docker", exec.Command)
    assert.Equal(t, []string{"compose", "--project-directory", "/projects/bee", "--project-name", "supabase-manager-bee", "up", "-d", "--wait", "db"}, exec.Args)
}
```

- [ ] **Step 2: Write failing redaction and health tests**

```go
func TestRedactorRemovesKnownValuesAndCredentialAssignments(t *testing.T) {
    r := New([]string{"actual-secret"})
    got := r.String("Authorization: Bearer actual-secret POSTGRES_PASSWORD=hunter2")
    assert.NotContains(t, got, "actual-secret")
    assert.NotContains(t, got, "hunter2")
}
```

- [ ] **Step 3: Run tests and verify RED**

Run: `go test ./apps/provisioner/internal/compose ./apps/provisioner/internal/health ./apps/provisioner/internal/redact`
Expected: FAIL because packages are absent.

- [ ] **Step 4: Implement a non-shell executor and dependency-ordered lifecycle**

Use `exec.CommandContext` with fixed executable and individual arguments. Pass a minimal allow-listed environment. Start `db` first with `--wait`, then gateway dependencies/Auth/REST, then postgres-meta and Studio. Stop retains volumes. Delete runtime runs `down --remove-orphans` without `-v`; data removal is a separate exact-confirmation command.

- [ ] **Step 5: Implement Docker inspection and health derivation**

Inspect containers by Compose project labels through the Docker Engine client. Derive project health using the approved state rules and return per-service status without environment values.

- [ ] **Step 6: Implement compensating cleanup**

On failed creation, stop containers, remove temporary containers and the project network, preserve volume directories, and report every cleanup step as a redacted event.

- [ ] **Step 7: Run verification**

Run: `go test -race ./apps/provisioner/internal/...`
Expected: PASS.

- [ ] **Step 8: Commit**

```sh
git add apps/provisioner/internal
git commit -m "feat: manage compose lifecycle and project health"
```

---

### Task 9: Manager Project API, Install Orchestrator, and SSE

**Files:**
- Create: `apps/manager/internal/project/service.go`
- Create: `apps/manager/internal/project/service_test.go`
- Create: `apps/manager/internal/install/orchestrator.go`
- Create: `apps/manager/internal/install/orchestrator_test.go`
- Create: `apps/manager/internal/httpapi/projects.go`
- Create: `apps/manager/internal/httpapi/projects_test.go`
- Create: `apps/manager/internal/httpapi/operations.go`
- Create: `apps/manager/internal/httpapi/operations_test.go`
- Create: `apps/manager/internal/httpapi/router.go`

**Interfaces:**
- Produces: PRD project CRUD and lifecycle routes.
- Produces: `GET /api/operations/:id` and `GET /api/operations/:id/events` SSE with `Last-Event-ID` replay.

- [ ] **Step 1: Write failing API behavior tests**

```go
func TestCreateProjectReturnsOperationAndNeverSecret(t *testing.T) {
    api := newAuthenticatedAPI(t)
    response := api.PostJSON("/api/projects", lightweightDraft())
    assert.Equal(t, http.StatusAccepted, response.Code)
    assert.JSONEq(t, `{"projectId":"project-1","operationId":"operation-1"}`, response.Body.String())
    assert.NotContains(t, response.Body.String(), "postgresPassword")
}

func TestOperationEventsResumeAfterLastEventID(t *testing.T) {
    api := newAuthenticatedAPIWithEvents(t, 1, 2, 3)
    response := api.GetSSE("/api/operations/op-1/events", map[string]string{"Last-Event-ID": "1"})
    assert.Equal(t, []int64{2, 3}, response.EventIDs())
}
```

- [ ] **Step 2: Write failing orchestration rollback test**

```go
func TestInstallFailureRestoresSafeMetadataAndPreservesData(t *testing.T) {
    provisioner := fakeProvisioner{failAt: "START_AUTH"}
    orchestrator := newOrchestrator(t, provisioner)
    result := orchestrator.Install(ctx, projectFixture())
    assert.Equal(t, operation.RolledBack, result.Status)
    assert.True(t, provisioner.RuntimeRemoved)
    assert.False(t, provisioner.DataRemoved)
}
```

- [ ] **Step 3: Run tests and verify RED**

Run: `go test ./apps/manager/internal/project ./apps/manager/internal/install ./apps/manager/internal/httpapi`
Expected: FAIL because public project behavior is absent.

- [ ] **Step 4: Implement authenticated project CRUD and lifecycle endpoints**

Create returns `202 Accepted`; validation failures return `422` with field errors; collisions return `409`; operations use stable machine error codes and correlation IDs. Delete runtime and delete data are separate request modes, with exact project-name confirmation required for data.

- [ ] **Step 5: Implement the 29-step installation pipeline**

Persist each named PRD step, generate and encrypt secrets before Provisioner preparation, reserve ports transactionally, call typed Provisioner methods, verify final health, and mark the project running only after the final health check. Roll back in reverse order on failure.

- [ ] **Step 6: Implement SSE replay and heartbeat**

Replay persisted events after `Last-Event-ID`, then stream new events with a 15-second comment heartbeat. Stop promptly on request cancellation and never include raw Provisioner logs before redaction.

- [ ] **Step 7: Run verification**

Run: `go test -race ./apps/manager/internal/...`
Expected: PASS.

- [ ] **Step 8: Commit**

```sh
git add apps/manager/internal
git commit -m "feat: expose project install and lifecycle API"
```

---

### Task 10: React Shell, Setup, Login, and Projects Dashboard

**Files:**
- Create: `apps/web/package.json`
- Create: `apps/web/vite.config.ts`
- Create: `apps/web/tsconfig.json`
- Create: `apps/web/src/main.tsx`
- Create: `apps/web/src/app/router.tsx`
- Create: `apps/web/src/app/AppShell.tsx`
- Create: `apps/web/src/api/client.ts`
- Create: `apps/web/src/features/auth/SetupPage.tsx`
- Create: `apps/web/src/features/auth/LoginPage.tsx`
- Create: `apps/web/src/features/projects/ProjectsPage.tsx`
- Create: `apps/web/src/styles.css`
- Create: `apps/web/src/test/setup.ts`
- Create: `apps/web/src/features/auth/SetupPage.test.tsx`
- Create: `apps/web/src/features/projects/ProjectsPage.test.tsx`

**Interfaces:**
- Consumes: setup/session and project list API contracts.
- Produces: `/setup`, `/login`, and `/projects` routes with authenticated route guards.

- [ ] **Step 1: Write failing setup flow test**

```tsx
it('creates the first administrator and displays recovery codes once', async () => {
  renderApp('/setup', serverWithSetupRequired())
  await user.type(screen.getByLabelText('Username'), 'admin')
  await user.type(screen.getByLabelText('Password'), 'correct horse battery staple')
  await user.type(screen.getByLabelText('Confirm password'), 'correct horse battery staple')
  await user.click(screen.getByRole('button', { name: 'Create administrator' }))
  expect(await screen.findByRole('heading', { name: 'Save your recovery codes' })).toBeVisible()
})
```

- [ ] **Step 2: Write failing project dashboard test**

```tsx
it('shows project health and the new project action', async () => {
  renderApp('/projects', serverWithProjects([{ name: 'Bee', status: 'RUNNING', health: 'HEALTHY' }]))
  expect(await screen.findByText('Bee')).toBeVisible()
  expect(screen.getByText('Healthy')).toBeVisible()
  expect(screen.getByRole('link', { name: 'New project' })).toHaveAttribute('href', '/projects/new')
})
```

- [ ] **Step 3: Run tests and verify RED**

Run: `npm install && npm run test --workspace apps/web -- --run`
Expected: FAIL because the React application is absent.

- [ ] **Step 4: Implement the accessible application shell and auth routes**

Use same-origin `fetch` with credentials, a CSRF header on mutations, Query error boundaries, and no browser secret persistence. Add compact responsive navigation, original neutral/green styling, light/dark theme, keyboard focus states, and status text that does not rely on color.

- [ ] **Step 5: Implement the Projects dashboard**

Show host resource summary placeholders backed by API fields, project cards/table, health/status badges, API URL, last operation, and empty/loading/error states. Keep destructive controls out of list rows.

- [ ] **Step 6: Run verification**

Run: `npm run test --workspace apps/web -- --run && npm run build --workspace apps/web`
Expected: PASS with a production `dist` directory.

- [ ] **Step 7: Commit**

```sh
git add apps/web package-lock.json
git commit -m "feat: add manager setup and projects dashboard"
```

---

### Task 11: Lightweight Project Wizard and Install Progress

**Files:**
- Create: `apps/web/src/features/projects/NewProjectPage.tsx`
- Create: `apps/web/src/features/projects/projectSchema.ts`
- Create: `apps/web/src/features/projects/BasicStep.tsx`
- Create: `apps/web/src/features/projects/PresetStep.tsx`
- Create: `apps/web/src/features/projects/ReviewStep.tsx`
- Create: `apps/web/src/features/operations/OperationPanel.tsx`
- Create: `apps/web/src/features/operations/useOperationEvents.ts`
- Create: `apps/web/src/features/projects/NewProjectPage.test.tsx`
- Create: `apps/web/src/features/operations/OperationPanel.test.tsx`

**Interfaces:**
- Consumes: `POST /api/projects`, operation snapshot, and operation SSE endpoints.
- Produces: `/projects/new` three-field fast path and reconnectable install progress UI.

- [ ] **Step 1: Write failing three-field wizard test**

```tsx
it('installs Lightweight after name, domain, and site URL', async () => {
  const api = serverCapturingProjectCreate()
  renderApp('/projects/new', api)
  await user.type(screen.getByLabelText('Project name'), 'Bee')
  expect(screen.getByLabelText('Project slug')).toHaveValue('bee')
  await user.type(screen.getByLabelText('Domain'), 'bee.example.com')
  await user.type(screen.getByLabelText('Site URL'), 'https://example.com')
  await user.click(screen.getByRole('button', { name: 'Review' }))
  expect(screen.getByText('Lightweight')).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Install project' }))
  expect(api.lastBody().preset).toBe('LIGHTWEIGHT')
})
```

- [ ] **Step 2: Write failing reconnectable progress test**

```tsx
it('resumes operation events and offers rollback after failure', async () => {
  render(<OperationPanel operationId="op-1" />, { wrapper: operationServerFailingAt('START_AUTH') })
  expect(await screen.findByText('Start Auth')).toBeVisible()
  expect(await screen.findByRole('button', { name: 'Rollback' })).toBeEnabled()
})
```

- [ ] **Step 3: Run tests and verify RED**

Run: `npm run test --workspace apps/web -- --run NewProjectPage OperationPanel`
Expected: FAIL because wizard and operation UI are absent.

- [ ] **Step 4: Implement Basic, Preset, Review, and progress experiences**

Default to Lightweight, auto-generate but permit editing a valid slug, show enabled and disabled service lists, warn on resource data from the API, and post only typed non-secret fields. Show all operation steps, timestamps, safe details, final links, retry eligibility, rollback, and log access.

- [ ] **Step 5: Run verification**

Run: `npm run test --workspace apps/web -- --run && npm run build --workspace apps/web`
Expected: PASS.

- [ ] **Step 6: Commit**

```sh
git add apps/web/src
git commit -m "feat: add lightweight installer wizard and progress"
```

---

### Task 12: Project Overview and Lifecycle Controls

**Files:**
- Create: `apps/web/src/features/project/ProjectLayout.tsx`
- Create: `apps/web/src/features/project/OverviewPage.tsx`
- Create: `apps/web/src/features/project/ServiceTable.tsx`
- Create: `apps/web/src/features/project/LifecycleActions.tsx`
- Create: `apps/web/src/features/project/DeleteProjectDialog.tsx`
- Create: `apps/web/src/features/project/OverviewPage.test.tsx`
- Create: `apps/web/src/features/project/DeleteProjectDialog.test.tsx`

**Interfaces:**
- Consumes: project detail, health, start, stop, restart, and delete API routes.
- Produces: `/projects/:id/overview` and the complete project navigation shell for later milestones.

- [ ] **Step 1: Write failing lifecycle and delete-safety tests**

```tsx
it('starts a stopped project through a durable operation', async () => {
  const api = serverWithStoppedProject()
  renderApp('/projects/bee/overview', api)
  await user.click(await screen.findByRole('button', { name: 'Start project' }))
  expect(api.lastPath()).toBe('/api/projects/bee/start')
  expect(await screen.findByText('Starting project')).toBeVisible()
})

it('requires the exact project name before deleting data', async () => {
  render(<DeleteProjectDialog project={{ id: 'bee', name: 'Bee' }} />)
  await user.click(screen.getByLabelText('Delete runtime and data'))
  await user.type(screen.getByLabelText('Type Bee to confirm'), 'bee')
  expect(screen.getByRole('button', { name: 'Delete permanently' })).toBeDisabled()
})
```

- [ ] **Step 2: Run tests and verify RED**

Run: `npm run test --workspace apps/web -- --run OverviewPage DeleteProjectDialog`
Expected: FAIL because project pages are absent.

- [ ] **Step 3: Implement overview, service state, and safe lifecycle controls**

Show API URL, allocated gateway port, current pinned version, project health, enabled/disabled services, and operation history. Default delete to runtime only. Require exact case-sensitive name confirmation and a separate checkbox for data removal.

- [ ] **Step 4: Run verification**

Run: `npm run test --workspace apps/web -- --run && npm run build --workspace apps/web`
Expected: PASS.

- [ ] **Step 5: Commit**

```sh
git add apps/web/src
git commit -m "feat: add project overview and lifecycle controls"
```

---

### Task 13: Embed the Web Build and Package the Two-Container Deployment

**Files:**
- Create: `apps/manager/webdist/embed.go`
- Modify: `apps/manager/cmd/manager/main.go`
- Create: `deploy/Dockerfile.manager`
- Create: `deploy/Dockerfile.provisioner`
- Create: `deploy/docker-compose.yml`
- Create: `deploy/.env.example`
- Create: `deploy/healthcheck.sh`
- Create: `tests/integration/deployment_test.go`
- Create: `docs/operations/install.md`

**Interfaces:**
- Produces: Manager health endpoints `/health/live` and `/health/ready`.
- Produces: one-command deployment exposing only `${MANAGER_PORT:-8080}`.

- [ ] **Step 1: Write failing deployment-structure test**

```go
func TestComposeExposesOnlyManagerAndMountsSocketOnlyIntoProvisioner(t *testing.T) {
    cfg := loadCompose(t, "../../deploy/docker-compose.yml")
    assert.Equal(t, []string{"${MANAGER_PORT:-8080}:8080"}, cfg.Services["manager"].Ports)
    assert.Empty(t, cfg.Services["provisioner"].Ports)
    assert.NotContains(t, strings.Join(cfg.Services["manager"].Volumes, " "), "docker.sock")
    assert.Contains(t, strings.Join(cfg.Services["provisioner"].Volumes, " "), "docker.sock")
}
```

- [ ] **Step 2: Run test and verify RED**

Run: `go test ./tests/integration -run TestComposeExposesOnlyManager`
Expected: FAIL because deployment files are absent.

- [ ] **Step 3: Implement the production image builds**

Use a Node build stage for React, Go build stages with `CGO_ENABLED=0`, and minimal non-root runtime stages. Embed `apps/web/dist` into Manager. Serve static routes with SPA fallback while never falling back for `/api` or health endpoints.

- [ ] **Step 4: Implement secure Compose deployment**

Use separate management and runtime-facing networks, read-only root filesystems where compatible, dropped Linux capabilities, named Manager data, explicit project-root bind mount for Provisioner, Docker Socket only in Provisioner, dependency health checks, and no Provisioner host port. Generate Manager/Provisioner secrets in documented setup commands rather than shipping defaults.

- [ ] **Step 5: Validate packaging**

Run: `docker compose -f deploy/docker-compose.yml config --quiet && docker build -f deploy/Dockerfile.manager . && docker build -f deploy/Dockerfile.provisioner .`
Expected: all commands PASS on Apple Silicon Docker Desktop.

- [ ] **Step 6: Run deployment contract tests**

Run: `go test ./tests/integration -run TestComposeExposesOnlyManager`
Expected: PASS.

- [ ] **Step 7: Commit**

```sh
git add apps/manager deploy tests/integration docs/operations/install.md
git commit -m "feat: package one-url two-container deployment"
```

---

### Task 14: Real Docker and Lightweight Supabase Acceptance

**Files:**
- Create: `tests/integration/manager_stack_test.go`
- Create: `tests/integration/lightweight_install_test.go`
- Create: `tests/integration/isolation_test.go`
- Create: `tests/integration/failure_rollback_test.go`
- Create: `tests/e2e/installer.spec.ts`
- Create: `playwright.config.ts`
- Create: `docs/operations/troubleshooting.md`

**Interfaces:**
- Consumes: complete Milestone 1 deployment and public API.
- Produces: executable acceptance evidence for one-URL startup, Lightweight installation, isolation, lifecycle, and rollback.

- [ ] **Step 1: Write acceptance tests before running the real stack**

The Go integration suite must create unique temporary project roots and Compose prefixes. The Playwright test must bootstrap the administrator, create Bee with three fields, observe all install steps, open Bee Overview, stop, start, and restart it, then delete runtime while retaining data.

- [ ] **Step 2: Run unit and contract suites first**

Run: `make test`
Expected: PASS before any long-running images are pulled.

- [ ] **Step 3: Start the Manager deployment**

Run: `docker compose -f deploy/docker-compose.yml --env-file deploy/.env.test up -d --build --wait`
Expected: Manager is healthy, Provisioner is healthy, only Manager publishes a host port.

- [ ] **Step 4: Run real Lightweight installation**

Run: `go test -tags=integration ./tests/integration -run 'TestManagerStack|TestLightweightInstall' -v -timeout 30m`
Expected: official pinned PostgreSQL, Envoy, Auth, PostgREST, Studio, and postgres-meta become healthy; optional services do not exist.

- [ ] **Step 5: Prove project isolation and rollback**

Run: `go test -tags=integration ./tests/integration -run 'TestProjectIsolation|TestFailedInstallRollback' -v -timeout 30m`
Expected: Bee and Nomo have different networks, volumes, database/JWT/service-role secrets, and allocated ports; injected Auth failure removes temporary runtime resources but retains the project data directory.

- [ ] **Step 6: Run browser acceptance**

Run: `npm run test:e2e`
Expected: PASS for bootstrap, create, progress, overview, stop/start/restart, and safe delete.

- [ ] **Step 7: Run final static and security checks**

Run: `go test -race ./... && go vet ./... && npm run test --workspace apps/web -- --run && npm run build --workspace apps/web && docker compose -f deploy/docker-compose.yml config --quiet`
Expected: PASS with no race, vet, test, build, or Compose errors.

- [ ] **Step 8: Document observed resource requirements and troubleshooting**

Record Docker Desktop memory allocation, image pull size, cold install duration, steady-state RAM, known ARM64 differences, and exact commands for redacted logs and safe cleanup. Do not include generated credentials.

- [ ] **Step 9: Commit**

```sh
git add tests playwright.config.ts docs/operations/troubleshooting.md
git commit -m "test: verify installer core on real Docker"
```

---

## Milestone 1 Completion Gate

Before claiming Installer Core complete, run every Task 14 verification command from a clean checkout and fresh Manager data directory. Confirm manually that:

- `docker compose up -d` starts Manager and Provisioner together;
- one URL serves setup, login, projects, wizard, progress, and overview;
- Provisioner has no host port and Manager has no Docker Socket mount;
- a three-field Lightweight install reaches healthy state;
- Studio, Auth, and REST respond through the allocated gateway;
- stopped projects retain volumes, configuration, secrets, and port reservations;
- delete defaults to runtime-only and exact-name confirmation protects data deletion;
- installation failure exposes retry/rollback while preserving data;
- no plaintext generated secret appears in SQLite, API responses, operation events, or logs.

Only after this gate passes should Milestone 2 receive its own detailed implementation plan.
