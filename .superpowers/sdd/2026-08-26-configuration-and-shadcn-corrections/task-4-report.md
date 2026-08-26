# Task 4 — Provisioner reconciliation and last-known-good rollback

## RED evidence

Added tests before implementation for fixed Compose argument vectors, service
allowlisting, affected-service recreation, rollback after a failed health
check, WriteRuntimeFiles commit-failure cleanup, and startup recovery of an
abandoned legacy quarantine. The first focused run failed because the new
Compose/runtime APIs and typed failure did not exist, and the two carried
Task 3 cases reproduced their defects: a failed compatibility write retained
an extra generation and startup did not restore quarantine entries.

## GREEN evidence

- `GOCACHE=/tmp/supabase-installer-go-cache GOMODCACHE=/tmp/supabase-installer-go-mod-cache go test ./apps/provisioner/internal/compose ./apps/provisioner/internal/runtime ./apps/provisioner/internal/projectfs ./apps/provisioner/internal/server` — pass.
- `GOCACHE=/tmp/supabase-installer-go-cache GOMODCACHE=/tmp/supabase-installer-go-mod-cache go test -race ./apps/provisioner/...` — pass, no race report.
- `GOCACHE=/tmp/supabase-installer-go-cache GOMODCACHE=/tmp/supabase-installer-go-mod-cache go test ./...` — pass.
- `GOCACHE=/tmp/supabase-installer-go-cache GOMODCACHE=/tmp/supabase-installer-go-mod-cache go vet ./...` — pass.
- `git diff --check` — pass.

## Changes

- Added fixed-vector Compose validation, selected startup, recreation, and
  stopped-container removal operations with an internal service allowlist;
  volume-destructive `down -v` is not used.
- Added typed reconciliation request handling in the runtime backend: expected
  revision and idempotency checks, candidate render/stage/validate/commit,
  minimal affected-service recreation, disabled-container cleanup, initial DB
  ordering, complete enabled-service health verification, metadata advancement,
  and last-known-good rollback with recovery health verification.
- Registered `POST /internal/v1/projects/reconcile` with stale-revision and
  redacted runtime-failure responses.
- Hardened compatibility writes to invoke restore after commit failure.
- Added durable legacy quarantine startup recovery/cleanup for interrupted and
  partially populated migrations, restricted to the three legacy runtime names.
- Retained an in-memory backup of migrated legacy entries so a later runtime
  failure can restore them after quarantine finalization.

## Commit

The implementation is committed as `feat: reconcile project configuration with rollback`
(`10feb83`). This report is updated in the follow-up documentation commit.

Legacy post-commit rollback hardening is in `fix: preserve legacy runtime rollback`
(`3347616`).

## Fix Round 1

### RED

Added regression tests proving candidate validation must use staged paths,
disabled removal must use the previous model, post-delete fsync failure must
restore all three legacy files, starting health must be polled, topology fields
must map to affected services, typed failures must replay without Docker, and
metadata publication failure must restore runtime. Added HTTP and production
Runner chain coverage for redacted rollback responses, candidate validation,
and db-first startup. These tests failed against the prior implementation's
current-path validation, one-shot health check, incomplete change map, and
uncached failure/metadata behavior.

### GREEN

- `GOCACHE=/tmp/supabase-installer-go-cache GOMODCACHE=/tmp/supabase-installer-go-mod-cache go test ./apps/provisioner/internal/compose ./apps/provisioner/internal/projectfs ./apps/provisioner/internal/runtime ./apps/provisioner/internal/server -count=1` — pass.
- `GOCACHE=/tmp/supabase-installer-go-cache GOMODCACHE=/tmp/supabase-installer-go-mod-cache go test -race ./apps/provisioner/...` — pass, no race report.
- `GOCACHE=/tmp/supabase-installer-go-cache GOMODCACHE=/tmp/supabase-installer-go-mod-cache go test ./...` — pass.
- `GOCACHE=/tmp/supabase-installer-go-cache GOMODCACHE=/tmp/supabase-installer-go-mod-cache go vet ./...` — pass.
- `git diff --check` — pass.

### Fix commit

The Fix Round 1 implementation is committed as `fix: harden reconcile
candidate rollback and idempotency` (`733ffa5`).

## Residual risks

The bounded health wait treats a non-starting unhealthy/degraded report as an
immediate failure and polls only transient `starting` states. This matches the
health inspector's state model while avoiding an unbounded retry on a clearly
failed service.
