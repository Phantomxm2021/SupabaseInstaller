# Shadcn Management Surfaces Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Apply the shared shadcn experience to project list/detail, authentication, and project settings while retaining current routes, configuration mutation contracts, and safe data boundaries.

**Architecture:** Build on components/app introduced by the foundation plan. Existing TanStack Query keys and configuration mutation flows stay intact; pages replace custom loading/error/empty and layout markup with PageHeader, AsyncState, SettingRow, and shadcn primitives.

**Tech Stack:** React 19, TypeScript, Tailwind v4, shadcn Base UI, TanStack Query, React Hook Form, Zod, Sonner, Vitest, Testing Library.

---

## File Structure

- Modify: apps/web/src/features/projects/ProjectsPage.tsx and ProjectsPage.test.tsx — toolbar search, async states, status list.
- Modify: apps/web/src/features/project/OverviewPage.tsx and OverviewPage.test.tsx — shared header/status cards and skeleton/error state.
- Modify: apps/web/src/features/authentication/AuthenticationWorkspace.tsx — shared async state and existing confirmation flow.
- Modify: apps/web/src/features/authentication/SignInProvidersPage.tsx and SignInProvidersPage.test.tsx — searchable dynamic provider list and state rows.
- Modify: apps/web/src/features/authentication/ProviderSheet.tsx — preserve configuration sheet, normalized Field/SettingRow composition.
- Modify: apps/web/src/features/project/configuration/ConfigurationPage.tsx and ConfigurationPage.test.tsx — shared settings workspace states and right-aligned toggles.
- Modify: apps/web/src/features/project/configuration/fields.tsx — make Toggle delegate to SettingRow without changing form signatures.
- Modify: apps/web/src/features/settings/ManagerSettingsPage.tsx and ManagerSettingsPage.test.tsx — PageHeader and AsyncState.
- Modify: apps/web/src/styles.css — delete replaced custom project/auth/settings presentation selectors only after each surface has migrated.

### Task 1: Refactor project list and overview state presentation

- [ ] **Step 1: Add failing list/detail tests**

In ProjectsPage.test.tsx require Skeleton placeholders during unresolved project/host queries, an input named Search projects, and an Empty state primary link named Create project. In OverviewPage.test.tsx require a loading skeleton, a retry action after a failed project request, and a status badge with text for a loaded project.

- [ ] **Step 2: Run focused tests to verify they fail**

Run: npm test --workspace apps/web -- ProjectsPage.test.tsx OverviewPage.test.tsx  
Expected: FAIL because current pages render plain loading text and no search/retry controls.

- [ ] **Step 3: Implement list/detail using shared compositions**

Use PageHeader for ProjectsPage and OverviewPage. In ProjectsPage, derive visibleProjects from a local search state and render only matching rows. Replace plain loading/error/empty markup with AsyncState; retry refetches the corresponding query. Preserve the existing failed-project retry mutation and query invalidation.

In OverviewPage, retain studio URL fallback and lifecycle actions. Replace custom header/fact wrappers with Cards and Badge while keeping ServiceTable and every existing project datum. Never render health as color-only content.

Keep search local and query semantics unchanged:

~~~tsx
const [queryText, setQueryText] = useState('')
const visibleProjects = projects.filter((project) =>
  project.name.toLocaleLowerCase().includes(queryText.trim().toLocaleLowerCase()),
)
<Input aria-label="Search projects" value={queryText} onChange={(event) => setQueryText(event.target.value)} />
~~~

- [ ] **Step 4: Run focused tests**

Run: npm test --workspace apps/web -- ProjectsPage.test.tsx OverviewPage.test.tsx  
Expected: PASS.

- [ ] **Step 5: Commit**

~~~sh
git add apps/web/src/features/projects/ProjectsPage.tsx apps/web/src/features/projects/ProjectsPage.test.tsx apps/web/src/features/project/OverviewPage.tsx apps/web/src/features/project/OverviewPage.test.tsx apps/web/src/styles.css
git commit -m "feat: standardize project list and overview states"
~~~

