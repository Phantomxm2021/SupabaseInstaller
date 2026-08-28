# Project Wizard Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the six-step project form with a four-step, progressively disclosed project-creation wizard without changing the authoritative project configuration request.

**Architecture:** `NewProjectPage` retains ownership of the React Hook Form aggregate and install mutation. New focused wizard components render each redesigned step, while the existing service dependency mutation function and configuration schema stay authoritative. A debounced `GET /api/projects` client-side check gives early name/slug feedback; the existing create endpoint remains the final conflict authority.

**Tech Stack:** React 19, TypeScript, React Hook Form, Zod, TanStack Query, Base UI, Tailwind CSS, Vitest, Testing Library.

**Baseline:** `npm test` has 132 passing tests and four pre-existing failures in `apps/web/src/features/project/ConfigurationPage.test.tsx`: those tests call `user.clear()` on Domain after `GeneralSection` made it read-only. This plan must not modify that unrelated configuration workspace; no fifth failure is acceptable.

---

## File structure

- Create: `apps/web/src/features/projects/wizard/types.ts` — four-step identifiers and availability state.
- Create: `apps/web/src/features/projects/wizard/useProjectIdentityAvailability.ts` — debounced independent name/slug status.
- Create: `apps/web/src/features/projects/wizard/useProjectIdentityAvailability.test.ts` — hook coverage.
- Create: `apps/web/src/features/projects/wizard/WizardStepFrame.tsx` — progress and directional animated content frame.
- Create: `apps/web/src/features/projects/wizard/ServiceConfiguration.tsx` — preset side navigation and grouped service controls.
- Create: `apps/web/src/features/projects/wizard/AuthMethodDialog.tsx` — searchable single-column auth/OAuth picker.
- Create: `apps/web/src/features/projects/wizard/AuthMethodDialog.test.tsx` — picker behavior coverage.
- Create: `apps/web/src/features/projects/wizard/SecurityIntegrationsStep.tsx` — collapsible integration modules.
- Create: `apps/web/src/features/projects/wizard/InfrastructureReviewStep.tsx` — advanced infrastructure and review composition.
- Modify: `apps/web/src/features/projects/{BasicStep,NewProjectPage,OAuthProviderFields,PresetStep,ReviewStep,projectSchema}.tsx` — compose the new wizard without changing DTO semantics.
- Modify: `apps/web/src/features/projects/NewProjectPage.test.tsx` — replace six-step assertions and cover the new path.
- Modify: `apps/web/src/styles.css` — wizard grid and reduced-motion-safe transitions.

### Task 1: Add typed wizard metadata and debounced availability checking

**Files:**
- Create: `apps/web/src/features/projects/wizard/types.ts`
- Create: `apps/web/src/features/projects/wizard/useProjectIdentityAvailability.ts`
- Create: `apps/web/src/features/projects/wizard/useProjectIdentityAvailability.test.ts`
- Modify: `apps/web/src/features/projects/projectSchema.ts:832-851`

- [ ] **Step 1: Write the failing hook tests**

