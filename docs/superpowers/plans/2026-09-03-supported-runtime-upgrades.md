# Runtime Image Manifest Upgrades Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every new server default to a concrete latest official Supabase image manifest, expose supported official and experimental manifests, and provide safe upgrade eligibility without accepting mutable Docker tags.

**Architecture:** A single embedded-template catalog supplies image-manifest metadata, template roots, and exact image pins to Manager and Provisioner. The persisted configuration remains an immutable manifest ID; legacy `self-hosted/v0.8.0` values map to the existing image matrix. The first delivery introduces the catalog and upgrade eligibility contract; a runnable cross-manifest upgrade is enabled only after a second official matrix is bundled and tested.

**Tech Stack:** Go 1.24, `embed.FS`, `net/http`, SQLite-backed Manager operations, React, TypeScript, TanStack Query, Vitest.

**Spec:** `docs/superpowers/specs/2026-09-03-supported-runtime-upgrades-design.md`

## Global Constraints

- `latest`, `master`, blank values, and arbitrary Docker image tags are invalid persisted manifest IDs.
- Latest official resolves to an exact catalog ID before it reaches SQLite or Compose.
- Existing `self-hosted/v0.8.0` rows map to the imported existing image manifest without a runtime migration.
- Runtime-version changes must never be accepted through the normal configuration PATCH route.
- No runtime is stopped, rebuilt, or pulled when a target is missing, unsupported, identical, or requires a manual migration.
- No API response, event, or test output may expose secret values or rendered environment contents.

---

### Task 1: Create the shared embedded image-manifest catalog

**Files:**
- Create: `internal/templates/catalog.go`
- Create: `internal/templates/catalog_test.go`
- Modify: `internal/templates/embed.go`
- Modify: `apps/provisioner/internal/render/render.go`
- Test: `apps/provisioner/internal/render/render_test.go`

**Interfaces:**
- Produces `templates.RuntimeManifest`, `templates.SupportedManifests()`, `templates.LatestOfficial()`, `templates.Lookup(id string)`, and `templates.ResolveLegacy(id string)`.
- Consumes `contracts.ProjectConfiguration.General.SupabaseVersion` in the renderer.
- Produces a version-selected official Compose root for `loadOfficialCompose`.

- [ ] **Step 1: Write catalog tests before implementation**

```go
func TestCatalogResolvesOneConcreteLatestOfficialManifest(t *testing.T) {
    latest := LatestOfficial()
    if latest.ID != "official-2026-08-03" || latest.Images["studio"] != "supabase/studio:2026.08.03-sha-022b374" {
        t.Fatalf("latest=%+v", latest)
    }
    if _, ok := ResolveLegacy("self-hosted/v0.8.0"); !ok { t.Fatal("legacy map missing") }
    if _, ok := Lookup("latest"); ok { t.Fatal("latest must be rejected") }
}
```

- [ ] **Step 2: Run the catalog test and confirm it fails**

Run: `go test ./internal/templates -run TestCatalogResolvesOneConcreteLatestOfficialManifest -count=1`

Expected: FAIL because the catalog symbols do not exist.

- [ ] **Step 3: Implement the immutable catalog**

```go
type RuntimeManifest struct {
    ID, Label, Channel, TemplateRoot string
    Images map[string]string
    MigrationClass MigrationClass
    ReleaseNotesURL string
}

var supported = []RuntimeManifest{{
    ID: "official-2026-08-03", Label: "Latest official", Channel: "OFFICIAL",
    TemplateRoot: "self-hosted-v0.8.0",
    Images: map[string]string{"studio": "supabase/studio:2026.08.03-sha-022b374", "auth": "supabase/gotrue:v2.189.0", "db": "supabase/postgres:17.6.1.136"},
    MigrationClass: MigrationCompatible,
}}

func LatestOfficial() RuntimeManifest { return supported[0] }
```

`Lookup` must return `(RuntimeManifest, bool)`. `ResolveLegacy("self-hosted/v0.8.0")` must return the first official manifest. Change `DockerCompose` and `EnvExample` to read from `LatestOfficial().TemplateRoot`; leave their public signatures unchanged.

- [ ] **Step 4: Select the template through the catalog in the renderer**

Resolve legacy values before rendering, then replace the hard-coded version equality check and `"self-hosted-v0.8.0/"` path in `render.go` with `templates.Lookup`. Return `general.supabaseVersion: unsupported runtime manifest` when lookup fails. Pass the resolved descriptor into `loadOfficialCompose`; after decoding Compose, replace every catalog-owned service image with the exact `descriptor.Images` entry before pin validation.

- [ ] **Step 5: Add renderer coverage for rejection and exact template selection**

