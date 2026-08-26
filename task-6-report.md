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