```tsx
it('reports independent name and slug conflicts after 400ms', async () => {
  const projects = [{ id: 'one', name: 'Production API', slug: 'production-api' }] as Project[]
  const { result } = renderHook(() => useProjectIdentityAvailability('Production API', 'production-api', projects))
  expect(result.current.name.status).toBe('checking')
  await act(async () => { await vi.advanceTimersByTimeAsync(400) })
  expect(result.current.name.status).toBe('conflict')
  expect(result.current.slug.status).toBe('conflict')
})

it('reports a lookup error as unavailable, not a conflict', () => {
  const { result } = renderHook(() => useProjectIdentityAvailability('Production API', 'production-api', undefined, new Error('offline')))
  expect(result.current.name).toMatchObject({ status: 'unavailable' })
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `npm run test --workspace apps/web -- src/features/projects/wizard/useProjectIdentityAvailability.test.ts`

Expected: FAIL because the hook does not exist.

- [ ] **Step 3: Implement the types and hook**

```ts
export const wizardSteps = [
  { id: 'identity', label: 'Project details' },
  { id: 'services', label: 'Services' },
  { id: 'integrations', label: 'Security & integrations' },
  { id: 'review', label: 'Review & install' },
] as const
export type WizardStepId = (typeof wizardSteps)[number]['id']
export type Availability = { status: 'idle' | 'checking' | 'available' | 'conflict' | 'unavailable'; message?: string }
```

In the hook, use a 400ms effect timer, trim/lowercase comparisons, return `idle` for empty/invalid values, `checking` before the timer resolves, `available` when no project matches, `conflict` only when a name or slug matches, and `unavailable` when the list request fails.

Export the existing slug validator as `projectSlugSchema` and use it in `projectSchema`:

```ts
export const projectSlugSchema = z.string().regex(
  /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/,
  'Use lowercase letters, numbers, and hyphens',
)
```

- [ ] **Step 4: Run focused verification and commit**

Run: `npm run test --workspace apps/web -- src/features/projects/wizard/useProjectIdentityAvailability.test.ts && npm run lint --workspace apps/web`

Expected: PASS and exit 0.

```bash
git add apps/web/src/features/projects/projectSchema.ts apps/web/src/features/projects/wizard/types.ts apps/web/src/features/projects/wizard/useProjectIdentityAvailability.ts apps/web/src/features/projects/wizard/useProjectIdentityAvailability.test.ts
git commit -m "feat: add project identity availability state"
```

### Task 2: Make project identity a single-column guarded first step

**Files:**
- Modify: `apps/web/src/features/projects/BasicStep.tsx`
- Modify: `apps/web/src/features/projects/NewProjectPage.tsx`
- Modify: `apps/web/src/features/projects/NewProjectPage.test.tsx:1-110`

- [ ] **Step 1: Write failing behavior tests**

```tsx
await user.type(screen.getByLabelText('Project name'), 'Production API')
await act(async () => { await vi.advanceTimersByTimeAsync(400) })
expect(screen.getByText('A project named “Production API” already exists')).toBeVisible()
expect(screen.getByRole('button', { name: /Continue/ })).toBeDisabled()
expect(screen.getByLabelText('Studio username')).toBeVisible()
```

Mock `GET /api/projects` for conflict, availability, and error cases. Assert the step contains the name, slug, hostname, username, and password in a single form flow, with version only in an “Runtime settings” collapsible.

- [ ] **Step 2: Run the test to verify it fails**

Run: `npm run test --workspace apps/web -- src/features/projects/NewProjectPage.test.tsx`

Expected: FAIL because the page does not request the project list or guard the first Continue button.

- [ ] **Step 3: Implement identity rendering and guard**

Use `FieldGroup className="space-y-6"` in `BasicStep`; do not apply `md:grid-cols-*`. Keep username and password before the optional runtime `Collapsible`. Give name and slug availability text `role="status" aria-live="polite"`; mark fields invalid on a form error or a conflict.

In `NewProjectPage`, add:

```tsx
const projectsQuery = useQuery({ queryKey: ['projects'], queryFn: () => apiFetch<{ projects: Project[] }>('/api/projects'), retry: false })
const availability = useProjectIdentityAvailability(name, form.watch('slug'), projectsQuery.data?.projects, projectsQuery.error)
const identityReady = availability.name.status === 'available' && availability.slug.status === 'available'
```

Before step 2, call `form.trigger(['name', 'slug', 'configuration.general.siteUrl', 'configuration.general.studioUsername', 'configuration.general.studioPassword'])`; proceed only if it returns true and `identityReady`. Preserve the existing POST conflict response as final race-condition protection.

- [ ] **Step 4: Verify and commit**

Run: `npm run test --workspace apps/web -- src/features/projects/NewProjectPage.test.tsx && npm run lint --workspace apps/web`

```bash
git add apps/web/src/features/projects/BasicStep.tsx apps/web/src/features/projects/NewProjectPage.tsx apps/web/src/features/projects/NewProjectPage.test.tsx
git commit -m "feat: validate project identity before progression"
```

### Task 3: Introduce four-step navigation and directional motion

**Files:**
- Create: `apps/web/src/features/projects/wizard/WizardStepFrame.tsx`
- Modify: `apps/web/src/features/projects/NewProjectPage.tsx`
- Modify: `apps/web/src/styles.css:1352-1370`
- Modify: `apps/web/src/features/projects/NewProjectPage.test.tsx`

- [ ] **Step 1: Write failing navigation tests**

```tsx
expect(screen.getByText('Step 1 of 4 · Project details')).toBeVisible()
await user.click(screen.getByRole('button', { name: 'Continue' }))
expect(screen.getByTestId('wizard-step-frame')).toHaveAttribute('data-direction', 'forward')
await user.click(screen.getByRole('button', { name: 'Back' }))
expect(screen.getByTestId('wizard-step-frame')).toHaveAttribute('data-direction', 'backward')
```

Also assert that invalid advancement keeps focus on the first `[aria-invalid="true"]` input.

- [ ] **Step 2: Run the test to verify it fails**

Run: `npm run test --workspace apps/web -- src/features/projects/NewProjectPage.test.tsx`

Expected: FAIL because the current wizard exposes six steps and no frame.

- [ ] **Step 3: Implement the frame and CSS**

```tsx
export function WizardStepFrame({ step, direction, children }: Props) {
  return <section key={step} data-testid="wizard-step-frame" data-direction={direction} className="wizard-step-frame">{children}</section>
}
```

Replace the six-step array with `wizardSteps`, remove the intermediate Review shortcut, and set `direction` before every step change. Add 200ms forward/backward slide-and-fade keyframes plus `@media (prefers-reduced-motion: reduce)` that removes transforms and uses `1ms` duration. After invalid validation, focus the first invalid field; after valid navigation, focus the wizard heading.

- [ ] **Step 4: Verify and commit**

Run: `npm run test --workspace apps/web -- src/features/projects/NewProjectPage.test.tsx && npm run lint --workspace apps/web`

```bash
git add apps/web/src/features/projects/wizard/WizardStepFrame.tsx apps/web/src/features/projects/NewProjectPage.tsx apps/web/src/styles.css apps/web/src/features/projects/NewProjectPage.test.tsx
git commit -m "feat: add animated four-step project wizard"
```

### Task 4: Implement persistent presets and grouped services

**Files:**
- Create: `apps/web/src/features/projects/wizard/ServiceConfiguration.tsx`
- Modify: `apps/web/src/features/projects/PresetStep.tsx`
- Modify: `apps/web/src/features/projects/NewProjectPage.tsx`
- Modify: `apps/web/src/features/projects/NewProjectPage.test.tsx`
- Modify: `apps/web/src/styles.css`

- [ ] **Step 1: Write failing layout/behavior tests**

```tsx
expect(screen.getByRole('navigation', { name: 'Service presets' })).toBeVisible()
expect(screen.getByRole('heading', { name: 'Core services' })).toBeVisible()
await user.click(screen.getByRole('switch', { name: 'Edge Functions' }))
expect(screen.getByText('Custom')).toHaveAttribute('aria-current', 'true')
expect(screen.getByText('API Gateway is required by enabled services')).toBeVisible()
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `npm run test --workspace apps/web -- src/features/projects/NewProjectPage.test.tsx`

