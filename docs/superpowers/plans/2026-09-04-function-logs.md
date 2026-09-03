# Per-Function Edge Function Logs Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a strictly attributed, secret-safe, seven-day Edge Function log viewer opened from each function row's Actions menu.

**Architecture:** A pinned Edge Runtime event-worker adapter forwards structured events to a private project-local collector mode of the Provisioner image. The collector validates function attribution, redacts and stores records in a project-local SQLite database; authenticated Provisioner and Manager APIs expose cursor-based reads to a React page that polls every five seconds.

**Tech Stack:** Go 1.27, `modernc.org/sqlite`, `net/http`, Supabase Edge Runtime v1.74.0, TypeScript/Deno, React 19, React Router, TanStack Query, Vitest, Docker Compose.

---

## File Map

- Create `internal/contracts/function_logs.go`: shared log record, query, cursor, health, and ingest contracts.
- Create `apps/provisioner/internal/functionlogs/store.go`: isolated SQLite persistence, filtering, pagination, and retention.
- Create `apps/provisioner/internal/functionlogs/redactor.go`: secret loading and pre-persistence sanitization.
- Create `apps/provisioner/internal/functionlogs/collector.go`: private ingestion and health HTTP handler.
- Create `apps/provisioner/internal/functionlogs/store_test.go`, `redactor_test.go`, and `collector_test.go`: focused storage/security/HTTP tests.
- Create `apps/provisioner/cmd/provisioner/collector_mode.go`: collector process mode selected by environment.
- Create `internal/templates/manager/function-logs/event-worker/index.ts`: version-pinned Edge Runtime event adapter.
- Modify `apps/provisioner/internal/render/render.go`: inject collector service, event-worker mount, command arguments, and image interpolation when Functions is enabled.
- Modify `apps/provisioner/internal/projectfs/root.go`: publish the Manager-owned event-worker asset beside generated runtime files.
- Modify `apps/provisioner/internal/server/server.go`: authenticated internal function-log query endpoint.
- Modify `apps/provisioner/internal/runtime/backend.go`: validate the managed function and query its log database.
- Modify `apps/manager/internal/provisioner/client.go`: typed Provisioner log client.
- Modify `apps/manager/internal/functions/service.go`: project-aware function log query method.
- Modify `apps/manager/internal/httpapi/functions.go`: authenticated public read endpoint and query validation.
- Create `apps/web/src/features/project/FunctionLogsPage.tsx`: dedicated log viewer.
- Create `apps/web/src/features/project/FunctionLogsPage.test.tsx`: UI, polling, filtering, and state tests.
- Modify `apps/web/src/features/project/FunctionsPage.tsx`: Actions menu entry.
- Modify `apps/web/src/app/router.tsx` and `router.test.tsx`: nested function log route.
- Modify `apps/web/src/api/types.ts`: browser log contracts.
- Modify `apps/web/src/styles.css`: compact responsive log table styling.
- Create `tests/integration/function_logs_test.go`: cross-function isolation and collector failure coverage.

### Task 1: Lock the Edge Runtime Event Contract

**Files:**
- Create: `internal/templates/manager/function-logs/event-worker/fixtures/log-event.json`
- Create: `internal/templates/manager/function-logs/event-worker/fixtures/uncaught-exception.json`
- Create: `internal/templates/manager/function-logs/event-worker/fixtures/boot-event.json`
- Create: `internal/templates/manager/function-logs/event-worker/contract_test.go`

- [ ] **Step 1: Pull and inspect the exact pinned runtime**

Run:

```bash
docker pull supabase/edge-runtime:v1.74.0
docker run --rm supabase/edge-runtime:v1.74.0 start --help
```

Expected: the image pulls successfully and help identifies the supported event-worker service argument. Record the exact argument in the test fixture comments; do not substitute a guessed CLI flag.

- [ ] **Step 2: Capture three real structured events**

Run a temporary v1.74.0 runtime with its supported event worker and invoke one fixture function that logs and one that throws. Save one `LogEvent`, one `UncaughtException`, and one boot event exactly as delivered, replacing only secret-bearing values with `REDACTED_FIXTURE_VALUE`.

