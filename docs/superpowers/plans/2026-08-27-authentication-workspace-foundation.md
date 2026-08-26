# Authentication Workspace Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the configuration tabs with a dual-sidebar Authentication workspace while preserving and migrating the already-supported Email, SMTP, Phone, OAuth, signup and URL configuration flows.

**Architecture:** React Router gains dedicated Authentication routes under a single workspace shell. The shell loads the existing redacted configuration snapshot once, renders a content-area Authentication sidebar, and delegates edits to the current typed configuration endpoints. Existing generic configuration routes continue for non-auth settings without a horizontal tab strip.

**Tech Stack:** React 19, TypeScript, React Router, TanStack Query, React Hook Form, Zod, shadcn/Radix Sheet and Tabs, Vitest/Testing Library, Go HTTP API.

---

## File structure

- Create `apps/web/src/features/authentication/AuthenticationWorkspace.tsx`: snapshot loader, dual-sidebar layout, route outlet context, configuration-mutation confirmation flow.
- Create `apps/web/src/features/authentication/navigation.tsx`: grouped navigation metadata and content-side accessible sidebar.
- Create `apps/web/src/features/authentication/SignInProvidersPage.tsx`: migrated signup controls and provider list.
- Create `apps/web/src/features/authentication/ProviderSheet.tsx`: one-provider Sheet with Email/Phone/OAuth variants.
- Create `apps/web/src/features/authentication/EmailsPage.tsx`: Templates/SMTP local tabs; migrated SMTP form.
- Create `apps/web/src/features/authentication/URLConfigurationPage.tsx`: migrated Site URL/redirect URL editing.
- Create `apps/web/src/features/authentication/AuthenticationWorkspace.test.tsx`: route/layout/nav test coverage.
- Create `apps/web/src/features/authentication/SignInProvidersPage.test.tsx`: provider Sheet and patch-path tests.
- Create `apps/web/src/features/authentication/EmailsPage.test.tsx`: local tabs and SMTP save tests.
- Modify `apps/web/src/app/router.tsx`: register Authentication nested routes.
- Modify `apps/web/src/app/AppShell.tsx`: route global Authentication navigation to the workspace and remove `auth`, `smtp`, and `oauth` configuration links.
- Modify `apps/web/src/features/project/configuration/ConfigurationPage.tsx`: delete `Tabs` and render the requested non-auth section directly.
- Modify `apps/web/src/features/project/configuration/types.ts`: remove auth-only configuration section members and update impact/label mappings.
- Modify `apps/web/src/styles.css`: workspace grid, content-side navigation, desktop/mobile behavior.

### Task 1: Lock the routing contract with failing tests

**Files:**
- Modify: `apps/web/src/app/router.test.tsx`
- Test: `apps/web/src/features/authentication/AuthenticationWorkspace.test.tsx`

- [ ] **Step 1: Add failing route assertions.**

```tsx
it('renders the authentication workspace rather than the configuration tabs', async () => {
  renderRouter('/projects/bee/authentication/sign-in-providers')
  expect(await screen.findByRole('heading', { name: 'Sign In / Providers' })).toBeVisible()
  expect(screen.getByRole('navigation', { name: 'Authentication navigation' })).toBeVisible()
  expect(screen.queryByRole('tablist', { name: /configuration/i })).not.toBeInTheDocument()
})
```

- [ ] **Step 2: Run the test and verify it fails.**

Run: `npm test -- --run src/app/router.test.tsx src/features/authentication/AuthenticationWorkspace.test.tsx`

Expected: FAIL because the nested Authentication route and workspace component do not exist.

- [ ] **Step 3: Add nested route declarations.**

```tsx
{ path: 'authentication', element: <AuthenticationWorkspace />, children: [
  { index: true, element: <Navigate to="sign-in-providers" replace /> },
  { path: 'sign-in-providers', element: <SignInProvidersPage /> },
  { path: 'emails', element: <EmailsPage /> },
  { path: 'url-configuration', element: <URLConfigurationPage /> },
] }
```

