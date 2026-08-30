# Functions Deploy Dialog Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the inline Function ZIP upload card with a header Deploy button and reusable deployment dialog while preserving the Managed functions workspace.

**Architecture:** `FunctionsPage` remains the single owner of deployment form and mutation state. A controlled Dialog is opened from the page header for a new deployment and from managed-function actions for a version update. The existing deployment API and operation status card are reused unchanged.

**Tech Stack:** React, TypeScript, TanStack Query, React Testing Library, Vitest, existing Base UI Dialog and shadcn-style components.

---

### Task 1: Cover the dialog entry points with failing UI tests

**Files:**
- Modify: `apps/web/src/features/project/FunctionsPage.test.tsx`

- [ ] **Step 1: Write the failing tests**

Add one test that renders an empty Functions page, presses the page-header `Deploy` button, and expects the dialog heading and both field labels. Add a second test that returns a managed `hello-world` item, opens its `Actions` menu, chooses `Deploy new version`, and expects the dialog function-name input to contain `hello-world`.

```tsx
await user.click(await screen.findByRole('button', { name: 'Deploy' }))
expect(await screen.findByRole('heading', { name: 'Deploy a function' })).toBeVisible()
expect(screen.getByLabelText('Function name')).toBeVisible()
expect(screen.getByLabelText('ZIP archive')).toBeVisible()
```

```tsx
await user.click(await screen.findByRole('button', { name: 'Actions for hello-world' }))
await user.click(await screen.findByRole('menuitem', { name: 'Deploy new version' }))
expect((await screen.findByLabelText('Function name'))).toHaveValue('hello-world')
```

- [ ] **Step 2: Run the focused test to verify it fails**

Run: `npm exec --workspace apps/web -- vitest run src/features/project/FunctionsPage.test.tsx`

Expected: FAIL because no `Deploy` header action and no deployment dialog exist.

- [ ] **Step 3: Commit the red test**

```bash
git add apps/web/src/features/project/FunctionsPage.test.tsx
git commit -m "test: cover functions deploy dialog entry points"
```

### Task 2: Move the deployment form into the existing Dialog component

**Files:**
- Modify: `apps/web/src/features/project/FunctionsPage.tsx`
- Modify: `apps/web/src/features/project/FunctionsPage.test.tsx`

- [ ] **Step 1: Add the controlled dialog state and header action**

Import `Dialog`, `DialogContent`, `DialogDescription`, `DialogFooter`, `DialogHeader`, and `DialogTitle`. Add `deployDialogOpen` state and an `openDeploymentDialog(functionName = '')` helper that assigns the supplied name only when given, then opens the dialog. Pass the following action through `PageHeader.actions`.

```tsx
<Button onClick={() => openDeploymentDialog()} disabled={!enabled || operationInProgress}>
  <Upload />
  Deploy
</Button>
```

- [ ] **Step 2: Replace the upload Card with the controlled Dialog**

Remove the inline `functions-upload-card`. Render the existing name/archive Inputs in a `DialogContent` with heading `Deploy a function`, the established ZIP archive guidance, Cancel, and Deploy function actions. Preserve `id="function-name"`, `id="function-archive"`, archive-name display, upload mutation, disabled conditions, and upload Progress.

```tsx
<Dialog open={deployDialogOpen} onOpenChange={setDeployDialogOpen}>
  <DialogContent>
    <DialogHeader>
      <DialogTitle>Deploy a function</DialogTitle>
      <DialogDescription>The ZIP must contain index.ts at its root, inside a same-named folder, or under supabase/functions/function-name/.</DialogDescription>
    </DialogHeader>
    {/* existing name and archive fields */}
    <DialogFooter>
      <Button variant="outline" onClick={() => setDeployDialogOpen(false)}>Cancel</Button>
      <Button onClick={() => upload.mutate()} disabled={!enabled || upload.isPending || operationInProgress || !archive || !name.trim()}>
        <Upload />
        {upload.isPending ? 'Uploading…' : 'Deploy function'}
      </Button>
    </DialogFooter>
  </DialogContent>
</Dialog>
```

Close the dialog after a successful upload in the existing `onSuccess` handler. Keep values when a user cancels or dismisses it.

- [ ] **Step 3: Connect managed function redeployment to the dialog**

Replace the current direct focus behavior for `Deploy new version` with `openDeploymentDialog(item.name)`. Do not alter rollback or delete behavior.

- [ ] **Step 4: Adapt the existing queued-deployment test**

Open the dialog before uploading its File so the test continues to exercise the live form submission and operation tracking.

```tsx
await user.click(await screen.findByRole('button', { name: 'Deploy' }))
await user.upload(await screen.findByLabelText('ZIP archive'), archive)
await user.click(screen.getByRole('button', { name: 'Deploy function' }))
```

- [ ] **Step 5: Run the focused test to verify it passes**

Run: `npm exec --workspace apps/web -- vitest run src/features/project/FunctionsPage.test.tsx`

Expected: PASS with the dialog entry-point and queued-deployment behaviors.

- [ ] **Step 6: Commit the implementation**

```bash
git add apps/web/src/features/project/FunctionsPage.tsx apps/web/src/features/project/FunctionsPage.test.tsx
git commit -m "feat: move function deployment into dialog"
```

### Task 3: Remove obsolete upload-card styles and verify the application

**Files:**
- Modify: `apps/web/src/styles.css`

- [ ] **Step 1: Remove selectors used solely by the eliminated upload Card**

Delete `.functions-upload-content`, `.functions-upload-grid`, `.functions-field`, `.functions-field-help`, `.functions-upload-action`, and their responsive-only rules. Retain the service alert, operation card, Managed functions card, table, and empty-state styles.

- [ ] **Step 2: Run the complete frontend suite and production build**

Run:

```bash
npm test
npm run build
```

Expected: both commands exit 0.

- [ ] **Step 3: Run backend regression tests**

Run: `go test ./... -count=1`

Expected: exit 0.

- [ ] **Step 4: Commit the cleanup**

```bash
git add apps/web/src/styles.css
git commit -m "style: remove legacy functions upload layout"
```

- [ ] **Step 5: Merge and rebuild the local deployment**

After all verification succeeds, merge `codex/functions-deploy-dialog` into `master`, then run:

```bash
docker compose -f deploy/docker-compose.yml up -d --build manager provisioner
```

Verify the running services with the existing Manager and Provisioner health endpoints before reporting completion.