Expected: FAIL because `PresetStep` uses a flat two-column grid.

- [ ] **Step 3: Implement `ServiceConfiguration` without changing dependency semantics**

Export `serviceLabels` and this grouping metadata from `PresetStep.tsx`:

```ts
export const serviceGroups = [
  { label: 'Core services', names: ['database', 'gateway', 'rest', 'auth', 'studio', 'postgresMeta'] as const },
  { label: 'Extended services', names: ['realtime', 'storage', 'imgproxy', 'functions', 'supavisor', 'logs', 'vector', 'directDb'] as const },
] as const
```

Render presets in `<nav aria-label="Service presets">` and groups in its content pane. `setServiceEnabled` remains the only service mutation function. Display existing required/disabled state and one persistent Custom hint. Add a desktop sidebar grid and a narrow-screen stacked fallback.

- [ ] **Step 4: Verify and commit**

Run: `npm run test --workspace apps/web -- src/features/projects/NewProjectPage.test.tsx && npm run lint --workspace apps/web`

```bash
git add apps/web/src/features/projects/PresetStep.tsx apps/web/src/features/projects/wizard/ServiceConfiguration.tsx apps/web/src/features/projects/NewProjectPage.tsx apps/web/src/features/projects/NewProjectPage.test.tsx apps/web/src/styles.css
git commit -m "feat: group services under persistent presets"
```

