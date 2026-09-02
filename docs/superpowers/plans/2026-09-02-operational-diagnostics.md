# Operational Diagnostics Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Preserve a secret-safe, actionable cause for every Manager, Provisioner, and Nginx Proxy operational failure without changing public error codes or validation/authentication behavior.

**Architecture:** Local runtime code retains dependency causes with `%w` and labelled joins. Private service boundaries serialize a bounded `diagnostic` separately from canonical errors after redaction; Manager clients consume diagnostics only for an allow-listed operational endpoint/code pair and persist that value through the durable operation. A shared internal sanitizer prevents the Provisioner and Nginx Proxy from diverging.

**Tech Stack:** Go, `net/http`, JSON contracts, `errors.Join`, `fmt.Errorf`, `testing`, existing operation store and Docker Compose runner.

---

## File Structure

- Create: `internal/diagnostic/diagnostic.go` — bounded redaction shared by all three processes.
- Create: `internal/diagnostic/diagnostic_test.go` — secret, token, certificate, control-character, and truncation coverage.
- Modify: `internal/contracts/provisioner.go`, `internal/contracts/functions.go`, `internal/contracts/tls.go` — typed optional diagnostics on private failure responses.
- Modify: `apps/provisioner/internal/server/server.go` and tests — sanitize and serialize diagnostics for each operational endpoint.
- Modify: `apps/provisioner/internal/runtime/{backend,reconcile,rotate,functions}.go` and tests — retain existing causes rather than replacing them with generic errors.
- Modify: `apps/manager/internal/provisioner/{client,client_test}.go` — decode typed diagnostics only for approved endpoint/code pairs.
- Modify: `apps/manager/internal/{configuration,lifecycle,functions,install}` and tests — carry the trusted client error into `operations.Fail`.
- Modify: `apps/nginxproxy/internal/server/{server,server_test}.go` and `apps/provisioner/internal/proxy/{client,client_test}.go` — introduce redacted Nginx diagnostic envelopes and decode them safely.

### Task 1: Shared diagnostic contract and sanitizer

**Files:**
- Create: `internal/diagnostic/diagnostic.go`
- Create: `internal/diagnostic/diagnostic_test.go`
- Modify: `internal/contracts/provisioner.go:176-184`
- Modify: `internal/contracts/functions.go:25-29`
- Modify: `internal/contracts/tls.go:32-35`

- [ ] **Step 1: Write failing sanitizer and contract tests.**

```go
func TestSanitizeRedactsKnownValuesPatternsAndBoundsOutput(t *testing.T) {
	got := diagnostic.Sanitize(
		"POSTGRES_PASSWORD=db-secret Authorization: Bearer proxy-token\nprivate-key-value",
		[]string{"db-secret", "proxy-token", "private-key-value"},
	)
	for _, secret := range []string{"db-secret", "proxy-token", "private-key-value"} {
		if strings.Contains(got, secret) { t.Fatalf("diagnostic leaked %q: %q", secret, got) }
	}
	if !strings.Contains(got, "[REDACTED]") || strings.Contains(got, "\n") { t.Fatalf("unsafe diagnostic: %q", got) }
}

func TestOperationalResponseDiagnosticsAreOptional(t *testing.T) {
	payload, err := json.Marshal(contracts.ErrorEnvelope{Error: contracts.APIError{Code: "LIFECYCLE_FAILED", Message: "Server lifecycle operation failed"}})
	if err != nil || strings.Contains(string(payload), "diagnostic") { t.Fatalf("legacy payload=%s err=%v", payload, err) }
}
```

- [ ] **Step 2: Run the tests to verify RED.**

Run: `go test ./internal/diagnostic -run '^(TestSanitizeRedactsKnownValuesPatternsAndBoundsOutput|TestOperationalResponseDiagnosticsAreOptional)$' -count=1`

Expected: compilation failure because package `internal/diagnostic` and `ErrorEnvelope.Diagnostic` do not exist.

- [ ] **Step 3: Implement the contract and sanitizer.**