Expected: every saved event contains a stable function or service identifier plus an event/execution identifier. If v1.74.0 does not deliver exact function attribution, stop implementation and revise the approved design rather than falling back to Docker text matching.

- [ ] **Step 3: Write a contract test for required attribution**

```go
func TestPinnedEventFixturesCarryExactFunctionAttribution(t *testing.T) {
    for _, name := range []string{"log-event.json", "uncaught-exception.json", "boot-event.json"} {
        raw, err := os.ReadFile(filepath.Join("fixtures", name))
        if err != nil { t.Fatal(err) }
        event, err := contracts.ParseEdgeRuntimeEvent(raw)
        if err != nil { t.Fatalf("%s: %v", name, err) }
        if event.FunctionName == "" || event.EventID == "" {
            t.Fatalf("%s lacks exact attribution: %#v", name, event)
        }
    }
}
```

- [ ] **Step 4: Run the contract test and verify it fails**

Run: `go test ./internal/templates/manager/function-logs/event-worker -run TestPinnedEventFixturesCarryExactFunctionAttribution -v`

Expected: FAIL because `contracts.ParseEdgeRuntimeEvent` is not defined.

- [ ] **Step 5: Commit the verified fixtures**

```bash
git add internal/templates/manager/function-logs/event-worker
git commit -m "test(logs): lock edge runtime event contract"
```

### Task 2: Define and Parse Function Log Contracts

**Files:**
- Create: `internal/contracts/function_logs.go`
- Create: `internal/contracts/function_logs_test.go`
- Modify: `internal/templates/manager/function-logs/event-worker/contract_test.go`

- [ ] **Step 1: Write failing validation and parser tests**

Cover: valid fixture parsing; missing function name; invalid function name; missing event ID; unknown event type; `limit` outside 1-200; simultaneous `before` and `after`; search larger than 256 UTF-8 bytes; invalid level.

```go
func TestValidateFunctionLogQueryRejectsConflictingCursors(t *testing.T) {
    err := contracts.ValidateFunctionLogQuery(contracts.FunctionLogQuery{Before: "a", After: "b", Limit: 200})
    if err == nil { t.Fatal("expected conflicting cursors to fail") }
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/contracts ./internal/templates/manager/function-logs/event-worker`

Expected: FAIL with undefined function-log types and parser.

- [ ] **Step 3: Implement the shared contracts**

Define these stable JSON shapes and closed values:

```go
type FunctionLogLevel string
const (
    FunctionLogDebug FunctionLogLevel = "debug"
    FunctionLogInfo  FunctionLogLevel = "info"
    FunctionLogWarn  FunctionLogLevel = "warn"
    FunctionLogError FunctionLogLevel = "error"
)

type FunctionLogRecord struct {
    ID, ProjectID, FunctionName, ExecutionID, EventType, Message string
    Timestamp, IngestedAt time.Time
    Level FunctionLogLevel
    Truncated bool
}

type FunctionLogQuery struct { Limit int; Before, After, Level, Search string }
type FunctionLogHealth struct { Status string; Dropped, Rejected uint64; Detail string }
type FunctionLogPage struct { Logs []FunctionLogRecord; OlderCursor, NewerCursor string; Health FunctionLogHealth; ServerTime time.Time }
type EdgeRuntimeEvent struct { Version int; EventID, FunctionName, ExecutionID, EventType, Message string; Timestamp time.Time; Level FunctionLogLevel }
type FunctionLogBatch struct { Version int; ProjectID string; Events []EdgeRuntimeEvent }
```

Implement parsing against the verified v1.74.0 fixtures only. Unknown shapes return a typed incompatibility error. Implement query validation and opaque base64url cursor encoding over `(timestamp, recordID)`.

- [ ] **Step 4: Run contract tests**

