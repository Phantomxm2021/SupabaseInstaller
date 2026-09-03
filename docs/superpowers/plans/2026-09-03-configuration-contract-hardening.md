# Configuration Contract Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Align project configuration with documented Supabase self-hosted Auth, pooling, Postgres, and proxy behavior without silently changing existing deployments.

**Architecture:** Extend the existing typed aggregate with narrow compatibility fields. Manager is the authoritative validation boundary; Provisioner remains the sole renderer; legacy Caddy receives a migration-required failure, never an automatic network-mode switch.

**Tech Stack:** Go, SQLite, Docker Compose YAML rendering, React, TypeScript, Zod, Vitest.

**Spec:** `docs/superpowers/specs/2026-09-03-configuration-contract-hardening-design.md`

## Global Constraints

- `authSiteUrl` is optional; empty legacy state renders `https://<General.Domain>` without write-back.
- JWT expiry is `1..604800`; new default and legacy-zero render fallback are `3600`.
- `internalDbPoolSize` defaults to `5` and maps only to `POOLER_DB_POOL_SIZE`.
- Required database budget is `10 + enabled Realtime pool + enabled Supavisor business pool + enabled Supavisor internal pool`, strictly less than `database.maxConnections`.
- A nonempty `shared_buffers` is positive integer plus exactly `B`, `kB`, `MB`, `GB`, `TB`, `KiB`, `MiB`, `GiB`, or `TiB`.
- Legacy Caddy stays readable but cannot be prepared, reconciled, or rendered; never auto-switch it to external.

---

### Task 1: Auth URL and JWT compatibility contract

**Files:**
- Modify: `internal/contracts/configuration.go`
- Modify: `apps/manager/internal/project/configuration.go`
- Modify: `apps/manager/internal/project/configuration_service.go`
- Modify: `apps/provisioner/internal/render/environment.go`
- Test: `apps/manager/internal/project/configuration_test.go`
- Test: `apps/manager/internal/project/configuration_service_test.go`
- Test: `apps/provisioner/internal/render/render_test.go`

**Interfaces:** Add `GeneralConfig.AuthSiteURL string` with JSON name `authSiteUrl`. Add renderer helpers `effectiveAuthSiteURL(contracts.GeneralConfig) string` and `effectiveJWTExpiry(int) int`. Do not change `NormalizeProjectAddress`: existing `General.SiteURL` remains the manager base domain.

- [ ] **Step 1: Write failing behavior tests**

```go
func TestDefaultConfigurationUsesOfficialJWTExpiry(t *testing.T) {
	if got := DefaultConfiguration(contracts.PresetLightweight).Auth.JWTExpiry; got != 3600 {
		t.Fatalf("JWTExpiry = %d, want 3600", got)
	}
}
```

Add cases that incoming 0 and 604801 are rejected, stored zero is compatible, explicit `authSiteUrl=https://app.example.com` renders only `SITE_URL` from that URL, and empty `authSiteUrl` uses `https://<project-domain>`.

- [ ] **Step 2: Verify RED**

Run: `go test ./apps/manager/internal/project ./apps/provisioner/internal/render -run 'Test(DefaultConfigurationUsesOfficialJWTExpiry|RenderUsesExplicitAuthSiteURL)' -count=1`

Expected: FAIL because the default is zero and the explicit field does not exist.

- [ ] **Step 3: Implement minimum production behavior**

```go
func effectiveJWTExpiry(value int) int {
	if value == 0 {
		return 3600
	}
	return value
}
```

Add `AuthSiteURL`, validate nonempty absolute HTTP(S) URLs, enforce incoming 1–604800, normalize only stored zero, and render `SITE_URL` using the explicit URL or domain fallback. Keep public and Auth external origins derived from `Domain`.

- [ ] **Step 4: Verify GREEN**

