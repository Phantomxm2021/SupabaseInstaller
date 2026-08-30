# Manager Functions ZIP Deployment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let authenticated Manager administrators deploy, inspect, roll back, and delete one self-hosted Edge Function from a safe ZIP archive.

**Architecture:** Add a Manager Functions orchestration layer that stores a bounded upload spool and records durable operations. Add a narrow Provisioner release manager that validates and extracts untrusted ZIPs only under a project's Functions volume, atomically switches a current-release pointer, and restarts only the Functions Compose service. Surface it with a route composed entirely from existing web UI primitives.

**Tech Stack:** Go 1.24 standard library (`archive/zip`, `net/http`, SQLite), React/TypeScript, TanStack Query, React Router, existing shadcn-style components, Docker Compose.

---

## File map

| File | Responsibility |
| --- | --- |
| `internal/contracts/functions.go` | Safe public/private Function DTOs and constants. |
| `internal/contracts/operation.go` | Function operation types. |
| `apps/provisioner/internal/projectfs/functions.go` | ZIP validation, release layout, pointer switching, retention, and recovery inspection. |
| `apps/provisioner/internal/projectfs/functions_test.go` | Archive and filesystem safety tests. |
| `apps/provisioner/internal/runtime/functions.go` | Runtime use case that calls the release store and restarts only Functions. |
| `apps/provisioner/internal/runtime/functions_test.go` | State transitions and restart-compensation tests. |
| `apps/provisioner/internal/server/functions.go` | Authenticated private HTTP handlers. |
| `apps/provisioner/internal/server/server.go` | Register private Functions routes. |
| `apps/provisioner/internal/server/server_test.go` | Private API authentication, sizes, and safe response tests. |
| `apps/manager/internal/config/config.go` | `FUNCTION_UPLOAD_SPOOL_DIR` configuration. |
| `apps/manager/internal/functions/spool.go` | Bounded private upload spool. |
| `apps/manager/internal/functions/service.go` | Manager queue/run/resume orchestration. |
| `apps/manager/internal/functions/*_test.go` | Spool, admission, recovery, and Provisioner-call tests. |
| `apps/manager/internal/provisioner/client.go` | Streaming/list/rollback/delete provisioner client methods. |
| `apps/manager/internal/httpapi/functions.go` | Public protected Functions routes. |
| `apps/manager/internal/httpapi/router.go`, `apps/manager/cmd/manager/main.go` | Wire service and routes/resume scheduler. |
| `apps/web/src/api/types.ts`, `apps/web/src/features/project/FunctionsPage.tsx` | API types and component-composed Functions page. |
| `apps/web/src/app/router.tsx`, `apps/web/src/app/AppShell.tsx` | Route and project navigation link. |
| `tests/integration/functions_deployment_test.go` | End-to-end ZIP deployment and rollback coverage. |

### Task 1: Define safe Function contracts and operation types

**Files:**
- Create: `internal/contracts/functions.go`
- Modify: `internal/contracts/operation.go`
- Test: `internal/contracts/functions_test.go`

- [ ] **Step 1: Write failing contract tests.**

```go
func TestValidateFunctionName(t *testing.T) {
  for _, name := range []string{"hello", "stripe-webhook", "x1"} {
    if err := ValidateFunctionName(name); err != nil { t.Fatal(err) }
  }
  for _, name := range []string{"", "Main", "main", "../escape", "-leading", "trailing-"} {
    if err := ValidateFunctionName(name); err == nil { t.Fatalf("%q accepted", name) }
  }
}
```

- [ ] **Step 2: Run `go test ./internal/contracts -run TestValidateFunctionName -count=1`.**  
Expected: FAIL because Functions contracts do not exist.

- [ ] **Step 3: Add immutable DTOs and validators.**

