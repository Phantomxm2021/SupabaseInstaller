# Final audit fix report

Date: 2026-09-03

## Audit item 1: authoritative shared_buffers and connection-budget diagnostics

- RED: `TestConfigurationAuthoritativelyValidatesSharedBuffersAndReportsBudget` initially passed `SharedBuffers="128"` through Manager and produced no validation error.
- GREEN: Manager now mirrors the renderer's anchored Postgres memory-unit grammar, returns `database.sharedBuffers`, and includes the fixed reserve (`10`) plus calculated required connection count in the `database.maxConnections` error.
- Verification: `go test ./apps/manager/internal/project -run TestConfigurationAuthoritativelyValidatesSharedBuffersAndReportsBudget -count=1` passed.

## Audit item 2: preserve explicit Auth site URL through wizard navigation

- RED: the new navigation test returned from Security & integrations to Basic and back, then observed the explicit URL replaced by the derived project URL.
- GREEN: Basic-step hydration now derives a fallback only while `authSiteUrl` is empty, preserving an explicitly entered URL across remounts and navigation.
- Verification: the focused Vitest test and the full web suite passed.

## Audit item 3: legacy Caddy snapshot readability and safe migration UI

- RED: Web API network types only admitted `external`, so a legacy Caddy snapshot could not be represented by the settings model.
- GREEN: snapshot-facing network types and the settings schema accept legacy `caddy`; Network settings display a migration warning and disable saving until `external` is selected. The create-project schema remains external-only.
- Verification: legacy snapshot normalization test passed; TypeScript production build passed.

## Full verification

- `go test ./...` — passed.
- `npm --prefix apps/web test -- --run` — 31 files, 219 tests passed.
- `npm --prefix apps/web run build` — passed.
- `git diff --check` — passed.

## Commit

The final audit fixes are included in commit `95230e0` (`fix(config): close final contract audit gaps`).