Run: `go test ./apps/manager/internal/project ./apps/provisioner/internal/render -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

Run: `git add internal/contracts/configuration.go apps/manager/internal/project/configuration.go apps/manager/internal/project/configuration_service.go apps/provisioner/internal/render/environment.go apps/manager/internal/project/configuration_test.go apps/manager/internal/project/configuration_service_test.go apps/provisioner/internal/render/render_test.go && git commit -m 'fix(config): align auth URL and JWT expiry'`

### Task 2: Connection budget, independent Supavisor pool, and safe buffers

**Files:**
- Modify: `internal/contracts/configuration.go`
- Modify: `apps/manager/internal/project/configuration.go`
- Modify: `apps/manager/internal/project/configuration_service.go`
- Modify: `apps/provisioner/internal/render/environment.go`
- Test: `apps/manager/internal/project/configuration_test.go`
- Test: `apps/manager/internal/project/configuration_service_test.go`
- Test: `apps/provisioner/internal/render/render_test.go`

**Interfaces:** Add `PoolerConfig.InternalDBPoolSize int` with JSON name `internalDbPoolSize`. Add `fixedDatabaseConnectionReserve = 10`, aggregate budget validation that reports `database.maxConnections`, and `validSharedBuffers(string) bool`.

- [ ] **Step 1: Write failing behavior tests**

```go
func TestRenderKeepsSupavisorInternalPoolIndependent(t *testing.T) {
	// Default output must contain DEFAULT_POOL_SIZE=20 and DB_POOL_SIZE=5.
	// Changing only InternalDBPoolSize changes only DB_POOL_SIZE.
}
```

Add cases for Realtime/Supavisor pools plus reserve equaling the Postgres maximum, a single enabled pool above the maximum, disabled consumers, accepted `256MB`/`1GiB`, and rejected `0MB`, `128`, `-1MB`, `10XB`, and shell-like buffers.

- [ ] **Step 2: Verify RED**

Run: `go test ./apps/manager/internal/project ./apps/provisioner/internal/render -run 'Test.*(Pool|Budget|SharedBuffers)' -count=1`

Expected: FAIL because the internal pool is coupled to `PoolSize`, capacity is not aggregated, and buffers are free text.

- [ ] **Step 3: Implement minimum production behavior**

```go
required := fixedDatabaseConnectionReserve
if cfg.Services.Realtime {
	required += cfg.Realtime.DatabasePoolSize
}
if cfg.Services.Supavisor {
	required += cfg.Pooler.PoolSize + cfg.Pooler.InternalDBPoolSize
}
if required >= cfg.Database.MaxConnections {
	validation.add("database.maxConnections", "must exceed reserved service connection budget")
}
```

Default/normalize internal pool to 5, map it only to `POOLER_DB_POOL_SIZE`, validate each enabled pool against database max, and use an anchored memory-size regex with only the permitted units.

- [ ] **Step 4: Verify GREEN**

Run: `go test ./apps/manager/internal/project ./apps/provisioner/internal/render -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

Run: `git add internal/contracts/configuration.go apps/manager/internal/project/configuration.go apps/manager/internal/project/configuration_service.go apps/provisioner/internal/render/environment.go apps/manager/internal/project/configuration_test.go apps/manager/internal/project/configuration_service_test.go apps/provisioner/internal/render/render_test.go && git commit -m 'fix(config): enforce database connection budget'`

### Task 3: Web schema and configuration UI

**Files:**
- Modify: `apps/web/src/features/projects/projectSchema.ts`
- Modify: `apps/web/src/features/project/configuration/types.ts`
- Modify: `apps/web/src/features/project/configuration/schema.ts`
- Modify: `apps/web/src/features/project/configuration/wire.ts`
- Modify: `apps/web/src/features/projects/BasicStep.tsx`
- Modify: `apps/web/src/features/projects/wizard/SecurityIntegrationsStep.tsx`
- Modify: `apps/web/src/features/project/configuration/GeneralSection.tsx`
- Modify: `apps/web/src/features/project/configuration/PoolerSection.tsx`
- Modify: `apps/web/src/features/project/configuration/DatabaseSection.tsx`
- Test: `apps/web/src/features/projects/projectSchema.test.ts`
- Test: `apps/web/src/features/projects/NewProjectPage.test.tsx`
- Test: `apps/web/src/features/project/ConfigurationPage.test.tsx`

**Interfaces:** Consume `authSiteUrl`, 3600/604800 JWT bounds, `internalDbPoolSize`, and the shared-buffer grammar. Produce exact camelCase API payloads and no raw environment editor.

- [ ] **Step 1: Write failing Vitest coverage**

```ts
it('defaults a new project to official JWT expiry', () => {
  expect(defaultConfiguration('LIGHTWEIGHT').auth.jwtExpiry).toBe(3600)
})
```

