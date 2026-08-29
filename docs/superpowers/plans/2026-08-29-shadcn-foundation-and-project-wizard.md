# Shadcn Foundation and Project Wizard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Establish the shared shadcn application layer and refactor the create-project wizard to use it without changing its API payload or validation semantics.

**Architecture:** Keep shadcn Base UI primitives in components/ui and add a small components/app layer for composed page framing, async content states, and setting rows. The existing React Hook Form + Zod wizard remains the source of truth; its step components consume the new primitives and retain the existing project identity availability hook.

**Tech Stack:** React 19, TypeScript, Vite, Tailwind v4, shadcn base-nova/Base UI, React Hook Form, Zod, TanStack Query, Vitest, Testing Library.

---

## File Structure

- Create: apps/web/src/components/app/PageHeader.tsx — consistent page title, description, metadata, and action region.
- Create: apps/web/src/components/app/AsyncState.tsx — skeleton, retryable error, and empty collection presentation.
- Create: apps/web/src/components/app/SettingRow.tsx — collapsible setting row with a far-right switch.
- Create: apps/web/src/components/app/PageHeader.test.tsx and SettingRow.test.tsx — component behavior contracts.
- Create through shadcn CLI: apps/web/src/components/ui/command.tsx, dialog.tsx, empty.tsx, input-group.tsx, spinner.tsx.
- Modify: apps/web/src/styles.css — token/base/motion rules only; remove obsolete wizard-specific visual rules as they are replaced.
- Modify: apps/web/src/features/projects/BasicStep.tsx — Field and Input Group composition, visible studio username.
- Modify: apps/web/src/features/projects/wizard/AuthMethodDialog.tsx — Dialog + Command search and one-column results.
- Modify: apps/web/src/features/projects/wizard/SecurityIntegrationsStep.tsx — SettingRow and dynamic method list.
- Modify: apps/web/src/features/projects/OAuthProviderFields.tsx — one-column enabled OAuth forms and icon-only removal.
- Modify: apps/web/src/features/projects/NewProjectPage.tsx — PageHeader, Spinner submission state, shared error presentation.
- Modify: apps/web/src/features/projects/NewProjectPage.test.tsx and wizard/AuthMethodDialog.test.tsx — flow, focus, and accessibility regressions.

### Task 1: Add approved shadcn primitives

- [ ] **Step 1: Write a failing primitive contract test**

Create apps/web/src/components/ui/primitives.test.tsx:

~~~tsx
import { render, screen } from '@testing-library/react'
import { Command, CommandInput, CommandItem, CommandList } from './command'
import { Spinner } from './spinner'

it('exposes a searchable command list and an accessible loading indicator', () => {
  render(<><Command><CommandInput placeholder="Search methods" /><CommandList><CommandItem>GitHub</CommandItem></CommandList></Command><Spinner aria-label="Saving" /></>)
  expect(screen.getByPlaceholderText('Search methods')).toBeVisible()
  expect(screen.getByText('GitHub')).toBeVisible()
  expect(screen.getByLabelText('Saving')).toBeVisible()
})
~~~

- [ ] **Step 2: Run the test to verify it fails**

Run: npm test --workspace apps/web -- primitives.test.tsx  
Expected: FAIL because command and spinner modules do not exist.

- [ ] **Step 3: Generate source-controlled shadcn primitives**

Run from apps/web:

~~~sh
npx shadcn@latest add command dialog empty input-group spinner
~~~

Retain the existing base-nova style, aliases, Base UI choice, and local source files. Do not add a second component runtime.

- [ ] **Step 4: Run the primitive contract test**

Run: npm test --workspace apps/web -- primitives.test.tsx  
Expected: PASS.

- [ ] **Step 5: Commit**

~~~sh
git add apps/web/src/components/ui apps/web/package.json apps/web/package-lock.json
git commit -m "feat: add shadcn interaction primitives"
~~~

### Task 2: Introduce application compositions and shared motion tokens

- [ ] **Step 1: Write failing application-component tests**

Create PageHeader.test.tsx and SettingRow.test.tsx with these assertions:

