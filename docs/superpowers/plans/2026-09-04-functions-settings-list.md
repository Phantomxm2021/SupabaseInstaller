# Functions Settings List and JWT Expiry Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Function environment-variable management compact and reliable, and expose JWT expiry in the active Manager Authentication UI.

**Architecture:** Keep existing typed configuration APIs and secret actions. Change only React presentation/state handling and reuse the existing confirmation/reconcile workflow.

**Tech Stack:** React 19, react-hook-form, Zod, Vitest, Testing Library, TypeScript.

---

### Task 1: Add Functions list regression tests

**Files:**
- Create: `apps/web/src/features/project/configuration/FunctionsSection.test.tsx`
- Modify: `apps/web/src/features/project/configuration/FunctionsSection.tsx`
- Modify: `apps/web/src/styles.css`

- [ ] Test semantic columns and compact rows.
- [ ] Test adding a new variable submits `replace`.
- [ ] Test removing a configured variable submits `remove` and Undo restores it.
- [ ] Implement the table editor and focused styles.
- [ ] Run the focused Vitest file.

### Task 2: Expose JWT expiry

**Files:**
- Modify: `apps/web/src/features/authentication/SignInProvidersPage.tsx`
- Modify: `apps/web/src/features/authentication/SignInProvidersPage.test.tsx`
- Modify: `apps/web/src/styles.css`

- [ ] Test the field is visible with min 1 and max 604800.
- [ ] Test changing it submits the Auth PATCH payload.
- [ ] Add the Session settings row using react-hook-form numeric registration.
- [ ] Run the focused Authentication tests.

### Task 3: Full verification

- [ ] Run all web tests, build, lint, full Go tests, and `git diff --check`.
- [ ] Commit as `fix(web): streamline function environment settings`.