```go
func TestProjectRejectsUnknownRuntimeManifest(t *testing.T) {
    input := validInput()
    input.Configuration.General.SupabaseVersion = "latest"
    _, err := Project(input)
    if err == nil || !strings.Contains(err.Error(), "unsupported runtime manifest") {
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

### Task 2: Make Manager defaults and validation manifest-backed

**Files:**
- Modify: `apps/manager/internal/project/configuration.go`
- Modify: `apps/manager/internal/project/validate.go`
- Modify: `apps/manager/internal/project/configuration_test.go`
- Modify: `apps/manager/internal/project/validate_test.go`
- Modify: `apps/web/src/features/projects/projectSchema.ts`
- Modify: `apps/web/src/features/projects/projectSchema.test.ts`

**Interfaces:**
- Consumes `templates.LatestOfficial().ID` and `templates.ResolveLegacy`.
- Produces persisted exact manifest IDs from `DefaultConfiguration` and create-form defaults.

- [ ] **Step 1: Write failing default and legacy tests**

```go
func TestDefaultConfigurationUsesLatestOfficialManifest(t *testing.T) {
    got := DefaultConfiguration(contracts.PresetLightweight).General.SupabaseVersion
    if got != templates.LatestOfficial().ID { t.Fatalf("manifest=%q", got) }
}