~~~tsx
render(<PageHeader eyebrow="Runtime" title="Projects" description="Managed stacks" actions={<button>New project</button>} />)
expect(screen.getByRole('heading', { name: 'Projects' })).toBeVisible()
expect(screen.getByRole('button', { name: 'New project' })).toBeVisible()

render(<SettingRow label="Storage" description="Object storage" checked onCheckedChange={vi.fn()}><p>Options</p></SettingRow>)
expect(screen.getByRole('switch', { name: 'Storage' })).toBeChecked()
expect(screen.getByRole('button', { name: /storage/i })).toBeVisible()
~~~

- [ ] **Step 2: Run tests to verify they fail**

Run: npm test --workspace apps/web -- PageHeader.test.tsx SettingRow.test.tsx  
Expected: FAIL because components/app modules do not exist.

- [ ] **Step 3: Implement focused compositions**

Implement PageHeader with semantic header markup and a right-aligned actions slot. Implement AsyncState with loading, error, and empty variants; its error variant accepts onRetry and its loading variant renders Skeleton. Implement SettingRow as a Card/Collapsible composition: the trigger owns label and description, the Switch is the final sibling in the header flex row, and switching stops propagation so it never toggles the collapsible.

Add token aliases for brand primary/foreground, surface, border, destructive, focus ring, and --motion-fast: 180ms to styles.css. Keep the existing reduced-motion wizard rule, but make the normal wizard animation duration use --motion-fast.

Use these component contracts:

~~~tsx
export function PageHeader({ eyebrow, title, description, actions }: {
  eyebrow?: string; title: string; description?: string; actions?: ReactNode
}) {
  return <header className="flex items-start justify-between gap-6">
    <div>{eyebrow && <p className="text-xs text-muted-foreground">{eyebrow}</p>}<h1>{title}</h1>{description && <p className="text-muted-foreground">{description}</p>}</div>
    {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
  </header>
}
~~~

- [ ] **Step 4: Run component tests and type check**

Run:

~~~sh
npm test --workspace apps/web -- PageHeader.test.tsx SettingRow.test.tsx
npm run lint --workspace apps/web
~~~

Expected: PASS.

- [ ] **Step 5: Commit**

~~~sh
git add apps/web/src/components/app apps/web/src/styles.css
git commit -m "feat: add shared application UI compositions"
~~~

### Task 3: Convert wizard basic identity and submission feedback

- [ ] **Step 1: Extend NewProjectPage tests before changing implementation**

Add assertions that:

1. Studio username is visible before Runtime settings is expanded.
2. An invalid name/slug exposes aria-invalid and described-by error text.
3. Continue remains disabled while availability is unavailable or conflicts.
4. The install action is disabled and exposes a Saving installation label while the create mutation is pending.

- [ ] **Step 2: Run wizard tests to verify the new assertion fails**

Run: npm test --workspace apps/web -- NewProjectPage.test.tsx  
Expected: FAIL on the new pending submission assertion.

- [ ] **Step 3: Refactor BasicStep and NewProjectPage**

Replace the hand-built Site URL prefix container in BasicStep with InputGroup/InputGroupAddon and preserve the Site URL hostname accessible name. Keep Project name, Project slug, Site URL, Studio username, and Studio password in a single vertical FieldGroup; Runtime settings remains the only collapsible portion.

Use PageHeader in NewProjectPage. Replace the pending Rocket icon with Spinner data-icon="inline-start" and text Creating operation…; retain the existing create mutation, 409 handling, focus movement, and WizardStepFrame direction contract.

The BasicStep URL control must use the generated Input Group structure:

~~~tsx
<InputGroup><InputGroupAddon>https://</InputGroupAddon>
  <Input aria-label="Site URL hostname" value={hostnameFromSiteURL(siteURL)}
    onChange={(event) => form.setValue('configuration.general.siteUrl', siteURLFromHostname(event.target.value), { shouldDirty: true, shouldValidate: true })} />
</InputGroup>
~~~

- [ ] **Step 4: Run targeted tests**

Run: npm test --workspace apps/web -- NewProjectPage.test.tsx  
Expected: PASS, including identity availability, duplicate prevention, reduced motion, step direction, and pending action coverage.

- [ ] **Step 5: Commit**

~~~sh
git add apps/web/src/features/projects/BasicStep.tsx apps/web/src/features/projects/NewProjectPage.tsx apps/web/src/features/projects/NewProjectPage.test.tsx
git commit -m "feat: standardize project wizard identity feedback"
~~~

### Task 4: Convert dynamic authentication-method selection

- [ ] **Step 1: Add failing behavior tests**

Extend AuthMethodDialog.test.tsx and NewProjectPage.test.tsx to require:

~~~tsx
expect(screen.getByRole('dialog', { name: 'Add authentication method' })).toBeVisible()
expect(screen.getByPlaceholderText('Search methods')).toBeVisible()
expect(screen.getByRole('option', { name: 'GitHub' })).toBeVisible()
expect(screen.getByRole('button', { name: 'Remove GitHub' })).toHaveTextContent('')
~~~

Also test that selecting GitHub immediately renders it as enabled configuration without an Enable GitHub switch, and that removal uses the existing confirmation dialog.

- [ ] **Step 2: Run tests to verify they fail**

Run: npm test --workspace apps/web -- AuthMethodDialog.test.tsx NewProjectPage.test.tsx  
Expected: FAIL because the picker currently renders Button rows rather than command options.

- [ ] **Step 3: Implement Dialog + Command picker and accessible icon removal**

Replace AlertDialog with Dialog in AuthMethodDialog. Use DialogTitle, DialogDescription, CommandInput placeholder Search methods, CommandList, CommandEmpty, and CommandItem for a single-column list. Keep category filtering only if it remains visible as compact controls above the list; search must operate across every available method and already-added OAuth providers stay excluded.

In SecurityIntegrationsStep, keep selecting OAuth as the enable action. In OAuthProviderFields, render each added provider once in a single-column list with its configuration fields and a Button variant ghost size icon-sm containing a Trash2 icon, aria-label Remove provider label, and no visible remove text. Preserve its existing AlertDialog confirmation and return focus to Add authentication method after a confirmed removal.

The remove control has this stable accessibility contract:

~~~tsx
<Button variant="ghost" size="icon-sm" aria-label={\`Remove \${providerLabels[provider]}\`} onClick={() => setRemoving(provider)}>
  <Trash2 aria-hidden="true" />
</Button>
~~~

- [ ] **Step 4: Run focused tests**

Run:

~~~sh
npm test --workspace apps/web -- AuthMethodDialog.test.tsx NewProjectPage.test.tsx
npm run build --workspace apps/web
~~~

Expected: PASS.

- [ ] **Step 5: Commit**

~~~sh
git add apps/web/src/features/projects/wizard/AuthMethodDialog.tsx apps/web/src/features/projects/wizard/AuthMethodDialog.test.tsx apps/web/src/features/projects/wizard/SecurityIntegrationsStep.tsx apps/web/src/features/projects/OAuthProviderFields.tsx apps/web/src/features/projects/NewProjectPage.test.tsx
git commit -m "feat: use shadcn command picker for auth methods"
~~~

### Task 5: Batch-1 verification

- [ ] **Step 1: Run all frontend checks**

~~~sh
npm test --workspace apps/web
npm run lint --workspace apps/web
npm run build --workspace apps/web
~~~

Expected: all commands exit 0.

- [ ] **Step 2: Run repository regression checks**

~~~sh
npm test
go test ./...
~~~

Expected: all commands exit 0.

- [ ] **Step 3: Manually verify desktop interaction**

Run npm run dev --workspace apps/web and verify: tab focus is visible; reduced motion prevents directional translation; Basic step is single-column; admin username is never hidden; duplicate name/slug prevents progression; add-auth dialog is searchable and one column; OAuth add has no toggle; OAuth removal is icon-only and confirmed; each Security and integrations switch is right aligned.

- [ ] **Step 4: Commit any verification-only test corrections**

~~~sh
git status --short
git add apps/web/src/components/app/PageHeader.test.tsx apps/web/src/components/app/SettingRow.test.tsx apps/web/src/components/ui/primitives.test.tsx apps/web/src/features/projects/NewProjectPage.test.tsx apps/web/src/features/projects/wizard/AuthMethodDialog.test.tsx
git commit -m "test: verify shadcn wizard interaction contracts"
~~~
