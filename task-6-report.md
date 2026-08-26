# Task 6 report — shadcn shell and navigation flows

## RED

The required focused suite was run before implementation:

```text
AppShell: failed because the global New project link was present
ManagerSettingsPage: failed because the page/module did not exist
DeleteProjectDialog: existing exact-name test passed
```

## GREEN

After implementation:

```text
npm run test --workspace apps/web -- --run
Test Files  9 passed (9)
Tests       11 passed (11)

npm run build --workspace apps/web
vite build completed successfully (TypeScript check passed)
```

The final focused suite (navigation, settings/CSRF redaction, deletion confirmation/cache order) also passed: 5 tests.

## Changes

- Added the official shadcn Vite foundation, `components.json`, `@/*` alias, theme tokens, and generated UI components (Sidebar, Card, DropdownMenu, AlertDialog, Badge, Progress, Sonner, and the full requested supporting set).
- Rebuilt the authenticated shell with Projects-only global navigation and an account footer menu for Manager settings and sign out.
- Added authenticated `/settings`, showing only safe username/password status and control-plane information.
- Converted project deletion to shadcn AlertDialog; preserved runtime-only/runtime-and-data and exact-name confirmation; added ordered query cancellation/removal/invalidation, replace navigation, and Sonner feedback.
- Removed the dead `PREPARE_SUPABASE` UI label.

Commit hash is recorded by `git log` for the accompanying `feat: adopt shadcn shell and fix navigation flows` commit.

## Fix round 1

Addressed all review Important/Minor findings:

- Centralized the `['session']` query and CSRF synchronization; Settings now uses the shared cache with `select` and the regression test refetches before sign out.
- Switched to the generated `SidebarProvider > Sidebar + SidebarInset` layout, added a focusable mobile trigger, named navigation landmark, and active-route assertion.
- Migrated legacy muted text to `muted-foreground`, kept surface `muted` semantics, and explicitly configured the fixed-dark Toaster.
- Delete now awaits cancellation of detail and configuration queries before removing either, invalidates the projects list, then replace-navigates; real API/toast/order coverage is included.
- Added the delete-mode fieldset legend.

Fix-round verification: focused suite 8 tests passed; full suite 9 files / 14 tests passed; production build and `git diff --check` passed.

## Fix round 2

- Removed the legacy `.app-shell` wrapper so `SidebarProvider` owns direct `Sidebar` and `SidebarInset` flex children.
- Added mobile Sheet coverage (including opening the Sheet and reaching the named Projects navigation), active `data-active` state from the current pathname, and dynamic Open/Close trigger labels.
- Strengthened real deletion integration coverage to assert DELETE URL/body, success/error Sonner events, complete cache/toast/navigation timeline, replace-history behavior, and no navigation on failure.

Fix-round-2 verification: full suite 9 files / 17 tests passed; production build and `git diff --check` passed.
