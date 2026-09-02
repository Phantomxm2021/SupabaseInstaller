# Phase 1 Configuration Remediation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix CFG-001–004 and CFG-006–010 without rotating secrets or copying object bytes.

**Architecture:** Manager validates typed policy, the renderer writes container settings, and Provisioner queries `storage.objects` before a backend location is changed. Supavisor's boot script becomes an idempotent tenant upsert.

**Tech Stack:** Go, React/TypeScript/Zod, Docker Compose, PostgreSQL, Elixir, Vitest.

**Spec:** `docs/superpowers/specs/2026-09-03-configuration-remediation-design.md`

## Global Constraints

- Reject non-empty Storage location changes before publishing Compose.
- Keep legacy Caddy runtime untouched; reject new Caddy configuration patches.
- Every production behavior must be introduced only after a focused test fails.

---

### Task 1: Typed policy and validation

**Files:**
- Modify: `internal/contracts/configuration.go`
- Modify: `apps/manager/internal/project/configuration.go`
- Test: `apps/manager/internal/project/configuration_test.go`

**Produces:** `StorageConfig.UploadFileSizeLimit int64`, default 50 MiB, bounded 1 MiB–5 GiB. R2 requires a 32-character lowercase hexadecimal Account ID and path style. Phone MFA requires configured Phone Auth. New Caddy values are rejected.

- [ ] **Step 1: Write failing tests**

```go
func TestConfigurationRejectsPhoneMFAWithoutProvider(t *testing.T) {
    cfg := DefaultConfiguration(contracts.PresetLightweight)
    cfg.Auth.MFA.PhoneEnrollEnabled = true
    err := ValidateConfiguration(cfg)
    var validation *ValidationError
    if !errors.As(err, &validation) || validation.Fields["auth.mfa.phoneEnrollEnabled"] == "" { t.Fatal(err) }
}
```

- [ ] **Step 2: Verify RED** — run `go test ./apps/manager/internal/project -run TestConfigurationRejectsPhoneMFAWithoutProvider -count=1`; expect failure because no provider is required.

- [ ] **Step 3: Implement minimum validation**

```go
const defaultStorageUploadFileSizeLimit int64 = 50 * 1024 * 1024
var r2AccountIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
if mfa.PhoneEnrollEnabled || mfa.PhoneVerifyEnabled {
    if !auth.Phone.Enabled || auth.Phone.Provider == "" || !auth.Phone.SecretSet { validation.add("auth.mfa.phoneEnrollEnabled", "requires configured Phone Auth") }
}
```

Add size defaults/range and R2/Caddy validation; preserve Caddy only when reading historical stored state.

- [ ] **Step 4: Verify GREEN** — run `go test ./apps/manager/internal/project -count=1`; expect pass.
- [ ] **Step 5: Commit** — commit the contract, validation, and tests as `fix: validate storage and auth configuration dependencies`.

### Task 2: R2 and file-limit rendering

**Files:**
- Modify: `apps/provisioner/internal/render/environment.go`
- Modify: `apps/provisioner/internal/render/render.go`
- Test: `apps/provisioner/internal/render/render_test.go`

**Produces:** R2 emits `GLOBAL_S3_FORCE_PATH_STYLE=true` and `TUS_ALLOW_S3_TAGS=false`; Storage receives `FILE_SIZE_LIMIT`; renderer rejects Caddy.

- [ ] **Step 1: Write failing test**

```go
func TestRenderR2ForcesCompatibleStorageOptions(t *testing.T) {
    cfg := testConfiguration(); cfg.Storage = r2StorageConfig()
    out, err := Project(Input{Slug: "r2", APIPort: 18001, Configuration: cfg})
    if err != nil { t.Fatal(err) }
    if !strings.Contains(out.Env, "GLOBAL_S3_FORCE_PATH_STYLE=true") || !strings.Contains(out.Compose, "TUS_ALLOW_S3_TAGS: \"false\"") { t.Fatal("R2 options missing") }
}
```

- [ ] **Step 2: Verify RED** — run `go test ./apps/provisioner/internal/render -run TestRenderR2ForcesCompatibleStorageOptions -count=1`; expect failure.

- [ ] **Step 3: Implement minimum render change**

```go
if cfg.Storage.Backend == contracts.StorageBackendR2 {
    values["GLOBAL_S3_FORCE_PATH_STYLE"] = "true"
    env["TUS_ALLOW_S3_TAGS"] = "false"
}
env["FILE_SIZE_LIMIT"] = "${STORAGE_FILE_SIZE_LIMIT}"
```

Wire file size into dotenv and Storage Compose, reject Caddy, update goldens.

- [ ] **Step 4: Verify GREEN** — run `go test ./apps/provisioner/internal/render -count=1`; expect pass.
- [ ] **Step 5: Commit** — commit as `fix: render compatible R2 storage settings`.

### Task 3: Safe Storage location transitions

**Files:**
- Modify: `apps/provisioner/internal/compose/runner.go`
- Modify: `apps/provisioner/internal/runtime/backend.go`
- Modify: `apps/provisioner/internal/runtime/reconcile.go`
- Test: `apps/provisioner/internal/compose/runner_test.go`
- Test: `apps/provisioner/internal/runtime/reconcile_test.go`

**Produces:** `LifecycleRunner.StorageObjectCount(ctx, project) (int64, error)`. Reconcile rejects a changed Storage location when `storage.objects` has rows before staging runtime files.

- [ ] **Step 1: Write failing test**