```go
const (
  OperationDeployFunction OperationType = "DEPLOY_FUNCTION"
  OperationRollbackFunction OperationType = "ROLLBACK_FUNCTION"
  OperationDeleteFunction OperationType = "DELETE_FUNCTION"
)
type FunctionSummary struct {
  Name string `json:"name"`
  Current *FunctionRelease `json:"current,omitempty"`
  Previous *FunctionRelease `json:"previous,omitempty"`
}
type FunctionRelease struct {
  SHA256 string `json:"sha256"`
  OperationID string `json:"operationId"`
  DeployedAt time.Time `json:"deployedAt"`
}
type DeployFunctionRequest struct { ProjectID, Slug, Name, OperationID, SHA256 string }
type FunctionOperationRequest struct { ProjectID, Slug, Name, OperationID string }
type FunctionDeploymentResult struct { Current *FunctionRelease `json:"current,omitempty"`; Previous *FunctionRelease `json:"previous,omitempty"`; RolledBack bool `json:"rolledBack"` }
var functionNamePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
func ValidateFunctionName(name string) error {
  if name == "main" || !functionNamePattern.MatchString(name) { return errors.New("invalid function name") }
  return nil
}
```

Put operation constants in `operation.go`; put name validation and response/request DTOs in `functions.go`. Do not put archive bytes, filesystem paths, Docker options, or raw manifest values in any contract.

- [ ] **Step 4: Run `go test ./internal/contracts -count=1`.**  
Expected: PASS.

- [ ] **Step 5: Commit.**

```bash
git add internal/contracts/functions.go internal/contracts/functions_test.go internal/contracts/operation.go
git commit -m "feat: define functions deployment contracts"
```

### Task 2: Build the Provisioner archive and release store

**Files:**
- Create: `apps/provisioner/internal/projectfs/functions.go`
- Create: `apps/provisioner/internal/projectfs/functions_test.go`

- [ ] **Step 1: Write failing ZIP safety tests.**

Create ZIP fixtures in-memory with `archive/zip` and assert `StageFunctionRelease` rejects `../outside.ts`, `/absolute.ts`, a symbolic-link mode, duplicate clean paths, 501 files, >20 MiB compressed input, >100 MiB declared/extracted content, and an enclosing `function/index.ts`. Assert a valid archive with root `index.ts` stages under `volumes/functions/.manager/<name>/staging/<operation-id>` without creating a live pointer.

- [ ] **Step 2: Run `go test ./apps/provisioner/internal/projectfs -run 'Test(StageFunctionRelease|RejectsUnsafe)' -count=1`.**  
Expected: FAIL because the release store is absent.

- [ ] **Step 3: Implement a closed filesystem API on `Root`.**

Expose only typed methods:

```go
type FunctionReleaseStage struct { Name, OperationID, SHA256, StagingPath string }
type FunctionActivation struct { Name string; Current, Previous *contracts.FunctionRelease }
func (r *Root) StageFunctionRelease(slug, name, operationID string, archive io.Reader) (FunctionReleaseStage, error)
func (r *Root) ActivateFunctionRelease(slug, name string, stage FunctionReleaseStage) (FunctionActivation, error)
func (r *Root) RestoreFunctionRelease(slug, name string, activation FunctionActivation) error
func (r *Root) DeleteFunction(slug, name string) (FunctionActivation, error)
func (r *Root) ListFunctions(slug string) ([]contracts.FunctionSummary, error)
```

Stream into a mode-`0600` staging archive while hashing; inspect it with `zip.NewReader`; normalize each entry with `path.Clean`; reject unsafe names and modes before writing. Extract only regular files to a `0700` stage and write a private Manager manifest. Enforce a 20 MiB per-entry uncompressed limit and verify root-level `index.ts`.

Create a relative current pointer at `volumes/functions/<name>` only after a complete valid stage exists. Move old current metadata into a two-release history, use `os.Rename` for pointer replacement on the same filesystem, and never touch `volumes/functions/main` or a non-Manager function unless the named deploy explicitly adopts it as the previous release. All cleanup must use paths constructed from validated slug/name/operation ID, then containment-checked with `filepath.Rel`.

- [ ] **Step 4: Extend tests for activation, two-release retention, rollback pointer restoration, delete, and symlink resolution.**  
Include an integration-shaped test that `os.Stat(volumes/functions/<name>/index.ts)` resolves through the generated relative pointer.

- [ ] **Step 5: Run `go test ./apps/provisioner/internal/projectfs -count=1`.**  
Expected: PASS.

- [ ] **Step 6: Commit.**

```bash
git add apps/provisioner/internal/projectfs/functions.go apps/provisioner/internal/projectfs/functions_test.go
git commit -m "feat: add safe functions release store"
```

### Task 3: Add Provisioner release operations and restart compensation

