# Task 3 report: atomic, confirmed auth-key operations

## RED/GREEN

The existing contract test first demonstrated that the six `ProjectSecrets`
fields are excluded from public JSON. Focused Manager/Provisioner packages were
then run after implementation and pass:

`go test ./apps/manager/... ./apps/provisioner/... ./internal/... -count=1`

## Behavior

- Added password-confirmed migration, opaque API-key rotation, and signing-key
  rotation routes. Successful responses contain only `projectId` and
  `operationId`.
- Migration rejects a complete existing bundle. API rotation generates only new
  opaque keys and preserves existing JWT fields. Signing rotation requires the
  exact project name and warns that ES256 sessions are invalidated.
- Candidates are generated, encrypted into durable operation payloads, sent
  through a private auth-key reconcile envelope, verified for operation/project
  identity and enabled services, then atomically published to encrypted secret
  storage. Runtime or persistence failure leaves old project secret rows intact.
- Provisioner diagnostics sanitize legacy and candidate values; candidate
  material is absent from public contracts and operation/API responses.

## Commit

## Review corrections

- Private auth-key reconciliation now defaults a missing wire idempotency key
  to `<operationId>:auth-keys`, preventing replay collisions across sequential
  rotations.
- Secret publication and fenced last-good advancement now share one SQLite
  transaction; a publication failure cannot leave candidate ciphertext behind.
- Deterministic runtime verification/reconciliation failures restore the
  admitted configuration candidate and reservations when the runtime outcome
  is known safe, while unknown outcomes remain recoverable.

Verification after the corrections:

`go test ./apps/manager/internal/configuration ./apps/manager/internal/store ./apps/provisioner/internal/server ./apps/manager/internal/provisioner -count=1`

Added direct regression coverage for typed auth-key failure classification,
including deterministic `RuntimeChanged:false` and preserving unknown runtime
outcomes. Verification mismatches intentionally retain the candidate snapshot
for recovery.

Added orchestrator coverage proving a typed unknown runtime outcome leaves the
admitted lease/candidate recoverable and does not publish candidate secrets.

The endpoint regression also verifies the server emits the constructed typed
`ReconcileProjectResponse` (rather than the generic envelope), allowing the
Manager client to classify deterministic and unknown runtime outcomes.

Auth-key typed failures now set `RuntimeStateKnown` to the inverse of
`RuntimeOutcomeUnknown`, matching database-rotation semantics; deterministic
and unknown endpoint/client regressions are both covered.

Commit: `feat: add explicit auth key migration operations`
