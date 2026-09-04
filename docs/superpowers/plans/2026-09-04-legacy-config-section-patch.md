# Legacy Configuration Section Patch Compatibility Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let existing projects update Function secrets without unrelated legacy configuration omissions causing HTTP 422.

**Architecture:** Normalize only the persisted merge base before applying a section patch. Preserve strict validation of values explicitly supplied by the current request.

**Tech Stack:** Go, SQLite-backed Manager configuration service, HTTP tests.

---

### Task 1: Normalize stored state before section merge

**Files:**
- Modify: `apps/manager/internal/project/configuration_service.go`
- Test: `apps/manager/internal/httpapi/configuration_test.go`

- [ ] **Step 1: Reproduce the 422 through the HTTP endpoint**

Create a legacy project with `JWTExpiry = 0`, submit a valid Function secret replacement, and expect HTTP 202. Verify the old code returns 422 with `auth.jwtExpiry`.

- [ ] **Step 2: Normalize the merge base**

Call `NormalizeStoredConfiguration` on the persisted aggregate before applying the incoming patch.

- [ ] **Step 3: Verify compatibility and strictness**

Run the focused HTTP test, project/configuration tests, the complete Go suite, Race tests for affected packages, and `go vet ./...`.

- [ ] **Step 4: Commit**

Commit the implementation, regression test, spec, and plan as `fix(config): normalize legacy state before section updates`.