```go
// internal/contracts/provisioner.go
type ErrorEnvelope struct {
	Error      APIError `json:"error"`
	Diagnostic string   `json:"diagnostic,omitempty"`
}

// Add the same optional field to failure-capable typed bodies.
type ReconcileProjectResponse struct { /* existing fields */ Diagnostic string `json:"diagnostic,omitempty"` }
type FunctionDeploymentResult struct { /* existing fields */ Error *APIError `json:"error,omitempty"`; Diagnostic string `json:"diagnostic,omitempty"` }
type StageManagedTLSResponse struct { /* existing fields */ Diagnostic string `json:"diagnostic,omitempty"` }
```

```go
// internal/diagnostic/diagnostic.go
const marker = "[REDACTED]"
const maxBytes = 4 << 10

func Sanitize(cause error, known []string) string {
	if cause == nil { return "" }
	return sanitizeString(cause.Error(), known) // replace longest known values, credential assignments, bearer tokens; flatten controls; truncate to maxBytes
}
```

Move the reusable credential-assignment regular expressions from
`apps/provisioner/internal/redact/redact.go` into this package, preserving the
existing `redact.New(...).String(...)` API as a forwarding wrapper so unrelated
callers do not change.

- [ ] **Step 4: Run the package tests to verify GREEN.**

Run: `go test ./internal/diagnostic ./apps/provisioner/internal/redact -count=1`

Expected: PASS, with existing Provisioner redaction tests still passing.

- [ ] **Step 5: Commit the contract foundation.**

```bash
git add internal/diagnostic internal/contracts/provisioner.go internal/contracts/functions.go internal/contracts/tls.go apps/provisioner/internal/redact
git commit -m "feat: add safe operational diagnostics contract"
```

### Task 2: Provisioner runtime cause preservation

**Files:**
- Modify: `apps/provisioner/internal/runtime/reconcile.go:40-293`
- Modify: `apps/provisioner/internal/runtime/rotate.go:77-354`
- Modify: `apps/provisioner/internal/runtime/backend.go:61-230`
- Modify: `apps/provisioner/internal/runtime/functions.go`
- Test: `apps/provisioner/internal/runtime/{reconcile,rotate,functions}_test.go`

- [ ] **Step 1: Write failing tests for discarded operational causes.**

```go
func TestRotateDatabasePasswordReportsHealthAndRollbackCauses(t *testing.T) {
	// Make waitHealthy return "runtime health is UNHEALTHY" and rollback Recreate return "restore auth failed".
	_, err := backend.RotateDatabasePassword(context.Background(), request)
	var failure *contracts.ReconcileFailure
	if !errors.As(err, &failure) || !strings.Contains(failure.Cause.Error(), "runtime health is UNHEALTHY") || !strings.Contains(failure.Cause.Error(), "restore auth failed") {
		t.Fatalf("failure=%v; want primary and compensation causes", err)
	}
}

func TestLifecycleWrapsComposeFailure(t *testing.T) {
	backend.runner = lifecycleFailingRunner{err: errors.New("compose action failed: output=auth exited")}
	if err := backend.Lifecycle(context.Background(), request); err == nil || !strings.Contains(err.Error(), "auth exited") {
		t.Fatalf("lifecycle error=%v", err)
	}
}
```

- [ ] **Step 2: Run the focused runtime tests to verify RED.**

Run: `go test ./apps/provisioner/internal/runtime -run '^(TestRotateDatabasePasswordReportsHealthAndRollbackCauses|TestLifecycleWrapsComposeFailure)$' -count=1`

Expected: at least one assertion fails because a branch returns a generic `errors.New("… failed")`.

- [ ] **Step 3: Replace only generic wrappers that discard a real cause.**

```go
// One dependency failure.
return fmt.Errorf("dependent service health check failed: %w", err)

// Primary failure plus failed compensation.
return fmt.Errorf("%w; rotation rollback failed: %v", primary, rollbackErr)
```

For each `errors.New("… failed")` in the listed runtime files, retain it only
when there is no lower-level `err`; otherwise replace it with the contextual
wrapped error. Keep operation status and rollback flags unchanged.

- [ ] **Step 4: Run all Provisioner runtime tests to verify GREEN.**

Run: `go test ./apps/provisioner/internal/runtime -count=1`

Expected: PASS.

