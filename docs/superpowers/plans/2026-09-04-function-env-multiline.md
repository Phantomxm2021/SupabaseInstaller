# Multiline Function Environment Values Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Allow safely encoded multiline PEM values in Edge Function environment variables while retaining control-character protection.

**Architecture:** Give dotenv validation an explicit policy for safe whitespace controls. The general runtime environment keeps the strict policy; only the Function environment uses the multiline policy before the existing quoted dotenv renderer.

**Tech Stack:** Go, table-driven tests, existing dotenv renderer.

---

### Task 1: Add multiline Function environment support

**Files:**
- Modify: `apps/provisioner/internal/render/environment.go`
- Test: `apps/provisioner/internal/render/render_test.go`

- [ ] **Step 1: Write failing tests**

Add a render test with LF and CRLF PEM values and assert rendering succeeds with escaped quoted output. Add a NUL case and assert the existing unsupported-control-character error remains.

- [ ] **Step 2: Verify the regression test fails**

Run: `go test ./apps/provisioner/internal/render -run 'Test.*Function.*Multiline' -count=1`

Expected: FAIL because LF/CR are rejected.

- [ ] **Step 3: Implement the narrow validation policy**

Change the validator to accept a policy flag or dedicated Function validator. Under the Function policy, accept only `\t`, `\n`, and `\r`; reject all other Unicode and C1 control characters. Keep the general environment call strict.

- [ ] **Step 4: Verify focused and full tests**

Run: `go test ./apps/provisioner/internal/render -count=1 && go test ./...`

Expected: PASS.

- [ ] **Step 5: Commit**

Run: `git add apps/provisioner/internal/render/environment.go apps/provisioner/internal/render/render_test.go docs/superpowers/specs/2026-09-04-function-env-multiline-design.md docs/superpowers/plans/2026-09-04-function-env-multiline.md && git commit -m 'fix(functions): allow multiline environment values'`
