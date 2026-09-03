# Supported Runtime Upgrades Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every new server default to a concrete latest supported Supabase runtime version, expose the supported version catalog, and provide safe upgrade eligibility without accepting mutable Docker tags.

**Architecture:** A single embedded-template catalog supplies version metadata and template roots to Manager and Provisioner. The persisted configuration remains an exact version ID. The first delivery introduces the generic catalog and upgrade eligibility contract while `self-hosted/v0.8.0` is the sole tested upstream release; a runnable cross-version upgrade is enabled only in the same release that vendors and tests a second official template.

**Tech Stack:** Go 1.24, `embed.FS`, `net/http`, SQLite-backed Manager operations, React, TypeScript, TanStack Query, Vitest.

**Spec:** `docs/superpowers/specs/2026-09-03-supported-runtime-upgrades-design.md`

## Global Constraints

- `latest`, `master`, blank values, and arbitrary Docker image tags are invalid persisted runtime versions.
- Latest supported resolves to an exact catalog ID before it reaches SQLite or Compose.
- Existing `self-hosted/v0.8.0` rows stay readable and runnable without migration.
- Runtime-version changes must never be accepted through the normal configuration PATCH route.
- No runtime is stopped, rebuilt, or pulled when a target is missing, unsupported, identical, or requires a manual migration.
- No API response, event, or test output may expose secret values or rendered environment contents.

---

### Task 1: Create the shared embedded runtime catalog

**Files:**
- Create: `internal/templates/catalog.go`
- Create: `internal/templates/catalog_test.go`
- Modify: `internal/templates/embed.go`
- Modify: `apps/provisioner/internal/render/render.go`
- Test: `apps/provisioner/internal/render/render_test.go`

**Interfaces:**
- Produces `templates.RuntimeVersion`, `templates.SupportedVersions()`, `templates.LatestSupported()`, `templates.Lookup(id string)`, and `templates.Validate(id string)`.
- Consumes `contracts.ProjectConfiguration.General.SupabaseVersion` in the renderer.
- Produces a version-selected official Compose root for `loadOfficialCompose`.

- [ ] **Step 1: Write catalog tests before implementation**

```go
func TestCatalogResolvesOneConcreteLatestVersion(t *testing.T) {
    latest := LatestSupported()
    if latest.ID != "self-hosted/v0.8.0" || latest.TemplateRoot != "self-hosted-v0.8.0" {
        t.Fatalf("latest=%+v", latest)
    }
    if err := Validate("latest"); err == nil { t.Fatal("latest must be rejected") }
    if err := Validate("self-hosted/v9.9.9"); err == nil { t.Fatal("unknown version must be rejected") }
}
```

- [ ] **Step 2: Run the catalog test and confirm it fails**

Run: `go test ./internal/templates -run TestCatalogResolvesOneConcreteLatestVersion -count=1`

Expected: FAIL because the catalog symbols do not exist.

- [ ] **Step 3: Implement the immutable catalog**

```go
type MigrationClass string
const MigrationCompatible MigrationClass = "COMPATIBLE"
const MigrationManual MigrationClass = "MANUAL_MIGRATION_REQUIRED"

type RuntimeVersion struct {
    ID, Label, TemplateRoot string
    MigrationClass MigrationClass
    ReleaseNotesURL string
}

var supported = []RuntimeVersion{{
    ID: "self-hosted/v0.8.0", Label: "Latest supported — self-hosted/v0.8.0",
    TemplateRoot: "self-hosted-v0.8.0", MigrationClass: MigrationCompatible,
}}

func LatestSupported() RuntimeVersion { return supported[0] }
```

`Lookup` must return `(RuntimeVersion, bool)` and `Validate` must only accept an exact `ID`. Change `DockerCompose` and `EnvExample` to read from `LatestSupported().TemplateRoot`; leave their public signatures unchanged.

- [ ] **Step 4: Select the template through the catalog in the renderer**

Replace the hard-coded version equality check and `"self-hosted-v0.8.0/"` path in `render.go` with `templates.Lookup`. Return `general.supabaseVersion: unsupported pinned version` when lookup fails. Pass the resolved descriptor into `loadOfficialCompose` so all template reads use `descriptor.TemplateRoot + "/" + name`.

- [ ] **Step 5: Add renderer coverage for rejection and exact template selection**

```go
func TestProjectRejectsUnknownPinnedVersion(t *testing.T) {
    input := validInput()
    input.Configuration.General.SupabaseVersion = "latest"
    _, err := Project(input)
    if err == nil || !strings.Contains(err.Error(), "unsupported pinned version") {
        t.Fatalf("err=%v", err)
    }
}
```

- [ ] **Step 6: Run focused tests**

Run: `go test ./internal/templates ./apps/provisioner/internal/render -count=1`