func TestPreparePatchMapsExistingTemplateSnapshotToManifest(t *testing.T) {
    cfg := DefaultConfiguration(contracts.PresetLightweight)
    cfg.General.SupabaseVersion = "self-hosted/v0.8.0"
    if err := ValidateStoredConfiguration(cfg); err != nil { t.Fatal(err) }
}
```

- [ ] **Step 2: Run the focused Manager tests and confirm the old literal assertion fails**

Run: `go test ./apps/manager/internal/project -run 'Test(DefaultConfigurationUsesLatestOfficialManifest|PreparePatchMapsExistingTemplateSnapshotToManifest)' -count=1`

Expected: FAIL until the application imports the catalog.

- [ ] **Step 3: Replace version literals with catalog calls**

Use `templates.LatestOfficial().ID` in `DefaultConfiguration`; resolve the legacy snapshot before complete and stored configuration validation. Keep the existing field name `general.supabaseVersion` for wire compatibility while browser copy calls it `Runtime image manifest`. Change the TypeScript schema from `z.literal(SUPABASE_VERSION)` to a catalog-fed enum only after Task 3 exposes the public catalog; until then keep a local exact fallback and accept the server as authority.

- [ ] **Step 4: Make create-form version data API-backed**

Create `useRuntimeManifests` next to the project create hooks. It fetches `/api/runtime-manifests`, uses `latest.id` for `defaultConfiguration`, and does not submit an alias. While the catalog query is loading, the form uses the build-time exact manifest fallback; on success it updates untouched forms to the returned exact ID.

- [ ] **Step 5: Add form tests**

```tsx
it('uses the API latest supported ID for a pristine create form', async () => {
  mockFetch('/api/runtime-manifests', { latest: { id: 'official-2026-08-03', label: 'Latest official' }, manifests: [] })
  renderNewProjectPage()
  expect(await screen.findByText('Latest official')).toBeVisible()
})
```

- [ ] **Step 6: Run Manager and Web focused tests**

Run: `go test ./apps/manager/internal/project -count=1 && npm --prefix apps/web test -- --run src/features/projects/projectSchema.test.ts src/features/projects/NewProjectPage.test.tsx`

Expected: PASS.

- [ ] **Step 7: Commit the default/validation slice**

```bash
git add apps/manager/internal/project apps/web/src/features/projects
git commit -m "feat(runtime): default new servers to latest official manifest"
```

### Task 3: Publish manifests and upgrade eligibility over the protected API

**Files:**
- Create: `apps/manager/internal/runtimeversions/service.go`
- Create: `apps/manager/internal/runtimeversions/service_test.go`
- Create: `apps/manager/internal/httpapi/runtime_versions.go`
- Create: `apps/manager/internal/httpapi/runtime_versions_test.go`
- Modify: `apps/manager/internal/httpapi/router.go`
- Modify: `apps/web/src/api/types.ts`
- Modify: `apps/web/src/api/client.ts`

**Interfaces:**
- Produces `GET /api/runtime-manifests` with `{latest, manifests}` and no secrets.
- Produces `GET /api/projects/{id}/runtime-upgrade` with current manifest, latest manifest, eligibility, migration class, component differences, and reason.
- Consumes the catalog and project store; does not mutate projects.

- [ ] **Step 1: Write failing HTTP contract tests**

```go
func TestRuntimeManifestsReturnsConcreteLatestOfficial(t *testing.T) {
    request := httptest.NewRequest(http.MethodGet, "/api/runtime-manifests", nil)
    response := httptest.NewRecorder()
    router.ServeHTTP(response, request)
    if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"id":"official-2026-08-03"`) {
        t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
    }
}
```

- [ ] **Step 2: Implement a read-only service**

Return browser-safe descriptors (`id`, `label`, `channel`, `migrationClass`, `releaseNotesURL`, representative `images`, `isLatest`) from the template catalog. The eligibility service returns `eligible=false, reason="Already running the latest official image manifest"` for a project on the latest ID, includes a sorted component image diff for an older manifest, and returns `eligible=false` for any target with `MANUAL_MIGRATION_REQUIRED`.

- [ ] **Step 3: Register protected routes and preserve auth behavior**

Register the catalog endpoint through the same protected mux used by project routes. Add `Cache-Control: no-store` to upgrade eligibility responses because project state can change while viewed.

- [ ] **Step 4: Define client types and fetch functions**

```ts
export interface RuntimeManifest { id: string; label: string; channel: 'OFFICIAL' | 'EXPERIMENTAL'; images: Record<string, string>; migrationClass: 'COMPATIBLE' | 'MANUAL_MIGRATION_REQUIRED'; releaseNotesUrl?: string; isLatest: boolean }
export interface RuntimeUpgradeEligibility { currentManifest: string; latestManifest: RuntimeManifest; changedImages: string[]; eligible: boolean; reason: string }
export const getRuntimeManifests = () => apiFetch<RuntimeManifestCatalog>('/api/runtime-manifests')
export const getRuntimeUpgradeEligibility = (id: string) => apiFetch<RuntimeUpgradeEligibility>(`/api/projects/${id}/runtime-upgrade`)
```

- [ ] **Step 5: Run API and type tests**

Run: `go test ./apps/manager/internal/runtimeversions ./apps/manager/internal/httpapi -count=1 && npm --prefix apps/web run build`

Expected: PASS.

- [ ] **Step 6: Commit the API contract**

```bash
git add apps/manager/internal/runtimeversions apps/manager/internal/httpapi apps/web/src/api
git commit -m "feat(runtime): expose image manifests and upgrade eligibility"
```

### Task 4: Surface manifest choice and safe upgrade status in the dashboard

**Files:**
- Modify: `apps/web/src/features/projects/BasicStep.tsx`
- Modify: `apps/web/src/features/projects/NewProjectPage.test.tsx`
- Modify: `apps/web/src/features/project/OverviewPage.tsx`
- Modify: `apps/web/src/features/project/OverviewPage.test.tsx`
- Create: `apps/web/src/features/project/RuntimeUpgradeCard.tsx`
- Create: `apps/web/src/features/project/RuntimeUpgradeCard.test.tsx`

**Interfaces:**
- Consumes `RuntimeManifestCatalog` and `RuntimeUpgradeEligibility` from Task 3.
- Produces exact selected manifest IDs in create submissions and a read-only upgrade state on existing project overview pages.

- [ ] **Step 1: Write failing component tests**

```tsx
it('labels the default manifest as Latest official and submits its exact ID', async () => {
  renderNewProjectPageWithCatalog()
  expect(await screen.findByText('Latest official')).toBeVisible()
})

it('explains that an already-current server has no upgrade action', async () => {
  renderRuntimeUpgradeCard({ eligible: false, reason: 'Already running the latest official image manifest' })
  expect(await screen.findByText(/Already running the latest official image manifest/)).toBeVisible()
  expect(screen.queryByRole('button', { name: /upgrade runtime/i })).not.toBeInTheDocument()
})
```

- [ ] **Step 2: Render catalog options in the wizard**

Display `Latest official` as the first option, include its representative Studio/Auth/Postgres image tags, and list historical official manifests underneath. Experimental Docker Hub manifests are visually distinct and require a confirmation dialog before selection. Preserve the field's existing progressive-disclosure placement in Runtime settings. If no query data is available, show the build-time exact manifest without the word `latest`; do not create a selector containing invented choices.

- [ ] **Step 3: Add the overview card**

Place `RuntimeUpgradeCard` under the version fact. It shows current manifest, latest official manifest, representative component changes, and the eligibility reason. For a manual-migration target it shows the warning and release-notes link but no activation control. This first delivery deliberately has no POST upgrade button until a second, tested official manifest is bundled.

- [ ] **Step 4: Run focused Web tests**

Run: `npm --prefix apps/web test -- --run src/features/projects/NewProjectPage.test.tsx src/features/project/OverviewPage.test.tsx src/features/project/RuntimeUpgradeCard.test.tsx`

Expected: PASS.

- [ ] **Step 5: Commit the dashboard slice**

```bash
git add apps/web/src/features/projects apps/web/src/features/project
git commit -m "feat(web): show supported runtime manifests"
```

## Final verification

- [ ] Run `go test ./...`.
- [ ] Run `npm --prefix apps/web test -- --run`.
- [ ] Run `npm --prefix apps/web run build`.
- [ ] Run `git diff --check` and inspect `git status --short`.
- [ ] Rebuild the control plane with `docker compose -f deploy/docker-compose.yml --env-file deploy/.env up -d --build --wait`.
- [ ] Verify `curl -fsS -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/health/ready` returns `204`.