Run: `go test ./internal/contracts ./internal/templates/manager/function-logs/event-worker`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/contracts internal/templates/manager/function-logs/event-worker/contract_test.go
git commit -m "feat(logs): define edge function log contracts"
```

### Task 3: Build the Secret-Safe Bounded Log Store

**Files:**
- Create: `apps/provisioner/internal/functionlogs/store.go`
- Create: `apps/provisioner/internal/functionlogs/store_test.go`
- Create: `apps/provisioner/internal/functionlogs/redactor.go`
- Create: `apps/provisioner/internal/functionlogs/redactor_test.go`

- [ ] **Step 1: Write failing store tests**

Use a temporary database. Test transactional batch insertion, duplicate event IDs, newest-first order, exact function isolation, level/search filtering, `before`/`after` cursors, seven-day deletion, and oldest-first deletion when the injected size probe exceeds 512 MiB.

```go
func TestStoreNeverCrossesFunctionBoundary(t *testing.T) {
    store := openTestStore(t)
    insertFixture(t, store, "api", "api-event")
    insertFixture(t, store, "deliver-push", "push-event")
    page, err := store.Query(context.Background(), "project-1", "api", contracts.FunctionLogQuery{Limit: 200})
    if err != nil { t.Fatal(err) }
    if len(page.Logs) != 1 || page.Logs[0].FunctionName != "api" { t.Fatalf("unexpected logs: %#v", page.Logs) }
}
```

- [ ] **Step 2: Write failing redaction tests**

Load representative `.env` and `.env.functions` files and prove that Authorization values, JWTs, database/service-role/OAuth/SMTP/storage credentials, and function secrets are replaced before insert. Verify control-character normalization and 10 KiB truncation.

- [ ] **Step 3: Run tests to verify failure**

Run: `go test ./apps/provisioner/internal/functionlogs -v`

Expected: FAIL because the package is not implemented.

- [ ] **Step 4: Implement SQLite schema and queries**

Use WAL mode and a schema with `event_id UNIQUE`, `project_id`, `function_name`, `timestamp_ns`, `ingested_at_ns`, `execution_id`, `level`, `event_type`, `message`, and `truncated`. Add index:

```sql
CREATE INDEX function_logs_lookup
ON function_logs(project_id, function_name, timestamp_ns DESC, event_id DESC);
```

Implement parameterized queries only. Maintenance runs on open and hourly, deletes at most 10,000 rows per transaction, removes records older than `now-7d`, then deletes oldest batches while the injected on-disk size is above 512 MiB.

- [ ] **Step 5: Implement pre-persistence redaction**

Reuse `internal/diagnostic.Sanitize` after loading only allow-listed secret keys from the two mounted environment files. Never log file contents or rejected messages. Normalize control characters and truncate UTF-8 safely to 10 KiB, returning `Truncated=true`.

- [ ] **Step 6: Run focused tests**

Run: `go test ./apps/provisioner/internal/functionlogs -v`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add apps/provisioner/internal/functionlogs
git commit -m "feat(logs): add bounded redacted function log store"
```

### Task 4: Add the Private Collector Process

**Files:**
- Create: `apps/provisioner/internal/functionlogs/collector.go`
- Create: `apps/provisioner/internal/functionlogs/collector_test.go`
- Create: `apps/provisioner/cmd/provisioner/collector_mode.go`
- Modify: `apps/provisioner/cmd/provisioner/main.go`

- [ ] **Step 1: Write failing collector tests**