### Task 2: Refactor authentication provider discovery and editing

- [ ] **Step 1: Add failing authentication tests**

In SignInProvidersPage.test.tsx add coverage for a button named Add authentication method, a dialog search field, one provider result per row, no Enable Google switch immediately after selecting Google, and an icon-only Remove Google button with accessible name. Keep existing tests for dirty-sheet discard confirmation and PATCH payloads.

- [ ] **Step 2: Run focused test to verify it fails**

Run: npm test --workspace apps/web -- SignInProvidersPage.test.tsx  
Expected: FAIL because providers are currently pre-rendered and opened by row click.

- [ ] **Step 3: Implement dynamic provider list**

Reuse the Dialog + Command picker pattern from the wizard. Display built-in login methods and enabled OAuth methods as single-column status rows; do not render every disabled OAuth provider upfront. Selecting OAuth opens ProviderSheet with an enabled unsaved OAuth draft; its first valid save persists enabled: true together with credentials through the existing requestSave flow. ProviderSheet continues to save only dirty values through requestSave. Remove uses an icon-only destructive action and the existing discard/confirmation semantics.

Replace duplicated auth settings rows with SettingRow so Switch is the final right-aligned control. Retain the Supabase Auth/Third-Party Auth tab behavior unless the second tab contains no actionable content, in which case present Empty rather than a bespoke paragraph.

Provider selection passes an enabled draft flag to the existing sheet without changing backend contracts:

~~~tsx
const [newOAuthProvider, setNewOAuthProvider] = useState<OAuthProvider>()
const addOAuth = (nextProvider: OAuthProvider) => {
  setNewOAuthProvider(nextProvider)
  setProvider(nextProvider)
  setPickerOpen(false)
}
<ProviderSheet provider={provider} initialOAuthEnabled={provider === newOAuthProvider}
  auth={auth} revision={revision} general={general} onClose={() => { setProvider(undefined); setNewOAuthProvider(undefined) }} onSave={requestSave} />
~~~

Extend ProviderSheet with optional initialOAuthEnabled. When provider has no saved auth.oauth entry and this flag is true, pass oauthDefaults an initial object made from emptyProvider with enabled: true. Do not render the OAuth enable Toggle in this first-time draft; after the provider is saved, its standard existing enable control can remain available for later disable/enable edits.

- [ ] **Step 4: Run focused tests**

Run: npm test --workspace apps/web -- AuthenticationWorkspace.test.tsx SignInProvidersPage.test.tsx  
Expected: PASS, including existing secret redaction and dirty-state tests.

- [ ] **Step 5: Commit**

~~~sh
git add apps/web/src/features/authentication/AuthenticationWorkspace.tsx apps/web/src/features/authentication/SignInProvidersPage.tsx apps/web/src/features/authentication/SignInProvidersPage.test.tsx apps/web/src/features/authentication/ProviderSheet.tsx apps/web/src/styles.css
git commit -m "feat: streamline authentication provider management"
~~~

### Task 3: Refactor project and manager settings composition

- [ ] **Step 1: Add failing settings tests**

Extend ConfigurationPage.test.tsx to require a retry action when configuration loading fails, a visible revision badge after load, and switches whose parent setting row places the control after its label content. Extend ManagerSettingsPage.test.tsx to require a skeleton while session is loading and a retryable alert on session failure.

- [ ] **Step 2: Run focused tests to verify they fail**

Run: npm test --workspace apps/web -- ConfigurationPage.test.tsx ManagerSettingsPage.test.tsx  
Expected: FAIL on skeleton/retry and row-order assertions.

- [ ] **Step 3: Implement settings composition without mutation changes**

Use AsyncState for configuration/session loading and errors; retry invokes the current query refetch. Keep ConfigurationPage query keys, redirect logic, conflict alert, server field errors, rotation action, and useConfigurationMutation payloads unchanged.