Add cases that 0/604801 JWT inputs fail, the old field is labelled “Server base domain”, “Auth site URL” is exposed, pooler preserves `internalDbPoolSize`, invalid shared buffers block submit, and database copy warns that saving recreates Postgres and dependent services.

- [ ] **Step 2: Verify RED**

Run: `npm --prefix apps/web test -- --run src/features/projects/projectSchema.test.ts src/features/projects/NewProjectPage.test.tsx src/features/project/ConfigurationPage.test.tsx`

Expected: FAIL because the new fields, official default, and validation/copy do not exist.

- [ ] **Step 3: Implement schema and UI wiring**

Mirror exact server ranges and buffer grammar in Zod. Add the fields to project/configuration form and wire types. Derive new-project Auth URL from slug plus server base domain before submit while retaining domain derivation. Add labels and restart warning.

- [ ] **Step 4: Verify GREEN**

Run: `npm --prefix apps/web test -- --run`

Expected: PASS.

- [ ] **Step 5: Commit**

Run: `git add apps/web/src/features/projects/projectSchema.ts apps/web/src/features/project/configuration/types.ts apps/web/src/features/project/configuration/schema.ts apps/web/src/features/project/configuration/wire.ts apps/web/src/features/projects/BasicStep.tsx apps/web/src/features/projects/wizard/SecurityIntegrationsStep.tsx apps/web/src/features/project/configuration/GeneralSection.tsx apps/web/src/features/project/configuration/PoolerSection.tsx apps/web/src/features/project/configuration/DatabaseSection.tsx apps/web/src/features/projects/projectSchema.test.ts apps/web/src/features/projects/NewProjectPage.test.tsx apps/web/src/features/project/ConfigurationPage.test.tsx && git commit -m 'feat(web): expose hardened configuration controls'`

### Task 4: Legacy Caddy migration guard and verified closure

**Files:**
- Modify: `apps/manager/internal/project/configuration.go`
- Modify: `apps/manager/internal/project/configuration_service.go`
- Modify: `apps/provisioner/internal/render/render.go`
- Modify: `apps/manager/internal/project/configuration_service_test.go`
- Modify: `apps/provisioner/internal/render/render_test.go`
- Modify: `docs/operations/project-host-nginx.md`
- Modify: `docs/audits/2026-09-03-configuration-official-alignment.md`

**Interfaces:** Error text contains `network.httpsMode` and `external reverse proxy`. Stored legacy Caddy remains readable; an external-mode patch succeeds and Compose has no `caddy` service.

- [ ] **Step 1: Write failing Caddy tests**

```go
func TestPreparePatchRequiresLegacyCaddyMigration(t *testing.T) {
	// Unchanged stored Caddy must explain external reverse proxy migration.
}

func TestRenderRejectsCaddyBeforeComposeGeneration(t *testing.T) {
	// HTTPSModeCaddy must fail without returning Compose.
}
```

Also test patching the project to external succeeds and emits no Caddy service.

- [ ] **Step 2: Verify RED**

Run: `go test ./apps/manager/internal/project ./apps/provisioner/internal/render -run 'Test.*Caddy' -count=1`

Expected: FAIL because legacy Caddy currently passes preparation and renderer overlay selection.

- [ ] **Step 3: Implement migration-required guards**

Keep Store read behavior. Make `PreparePatch` reject candidates retaining Caddy with an external-proxy migration message. Make renderer reject Caddy before overlay merge. Do not touch upstream template or auto-switch a project.

- [ ] **Step 4: Document operator migration**

Document: configure external TLS proxy to each project loopback API port; verify API/Studio reachability; set external mode; reconcile. State Manager never auto-switches because that could cause outage.

- [ ] **Step 5: Verify stack and record closure**

Run: `go test ./... && npm --prefix apps/web test -- --run && npm --prefix apps/web run build && git diff --check`

Update CFG-011–CFG-017 to verified fixed or explicit operator migration guard status.

- [ ] **Step 6: Commit**

Run: `git add apps/manager/internal/project/configuration.go apps/manager/internal/project/configuration_service.go apps/provisioner/internal/render/render.go apps/manager/internal/project/configuration_service_test.go apps/provisioner/internal/render/render_test.go docs/operations/project-host-nginx.md docs/audits/2026-09-03-configuration-official-alignment.md && git commit -m 'fix(config): require legacy Caddy migration'`