Test `POST /internal/v1/events`, body cap of 1 MiB, batch cap of 100, version mismatch, unknown function directory, duplicate event acceptance, health counters, and `GET /health/live`. Assert no request body appears in captured logs.

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./apps/provisioner/internal/functionlogs ./apps/provisioner/cmd/provisioner -v`

Expected: FAIL with missing collector types.

- [ ] **Step 3: Implement collector HTTP handling**

The collector listens only on `0.0.0.0:8081` inside the private Compose network. Accept `FunctionLogBatch{Version:1}`, require the configured project ID, validate each function by safe directory lookup under the read-only functions root, redact, then insert. Return `204` for accepted or duplicate batches, `400` for invalid input, `413` for limits, and `422` for incompatible event versions.

- [ ] **Step 4: Add collector mode to the existing binary**

At the start of `main`, branch on `PROVISIONER_MODE=function-log-collector`. Require `FUNCTION_LOG_PROJECT_ID`, `FUNCTION_LOG_DATABASE_PATH`, `FUNCTION_LOG_FUNCTIONS_ROOT`, `FUNCTION_LOG_PROJECT_ENV`, and `FUNCTION_LOG_FUNCTIONS_ENV`; start the collector and return without initializing Docker or the ordinary Provisioner server.

- [ ] **Step 5: Run tests**

Run: `go test ./apps/provisioner/internal/functionlogs ./apps/provisioner/cmd/provisioner -v`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add apps/provisioner/internal/functionlogs apps/provisioner/cmd/provisioner
git commit -m "feat(logs): run private function log collector"
```

### Task 5: Wire Event Collection into Rendered Projects

**Files:**
- Create: `internal/templates/manager/function-logs/event-worker/index.ts`
- Modify: `apps/provisioner/internal/render/render.go`
- Modify: `apps/provisioner/internal/render/render_test.go`
- Modify: `apps/provisioner/internal/projectfs/root.go`
- Modify: `apps/provisioner/internal/projectfs/root_test.go`
- Modify: `deploy/.env.example`
- Modify: `deploy/docker-compose.yml`

- [ ] **Step 1: Write failing renderer and publication tests**

Assert that Functions-enabled output contains `function-log-collector`, no published port, a private dependency, a database volume under `.manager-runtime/function-logs`, read-only mounts for functions and env files, the exact verified event-worker runtime argument, and `SUPABASE_PROVISIONER_IMAGE`. Assert Functions-disabled output contains none of these. Assert the event-worker file is atomically published with generated runtime files.

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./apps/provisioner/internal/render ./apps/provisioner/internal/projectfs -v`

Expected: FAIL because the collector service and event-worker asset are absent.

- [ ] **Step 3: Implement the event-worker adapter**

Normalize the verified v1.74.0 callback payload into contract version 1. Queue at most 1,000 entries, flush at 100 entries or 250 ms, use a 500 ms fetch timeout to `http://function-log-collector:8081/internal/v1/events`, and drop the oldest entry when full. Never throw an ingestion error back into Edge Runtime.

- [ ] **Step 4: Inject the collector and adapter**

When Functions is selected, render the collector with:

```yaml
image: ${SUPABASE_PROVISIONER_IMAGE}
environment:
  PROVISIONER_MODE: function-log-collector
  FUNCTION_LOG_PROJECT_ID: <project-id>
  FUNCTION_LOG_DATABASE_PATH: /var/lib/function-logs/function-logs.db
  FUNCTION_LOG_FUNCTIONS_ROOT: /srv/functions
volumes:
  - ./.manager-runtime/function-logs:/var/lib/function-logs
  - ./volumes/functions:/srv/functions:ro
  - ./.manager-runtime/current/.env:/run/project.env:ro
  - ./.manager-runtime/current/.env.functions:/run/functions.env:ro
```

Add the verified event-worker argument and read-only asset mount to `functions`. Add `SUPABASE_PROVISIONER_IMAGE=supabase-provisioner:${MANAGER_IMAGE_TAG:-local}` to the generated/deployment environment path without exposing it to user workers.

- [ ] **Step 5: Run renderer and project filesystem tests**

