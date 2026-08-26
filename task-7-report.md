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

Commit: `70a1205 fix: rebuild configurable project wizard forms`
