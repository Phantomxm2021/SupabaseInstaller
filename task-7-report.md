# Task 7 report — configurable project wizard and success navigation

## RED

The pre-implementation wizard suite failed because New Project only rendered a fixed Lightweight path and `OperationPanel` had no project-success navigation contract. The existing operation test also exposed that the panel must remain renderable without a Router.

## GREEN

Implemented a six-step Basic, Preset & Services, Auth/SMTP/OAuth, Storage/Functions, Database/Network, and redacted Review wizard. Added the complete typed `ProjectConfiguration` API mirror, Go-compatible defaults and preset closure, Zod dependency/conditional validation, all twenty OAuth providers with read-only callbacks, write-only secret replacement markers, and complete create payloads. Success invalidates `['projects']` and replaces navigation to `/projects/:id/overview` once; terminal failure states do not navigate.

Verification:

```text
npm run test --workspace apps/web -- --run
Test Files  9 passed (9)
Tests  17 passed (17)

npm run build --workspace apps/web
TypeScript check and Vite build passed

git diff --check
passed
```

## Commit

The Task 7 implementation is committed as `feat: add complete configurable project wizard`.

## Round 1 correction

Rebuilt the wizard around one real React Hook Form submission boundary. Every Next/Review action runs `trigger` and `handleSubmit`; Install is a native form submit, and field errors render next to the corresponding typed control. The six steps now compose the generated shadcn Card, Tabs, Field/Form, Input, Textarea, Select, Switch, Collapsible, and Alert primitives. The aggregate UI exposes all Auth/Phone/SMTP/OAuth, Storage/S3, Functions, Realtime, database, pooler, and network fields. Service mutations use one dependency-aware Custom action, storage switching clears mutually exclusive fields, and unsupported manual TLS is not selectable. Operation success awaits project invalidation and has one navigation owner with fixed hook topology.

Round 1 regression tests cover invalid Review bypass prevention and complete six-step payload shape.

```text
npm run test --workspace apps/web -- --run src/features/projects/NewProjectPage.test.tsx
Test Files 1 passed (1)
Tests 3 passed (3)

npm run test --workspace apps/web -- --run
Test Files 9 passed (9)
Tests 19 passed (19)

npm run build --workspace apps/web
TypeScript check and Vite build passed

git diff --check
passed
```

Commit: `5bcb974 fix: rebuild configurable project wizard forms`

## Round 2 correction

Creation now uses create-safe secret actions (`""`, `replace`, `remove`) and normalizes any update-only `retain` marker/value at the POST boundary. The generated RHF Form/FormField primitives and Alert variants are used directly; nested service, provider, SMTP and Functions array errors are rendered beside their controls. Preset selection resets the complete aggregate closure while preserving project identity, Direct DB and all allocated ports remain Manager-owned (`0`/read-only), unsupported extensions/internal gateway values are rejected, and Phone includes Twilio Verify SID with disabled-provider fields not required. Schema parity covers DNS hostnames, HTTP(S), secret truth tables, dependencies, port uniqueness/relationships and renderer constraints. Operation success tests cover awaited invalidation, operation project ID, exactly-once navigation and all terminal no-navigation states; Functions zero-variable review text is explicit.

Round 2 verification:

```text
npm test --workspace apps/web -- --run
Test Files 10 passed (10)
Tests 34 passed (34)

npm run build --workspace apps/web
TypeScript check and Vite build passed

git diff --check
passed
```

Commit: pending (Round 2 working tree)