Run: `go test ./apps/provisioner/internal/render ./apps/provisioner/internal/projectfs -v`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/templates/manager apps/provisioner/internal/render apps/provisioner/internal/projectfs deploy
git commit -m "feat(logs): wire edge runtime event collection"
```

### Task 6: Expose Provisioner and Manager Query APIs

**Files:**
- Modify: `apps/provisioner/internal/runtime/backend.go`
- Modify: `apps/provisioner/internal/server/server.go`
- Modify: `apps/provisioner/internal/server/server_test.go`
- Modify: `apps/manager/internal/provisioner/client.go`
- Modify: `apps/manager/internal/provisioner/client_test.go`
- Modify: `apps/manager/internal/functions/service.go`
- Modify: `apps/manager/internal/functions/service_test.go`
- Modify: `apps/manager/internal/httpapi/functions.go`
- Modify: `apps/manager/internal/httpapi/projects_test.go`

- [ ] **Step 1: Write failing Provisioner endpoint tests**

Add `GET /internal/v1/projects/{slug}/functions/{name}/logs`. Verify manager-token authentication, function name validation, managed-function existence, query bounds, opaque cursors, successful page JSON, and safe typed failures.

- [ ] **Step 2: Write failing Manager endpoint tests**

Add `GET /api/projects/{id}/functions/{name}/logs`. Verify session authentication, project lookup, function isolation, query forwarding, disabled Functions state, and that Provisioner errors become canonical safe API errors.

- [ ] **Step 3: Run tests to verify failure**

Run: `go test ./apps/provisioner/internal/server ./apps/manager/internal/provisioner ./apps/manager/internal/functions ./apps/manager/internal/httpapi -v`

Expected: FAIL because log query methods and routes are absent.

- [ ] **Step 4: Implement Provisioner reads**

Resolve the database path only through `projectfs.Root` using a validated slug. Confirm `projectfs.ListFunctions` contains the requested managed function, open the log store read-only, execute the validated query, and return health plus `ServerTime`. Never accept a path or container name from the request.

- [ ] **Step 5: Implement Manager forwarding**

Extend `FunctionsClient` with:

```go
FunctionLogs(context.Context, string, string, contracts.FunctionLogQuery) (contracts.FunctionLogPage, error)
```

Parse `limit`, `before`, `after`, `level`, and `search` in the public handler, call shared validation, fetch the project, then call the Functions service with its slug. Use `400 INVALID_FUNCTION_LOG_QUERY`, `404 FUNCTION_NOT_FOUND`, and `502 FUNCTION_LOGS_UNAVAILABLE` as canonical public failures.

- [ ] **Step 6: Run API tests**

Run: `go test ./apps/provisioner/internal/server ./apps/manager/internal/provisioner ./apps/manager/internal/functions ./apps/manager/internal/httpapi -v`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add apps/provisioner/internal/runtime apps/provisioner/internal/server apps/manager/internal/provisioner apps/manager/internal/functions apps/manager/internal/httpapi
git commit -m "feat(logs): expose per-function log queries"
```

### Task 7: Build the Function Logs Page

**Files:**
- Modify: `apps/web/src/api/types.ts`
- Modify: `apps/web/src/features/project/FunctionsPage.tsx`
- Create: `apps/web/src/features/project/FunctionLogsPage.tsx`
- Create: `apps/web/src/features/project/FunctionLogsPage.test.tsx`
- Modify: `apps/web/src/app/router.tsx`
- Modify: `apps/web/src/app/router.test.tsx`
- Modify: `apps/web/src/styles.css`

- [ ] **Step 1: Write failing route and Actions-menu tests**

Assert that `View logs` is present for each row and navigates to `/projects/bee/functions/api/logs`. Assert the nested route renders `FunctionLogsPage` and does not add a secondary-navigation item.

- [ ] **Step 2: Write failing page behavior tests**

Mock the API and verify the heading, newest-first rows, level and search query parameters, empty/offline/incompatible/dropped/disabled/error states, five-second polling, pause/resume, manual refresh, deduplication, and `Load older` cursor behavior. Use fake timers for polling.

- [ ] **Step 3: Run tests to verify failure**

Run: `npm --prefix apps/web test -- --run src/features/project/FunctionLogsPage.test.tsx src/app/router.test.tsx`

Expected: FAIL because the page and route do not exist.

- [ ] **Step 4: Implement types, route, and menu entry**

Add TypeScript equivalents of `FunctionLogRecord`, `FunctionLogHealth`, and `FunctionLogPage`. Add nested route `:functionName/logs`. Insert a non-destructive `ScrollText` menu item before deploy/rollback/delete and navigate using encoded route parameters.

- [ ] **Step 5: Implement polling and filters**