- [ ] **Step 5: Commit source-cause preservation.**

```bash
git add apps/provisioner/internal/runtime
git commit -m "fix: preserve provisioner operation failure causes"
```

### Task 3: Provisioner HTTP diagnostic boundaries

**Files:**
- Modify: `apps/provisioner/internal/server/server.go:90-478`
- Test: `apps/provisioner/internal/server/server_test.go`

- [ ] **Step 1: Write endpoint-level failing tests.**

```go
func TestLifecycleEndpointReturnsRedactedDiagnostic(t *testing.T) {
	backend := &lifecycleFailureStub{err: errors.New("compose failed POSTGRES_PASSWORD=db-secret")}
	response := authenticatedJSON(t, New(Options{ManagerToken: token, Backend: backend}), "/internal/v1/projects/lifecycle", requestWithSecretSentinel())
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "compose failed") || strings.Contains(response.Body.String(), "db-secret") {
		t.Fatalf("response=%s", response.Body.String())
	}
}

func TestTLSStageEndpointRedactsCertificateAndPrivateKey(t *testing.T) {
	response := authenticatedJSON(t, handlerWithStageError(errors.New("reject certificate-pem private-key-pem")), "/internal/v1/nginx/certificates/stage", tlsRequest("certificate-pem", "private-key-pem"))
	if strings.Contains(response.Body.String(), "certificate-pem") || strings.Contains(response.Body.String(), "private-key-pem") { t.Fatal(response.Body.String()) }
}
```

Cover reconcile, rotation, lifecycle, inspect, deploy/rollback/delete function,
and TLS stage.  Assert each keeps its existing canonical `error.code` and
places the safe detail in `diagnostic`, not `error.message`.

- [ ] **Step 2: Run the endpoint tests to verify RED.**

Run: `go test ./apps/provisioner/internal/server -run '^(TestLifecycleEndpointReturnsRedactedDiagnostic|TestTLSStageEndpointRedactsCertificateAndPrivateKey)$' -count=1`

Expected: FAIL because generic envelopes have no diagnostic or raw secrets are returned.

- [ ] **Step 3: Add one boundary helper and use it for operational endpoints.**

```go
func writeOperationalFailure(w http.ResponseWriter, status int, code, message string, cause error, secrets []string) {
	writeJSON(w, status, contracts.ErrorEnvelope{
		Error: contracts.APIError{Code: code, Message: message},
		Diagnostic: diagnostic.Sanitize(cause, secrets),
	})
}
```

Build `secrets` from `ProjectSecrets`, runtime secrets, rotation old/new
passwords, function request context, and TLS certificate/private-key bytes as
appropriate.  Set typed response `Error` and `Diagnostic` for reconcile,
rotation, and compensated function responses.  Log exactly this helper's
sanitized diagnostic.  Leave all `INVALID_*`, authentication, and unavailable
capability branches using `writeError` without a diagnostic.

- [ ] **Step 4: Run all Provisioner server tests to verify GREEN.**

Run: `go test ./apps/provisioner/internal/server -count=1`

Expected: PASS.

- [ ] **Step 5: Commit the Provisioner boundary work.**

```bash
git add apps/provisioner/internal/server apps/provisioner/internal/runtime internal/contracts
git commit -m "fix: return redacted provisioner operation diagnostics"
```

### Task 4: Manager client and durable-operation propagation

**Files:**
- Modify: `apps/manager/internal/provisioner/client.go:16-355`
- Test: `apps/manager/internal/provisioner/client_test.go`
- Modify: `apps/manager/internal/configuration/orchestrator.go:450-640,880-1110`
- Modify: `apps/manager/internal/lifecycle/service.go:80-130`
- Modify: `apps/manager/internal/functions/service.go:85-180`
- Modify: `apps/manager/internal/install/orchestrator.go:180-265`
- Test: `apps/manager/internal/{provisioner,configuration,lifecycle,functions,install}/*_test.go`

- [ ] **Step 1: Write failing Manager client and operation tests.**