- [ ] **Step 4: Add the smallest workspace component.**

```tsx
export function AuthenticationWorkspace() {
  return <section className="authentication-workspace">
    <AuthenticationNavigation />
    <main className="authentication-content"><Outlet /></main>
  </section>
}
```

- [ ] **Step 5: Re-run the route tests.**

Run: `npm test -- --run src/app/router.test.tsx src/features/authentication/AuthenticationWorkspace.test.tsx`

Expected: PASS.

- [ ] **Step 6: Commit the routing contract.**

```bash
git add apps/web/src/app/router.tsx apps/web/src/app/router.test.tsx apps/web/src/features/authentication/AuthenticationWorkspace.tsx apps/web/src/features/authentication/AuthenticationWorkspace.test.tsx
git commit -m "feat: add authentication workspace routes"
```

### Task 2: Add dual-sidebar navigation and remove horizontal configuration tabs

**Files:**
- Create: `apps/web/src/features/authentication/navigation.tsx`
- Modify: `apps/web/src/app/AppShell.tsx`
- Modify: `apps/web/src/features/project/configuration/ConfigurationPage.tsx`
- Modify: `apps/web/src/features/project/configuration/types.ts`
- Modify: `apps/web/src/styles.css`
- Test: `apps/web/src/features/authentication/AuthenticationWorkspace.test.tsx`

- [ ] **Step 1: Add a failing navigation/highlight test.**

```tsx
it('keeps project navigation and highlights the active Authentication item', async () => {
  renderRouter('/projects/bee/authentication/emails')
  expect(await screen.findByRole('link', { name: 'Overview' })).toBeVisible()
  expect(screen.getByRole('link', { name: 'Emails' })).toHaveAttribute('aria-current', 'page')
  expect(screen.getByText('NOTIFICATIONS')).toBeVisible()
})
```

- [ ] **Step 2: Run it to verify failure.**

Run: `npm test -- --run src/features/authentication/AuthenticationWorkspace.test.tsx`

Expected: FAIL because no grouped content-side navigation exists.

- [ ] **Step 3: Define grouped route metadata and semantic navigation.**

```tsx
export const authenticationGroups = [
  { label: 'Manage', items: [['users', 'Users'], ['oauth-apps', 'OAuth Apps']] },
  { label: 'Notifications', items: [['emails', 'Emails']] },
  { label: 'Configuration', items: [['sign-in-providers', 'Sign In / Providers'], ['sessions', 'Sessions'], ['rate-limits', 'Rate Limits'], ['multi-factor', 'Multi-Factor'], ['url-configuration', 'URL Configuration'], ['attack-protection', 'Attack Protection'], ['auth-hooks', 'Auth Hooks'], ['audit-logs', 'Audit Logs'], ['performance', 'Performance']] },
] as const
```

- [ ] **Step 4: Replace global Auth configuration links with one workspace link and remove the `Tabs` import/rendering.**

```tsx
<SidebarMenuButton isActive={location.pathname.includes('/authentication')} render={<Link to={`/projects/${projectId}/authentication/sign-in-providers`} />}>
  <ShieldCheck /><span>Authentication</span>
</SidebarMenuButton>
```

```tsx
return <main className="page configuration-page">{section === 'general' && <GeneralSection ... />}</main>
```

- [ ] **Step 5: Add responsive CSS.**

```css
.authentication-workspace { display:grid; grid-template-columns: 20rem minmax(0, 1fr); min-height:calc(100vh - var(--topbar-height)); }
.authentication-navigation { border-right:1px solid hsl(var(--border)); padding:1.5rem 1rem; }
@media (max-width: 900px) { .authentication-workspace { display:block; } .authentication-navigation { border-right:0; border-bottom:1px solid hsl(var(--border)); } }
```

- [ ] **Step 6: Run focused tests.**

Run: `npm test -- --run src/features/authentication/AuthenticationWorkspace.test.tsx src/app/AppShell.test.tsx src/features/project/ConfigurationPage.test.tsx`

Expected: PASS, including the absence of a configuration `tablist`.

