# Dashboard Visual System Alignment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every implemented Manager route and shared UI primitive visually match the current Supabase Dashboard while preserving Manager copy and behavior.

**Architecture:** Establish one dark Dashboard token layer and make every primitive consume it. Replace page-level primitive overrides with shared patterns, then migrate route families in dependency order and verify each at desktop, tablet, and mobile widths.

**Tech Stack:** React 19, TypeScript, Tailwind CSS 4, Base UI, CVA, Vitest, Chrome visual inspection.

**Spec:** `docs/superpowers/specs/2026-08-31-dashboard-visual-system-alignment-design.md`

## Global Constraints

- Preserve Manager routes, API calls, mutation payloads, data models, permissions, and Manager-specific text.
- Use Dashboard computed styles as the visual source of truth; do not estimate values.
- Centralize primitive typography, dimensions, colors, radii, and states; route CSS defines only layout and genuine route-specific visuals.
- Support 1440px, 1024px, and 375px for each route family.
- Do not commit unrelated root changes to `package.json`, `package-lock.json`, or `scripts/generate-apple-secret.js`.

---

### Task 1: Capture visual baseline and font dependency

**Files:**
- Create: `docs/superpowers/audits/2026-08-31-dashboard-visual-baseline.md`
- Modify: `apps/web/package.json`, `package.json`, `package-lock.json`

**Produces:** Captured token values and `@fontsource-variable/source-code-pro` for Dashboard form/code typography.

- [ ] Write the baseline table for type hierarchy, controls, tables, cards, dialogs, shell, and mobile navigation. Record official and current Manager computed values at 1440px.
- [ ] Run `npm exec --workspace apps/web -- vitest run src/components/ui/primitives.test.tsx` to establish the behavioral baseline.
- [ ] Run `npm install @fontsource-variable/source-code-pro --workspace apps/web` and verify only the expected dependency files change on the feature branch.
- [ ] Run `npm run build`; expected: production build passes.
- [ ] Commit audit and dependency updates with `docs: capture dashboard visual baseline`.

### Task 2: Build shared Dashboard tokens and primitives

**Files:**
- Modify: `apps/web/src/styles.css`
- Modify: `apps/web/src/components/ui/{button,input,textarea,select,badge,card,table,dialog,dropdown-menu}.tsx`
- Test: `apps/web/src/components/ui/primitives.test.tsx`

**Produces:** Token-backed named variants for Dashboard compact controls, cards, tables, menus, and dialogs.

- [ ] Add failing tests for primary, outline, ghost, destructive, compact, and icon buttons; Input, Textarea, Badge, Card, and Table header semantic presentation classes.
- [ ] Run `npm exec --workspace apps/web -- vitest run src/components/ui/primitives.test.tsx`; expected: failure because Dashboard variants are absent.
- [ ] Import Source Code Pro and define one dark token layer for typography, 26/30/34px control heights, 6/7/8px radii, surfaces, borders, focus, disabled, and error states. Migrate primitives to token-backed CVA variants.
- [ ] Rerun the focused primitive test; expected: pass.
- [ ] Commit with `feat: establish dashboard UI primitives`.

### Task 3: Align shell and shared page patterns

**Files:**
- Modify: `apps/web/src/app/AppShell.tsx`
- Modify: `apps/web/src/components/app/{PageHeader,SettingRow,AsyncState}.tsx`
- Modify: corresponding tests and `apps/web/src/styles.css`

**Produces:** Fixed top bar, global rail, workspace shell, page headers, setting rows, table shells, and feedback states shared by all routes.

- [ ] Add failing tests for desktop rail/top-bar landmarks, mobile navigation trigger, and PageHeader/SettingRow structural classes.
- [ ] Run `npm exec --workspace apps/web -- vitest run src/app/AppShell.test.tsx src/components/app`; expected: failure for the new shared structure.
- [ ] Implement Dashboard shell dimensions and responsive offsets. Remove global heading and broad `[data-slot]` overrides that compete with primitives.
- [ ] Inspect `/projects`, `/projects/:id/overview`, and `/projects/:id/functions` at all three target widths; verify fixed offsets and no document-level horizontal overflow.
- [ ] Rerun focused tests and commit with `feat: align dashboard shell patterns`.