**Files:**
- Create: `apps/provisioner/internal/runtime/functions.go`
- Create: `apps/provisioner/internal/runtime/functions_test.go`
- Modify: `apps/provisioner/internal/runtime/backend.go`
- Modify: `apps/provisioner/internal/compose/runner.go`
- Modify: `apps/provisioner/internal/compose/runner_test.go`

- [ ] **Step 1: Write failing runtime tests using a recording release store and runner.**

Assert a deploy calls, in order, `StageFunctionRelease`, `ActivateFunctionRelease`, and `Restart(project, "functions")`. Make restart return an error and assert `RestoreFunctionRelease` then exactly one further `Restart(project, "functions")`; require a returned typed result to mark whether rollback completed.

- [ ] **Step 2: Run `go test ./apps/provisioner/internal/runtime -run TestFunction -count=1`.**  
Expected: FAIL because the runtime service is absent.

- [ ] **Step 3: Implement a focused `FunctionService`.**

```go
type FunctionService struct { releases ReleaseStore; runner FunctionRunner }
func (s *FunctionService) Deploy(ctx context.Context, project compose.ProjectRef, request contracts.DeployFunctionRequest, archive io.Reader) (contracts.FunctionDeploymentResult, error)
func (s *FunctionService) Rollback(ctx context.Context, project compose.ProjectRef, request contracts.FunctionOperationRequest) (contracts.FunctionDeploymentResult, error)
func (s *FunctionService) Delete(ctx context.Context, project compose.ProjectRef, request contracts.FunctionOperationRequest) (contracts.FunctionDeploymentResult, error)
```

The only runner call allowed by this service is `Restart(ctx, project, "functions")`. Before activation, validate that `functions` is a rendered Compose service and enabled in the request's trusted project context. On post-activation restart failure, restore the old pointer and restart only Functions; return a typed `RolledBack` result, not raw archive or filesystem details.

- [ ] **Step 4: Run the runtime and compose unit tests.**

