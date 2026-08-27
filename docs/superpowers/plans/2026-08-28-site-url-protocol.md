# Fixed HTTPS Site URL Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the create-project Basic step prepend `https://` automatically so users enter only the Site URL hostname.

**Architecture:** Keep `configuration.general.siteUrl` as the canonical full URL because the backend validates and persists it. The Basic step will present an editable hostname and derive the full URL through React Hook Form.

**Tech Stack:** React, TypeScript, React Hook Form, Vitest, Testing Library.

---

### Task 1: Add a failing form contract test

**Files:**
- Modify: `apps/web/src/features/projects/NewProjectPage.test.tsx`

- [ ] Add a test that fills `Site URL hostname` with `app.example.com`, navigates to Review, installs the project, and asserts `body.configuration.general.siteUrl === 'https://app.example.com'`.
- [ ] Run `npm --prefix apps/web test -- NewProjectPage.test.tsx -t "prefixes the Basic-step Site URL"` and confirm it fails because the current free-form input expects the protocol.

### Task 2: Replace the free-form Site URL input

**Files:**
- Modify: `apps/web/src/features/projects/BasicStep.tsx`

- [ ] Add a `httpsPrefix` constant plus helpers that strip one leading `https://` for display and produce either an empty string or a complete `https://<hostname>` value for storage.
- [ ] Replace `project-site-url` with a grouped control: a non-editable `https://` prefix and `project-site-url-hostname` text input. Its change handler must call `form.setValue('configuration.general.siteUrl', normalizedValue, { shouldDirty: true, shouldValidate: true })`.
- [ ] Re-run the focused test and confirm it passes.

### Task 3: Verify the create wizard

**Files:**
- Modify: `apps/web/src/features/projects/NewProjectPage.test.tsx`

- [ ] Update existing test input from `Site URL` / `https://example.com` to `Site URL hostname` / `example.com`.
- [ ] Run `npm --prefix apps/web test -- NewProjectPage.test.tsx` and `npm --prefix apps/web run build`.
- [ ] Commit the implementation with `git commit -m "fix: prefix project site URL with https"`.