Use TanStack Query with a five-second `refetchInterval` only while not paused. Fetch 200 newest records initially; for incremental refresh pass the newest cursor. Merge by record ID and preserve newest-first order. Debounce search by 300 ms and cap input at 256 UTF-8 bytes before requesting. Retain rendered records on transient query failure.

- [ ] **Step 6: Implement responsive presentation**

Use existing PageHeader, Button, Input, Select, Badge, Alert, Card, and Table components. Keep timestamp/level/event type compact, render messages in the existing monospace font with whitespace preserved, and allow horizontal scrolling on narrow displays.

- [ ] **Step 7: Run web tests and build**

Run:

```bash
npm --prefix apps/web test -- --run src/features/project/FunctionLogsPage.test.tsx src/app/router.test.tsx
npm --prefix apps/web run build
```

Expected: tests PASS and the production build succeeds.

- [ ] **Step 8: Commit**

```bash
git add apps/web/src
git commit -m "feat(web): add per-function log viewer"
```

### Task 8: Add End-to-End Isolation and Failure Coverage

**Files:**
- Create: `tests/integration/function_logs_test.go`
- Modify: `scripts/run-acceptance.sh`
- Modify: `README.md`
- Modify: `docs/operations/troubleshooting.md`

- [ ] **Step 1: Write the failing integration test**

Deploy `api` and `deliver-push`; invoke both concurrently with unique console output; make `api` throw once. Poll their Manager log endpoints and assert each unique message appears only for its owning function, the exception is attributed to `api`, and boot events have an exact owner.

- [ ] **Step 2: Add collector failure coverage**

Stop `function-log-collector`, invoke a healthy function, and assert its response remains successful. Assert the log endpoint reports `offline` or dropped events without returning unfiltered Docker logs. Restart the collector and assert health recovers.

- [ ] **Step 3: Run the integration tests**

Run: `go test -tags=integration ./tests/integration -run FunctionLogs -v`

Expected: PASS against the acceptance stack.

- [ ] **Step 4: Document operation and recovery**

Document the Actions-menu entry, five-second refresh, seven-day/512 MiB policy, collector health meanings, upgrade compatibility gate, and commands to inspect only the collector container when collection is offline. State explicitly that optional Logflare/Vector is not required.

- [ ] **Step 5: Run the full repository verification**

Run:

```bash
go test ./...
npm --prefix apps/web test -- --run
npm --prefix apps/web run build
make verify-template
```

Expected: every command exits zero.

- [ ] **Step 6: Commit**

```bash
git add tests/integration scripts/run-acceptance.sh README.md docs/operations/troubleshooting.md
git commit -m "test(logs): verify function log isolation"
```

### Task 9: Final Security and Compatibility Verification

**Files:**
- Modify only files required by failures found in this task.

- [ ] **Step 1: Prove secrets do not escape**

Run the focused redaction and API tests with sentinel values in every project secret class, then search captured responses, collector logs, Manager logs, and the SQLite file for those sentinels.

Expected: no sentinel appears outside the mounted source environment fixture.

- [ ] **Step 2: Prove resource bounds**

Generate more than 1,000 queued events, batches larger than 100, messages larger than 10 KiB, and enough fixture records to trigger injected capacity cleanup.

Expected: the adapter drops oldest queued events, collector rejects oversized batches, messages are marked truncated, and oldest persisted rows are deleted first.

- [ ] **Step 3: Re-run pinned-runtime compatibility**

Run: `go test ./internal/templates/manager/function-logs/event-worker -run TestPinnedEventFixturesCarryExactFunctionAttribution -v`

Expected: PASS against fixtures captured from v1.74.0.

- [ ] **Step 4: Run final verification and inspect the diff**

Run:

```bash
git diff --check
go test ./...
npm --prefix apps/web test -- --run
npm --prefix apps/web run build
make verify-template
git status --short
```

Expected: checks pass and the worktree is clean. If a check fails, return to
the task that owns the failing file, add a regression test there, apply the
minimal fix, repeat that task's verification command, and commit it with that
task's listed file set before repeating this final step.