Expected: PASS.

- [ ] **Step 7: Commit the catalog slice**

```bash
git add internal/templates apps/provisioner/internal/render
git commit -m "feat(runtime): add supported version catalog"
```

### Task 2: Make Manager defaults and validation catalog-backed

**Files:**
- Modify: `apps/manager/internal/project/configuration.go`
- Modify: `apps/manager/internal/project/validate.go`
- Modify: `apps/manager/internal/project/configuration_test.go`
- Modify: `apps/manager/internal/project/validate_test.go`
- Modify: `apps/web/src/features/projects/projectSchema.ts`
- Modify: `apps/web/src/features/projects/projectSchema.test.ts`

**Interfaces:**
- Consumes `templates.LatestSupported().ID` and `templates.Validate`.
- Produces persisted exact runtime version IDs from `DefaultConfiguration` and create-form defaults.

- [ ] **Step 1: Write failing default and legacy tests**

```go
func TestDefaultConfigurationUsesLatestSupportedVersion(t *testing.T) {
    got := DefaultConfiguration(contracts.PresetLightweight).General.SupabaseVersion
    if got != templates.LatestSupported().ID { t.Fatalf("version=%q", got) }
}

func TestValidateStoredConfigurationAcceptsExistingV080(t *testing.T) {
    cfg := DefaultConfiguration(contracts.PresetLightweight)
    cfg.General.SupabaseVersion = "self-hosted/v0.8.0"
    if err := ValidateStoredConfiguration(cfg); err != nil { t.Fatal(err) }
}
```

- [ ] **Step 2: Run the focused Manager tests and confirm the old literal assertion fails**

Run: `go test ./apps/manager/internal/project -run 'Test(DefaultConfigurationUsesLatestSupportedVersion|ValidateStoredConfigurationAcceptsExistingV080)' -count=1`

Expected: FAIL until the application imports the catalog.

- [ ] **Step 3: Replace version literals with catalog calls**

Use `templates.LatestSupported().ID` in `DefaultConfiguration`; use `templates.Validate` in complete and stored configuration validation. Keep the existing field error name `general.supabaseVersion` so browser validation remains stable. Change the TypeScript schema from `z.literal(SUPABASE_VERSION)` to a catalog-fed enum only after Task 3 exposes the public catalog; until then keep a local exact default and accept the server as authority.

- [ ] **Step 4: Make create-form version data API-backed**

Create `useRuntimeCatalog` next to the project create hooks. It fetches `/api/runtime-versions`, uses `latest.id` for `defaultConfiguration`, and does not submit an alias. While the catalog query is loading, the form uses the build-time `SUPABASE_VERSION` fallback; on success it updates untouched forms to the returned exact ID.

- [ ] **Step 5: Add form tests**

```tsx
it('uses the API latest supported ID for a pristine create form', async () => {
  mockFetch('/api/runtime-versions', { latest: { id: 'self-hosted/v0.8.0' }, versions: [] })
  renderNewProjectPage()
  expect(await screen.findByText('Latest supported — self-hosted/v0.8.0')).toBeVisible()
})
```

- [ ] **Step 6: Run Manager and Web focused tests**

Run: `go test ./apps/manager/internal/project -count=1 && npm --prefix apps/web test -- --run src/features/projects/projectSchema.test.ts src/features/projects/NewProjectPage.test.tsx`

Expected: PASS.

- [ ] **Step 7: Commit the default/validation slice**

```bash
git add apps/manager/internal/project apps/web/src/features/projects
git commit -m "feat(runtime): default new servers to latest supported"
```

### Task 3: Publish the catalog and upgrade eligibility over the protected API

**Files:**
- Create: `apps/manager/internal/runtimeversions/service.go`
- Create: `apps/manager/internal/runtimeversions/service_test.go`
- Create: `apps/manager/internal/httpapi/runtime_versions.go`
- Create: `apps/manager/internal/httpapi/runtime_versions_test.go`
- Modify: `apps/manager/internal/httpapi/router.go`
- Modify: `apps/web/src/api/types.ts`
- Modify: `apps/web/src/api/client.ts`

**Interfaces:**
- Produces `GET /api/runtime-versions` with `{latest, versions}` and no secrets.
- Produces `GET /api/projects/{id}/runtime-upgrade` with current version, latest version, eligibility, migration class, and reason.
- Consumes the catalog and project store; does not mutate projects.

- [ ] **Step 1: Write failing HTTP contract tests**