### Task 5: Add dynamic Authentication and collapsed integration modules

**Files:**
- Create: `apps/web/src/features/projects/wizard/AuthMethodDialog.tsx`
- Create: `apps/web/src/features/projects/wizard/AuthMethodDialog.test.tsx`
- Create: `apps/web/src/features/projects/wizard/SecurityIntegrationsStep.tsx`
- Modify: `apps/web/src/features/projects/OAuthProviderFields.tsx`
- Modify: `apps/web/src/features/projects/NewProjectPage.tsx`
- Modify: `apps/web/src/features/projects/NewProjectPage.test.tsx`

- [ ] **Step 1: Write failing dialog tests**

```tsx
await user.click(screen.getByRole('button', { name: 'Add authentication method' }))
expect(screen.getByRole('dialog', { name: 'Add authentication method' })).toBeVisible()
await user.type(screen.getByRole('textbox', { name: 'Search authentication methods' }), 'git')
expect(screen.getByRole('button', { name: 'Add GitHub' })).toBeVisible()
expect(screen.queryByRole('button', { name: 'Add Google' })).not.toBeInTheDocument()
await user.click(screen.getByRole('button', { name: 'Add GitHub' }))
expect(screen.getByRole('button', { name: 'Remove GitHub' })).toBeVisible()
expect(screen.queryByRole('switch', { name: /Enable GitHub/ })).not.toBeInTheDocument()
```

Add coverage that the dialog is a single-column list, already-added providers cannot be selected twice, and the remove icon requires `AlertDialog` confirmation.

- [ ] **Step 2: Run the test to verify it fails**

Run: `npm run test --workspace apps/web -- src/features/projects/wizard/AuthMethodDialog.test.tsx`

Expected: FAIL because all OAuth provider cards are eagerly rendered with switches.

- [ ] **Step 3: Implement searchable provider selection**

Use `@base-ui/react/dialog` following the repository’s `Sheet` primitive structure. Give the dialog a title, `aria-label="Search authentication methods"` input, All/Login methods/OAuth providers filters, and a single-column list. Build OAuth choices from `OAUTH_PROVIDERS` and `providerLabels`; build method choices for Email password, Magic Link, Phone Auth, Anonymous sign-in, and Custom SMTP.

Adding a provider must set its existing aggregate path to:

```ts
{ enabled: true, clientId: '', secretSet: false, secret: { action: '' }, fields: {} }
```

No per-provider switch is rendered. `OAuthProviderFields` iterates only enabled entries and renders an icon-only `Trash2` Button with `aria-label={`Remove ${label}`}`. Confirm removal in `AlertDialog`, then remove the provider key from `configuration.auth.oauth`; normalize undefined keys before create submission. Adding a login method updates the existing `email`, `phone`, `anonymousSignIn`, or `smtp` fields rather than creating a new model.

- [ ] **Step 4: Implement the folded integration owner**

`SecurityIntegrationsStep` renders Authentication, Custom SMTP, Storage & Image Transformation, and Edge Functions as `Collapsible` modules. Place each module’s switch in its title row’s right edge; render its body only while enabled. Extract reusable field blocks from `AuthStep` and `StorageFunctionsStep` instead of duplicating schema paths. Do not render a top “enabled” summary.

- [ ] **Step 5: Verify and commit**

Run: `npm run test --workspace apps/web -- src/features/projects/wizard/AuthMethodDialog.test.tsx src/features/projects/NewProjectPage.test.tsx && npm run lint --workspace apps/web`

```bash
git add apps/web/src/features/projects/wizard/AuthMethodDialog.tsx apps/web/src/features/projects/wizard/AuthMethodDialog.test.tsx apps/web/src/features/projects/wizard/SecurityIntegrationsStep.tsx apps/web/src/features/projects/OAuthProviderFields.tsx apps/web/src/features/projects/NewProjectPage.tsx apps/web/src/features/projects/NewProjectPage.test.tsx
git commit -m "feat: manage authentication methods dynamically"
```

### Task 6: Combine advanced infrastructure with review and protect final submit

