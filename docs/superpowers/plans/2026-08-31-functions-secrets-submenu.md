# Functions Secrets Submenu Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Functions-local secondary navigation with Deployments and Secrets, keeping Function environment values encrypted and write-only.

**Architecture:** A shared route-aware `FunctionsNavigation` component is rendered by the existing deployment view and a new secrets view. The secrets view uses the existing redacted configuration API, `FunctionsSection`, and configuration mutation workflow; it introduces no new secret API or storage.

**Tech Stack:** React, React Router, TanStack Query, React Hook Form, Base UI, Vitest, Testing Library.

---

### Task 1: Navigation

**Files:**

- Create: `apps/web/src/features/project/FunctionsNavigation.tsx`
- Create: `apps/web/src/features/project/FunctionsNavigation.test.tsx`
- Modify: `apps/web/src/features/project/FunctionsPage.tsx`

- [ ] Write a failing test that renders `FunctionsNavigation` at `/projects/bee/functions`, clicks the `Secrets` tab, and expects navigation to `/projects/bee/functions/secrets`.
- [ ] Run `npm run test --workspace apps/web -- --run src/features/project/FunctionsNavigation.test.tsx` and confirm the missing-component failure.
- [ ] Implement a controlled `Tabs` component using `useLocation` and `useNavigate`. Its values are `deployments` and `secrets`; changing values navigates to `/projects/${projectId}/functions` and `/projects/${projectId}/functions/secrets` respectively.
- [ ] Render the component below `PageHeader` in `FunctionsPage`.
- [ ] Re-run the focused test and commit with `feat: add functions secondary navigation`.

### Task 2: Secrets workspace

**Files:**

- Create: `apps/web/src/features/project/FunctionSecretsPage.tsx`
- Create: `apps/web/src/features/project/FunctionSecretsPage.test.tsx`
- Modify: `apps/web/src/app/router.tsx`

- [ ] Write a failing route test using a redacted configuration response with `{ name: 'STRIPE_KEY', valueSet: true, value: { action: '' } }`. It must display `STRIPE_KEY` and `Configured`, but not `stored-secret`.
- [ ] Run `npm run test --workspace apps/web -- --run src/features/project/FunctionSecretsPage.test.tsx` and confirm the route/page failure.
- [ ] Implement `FunctionSecretsPage` with `useQuery(['project-configuration', projectId])`, `normalizeRedactedConfiguration`, `FunctionsSection`, `useConfigurationMutation`, `PageHeader`, `FunctionsNavigation`, confirmation dialog, field-error handling, stale-revision handling, and `OperationPanel`.
- [ ] The pending save must set `section: 'functions'`; calculate labels with `dirtyLabels`, services with `affectedServices`, and impact with `sectionImpact`.
- [ ] Add `{ path: 'functions/secrets', element: <FunctionSecretsPage /> }` to the project router and verify the focused test passes.
- [ ] Commit with `feat: add functions secrets workspace`.

### Task 3: Secure save regression coverage

**Files:**

- Modify: `apps/web/src/features/project/FunctionSecretsPage.test.tsx`
- Modify: `apps/web/src/features/project/FunctionsPage.test.tsx`

- [ ] Add a failing test that types `replacement-secret`, confirms the save dialog, and asserts the PATCH body contains `{ action: 'replace', value: 'replacement-secret' }` while no rendered element contains that value.
- [ ] Run `npm run test --workspace apps/web -- --run src/features/project/FunctionSecretsPage.test.tsx src/features/project/FunctionsPage.test.tsx` and confirm it fails before final wiring.
- [ ] Finish the minimal mutation wiring; retain Base UI tab semantics and do not add an API endpoint or write plaintext values to query data.
- [ ] Run focused verification: `npm run test --workspace apps/web -- --run src/features/project/FunctionsNavigation.test.tsx src/features/project/FunctionSecretsPage.test.tsx src/features/project/FunctionsPage.test.tsx src/app/router.test.tsx`.
- [ ] Run full verification: `npm test`, `npm run build`, and `go test ./... -count=1`.
- [ ] Commit tests with `test: cover functions secrets navigation`.