```go
func TestReconcileRejectsNonEmptyStorageLocationChangeBeforePublish(t *testing.T) {
    runner := &fakeReconcileRunner{storageObjects: 1}
    backend := NewBackend(root, runner, &sequenceInspector{})
    _, err := backend.Reconcile(context.Background(), changedStorageRequest)
    if err == nil || !strings.Contains(err.Error(), "Storage contains objects") { t.Fatal(err) }
    if len(runner.recreated) != 0 { t.Fatalf("recreated=%v", runner.recreated) }
}
```

- [ ] **Step 2: Verify RED** — run `go test ./apps/provisioner/internal/runtime -run TestReconcileRejectsNonEmptyStorageLocationChangeBeforePublish -count=1`; expect failure.

- [ ] **Step 3: Implement preflight**

```go
func storageLocationChanged(before, after contracts.StorageConfig) bool {
    return before.Backend != after.Backend || before.Bucket != after.Bucket || before.Region != after.Region || before.Endpoint != after.Endpoint || before.AccountID != after.AccountID || before.LocalPath != after.LocalPath
}
```

Use `docker compose exec -T db psql -v ON_ERROR_STOP=1 -U supabase_admin -d postgres -At -c "SELECT count(*) FROM storage.objects;"`, parse exactly one non-negative integer, and fail closed if count/query is unavailable. Add zero-count success coverage.

- [ ] **Step 4: Verify GREEN** — run `go test ./apps/provisioner/internal/compose ./apps/provisioner/internal/runtime -count=1`; expect pass.
- [ ] **Step 5: Commit** — commit as `fix: guard non-empty storage transitions`.

### Task 4: Supavisor upsert and Functions contract cleanup

**Files:**
- Modify: `internal/templates/self-hosted-v0.8.0/volumes/pooler/pooler.exs`
- Modify: `internal/contracts/configuration.go`
- Modify: `apps/manager/internal/project/configuration.go`
- Modify: `apps/manager/internal/store/migrations/002_project_configuration.sql`
- Modify: `apps/provisioner/internal/runtime/reconcile.go`
- Test: renderer, project, store, and runtime tests identified by `rg 'Directory|directory|pooler'`

**Produces:** Supavisor updates an existing tenant's capacity fields. `FunctionsConfig.Directory` is deleted; unknown historical JSON keys remain harmless.

- [ ] **Step 1: Write failing tests**

```go
func TestSupavisorBootstrapUpsertsExistingTenantLimits(t *testing.T) {
    script, _ := templates.Files().ReadFile("self-hosted-v0.8.0/volumes/pooler/pooler.exs")
    if !strings.Contains(string(script), "update_tenant") { t.Fatal("missing update path") }
}
```

```go
func TestFunctionsConfigurationDoesNotExposeDirectory(t *testing.T) {
    payload, _ := json.Marshal(DefaultConfiguration(contracts.PresetLightweight).Functions)
    if strings.Contains(string(payload), "directory") { t.Fatal(string(payload)) }
}
```

- [ ] **Step 2: Verify RED** — run the two targeted Go tests; expect both failures.

- [ ] **Step 3: Implement minimum behavior**

```elixir
case Supavisor.Tenants.get_tenant_by_external_id(params["external_id"]) do
  nil -> Supavisor.Tenants.create_tenant(params)
  tenant -> Supavisor.Tenants.update_tenant(tenant, Map.take(params, ["default_max_clients", "default_pool_size", "users"]))
end
```

Verify the exact Supavisor 2.9.5 `update_tenant` signature before change; preserve errors. Remove Functions directory DTO/default/migration/diff references but not managed function filesystem volumes.

- [ ] **Step 4: Verify GREEN** — run `go test ./apps/manager/internal/project ./apps/manager/internal/store ./apps/provisioner/internal/render ./apps/provisioner/internal/runtime -count=1`; expect pass.
- [ ] **Step 5: Commit** — commit as `fix: apply persistent pooler configuration`.

### Task 5: Align UI contracts and document status

**Files:**
- Modify: `apps/web/src/api/types.ts`
- Modify: `apps/web/src/features/project/configuration/schema.ts`
- Modify: `apps/web/src/features/project/configuration/StorageSection.tsx`
- Modify: `apps/web/src/features/project/configuration/FunctionsSection.tsx`
- Modify: `apps/web/src/features/project/configuration/NetworkSection.tsx`
- Modify: `apps/web/src/features/projects/StorageFunctionsStep.tsx`
- Test: related Vitest files
- Modify: `docs/audits/2026-09-03-configuration-official-alignment.md`

**Produces:** R2 path-style is forced/hidden; Account ID and upload limit are validated; Caddy and Functions directory are absent; audit marks only Phase 1 items fixed.

- [ ] **Step 1: Write failing UI test**

```tsx
it('does not offer Caddy managed HTTPS', () => {
  render(<NetworkSection initial={network} revision={1} siteURL="https://example.com" onSave={vi.fn()} onUploadTLS={vi.fn()} />)
  expect(screen.queryByText('Caddy managed')).not.toBeInTheDocument()
})
```

- [ ] **Step 2: Verify RED** — run `npm --prefix apps/web test -- --run NetworkSection`; expect failure.
- [ ] **Step 3: Implement UI** — map 1–5120 MiB to bytes, force/hide R2 path-style, validate 32-hex ID, remove Caddy and Functions directory from schemas/controls/fixtures.
- [ ] **Step 4: Verify GREEN** — run `npm --prefix apps/web test -- --run`; expect pass.
- [ ] **Step 5: Final verification and commit** — run `go test ./... && npm --prefix apps/web test -- --run && git diff --check`; update audit status; commit intended changes as `docs: record phase one configuration remediation`.