```go
func TestClientPreservesAllowListedLifecycleDiagnostic(t *testing.T) {
	client := NewClient(url, token, responseClient(`{"error":{"code":"LIFECYCLE_FAILED","message":"Server lifecycle operation failed"},"diagnostic":"compose action failed: auth exited"}`))
	err := client.Lifecycle(context.Background(), contracts.LifecycleRequest{})
	if err == nil || !strings.Contains(err.Error(), "auth exited") { t.Fatalf("error=%v", err) }
}

func TestClientRejectsDiagnosticForValidationCode(t *testing.T) {
	// INVALID_REQUEST with a fake diagnostic must remain the local canonical message.
}

func TestLifecycleRunPersistsProvisionerDiagnostic(t *testing.T) {
	// Fake Provisioner returns ClientError{Code: "LIFECYCLE_FAILED", Message: "compose action failed: auth exited"}.
	// Assert the durable operation ErrorMessage contains "auth exited".
}
```

- [ ] **Step 2: Run the focused Manager tests to verify RED.**

Run: `go test ./apps/manager/internal/provisioner ./apps/manager/internal/lifecycle -run '^(TestClientPreservesAllowListedLifecycleDiagnostic|TestClientRejectsDiagnosticForValidationCode|TestLifecycleRunPersistsProvisionerDiagnostic)$' -count=1`

Expected: FAIL because generic `post`, function-specific clients, or an orchestrator overwrites the diagnostic.

- [ ] **Step 3: Decode diagnostics through one allow-list.**

```go
var operationalDiagnostics = map[string]map[string]struct{}{
	"/internal/v1/projects/lifecycle": {"LIFECYCLE_FAILED": {}},
	"/internal/v1/projects/reconcile": {"RECONCILE_FAILED": {}},
	"/internal/v1/projects/rotate-database-password": {"ROTATE_DATABASE_PASSWORD_FAILED": {}},
	"/internal/v1/nginx/certificates/stage": {"TLS_STAGE_FAILED": {}},
}

func diagnosticFor(path, code, raw string) string {
	if _, ok := operationalDiagnostics[path][code]; ok && strings.TrimSpace(raw) != "" { return raw }
	return canonicalMessage(code)
}
```

Use this helper in generic `post`, the streamed function deploy path, and
`functionAction`.  Preserve `ClientError.Message` only after allow-list
validation.  In Manager operation runners, pass that `ClientError` unchanged
to `operations.Fail`; replace local generic errors only when they overwrite a
concrete downstream cause.

- [ ] **Step 4: Run the Manager operation packages to verify GREEN.**

Run: `go test ./apps/manager/internal/provisioner ./apps/manager/internal/configuration ./apps/manager/internal/lifecycle ./apps/manager/internal/functions ./apps/manager/internal/install -count=1`

Expected: PASS.

- [ ] **Step 5: Commit Manager propagation.**

```bash
git add apps/manager/internal/provisioner apps/manager/internal/configuration apps/manager/internal/lifecycle apps/manager/internal/functions apps/manager/internal/install
git commit -m "fix: persist trusted operational diagnostics"
```

### Task 5: Nginx Proxy diagnostic boundary

**Files:**
- Modify: `apps/nginxproxy/internal/server/server.go:1-160`
- Test: `apps/nginxproxy/internal/server/server_test.go`
- Modify: `apps/provisioner/internal/proxy/client.go:1-122`
- Test: `apps/provisioner/internal/proxy/client_test.go`

- [ ] **Step 1: Write failing Nginx boundary tests.**

```go
func TestApplyReturnsRedactedDiagnostic(t *testing.T) {
	h := New(token, renderer, &recordingStore{applyErr: errors.New("nginx reload failed Authorization: Bearer proxy-token")})
	response := authenticatedJSON(t, h, "/v1/sites/apply", routeWithPassword("dashboard-secret"))
	if !strings.Contains(response.Body.String(), "nginx reload failed") || strings.Contains(response.Body.String(), "proxy-token") || strings.Contains(response.Body.String(), "dashboard-secret") { t.Fatal(response.Body.String()) }
}

func TestManagedClientUsesNginxDiagnosticInsteadOfRawBody(t *testing.T) {
	// Return ErrorEnvelope{Error: APIError{Code:"PROXY_APPLY_FAILED"}, Diagnostic:"nginx -t: unknown directive"}.
	// Assert ClientError text includes the diagnostic and excludes unrelated body content.
}
```

