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

Commit: `0841220 fix: close Task 7 create contract gaps`

Follow-up closure commit: `16cd2da fix: enforce service and storage closures`

DTO modeling commit: `02f65a2 fix: model redacted and editable configuration DTOs`

## Round 3 correction

Manager installation now atomically allocates the complete selected server-owned port set (API, Studio, Direct DB, Supavisor transaction/session), persists the allocated aggregate and SQL projections together, synchronizes Direct DB fields, and releases disabled capability reservations. Allocation uses unique SQLite claims and retries safely on conflicts. The pinned service admission rules now cover Functions/Caddy/Storage and Auth/service drift.

The create wire contract is authoritative: legacy top-level `domain`, `siteUrl`, and `services` projections were removed from `ProjectDraft`/`CreateProjectRequest`; domain, site URL and services are derived from `configuration`. Manager no longer normalizes an omitted aggregate or falls back to legacy fields. The frontend dependency action restores database/gateway closure and clears dependent Studio/Storage/Logs children. IPv4/IPv6/DNS and HTTP(S) validation was tightened to match Go, and redacted secret DTOs always require `{ action: "" }` while create/update actions use discriminated unions.

Round 3 verification:

```text
go test ./...
all packages passed

npm test --workspace apps/web -- --run
Test Files 10 passed (10)
Tests 38 passed (38)

npm run build --workspace apps/web
TypeScript check and Vite build passed (existing chunk-size warning only)

git diff --check
passed
```

Commit: `add061a fix: make Task 7 aggregate and allocations authoritative`

Final Round 3 create-secret hardening:

- `CreateSecretInput` and the create Zod schema now admit only `{action:""}` or `{action:"replace",value}`. `retain` and `remove` are update-only and are normalized to an empty marker before create serialization; Manager rejects either update-only action during initial creation. Redaction remains the Go-compatible non-pointer `{action:""}` shape.
- Added regression coverage for create action truth tables and Manager rejection without leaving a project row.

Final verification:

```text
go test ./...
all packages passed

npm test --workspace apps/web -- --run
Test Files 10 passed (10)
Tests 38 passed (38)

npm run build --workspace apps/web
TypeScript check and Vite build passed (existing chunk-size warning only)

git diff --check
passed
```

Final contract commit: `60ae3c4 fix: restrict create secrets to create-safe actions`

Legacy-track scan: no exact matches for `NormalizeDraft`, `configurationSupplied`, `firstConfiguration`, `TryReservePort`, or `configuration omitted`; no `domain`, `siteUrl`, or `services` fields remain on `ProjectDraft`/`CreateProjectRequest`. Those names remain only where authoritative aggregate configuration or the read-only `Project` response requires them.

## Round 4 correction

Create requests now carry only `name`, `slug`, `preset`, and the complete authoritative `configuration`; `supabaseVersion` is read exclusively from `configuration.general`. The equality check and duplicate fixtures were removed. Installation now fails closed when `GetDesiredConfiguration` cannot read the aggregate and never synthesizes a sparse `Project` projection. Manager-owned pooler transaction/session ports default to zero and are shown read-only until the atomic allocator assigns them.

Storage and Image Transformation mutations restore the database/REST/gateway closure, while required REST remains disabled in the UI when Storage is enabled. OAuth and Storage secret fields surface nested Zod `.value.message` errors. PATCH no longer converts empty secret actions to `retain`; existing secrets require explicit `retain`, `remove`, or `replace`, with a frontend helper for converting redacted markers to an explicit update command. Manager JWT expiry validation now matches the frontend range.

Round 4 verification:

```text
go test ./...
all packages passed

npm test --workspace apps/web -- --run
Test Files 10 passed (10)
Tests 43 passed (43)

npm run build --workspace apps/web
TypeScript check and Vite build passed (existing chunk-size warning only)

git diff --check
passed
```

Round 4 commits: `1228b35 fix: make Round4 aggregate and port contracts authoritative`, `8a8f595 fix: require explicit update secret actions`

Legacy scan:

```text
rg '\\b(NormalizeDraft|configurationSupplied|firstConfiguration|TryReservePort)\\b|configuration omitted' apps/manager internal apps/web/src
none
```

The only remaining top-level `supabaseVersion`/`domain`/`siteUrl`/`services` JSON fields are on the read-only `Project` response or inside the authoritative `ProjectConfiguration`; neither create request contains those projections.

## Round 5 correction

Partial PATCH handling now distinguishes the redacted stored base from incoming sections. When a section is untouched, configured redacted leaves are given an internal retain marker solely to satisfy aggregate validation and secret lookup; the persisted/read response remains `{action:""}`. Any incoming configured secret with an empty action is rejected, while explicit `retain`, `remove`, and `replace` remain the only update commands. Unset or disabled secrets stay canonical empty markers.

The TypeScript update model now includes `UnsetSecretInput`, and `toUpdateSecretInput` returns empty for an unconfigured unchanged secret, retain for a configured unchanged secret, and remove only when a configured secret is explicitly requested for removal. Domain numeric-label behavior now matches Manager after failed IPv4 parsing, and SMTP sender validation rejects display-name addresses consistently with the frontend email rule.

Round 5 regression coverage includes default Local/disabled Phone General-only patches, configured Google/SMTP untouched General-only patches, modified configured secret empty-action rejection, unset/update helper truth tables, numeric-label domains, JWT bounds, and SMTP email parity.

Round 5 verification:

```text
go test ./...
all packages passed

npm test --workspace apps/web -- --run
Test Files 10 passed (10)
Tests 43 passed (43)

npm run build --workspace apps/web
TypeScript check and Vite build passed (existing chunk-size warning only)

git diff --check
passed
```

Round 5 implementation commit: `8bfe0a3 fix: close Round5 partial patch secret semantics`