**Files:**
- Create: `apps/web/src/features/projects/wizard/InfrastructureReviewStep.tsx`
- Modify: `apps/web/src/features/projects/ReviewStep.tsx`
- Modify: `apps/web/src/features/projects/NewProjectPage.tsx`
- Modify: `apps/web/src/features/projects/NewProjectPage.test.tsx`

- [ ] **Step 1: Write failing review tests**

```tsx
await navigateToStep('review')
expect(screen.getByRole('button', { name: 'Database and Realtime settings' })).toBeVisible()
await user.click(screen.getByRole('button', { name: 'Gateway and network settings' }))
expect(screen.getByLabelText('HTTPS mode')).toBeVisible()
expect(screen.getByText('Secrets are never shown')).toBeVisible()
```

Add a final request assertion that a Standard preset and dynamically added OAuth secret still produce the full normalized `configuration` aggregate.

- [ ] **Step 2: Run the test to verify it fails**

Run: `npm run test --workspace apps/web -- src/features/projects/NewProjectPage.test.tsx`

Expected: FAIL because database/network is a separate fifth configuration step.

- [ ] **Step 3: Implement review composition**

Extract Database, Realtime, Supavisor, and Gateway/network field sections from `DatabaseNetworkStep.tsx` into exported reusable sections. `InfrastructureReviewStep` wraps them in collapsibles named “Database and Realtime settings”, “Connection pooler settings”, and “Gateway and network settings”. Keep Manager-allocated ports read-only and preserve Caddy/direct-DB dependency behavior via `setServiceEnabled`.

Render `ReviewStep` beneath these controls. Add `onEditStep(step: WizardStepId)` to each non-secret summary row, returning users to its owning step. Before mutation, run `await form.trigger()`; map every error to one of the four steps, switch there, and focus its first invalid control. On 409, invalidate `['projects']`, return to identity, and retain the server message.

- [ ] **Step 4: Verify and commit**

Run: `npm run test --workspace apps/web -- src/features/projects/NewProjectPage.test.tsx && npm run lint --workspace apps/web`

```bash
git add apps/web/src/features/projects/wizard/InfrastructureReviewStep.tsx apps/web/src/features/projects/ReviewStep.tsx apps/web/src/features/projects/NewProjectPage.tsx apps/web/src/features/projects/NewProjectPage.test.tsx
git commit -m "feat: review infrastructure before installation"
```

### Task 7: Run complete verification and capture the baseline exception

**Files:**
- Modify: `docs/superpowers/specs/2026-08-28-service-configuration-layout-design.md` only if implementation exposes a documented contract mismatch.

- [ ] **Step 1: Run the wizard suite**

Run: `npm run test --workspace apps/web -- src/features/projects/NewProjectPage.test.tsx src/features/projects/wizard/useProjectIdentityAvailability.test.ts src/features/projects/wizard/AuthMethodDialog.test.tsx`

Expected: PASS.

- [ ] **Step 2: Run typecheck and production build**

Run: `npm run lint --workspace apps/web && npm run build --workspace apps/web`

Expected: both commands exit 0.

- [ ] **Step 3: Run repository regression checks**

Run: `npm test && go test ./...`

Expected: Go tests PASS; the Web suite has only the exact four documented `ConfigurationPage.test.tsx` baseline failures. Any additional failure is a regression to fix before completion.

- [ ] **Step 4: Check the final diff and commit**

Run: `git diff --check && git status --short`

Expected: no whitespace errors and only intentional wizard changes.

```bash
git add apps/web/src docs/superpowers/specs/2026-08-28-service-configuration-layout-design.md
git commit -m "test: verify project wizard redesign"
```

## Plan self-review

- Spec coverage: Tasks 1–2 cover single-column identity and duplicate validation; Task 3 covers four-step animation; Task 4 covers preset/service hierarchy; Task 5 covers dynamic, searchable, single-column authentication provider management and right-aligned module toggles; Task 6 covers every advanced infrastructure setting and safe final review; Task 7 covers regression verification.
- Placeholder scan: no incomplete markers or unspecified commands remain.
- Type consistency: all tasks use `ProjectForm`, `wizardSteps`, `WizardStepId`, `setServiceEnabled`, `OAUTH_PROVIDERS`, and the existing normalized create aggregate.