- [ ] **Step 2: Run Nginx and proxy-client tests to verify RED.**

Run: `go test ./apps/nginxproxy/internal/server ./apps/provisioner/internal/proxy -run '^(TestApplyReturnsRedactedDiagnostic|TestManagedClientUsesNginxDiagnosticInsteadOfRawBody)$' -count=1`

Expected: FAIL because Nginx returns raw strings and the client embeds the whole body in its error.

- [ ] **Step 3: Serialize and consume a typed Nginx failure envelope.**

```go
func writeOperationalFailure(w http.ResponseWriter, status int, code, message string, cause error, known []string) {
	json.NewEncoder(w).Encode(contracts.ErrorEnvelope{
		Error: contracts.APIError{Code: code, Message: message},
		Diagnostic: diagnostic.Sanitize(cause, known),
	})
}
```

Use fixed codes `PROXY_APPLY_FAILED`, `PROXY_REMOVE_FAILED`, and
`PROXY_TLS_STAGE_FAILED`; retain existing validation responses.  In
`ManagedClient.callJSON`, decode `contracts.ErrorEnvelope`, accept
`Diagnostic` only for these three codes, and return a contextual error using
that safe value.  Do not append an arbitrary response body to an error.

- [ ] **Step 4: Run all Nginx Proxy and Provisioner proxy tests to verify GREEN.**

Run: `go test ./apps/nginxproxy/internal/... ./apps/provisioner/internal/proxy -count=1`

Expected: PASS.

- [ ] **Step 5: Commit Nginx Proxy diagnostics.**

```bash
git add apps/nginxproxy/internal/server apps/provisioner/internal/proxy
git commit -m "fix: expose safe nginx proxy diagnostics"
```

### Task 6: Audit regression guard and final verification

**Files:**
- Create: `tests/integration/operational_diagnostics_test.go`
- Modify: `docs/operations/troubleshooting.md`

- [ ] **Step 1: Write the end-to-end failing regression test.**

```go
func TestOperationalDiagnosticsReachManagerWithoutSecrets(t *testing.T) {
	// Drive a fake Provisioner reconcile failure containing "auth exited" and every project-secret sentinel.
	// Read the Manager operation and assert it includes "auth exited" while excluding each sentinel.
}
```

- [ ] **Step 2: Run the integration test to verify RED.**

Run: `go test ./tests/integration -run '^TestOperationalDiagnosticsReachManagerWithoutSecrets$' -count=1`

Expected: FAIL until the selected end-to-end operation path persists `diagnostic` instead of its generic error.

- [ ] **Step 3: Implement the smallest integration fixture support and document retrieval.**

Update `docs/operations/troubleshooting.md` with the operation fields to
inspect (`errorCode`, `errorMessage`, `currentStep`, `rolledBack`) and state
that diagnostic text is redacted and bounded.  Do not document or expose raw
service logs as a substitute for an operation diagnostic.

- [ ] **Step 4: Run the integration test and full suite.**

Run: `go test ./tests/integration -run '^TestOperationalDiagnosticsReachManagerWithoutSecrets$' -count=1 && go test ./...`

Expected: PASS.

- [ ] **Step 5: Commit final coverage and documentation.**

```bash
git add tests/integration/operational_diagnostics_test.go docs/operations/troubleshooting.md
git commit -m "test: cover redacted operational diagnostics"
```

## Plan Self-Review

- Scope coverage: Tasks 2-5 cover all Manager, Provisioner, and Nginx Proxy
  operational families defined in the approved design; Task 6 checks the
  cross-process contract and documents it.
- Security coverage: Tasks 1, 3, 4, 5, and 6 each contain explicit
  secret-sentinel assertions and preserve canonical validation/auth errors.
- Compatibility coverage: Task 4 treats a missing diagnostic as the existing
  canonical message, so old Provisioner and Nginx responses remain valid.
- Type consistency: every private response uses `contracts.ErrorEnvelope` or
  an explicitly extended contracts result; Manager and Nginx clients decode
  the same `Diagnostic` field.