```bash
go test ./apps/provisioner/internal/runtime ./apps/provisioner/internal/compose -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit.**

```bash
git add apps/provisioner/internal/runtime apps/provisioner/internal/compose
git commit -m "feat: deploy functions through provisioner runtime"
```

### Task 4: Expose narrowly authenticated Provisioner Functions endpoints

**Files:**
- Create: `apps/provisioner/internal/server/functions.go`
- Modify: `apps/provisioner/internal/server/server.go`
- Modify: `apps/provisioner/internal/server/server_test.go`

- [ ] **Step 1: Add failing endpoint tests.**

Exercise no bearer token (401), bad function name (400), upload over request limit (413), disabled Functions (422), successful deploy (200 private response), rollback, delete confirmation, and list. Ensure responses contain only the contracts DTOs and never temporary paths, ZIP contents, or Docker output.

- [ ] **Step 2: Run `go test ./apps/provisioner/internal/server -run Function -count=1`.**  
Expected: FAIL because routes are not registered.

- [ ] **Step 3: Implement private routes.**

Register:

```text
GET    /internal/v1/projects/{slug}/functions
POST   /internal/v1/projects/{slug}/functions/{name}/deploy
POST   /internal/v1/projects/{slug}/functions/{name}/rollback
DELETE /internal/v1/projects/{slug}/functions/{name}
```

Use `http.MaxBytesReader` before multipart parsing. Derive project paths through `ProjectFS`, validate all path values through contracts, and pass the file stream only to `FunctionService`. Continue to wrap the private mux with `RequireManagerToken`.

- [ ] **Step 4: Run `go test ./apps/provisioner/internal/server -count=1`.**  
Expected: PASS.

- [ ] **Step 5: Commit.**

```bash
git add apps/provisioner/internal/server
git commit -m "feat: expose provisioner functions api"
```

### Task 5: Add Manager upload spool and durable function orchestrator

**Files:**
- Modify: `apps/manager/internal/config/config.go`
- Modify: `apps/manager/internal/config/config_test.go`
- Create: `apps/manager/internal/functions/spool.go`
- Create: `apps/manager/internal/functions/spool_test.go`
- Create: `apps/manager/internal/functions/service.go`
- Create: `apps/manager/internal/functions/service_test.go`
- Modify: `apps/manager/internal/store/configuration.go`
- Modify: `apps/manager/internal/store/store_test.go`

- [ ] **Step 1: Add failing config/spool tests.**

Verify `FUNCTION_UPLOAD_SPOOL_DIR` defaults to a directory sibling to `MANAGER_DATABASE_PATH`, rejects a relative path, writes an operation-named `0600` file, enforces the 20 MiB limit while copying, and removes only an operation-owned spool file.

- [ ] **Step 2: Add a failing admission test.**

Queue a configuration operation for one project, then queue a Functions deploy and expect `ErrConfigurationBusy`; queue a Functions deploy, repeat it with the same active operation, and expect the existing operation; ensure terminal release frees the lease.

- [ ] **Step 3: Run focused tests.**

```bash
go test ./apps/manager/internal/config ./apps/manager/internal/functions ./apps/manager/internal/store -count=1
```

Expected: FAIL before implementation.

- [ ] **Step 4: Implement the spool and `functions.Service`.**

```go
type Service struct { store *store.Store; operations *operation.Service; spool Spool; provisioner Provisioner; now func() time.Time }
func (s *Service) QueueDeploy(ctx context.Context, project contracts.Project, name string, archive io.Reader) (operation.Operation, error)
func (s *Service) QueueRollback(ctx context.Context, project contracts.Project, name string) (operation.Operation, error)
func (s *Service) QueueDelete(ctx context.Context, project contracts.Project, name, confirmation string) (operation.Operation, error)
func (s *Service) Run(ctx context.Context, project contracts.Project, queued operation.Operation) (operation.Operation, error)
func (s *Service) Resume(ctx context.Context, getProject func(context.Context, string) (contracts.Project, error)) error
```

Use a generalized project-operation lease in the Store, replacing configuration-only method names only where necessary; all configuration callers must retain their current behavior. The stored command payload contains operation kind, function name, SHA-256, and spool identifier—not archive bytes or host paths. Emit progress steps `VALIDATING_ARCHIVE` (20), `STAGING_RELEASE` (45), `ACTIVATING_RELEASE` (70), and `RESTARTING_FUNCTIONS` (90). Terminal success, failure, or confirmed rollback deletes the spool.

- [ ] **Step 5: Implement retry-safe resume tests.**  
Simulate a Manager restart after admission and after Provisioner acknowledgement; assert the same operation ID and package hash are supplied, terminal operations are not replayed, and missing spool turns into a safe `FAILED` operation without invoking Provisioner.

- [ ] **Step 6: Run focused tests again.**  
Expected: PASS.

- [ ] **Step 7: Commit.**

```bash
git add apps/manager/internal/config apps/manager/internal/functions apps/manager/internal/store
git commit -m "feat: orchestrate durable functions deployments"
```

### Task 6: Connect Manager to Provisioner and public HTTP APIs

**Files:**
- Modify: `apps/manager/internal/provisioner/client.go`
- Modify: `apps/manager/internal/provisioner/client_test.go`
- Create: `apps/manager/internal/httpapi/functions.go`
- Modify: `apps/manager/internal/httpapi/router.go`
- Modify: `apps/manager/internal/httpapi/projects.go`
- Modify: `apps/manager/internal/httpapi/projects_test.go`
- Create: `apps/manager/internal/httpapi/functions_test.go`
- Modify: `apps/manager/cmd/manager/main.go`

- [ ] **Step 1: Write failing client tests.**

Use an `httptest.Server` to assert the client sends its bearer token, propagates multipart boundary/body, uses only typed name/slug paths, and decodes redacted DTOs for list/deploy/rollback/delete.

- [ ] **Step 2: Write failing public handler tests.**

Assert unauthenticated requests redirect/reject under current middleware, invalid names and filename mismatch return 422, CSRF is required for mutations, exact delete confirmation is required, disabled Functions returns 422, and a valid upload returns `202` with one operation ID.

- [ ] **Step 3: Implement client and handlers.**

The client accepts an `io.Reader` or spool path from the Manager Functions service and sends a fresh multipart request to the typed Provisioner route. The HTTP handlers find the project through `project.Service`, call the Functions service, start `Run` in the existing bounded background-operation style, and expose:

```text
GET    /api/projects/{id}/functions
POST   /api/projects/{id}/functions/{name}/deploy
POST   /api/projects/{id}/functions/{name}/rollback
DELETE /api/projects/{id}/functions/{name}
```

Wire one Functions service in `main.go`, call `Resume` at startup and on the existing minute ticker, and place it in `RouterOptions`. Do not duplicate the auth middleware: public routes remain under the protected `/api/projects/` mux.

- [ ] **Step 4: Run handler/client tests.**

```bash
go test ./apps/manager/internal/provisioner ./apps/manager/internal/httpapi -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit.**