### Task 4: Align entry, project, wizard, overview, Manager settings, and Server Settings

**Files:**
- Modify: `apps/web/src/features/auth/{LoginPage,SetupPage}.tsx`
- Modify: `apps/web/src/features/projects/*.tsx`, `apps/web/src/features/projects/wizard/*.tsx`
- Modify: `apps/web/src/features/project/{OverviewPage,LifecycleActions,ServiceTable,DeleteProjectDialog,ConfigurationPage}.tsx`
- Modify: `apps/web/src/features/project/configuration/*.tsx`
- Modify: `apps/web/src/features/settings/ManagerSettingsPage.tsx`
- Test: matching route tests and `apps/web/src/styles.css`

**Produces:** Dashboard-density cards, toolbars, wizards, overview surfaces, settings navigation, fields, toggle rows, secret editors, and save footers.

- [ ] Add failing presentation assertions to existing route tests while retaining all API, payload, validation, secret action, and destructive-action assertions.
- [ ] Run `npm exec --workspace apps/web -- vitest run src/features/auth src/features/projects src/features/project/OverviewPage.test.tsx src/features/project/LifecycleActions.test.tsx src/features/project/DeleteProjectDialog.test.tsx src/features/project/ConfigurationPage.test.tsx src/features/settings`; expected: failure for visual hooks.
- [ ] Replace bespoke control/card/header CSS with Tasks 2–3 patterns without changing schemas, dirty tracking, server creation, lifecycle actions, configuration endpoints, or secret semantics.
- [ ] Inspect each route family at all three widths, rerun the focused suite, and commit with `feat: align project and settings visuals`.

### Task 5: Align the entire Authentication workspace

**Files:**
- Modify: `apps/web/src/features/authentication/{AuthenticationWorkspace,UsersPage,OAuthAppsPage,SignInProvidersPage,ProviderSheet,EmailsPage,EmailTemplateEditorPage,RateLimitsPage,MultiFactorPage,AuthenticationUnavailablePage}.tsx`
- Test: `apps/web/src/features/authentication/*.test.tsx`
- Modify: `apps/web/src/styles.css`

**Produces:** One Dashboard-consistent Authentication workspace across supported and unavailable subroutes.

- [ ] Add failing tests for selected navigation, compact table/toolbars, drawer/dialog structure, settings hierarchy, empty states, and mobile Sheet navigation while retaining auth mutation and unsaved-change tests.
- [ ] Run `npm exec --workspace apps/web -- vitest run src/features/authentication`; expected: failure for visual hooks.
- [ ] Migrate all Authentication subroutes from bespoke primitive overrides to shared patterns without changing payloads or confirmation behavior.
- [ ] Inspect all Authentication subroutes at all three widths, rerun the suite, and commit with `feat: align authentication workspace visuals`.

### Task 6: Align Functions, operations, final responsive states, and release

**Files:**
- Modify: `apps/web/src/features/project/{FunctionsWorkspace,FunctionsNavigation,FunctionsPage,FunctionSecretsPage}.tsx`
- Modify: `apps/web/src/features/operations/OperationPanel.tsx`
- Test: matching feature tests
- Modify: `apps/web/src/styles.css` and baseline audit

**Produces:** Final Dashboard-aligned Functions deployment and Secrets views without duplicate status UI.

- [ ] Add failing tests for secondary nav, deploy dialog/status, managed-functions table, Secrets form/search/default table, and no duplicate deployment status after close.
- [ ] Run `npm exec --workspace apps/web -- vitest run src/features/project/FunctionsPage.test.tsx src/features/project/FunctionSecretsPage.test.tsx src/features/project/FunctionsWorkspace.test.tsx src/features/operations/OperationPanel.test.tsx`; expected: failure for shared presentation hooks.
- [ ] Migrate Functions, Secrets, and operation UI to shared patterns without changing ZIP deployment, secret mutation, polling, rollback, delete, or dialog-only deployment feedback.
- [ ] Run `npm exec --workspace apps/web -- vitest run --reporter=dot`, `npm run build`, `go test ./... -count=1`, and `git diff --check`; expected: all pass.
- [ ] Inspect every implemented route from `app/router.tsx` at all three widths; update the audit and commit with `feat: align functions and final dashboard visuals`.
