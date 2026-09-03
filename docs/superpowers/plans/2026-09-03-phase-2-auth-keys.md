# Asymmetric Auth Keys and Opaque API Keys Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox syntax for tracking.

**Goal:** Add an explicit, password-confirmed migration from legacy API keys to Supabase opaque API keys and ES256 signing, with safe rotation operations.

**Architecture:** Legacy credentials remain active. Manager generates an all-or-nothing key bundle and sends it to Provisioner for reconciliation before encrypting it; a failure leaves both the stored legacy bundle and selected generation unchanged. Empty new-key fields render the official legacy-only mode.

**Tech Stack:** Go standard crypto, encrypted Manager secret store, private Provisioner reconciliation, React/TypeScript, Vitest.

**Spec:** `docs/superpowers/specs/2026-09-03-configuration-remediation-design.md`

## Global Constraints

- Keep `JWT_SECRET`, `ANON_KEY`, and `SERVICE_ROLE_KEY` valid after migration.
- New fields are `SUPABASE_PUBLISHABLE_KEY`, `SUPABASE_SECRET_KEY`, `ANON_KEY_ASYMMETRIC`, `SERVICE_ROLE_KEY_ASYMMETRIC`, `JWT_KEYS`, and `JWT_JWKS`.
- `JWT_KEYS` has private P-256 ES256 plus legacy HS256 `oct`; `JWT_JWKS` has public P-256 ES256 plus legacy HS256 `oct`.
- Do not expose private JWKs, asymmetric service JWTs, or secret opaque keys in diagnostics, normal reads, or operation responses.
- Opaque rotation preserves signing material. Signing rotation requires password and exact project-name confirmation because ES256 sessions end.
- Enable upstream `GOTRUE_JWT_KEYS`, `API_JWT_JWKS`, `JWT_JWKS`, and `SUPABASE_JWKS` consumer lines; empty values remain legacy-only.

### Task 1: Generate a validated key bundle

**Files:** Create `internal/authkeys/bundle.go`, `internal/authkeys/bundle_test.go`; modify `internal/contracts/provisioner.go` and its tests.

**Produces:** `authkeys.Bundle`, `Generate(io.Reader, legacyJWTSecret)`, `Validate(legacyJWTSecret)`, and the six `ProjectSecrets` fields.

- [ ] Write tests that a deterministic reader yields `sb_publishable_`/`sb_secret_` keys; that both ES256 role JWTs verify against `JWT_JWKS`; that `JWT_KEYS` has an EC private `d` while `JWT_JWKS` does not; and that malformed/partial/incorrect legacy-secret bundles fail.
- [ ] Run `go test ./internal/authkeys ./internal/contracts -count=1`; confirm RED because generator/types do not exist.
- [ ] Implement P-256 JWK generation, P1363 ES256 JWT signatures, base64url `oct` JWK for legacy secret, and opaque-key random/checksum generation using only standard crypto.
- [ ] Run the same command; confirm GREEN, then commit `feat: generate asymmetric auth key bundles`.

### Task 2: Persist and render a complete opt-in bundle

**Files:** Modify `apps/manager/internal/install/generator.go`, `apps/manager/internal/install/orchestrator.go`, `apps/manager/internal/configuration/orchestrator.go`, `apps/provisioner/internal/render/environment.go`, `internal/templates/self-hosted-v0.8.0/docker-compose.yml`; modify their direct tests.

**Produces:** Encrypted kinds `publishable-api-key`, `secret-api-key`, `anon-key-asymmetric`, `service-role-key-asymmetric`, `jwt-keys`, `jwt-jwks`; complete hydration and dotenv/Compose verifier input.

- [ ] Write tests that six encrypted kinds hydrate to `ProjectSecrets`, a full bundle emits all six dotenv values and all four verifier Compose variables, blank legacy records still render, and a partial bundle fails before stage.
- [ ] Run focused install/configuration/render tests; confirm RED.
- [ ] Add the six persistence specs and renderer values. Validate the complete bundle before rendering. Un-comment exactly `GOTRUE_JWT_KEYS: ${JWT_KEYS:-[]}`, `API_JWT_JWKS: ${JWT_JWKS:-{"keys":[]}}`, `JWT_JWKS: ${JWT_JWKS:-{"keys":[]}}`, and `SUPABASE_JWKS: ${JWT_JWKS:-{"keys":[]}}`.
- [ ] Run focused tests GREEN and commit `feat: render opt-in asymmetric auth keys`.

### Task 3: Add atomic, confirmed migration and rotation operations

**Files:** Modify `internal/contracts/provisioner.go`, Manager configuration/httpapi/provisioner client packages, and Provisioner server/runtime tests.

**Produces:** `POST /api/projects/{id}/auth-keys/migrate`, `/rotate-api`, and `/rotate-signing`, returning only operation/project IDs.

- [ ] Write RED tests: successful migration persists the six kinds only after successful reconciliation; reconciliation failure retains old kinds and runtime; API rotation keeps JWT fields byte-identical; signing rotation rejects wrong project confirmation; responses and diagnostics contain no candidate secret.
- [ ] Run targeted Go test packages; confirm RED.
- [ ] Implement a single durable operation path: password verification precedes queueing; migration rejects an already complete bundle; rotate-api changes only opaque keys; rotate-signing requires exact `project.Name`. Construct a validated candidate, reconcile it, verify response identity/services, then encrypt and put candidate kinds. On failure do not mutate encrypted keys or issue speculative rollback beyond Provisioner's own reconciliation outcome.
- [ ] Run focused packages GREEN and commit `feat: add explicit auth key migration operations`.

### Task 4: Safe UI, reveals, and audit closure

**Files:** Modify `apps/web/src/api/client.ts`, `apps/web/src/api/types.ts`, `apps/web/src/features/project/configuration/SecretsSection.tsx`, its test, and the audit document.

**Produces:** Password-confirmed migration/opaque rotation/signing rotation controls; public/secret opaque reveal only through existing no-store reauthentication endpoint; CFG-005 marked fixed only after cross-stack test passes.

- [ ] Write RED tests for password-required migration request, no private JWK/role-JWT UI content, opaque rotation warning, and disabled signing-replacement confirmation until password plus exact project name are supplied.
- [ ] Run `npm --prefix apps/web test -- --run SecretsSection`; confirm RED.
- [ ] Add client methods and controls. Extend reveal allowlist only for `publishable-api-key` and `secret-api-key`; never allow `jwt-keys`, `jwt-jwks`, or asymmetric JWT kinds. State that signing replacement ends ES256 sessions.
- [ ] Add a Go cross-stack test that a migrated project retains `ANON_KEY`/`SERVICE_ROLE_KEY` and renders all six new values plus four verifier inputs; run `go test ./...`, Web tests, Web build, and diff check; update CFG-005 with verification/maintenance-window caveat; commit `docs: close asymmetric auth key audit finding`.