```go
func TestRuntimeVersionsReturnsConcreteLatest(t *testing.T) {
    request := httptest.NewRequest(http.MethodGet, "/api/runtime-versions", nil)
    response := httptest.NewRecorder()
    router.ServeHTTP(response, request)
    if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"id":"self-hosted/v0.8.0"`) {
        t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
    }
}
```

- [ ] **Step 2: Implement a read-only service**

Return browser-safe descriptors (`id`, `label`, `migrationClass`, `releaseNotesURL`, `isLatest`) from the template catalog. The eligibility service returns `eligible=false, reason="Already running the latest supported version"` for a project on the latest ID, and `eligible=false` for any target with `MANUAL_MIGRATION_REQUIRED`.

- [ ] **Step 3: Register protected routes and preserve auth behavior**

Register the catalog endpoint through the same protected mux used by project routes. Add `Cache-Control: no-store` to upgrade eligibility responses because project state can change while viewed.

- [ ] **Step 4: Define client types and fetch functions**

```ts
export interface RuntimeVersion { id: string; label: string; migrationClass: 'COMPATIBLE' | 'MANUAL_MIGRATION_REQUIRED'; releaseNotesUrl?: string; isLatest: boolean }
export interface RuntimeUpgradeEligibility { currentVersion: string; latestVersion: RuntimeVersion; eligible: boolean; reason: string }
export const getRuntimeVersions = () => apiFetch<RuntimeCatalog>('/api/runtime-versions')
export const getRuntimeUpgradeEligibility = (id: string) => apiFetch<RuntimeUpgradeEligibility>(`/api/projects/${id}/runtime-upgrade`)
```

- [ ] **Step 5: Run API and type tests**

Run: `go test ./apps/manager/internal/runtimeversions ./apps/manager/internal/httpapi -count=1 && npm --prefix apps/web run build`

Expected: PASS.

- [ ] **Step 6: Commit the API contract**

```bash
git add apps/manager/internal/runtimeversions apps/manager/internal/httpapi apps/web/src/api
git commit -m "feat(runtime): expose version catalog and upgrade eligibility"
```

### Task 4: Surface version choice and safe upgrade status in the dashboard

**Files:**
- Modify: `apps/web/src/features/projects/BasicStep.tsx`
- Modify: `apps/web/src/features/projects/NewProjectPage.test.tsx`
- Modify: `apps/web/src/features/project/OverviewPage.tsx`
- Modify: `apps/web/src/features/project/OverviewPage.test.tsx`
- Create: `apps/web/src/features/project/RuntimeUpgradeCard.tsx`
- Create: `apps/web/src/features/project/RuntimeUpgradeCard.test.tsx`

**Interfaces:**
- Consumes `RuntimeCatalog` and `RuntimeUpgradeEligibility` from Task 3.
- Produces exact selected version in create submissions and a read-only upgrade state on existing project overview pages.

- [ ] **Step 1: Write failing component tests**

```tsx
it('labels the default version as Latest supported and submits its exact ID', async () => {
  renderNewProjectPageWithCatalog()
  expect(await screen.findByText('Latest supported — self-hosted/v0.8.0')).toBeVisible()
})

it('explains that an already-current server has no upgrade action', async () => {
  renderRuntimeUpgradeCard({ eligible: false, reason: 'Already running the latest supported version' })
  expect(await screen.findByText(/Already running the latest supported version/)).toBeVisible()
  expect(screen.queryByRole('button', { name: /upgrade runtime/i })).not.toBeInTheDocument()
})
```

- [ ] **Step 2: Render catalog options in the wizard**

Display `Latest supported — <id>` as the first option and concrete older IDs underneath. Preserve the field's existing progressive-disclosure placement in Runtime settings. If no query data is available, show the build-time exact version without the word `latest`; do not create a selector containing invented choices.

- [ ] **Step 3: Add the overview card**

Place `RuntimeUpgradeCard` under the version fact. It shows current version, latest supported version, and the eligibility reason. For a manual-migration target it shows the warning and release-notes link but no activation control. This first delivery deliberately has no POST upgrade button until a second, tested official template is bundled.

- [ ] **Step 4: Run focused Web tests**

Run: `npm --prefix apps/web test -- --run src/features/projects/NewProjectPage.test.tsx src/features/project/OverviewPage.test.tsx src/features/project/RuntimeUpgradeCard.test.tsx`

Expected: PASS.

- [ ] **Step 5: Commit the dashboard slice**

```bash
git add apps/web/src/features/projects apps/web/src/features/project
git commit -m "feat(web): show supported runtime versions"
```

## Final verification

- [ ] Run `go test ./...`.
- [ ] Run `npm --prefix apps/web test -- --run`.
- [ ] Run `npm --prefix apps/web run build`.
- [ ] Run `git diff --check` and inspect `git status --short`.
- [ ] Rebuild the control plane with `docker compose -f deploy/docker-compose.yml --env-file deploy/.env up -d --build --wait`.
- [ ] Verify `curl -fsS -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/health/ready` returns `204`.
