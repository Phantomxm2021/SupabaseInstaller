# Task 2 report: persist and render a complete opt-in bundle

## RED/GREEN

Added conditional persistence and hydration for six encrypted auth-key kinds.
Legacy installs remain without those records; partial bundles fail before
reconciliation/rendering, while complete bundles pass `authkeys.Bundle.Validate`.
Rendering emits all six dotenv values and four pinned verifier inputs with
legacy-safe defaults. Existing blank legacy fields and all presets remain valid.

Focused verification: `go test ./apps/provisioner/internal/render ./apps/manager/internal/configuration ./apps/manager/internal/install ./internal/authkeys ./internal/contracts -count=1` (pass).

## Commit

`feat: render opt-in asymmetric auth keys`

## Terra corrections

Install completeness now checks every legacy kind by name, even when a full
opt-in bundle exists. New bundle writes are encrypted first and committed in
one store transaction. Server and runtime reconcile/rotation diagnostics
redact all six candidate values, including private JWKS.