Modify Toggle in configuration/fields.tsx to render SettingRow while retaining its id, label, checked, onChange, disabled, description, error, and className API. This migrates all configuration sections to right-aligned switches without changing their form integrations. Use PageHeader in ManagerSettingsPage and preserve its safe session projection, including the rule that CSRF data never renders.

The Toggle adapter keeps all current callers source-compatible:

~~~tsx
export function Toggle(props: ToggleProps) {
  return <SettingRow label={props.label} description={props.description} checked={props.checked}
    disabled={props.disabled} error={props.error} onCheckedChange={props.onChange} className={props.className} />
}
~~~

- [ ] **Step 4: Run focused tests**

Run: npm test --workspace apps/web -- ConfigurationPage.test.tsx ManagerSettingsPage.test.tsx  
Expected: PASS.

- [ ] **Step 5: Commit**

~~~sh
git add apps/web/src/features/project/configuration/ConfigurationPage.tsx apps/web/src/features/project/configuration/ConfigurationPage.test.tsx apps/web/src/features/project/configuration/fields.tsx apps/web/src/features/settings/ManagerSettingsPage.tsx apps/web/src/features/settings/ManagerSettingsPage.test.tsx apps/web/src/styles.css
git commit -m "feat: unify project and manager settings interactions"
~~~

### Task 4: Simplify shell and remove superseded CSS

- [ ] **Step 1: Add AppShell navigation assertions**

Extend AppShell.test.tsx to assert that project routes expose Overview, Authentication, and Project Settings navigation; project creation does not show the sidebar; the active project menu search remains keyboard accessible.

- [ ] **Step 2: Run test to verify existing navigation behavior**

Run: npm test --workspace apps/web -- AppShell.test.tsx  
Expected: PASS before the refactor, establishing behavior to preserve.

- [ ] **Step 3: Remove only superseded custom selectors**

After the prior tasks use shadcn Cards, Field, Sidebar, Table, Empty, Skeleton, and application compositions, delete the obsolete page-specific selectors for project overview facts, auth provider rows, settings cards, and their duplicated hover/focus states. Keep responsive safeguards and route-specific layout rules that remain in use. Do not delete a selector until rg confirms its class is no longer referenced.

- [ ] **Step 4: Run style and route checks**

Run:

~~~sh
rg -n 'project-overview-fact|auth-provider-row|auth-settings-card' apps/web/src
npm test --workspace apps/web -- AppShell.test.tsx
npm run lint --workspace apps/web
~~~

Expected: no references to deleted selectors; tests and lint PASS.

- [ ] **Step 5: Commit**

~~~sh
git add apps/web/src/app/AppShell.tsx apps/web/src/app/AppShell.test.tsx apps/web/src/styles.css
git commit -m "refactor: remove superseded management surface styles"
~~~

### Task 5: Batch-2 verification

- [ ] **Step 1: Execute frontend validation**

~~~sh
npm test --workspace apps/web
npm run lint --workspace apps/web
npm run build --workspace apps/web
~~~

Expected: all commands exit 0.

- [ ] **Step 2: Execute repository regression validation**

~~~sh
npm test
go test ./...
~~~

Expected: all commands exit 0.

- [ ] **Step 3: Manual desktop acceptance**

Verify Projects search, empty/error retry, project health text, overview loading/error retry, authentication search and immediate OAuth enablement, icon-only removal, keyboard dialog focus return, right-aligned settings switches, configuration conflict reload, manager settings safe-session rendering, destructive confirmations, visible focus, and reduced-motion behavior.

- [ ] **Step 4: Commit verification-only corrections**

~~~sh
git status --short
git add apps/web/src/app/AppShell.test.tsx apps/web/src/features/projects/ProjectsPage.test.tsx apps/web/src/features/project/OverviewPage.test.tsx apps/web/src/features/authentication/SignInProvidersPage.test.tsx apps/web/src/features/project/configuration/ConfigurationPage.test.tsx apps/web/src/features/settings/ManagerSettingsPage.test.tsx
git commit -m "test: verify shadcn management surface contracts"
~~~
