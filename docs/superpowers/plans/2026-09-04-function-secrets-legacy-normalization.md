# Function Secrets Legacy Normalization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Allow valid Function secret updates on legacy projects and make GitHub template rate-limit failures actionable with a safe verified-cache fallback.

**Architecture:** Normalize only the stored merge base before section patches, preserving strict validation for request values. Classify official-template HTTP errors as typed safe errors, and let Runtime Sync retry the already-applied immutable ref from verified cache only when refreshing latest fails.

**Tech Stack:** Go, `net/http`, SQLite-backed configuration service, Vitest/React UI regression coverage.

---

### Task 1: Normalize legacy stored upload limits

**Files:**
- Modify: `apps/manager/internal/project/configuration.go`
- Test: `apps/manager/internal/project/configuration_test.go`
- Test: `apps/manager/internal/project/configuration_service_test.go`
- Test: `apps/manager/internal/httpapi/configuration_test.go`

- [x] Add a failing unit test proving `NormalizeStoredConfiguration` turns a zero upload limit into `50 * 1024 * 1024` while the existing service test still rejects an explicit zero Storage patch.
- [x] Add a failing HTTP test that stores a legacy zero limit and submits a valid new Functions replacement.
- [x] Run `go test ./apps/manager/internal/project ./apps/manager/internal/httpapi` and confirm the legacy Functions update fails with `storage.uploadFileSizeLimit`.
- [x] Set the compatibility default in `NormalizeStoredConfiguration`; do not alter request validation semantics.
- [x] Re-run the focused tests and commit as `fix(config): normalize legacy upload limits`.

### Task 2: Preserve GitHub rate-limit evidence

**Files:**
- Modify: `apps/provisioner/internal/officialtemplate/source.go`
- Test: `apps/provisioner/internal/officialtemplate/source_test.go`
- Modify: `apps/provisioner/internal/runtime/reconcile.go`
- Test: `apps/provisioner/internal/runtime/reconcile_test.go`

- [x] Add a failing HTTP-server test returning 403 plus `X-RateLimit-Remaining: 0` and `X-RateLimit-Reset`, asserting a typed/safe error reports the GitHub API rate limit and reset timestamp.
- [x] Add a failing diagnostic test asserting `redactedReconcileDiagnostic` preserves only that safe rate-limit explanation.
- [x] Implement bounded response classification without copying arbitrary response bodies into diagnostics.
- [x] Run `go test ./apps/provisioner/internal/officialtemplate ./apps/provisioner/internal/runtime` and commit as `fix(runtime): report GitHub template rate limits`.

### Task 3: Fall back to the verified current template cache

**Files:**
- Modify: `apps/provisioner/internal/runtime/reconcile.go`
- Test: `apps/provisioner/internal/runtime/reconcile_test.go`

- [x] Add a failing reconcile test where forced latest refresh fails, metadata contains a current immutable ref, and resolving that ref from cache succeeds.
- [x] Add a no-cache test proving the original retrieval failure remains fatal and runtime state is unchanged.
- [x] Implement a single fallback resolve of `metadata.TemplateRef` with `refresh=false`; log the fallback ref and never accept an unverified file set.
- [x] Run focused runtime tests and commit as `fix(runtime): reuse verified template cache on sync`.

### Task 4: Verify the complete change

**Files:**
- Test only: repository-wide suites

- [x] Run `go test ./...` and require zero failures.
- [x] Run `npm exec --workspace apps/web -- vitest run` and require all tests pass.
- [x] Run `npm run lint` and `npm run build`.
- [x] Run `git diff --check`, inspect the final diff for secret leakage and unrelated changes, then update all plan checkboxes.