- [ ] **Step 7: Commit navigation migration.**

```bash
git add apps/web/src/features/authentication/navigation.tsx apps/web/src/app/AppShell.tsx apps/web/src/features/project/configuration/ConfigurationPage.tsx apps/web/src/features/project/configuration/types.ts apps/web/src/styles.css apps/web/src/features/authentication/AuthenticationWorkspace.test.tsx
git commit -m "feat: use dual sidebar authentication navigation"
```

### Task 3: Migrate Sign In / Providers to list-and-sheet interaction

**Files:**
- Create: `apps/web/src/features/authentication/SignInProvidersPage.tsx`
- Create: `apps/web/src/features/authentication/ProviderSheet.tsx`
- Test: `apps/web/src/features/authentication/SignInProvidersPage.test.tsx`
- Reuse: `apps/web/src/features/project/configuration/fields.tsx`, `schema.ts`, `useConfigurationMutation.ts`

- [ ] **Step 1: Add a failing provider-sheet test.**

```tsx
it('opens Google in a Sheet and saves only that provider', async () => {
  renderSignInProviders()
  await userEvent.click(await screen.findByRole('button', { name: /Google.*Disabled/i }))
  expect(screen.getByRole('dialog', { name: 'Google' })).toBeVisible()
  await userEvent.click(screen.getByLabelText('Enable Google'))
  await userEvent.type(screen.getByLabelText('Client ID'), 'google-client')
  await userEvent.type(screen.getByLabelText('Google client secret'), 'secret')
  await userEvent.click(screen.getByRole('button', { name: 'Save changes' }))
  expect(fetch).toHaveBeenCalledWith(expect.stringContaining('/configuration/oauth/google'), expect.objectContaining({ method: 'PATCH' }))
})
```

- [ ] **Step 2: Verify it fails.**

Run: `npm test -- --run src/features/authentication/SignInProvidersPage.test.tsx`

Expected: FAIL because the provider list and Sheet are missing.

- [ ] **Step 3: Render signup controls and provider status rows.**

```tsx
<SettingsCard title="User Signups">
  <Toggle id="allow-signup" label="Allow new users to sign up" checked={auth.email.allowSignup} onChange={setAllowSignup} />
  <Toggle id="anonymous-sign-in" label="Allow anonymous sign-ins" checked={auth.anonymousSignIn} onChange={setAnonymous} />
  <Toggle id="confirm-email" label="Confirm email" checked={auth.email.confirmEmail} onChange={setConfirmEmail} />
</SettingsCard>
<ProviderList providers={providers} onSelect={setSelectedProvider} />
```

- [ ] **Step 4: Implement Sheet forms using existing `SecretEditor` and `oauthProviderSchema`.**

```tsx
<Sheet open={Boolean(provider)} onOpenChange={requestClose}>
  <SheetContent aria-describedby={undefined}>
    <SheetHeader><SheetTitle>{label}</SheetTitle></SheetHeader>
    <ProviderForm initial={providerConfig} onSave={saveProvider} />
  </SheetContent>
</Sheet>
```

- [ ] **Step 5: Add discard confirmation and run test.**

Run: `npm test -- --run src/features/authentication/SignInProvidersPage.test.tsx`

Expected: PASS; an edited Sheet cannot close until the user chooses Discard or Keep editing.

- [ ] **Step 6: Commit the provider UX.**

```bash
git add apps/web/src/features/authentication/SignInProvidersPage.tsx apps/web/src/features/authentication/ProviderSheet.tsx apps/web/src/features/authentication/SignInProvidersPage.test.tsx
git commit -m "feat: add authentication provider sheets"
```

### Task 4: Migrate Emails and URL Configuration

**Files:**
- Create: `apps/web/src/features/authentication/EmailsPage.tsx`
- Create: `apps/web/src/features/authentication/URLConfigurationPage.tsx`
- Test: `apps/web/src/features/authentication/EmailsPage.test.tsx`
- Test: `apps/web/src/features/authentication/AuthenticationWorkspace.test.tsx`

