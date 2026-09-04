# Function Log Collector Pull Policy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prevent official runtime synchronization from pulling the locally built Function log collector image from a registry.

**Architecture:** Express local-image ownership declaratively in the rendered Compose service with `pull_policy: never`. Keep the existing global official-image pull step unchanged.

**Tech Stack:** Go renderer, YAML, Docker Compose.

---

### Task 1: Mark collector image as local-only

**Files:**
- Modify: `apps/provisioner/internal/render/render.go`
- Test: `apps/provisioner/internal/render/render_test.go`

- [ ] **Step 1: Add a failing render test**

Extend the collector render test to require `pull_policy: never` and verify an official service does not receive that policy.

- [ ] **Step 2: Verify RED**

Run: `go test ./apps/provisioner/internal/render -run TestRenderFunctionsWiresPrivateEventCollector -count=1`

Expected: FAIL because the collector has no pull policy.

- [ ] **Step 3: Implement the minimal renderer change**

Add `"pull_policy": "never"` to the collector service map only.

- [ ] **Step 4: Verify**

Run render and runtime tests, `go test ./...`, `go vet ./...`, and validate the generated deployment Compose configuration.

- [ ] **Step 5: Commit**

Commit the two docs, renderer, and test as `fix(logs): keep collector image local during sync`.
