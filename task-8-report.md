# Task 8 report — installed project Configuration workspace

## Delivered

- Added `/projects/:projectId/configuration?section=<key>` as the single installed-project configuration workspace.
- Redirected legacy project section routes and project navigation entries to the workspace query sections; no parallel legacy forms remain.
- Added typed RHF/Zod-backed shadcn sections for General, Services, Authentication, Email/SMTP, OAuth Providers, Storage, Realtime, Functions, Database, Connection Pooler, Gateway/Network, and API/Secrets.
- Added configured secret badges, explicit retain/replace/remove controls, write-only inputs, read-only OAuth callbacks with copy actions, recent-auth reveal for four sensitive kinds, in-memory timeout clearing, and a separate database password rotation warning dialog.
- Added revision conflict handling that preserves dirty form state and offers Reload; update preview lists changed labels, affected services, and restart/recreate impact; accepted operations render `OperationPanel` and refetch project/configuration after success.
- Closed the Task 7 Auth subsection blocker: SMTP and per-provider OAuth handlers now mark only untouched redacted Auth siblings as internal retain markers, preserving explicit incoming leaf actions. Added SMTP+Google and Google+SMTP regression tests.
- Added API validation field propagation and accepted the canonical unset update-secret marker in the frontend schema.

## Verification

```text
go test ./apps/manager/internal/httpapi ./apps/manager/internal/project       PASS
npm test --workspace apps/web -- --run                                      PASS (11 files, 46 tests)
npm run build --workspace apps/web                                           PASS (TypeScript + Vite; existing chunk-size warning)
git diff --check                                                             PASS
```