```bash
git add apps/manager/internal/provisioner apps/manager/internal/httpapi apps/manager/cmd/manager/main.go
git commit -m "feat: add manager functions deployment api"
```

### Task 7: Build the component-only Functions page

**Files:**
- Modify: `apps/web/src/api/types.ts`
- Create: `apps/web/src/features/project/FunctionsPage.tsx`
- Create: `apps/web/src/features/project/FunctionsPage.test.tsx`
- Modify: `apps/web/src/app/router.tsx`
- Modify: `apps/web/src/app/router.test.tsx`
- Modify: `apps/web/src/app/AppShell.tsx`
- Modify: `apps/web/src/app/AppShell.test.tsx`

- [ ] **Step 1: Write failing UI tests.**

Mock `apiFetch` and verify a `Functions` navigation link and route; a page with `Card`, `Table`, and `Badge`; an actions `DropdownMenu` containing Deploy, Roll back, and Delete; a disabled runtime state; filename-to-function-name validation; a destructive `AlertDialog` requiring the exact name; and success/error toasts.

- [ ] **Step 2: Run `npm test -- --run apps/web/src/features/project/FunctionsPage.test.tsx apps/web/src/app/router.test.tsx apps/web/src/app/AppShell.test.tsx`.**  
Expected: FAIL because the route/page are absent.

- [ ] **Step 3: Implement the page with existing components only.**

Use TanStack Query key `["project-functions", projectId]`, `apiFetch` with `FormData` for deploy, and query invalidation on each terminal operation. Reuse `OperationPanel` or its existing operation query pattern for progress. Build actions with:

```tsx
<DropdownMenu>
  <DropdownMenuTrigger render={<Button variant="ghost" size="icon" aria-label={`Actions for ${fn.name}`} />}><MoreHorizontal /></DropdownMenuTrigger>
  <DropdownMenuContent align="end">
    <DropdownMenuItem onClick={() => openDeploy(fn.name)}>Deploy new version</DropdownMenuItem>
    <DropdownMenuItem disabled={!fn.previous} onClick={() => rollback.mutate(fn.name)}>Roll back</DropdownMenuItem>
    <DropdownMenuSeparator />
    <DropdownMenuItem variant="destructive" onClick={() => openDelete(fn.name)}>Delete function</DropdownMenuItem>
  </DropdownMenuContent>
</DropdownMenu>
```

Do not add hand-rolled dropdown, table, dialog, button, or toast primitives. Add the route and sidebar link using the existing `AppShell` pattern.

- [ ] **Step 4: Run focused web tests.**  
Expected: PASS.

- [ ] **Step 5: Commit.**

```bash
git add apps/web/src
git commit -m "feat: add functions deployment workspace"
```

### Task 8: Add end-to-end coverage and verify the complete feature

**Files:**
- Create: `tests/integration/functions_deployment_test.go`
- Modify: `README.md`

- [ ] **Step 1: Write the failing integration test.**

Create a temporary self-hosted project with Functions enabled, upload a valid ZIP whose root `index.ts` returns `{"version":"one"}`, wait for its operation, and invoke `/functions/v1/demo`. Deploy version two, verify it responds `two`, invoke Manager rollback, and verify it responds `one`. Upload a traversal ZIP and assert it returns an error while the live release still returns `one`.

- [ ] **Step 2: Run the targeted integration test.**

```bash
go test ./tests/integration -run TestFunctionsZipDeployment -count=1
```

Expected: FAIL until the feature is fully wired.

- [ ] **Step 3: Document operator constraints.**

Add a concise README section covering direct-root `index.ts`, the two-release rollback guarantee, safe size limits, the Functions-enabled prerequisite, and the endpoint URL.

- [ ] **Step 4: Run all verification commands.**

```bash
go test ./...
npm test -- --run
npm run build
git diff --check
```

Expected: all commands exit 0.

- [ ] **Step 5: Commit.**

```bash
git add tests/integration/functions_deployment_test.go README.md
git commit -m "test: cover functions zip deployment"
```