- [ ] **Step 1: Add failing tab and SMTP-save test.**

```tsx
it('keeps Templates and SMTP Settings local to Emails and patches SMTP', async () => {
  renderEmails()
  await userEvent.click(screen.getByRole('tab', { name: 'SMTP Settings' }))
  await userEvent.click(screen.getByLabelText('Enable custom SMTP'))
  await userEvent.type(screen.getByLabelText('Host'), 'smtp.example.test')
  await userEvent.click(screen.getByRole('button', { name: 'Save changes' }))
  expect(fetch).toHaveBeenCalledWith(expect.stringContaining('/configuration/smtp'), expect.anything())
})
```

- [ ] **Step 2: Verify failure.**

Run: `npm test -- --run src/features/authentication/EmailsPage.test.tsx`

Expected: FAIL because the Emails route is not implemented.

- [ ] **Step 3: Compose the local email tabs and re-use the existing SMTP form semantics.**

```tsx
<Tabs defaultValue="templates">
  <TabsList><TabsTrigger value="templates">Templates</TabsTrigger><TabsTrigger value="smtp">SMTP Settings</TabsTrigger></TabsList>
  <TabsContent value="templates"><EmailTemplateList unavailableReason="Email templates require the typed runtime mapping phase." /></TabsContent>
  <TabsContent value="smtp"><SMTPSection initial={config.auth.smtp} revision={snapshot.revision} onSave={saveSMTP} /></TabsContent>
</Tabs>
```

- [ ] **Step 4: Render URL Configuration from current General and Auth fields.**

```tsx
<SettingsCard title="Site URL"><TextField form={form} name="siteUrl" label="Site URL" /></SettingsCard>
<RedirectURLList values={redirectUrls} onChange={(next) => form.setValue('redirectUrls', next, { shouldDirty: true })} />
```

- [ ] **Step 5: Run component tests.**

Run: `npm test -- --run src/features/authentication/EmailsPage.test.tsx src/features/authentication/AuthenticationWorkspace.test.tsx`

Expected: PASS; SMTP preserves write-only secret semantics and URL list validates URLs.

- [ ] **Step 6: Commit Emails and URLs.**

```bash
git add apps/web/src/features/authentication/EmailsPage.tsx apps/web/src/features/authentication/URLConfigurationPage.tsx apps/web/src/features/authentication/EmailsPage.test.tsx apps/web/src/features/authentication/AuthenticationWorkspace.test.tsx
git commit -m "feat: migrate authentication email and URL settings"
```

### Task 5: Verify the foundation end-to-end

**Files:**
- Modify: `apps/web/src/app/router.test.tsx`
- Modify: `apps/web/src/features/project/ConfigurationPage.test.tsx`

- [ ] **Step 1: Add redirect coverage for legacy auth query routes.**

```tsx
it('redirects legacy auth configuration links to the Authentication workspace', async () => {
  renderRouter('/projects/bee/configuration?section=oauth')
  expect(await screen.findByRole('heading', { name: 'Sign In / Providers' })).toBeVisible()
})
```

- [ ] **Step 2: Run all web tests.**

Run: `npm test -- --run`

Expected: PASS.

- [ ] **Step 3: Run the production build.**

Run: `npm run build`

Expected: exit 0.

- [ ] **Step 4: Commit verification-only test changes.**

```bash
git add apps/web/src/app/router.test.tsx apps/web/src/features/project/ConfigurationPage.test.tsx
git commit -m "test: cover authentication workspace migration"
```

## Follow-on plans

This foundation is intentionally independently deployable: it migrates all currently rendered Auth settings without claiming runtime support for fields the Manager does not render. The approved specification's remaining systems require separate testable plans because each changes the shared domain schema and the GoTrue renderer:

1. `authentication-runtime-settings`: typed config, validation, and renderer for sessions, rate limits, MFA/WebAuthn/passkeys, CAPTCHA, hooks, OAuth Server, templates, notifications, and performance.
2. `authentication-management-data`: dedicated Manager endpoints and workspace pages for Users, OAuth Apps, and Audit Logs.
