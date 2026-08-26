# Task 3 report: version-aware runtime renderer

## Summary

Implemented the typed `render.Project` renderer as the authoritative runtime renderer. It consumes the redacted project configuration, generated project secrets, and private runtime secret map; maps Auth/SMTP/OAuth, storage, Functions, and Supavisor variables; injects deterministic provider configuration into Auth; selects Compose services from the configuration; validates pinned images; removes container names; and binds the selected API gateway to `127.0.0.1:<port>:8000`.

Runtime staging now uses `projectfs.StageRuntimeFiles`, which deep-copies and fsyncs Compose, `.env`, and `.env.functions` candidates, keeps `.manager-last-good` copies, and provides restore/commit closures with rollback on publication failure. `.env` and `.env.functions` are never written with less restrictive than `0600` permissions.

## RED / GREEN

- RED: the required new renderer tests initially failed to compile because `Project`, `Configuration`, and `RuntimeSecrets` did not exist on the fixed Lightweight API.
- GREEN: focused renderer and projectfs suites pass after implementation.

## Verification

- `GOCACHE=/tmp/supabase-installer-go-cache GOMODCACHE=/tmp/supabase-installer-go-mod-cache go test ./apps/provisioner/internal/render ./apps/provisioner/internal/projectfs` — PASS.
- `GOCACHE=/tmp/supabase-installer-go-cache GOMODCACHE=/tmp/supabase-installer-go-mod-cache go test ./apps/provisioner/... ./internal/...` — PASS.
- Generated representative lightweight, standard, and full files under `/tmp/supabase-manager-render` and ran:
  - `docker compose --file /tmp/supabase-manager-render/lightweight/docker-compose.yml --project-directory /tmp/supabase-manager-render/lightweight config --quiet` — PASS.
  - `docker compose --file /tmp/supabase-manager-render/standard/docker-compose.yml --project-directory /tmp/supabase-manager-render/standard config --quiet` — PASS.
  - `docker compose --file /tmp/supabase-manager-render/full/docker-compose.yml --project-directory /tmp/supabase-manager-render/full config --quiet` — PASS.
- `git diff --check` — PASS.

## Files

- Added `render/provider_registry.go`, `render/environment.go`, `render/services.go`, and standard/full/custom-auth golden fixtures.
- Replaced the fixed renderer in `render/render.go`; retained `Lightweight` only as a compatibility wrapper delegating to `Project`.
- Added renderer and staging tests; added atomic runtime file staging to `projectfs/root.go`.

## Self-review

- Runtime secret values are only consumed for their typed mappings; Function variables are isolated in `.env.functions`, and reserved Function overrides are rejected.
- Provider and service ordering is deterministic; disabled dependencies are pruned; selected images must have explicit non-`latest` tags.
- Existing prepare callers remain source-compatible through the deprecated wrapper while new code can use the typed `Project` and `StageRuntimeFiles` APIs.

## Concerns

- Compose callers must use `Root.RuntimePath(slug)` so they resolve the atomically selected generation; the compatibility `WriteRuntimeFiles` mirror remains for legacy callers until reconciliation migrates them.

## Fix Round 1

Addressed the review findings by merging the pinned PostgreSQL/gateway/logs/Caddy/S3 overlays, adding explicit dependency closure validation, normalizing Storage backends and R2 endpoints, wiring Functions through `.env.functions`, clearing template placeholder secrets, enforcing typed RuntimeSecrets markers, adding Realtime/DB/Supavisor tuning, correcting OAuth/Phone registries, hardening dotenv encoding and image-tag validation, and replacing sequential runtime publication with generation directories plus an atomic `current` symlink. The prior committed generation remains available for restore; aborted candidates are removed.

The earlier representative command appeared to hang because its environment-gated helper was invoked without a reliable exported output directory and the shell lacked `timeout`; the bounded rerun with explicit `env RENDER_OUTPUT=...` completed in 0.01s. The final representative helper is deterministic (`t.TempDir`) and no longer skips.

Named GREEN verification:

- `go test -v -timeout 30s ./apps/provisioner/internal/render -run TestWriteRepresentativeRenderFiles -count=1` — PASS.
- `go test -timeout 2m ./apps/provisioner/internal/render ./apps/provisioner/internal/projectfs` — PASS.
- `go test -timeout 5m ./apps/provisioner/... ./internal/...` — PASS.
- `go test -timeout 2m ./apps/provisioner/internal/render -run TestRepresentativeComposeConfig -count=1 -v` — PASS; the test executed real `docker compose ... config --quiet` for lightweight, standard, and full generated directories.
- Focused tests cover golden fixture comparisons, all 20 OAuth providers and special fields, Storage modes/R2/S3 protocol, PostgreSQL 15/17 and gateway choices, Realtime/DB/Pooler tuning, missing RuntimeSecrets, strict dotenv injection, image validation, generation restore/abort cleanup, and post-stage input mutation.

## Fix Round 2

Split project filesystem synchronization into `metadataMu` and `runtimeMu`. Metadata revision/idempotency callbacks retain serialization while safely publishing runtime generations; lock order is documented as metadata then runtime, and runtime paths never acquire metadata. Added real prepare completion and concurrent staging regression tests.

Completed the remaining renderer hardening: managed stable storage bind semantics, explicit S3 protocol enablement without the MinIO test overlay, generated internal Realtime/Logflare/S3-protocol/Pooler credentials, exact phone provider secret mappings (Twilio, MessageBird, Textlocal), DB command tuning, API/direct/pooler port validation, control-character rejection, and full canonical Compose golden comparisons. Complete lightweight/standard/full Compose goldens now include all parsed services and configuration.

Named GREEN verification:

- `go test -v -count=1 -timeout 30s ./apps/provisioner/internal/server -run TestProvisionerRejectsStaleConfigRevision` — PASS; this was the prior deadlock reproducer.
- `go test -count=1 -timeout 2m ./apps/provisioner/internal/render` — PASS.
- `go test -count=1 -timeout 2m ./apps/provisioner/internal/projectfs ./apps/provisioner/internal/server` — PASS.
- `go test -race -count=1 -timeout 2m ./apps/provisioner/internal/projectfs ./apps/provisioner/internal/server` — PASS.
- `go test -count=1 -timeout 5m ./apps/provisioner/...` — PASS.
- `go test -count=1 -timeout 5m ./apps/manager/...` — PASS.
- `go test -count=1 -timeout 5m ./internal/...` — PASS.
- Real `docker compose config --quiet` for lightweight, standard, and full representative generated files — PASS for all three.
- `git diff --check` — PASS.

The first broad-suite run timed out at five minutes in `TestProvisionerRejectsStaleConfigRevision`; the goroutine stack identified the `UpdateMetadata`/`StageRuntimeFiles` self-deadlock, fixed by the mutex split above. A later controller reproduction of representative generation passed in 0.57 seconds.

## Fix Round 3

Removed the compatibility root-file mirror entirely. `RuntimePath` now returns the stable project/data root, while `CurrentRuntimeFiles` resolves the atomic `current` generation’s Compose, public env, and private Functions env paths. Compose runtime callers pass the stable root as `--project-directory` and the current files as `--file`/`--env-file`; Functions uses the current-generation env file path. Initial template hydration excludes generated runtime files.

Runtime candidates remain `.candidate-*` until commit. Commit renames the candidate, atomically swaps `current`, records its generation, and prunes only safe prior generations. Restore performs a current-generation CAS check and rejects stale closures without changing a newer pointer. Candidate cleanup is explicit and no longer runs during concurrent staging. Added stable-reference, no-mirror, concurrent staging, stale restore CAS, startup cleanup, and prepare completion tests.

Generated Realtime keys are exactly 16 URL-safe characters. Supavisor and DirectDB ports are validated and mapped, phone provider variables use exact `GOTRUE_SMS_*` names, gateway closure and signup-path conflicts are rejected, and complete Compose golden fixtures remain canonical parsed-YAML comparisons.

GREEN verification:

- Focused render, projectfs, compose, runtime, server, secrets, and manager install suites with bounded 2-minute timeouts — PASS.
- `go test -race -count=1 -timeout 2m ./apps/provisioner/internal/projectfs ./apps/provisioner/internal/compose ./apps/provisioner/internal/runtime ./apps/provisioner/internal/server` — PASS.
- `go test -count=1 -timeout 5m ./apps/provisioner/...` — PASS.
- `go test -count=1 -timeout 5m ./apps/manager/...` — PASS.
- `go test -count=1 -timeout 5m ./internal/...` — PASS.
- Real `docker compose config --quiet` using stable project directories and current-generation `--file`/`--env-file` for lightweight, standard, and full outputs — PASS for all three.
- `git diff --check` — PASS.

Out of scope for Task 3 / Task 5 hydration note: after restart, later reconciliation must load and decrypt the newly generated internal secret kinds before rendering. This change adds generation and persistence but does not implement Manager reconciliation hydration.

## Fix Round 4

Addressed the remaining runtime migration, generation restore, phone mapping, and Auth semantic findings. Existing projects now remove only the exact root runtime filenames (`docker-compose.yml`, `.env`, `.env.functions`) after the new `current` pointer is durable, and only when those entries are regular files or symlinks. Non-runtime project data remains untouched. Root initialization invokes candidate cleanup for every direct project slug; startup removes only `.candidate-*` entries and preserves `current` plus all committed generations. Generation pruning was removed so chained A -> B -> C -> B -> A restores remain valid, with stale restore CAS protection retained. Phone fields preserve the `SMS_` prefix in their `GOTRUE_SMS_*` mappings. Global signup flags now require `DisableSignup == !AllowSignup`, reject disabled global signup when phone, anonymous, or OAuth signup paths are enabled, and emit `DISABLE_SIGNUP` directly. Secure email change and double-confirm changes require equal values and map the shared official capability without OR-collapsing.

RED evidence:

- New legacy migration, startup candidate cleanup, chained restore, SMS-prefix, and Auth truth-table tests failed on the pre-fix implementation (stale root files remained, ancestor restore was missing, candidates survived `New`, SMS prefixes were dropped, and invalid Auth combinations were accepted).

GREEN verification:

- `GOCACHE=/tmp/supabase-installer-go-cache GOMODCACHE=/tmp/supabase-installer-go-mod-cache go test -count=1 -timeout 2m ./apps/provisioner/internal/projectfs ./apps/provisioner/internal/render ./apps/manager/internal/project` — PASS.
- `GOCACHE=/tmp/supabase-installer-go-cache GOMODCACHE=/tmp/supabase-installer-go-mod-cache go test -race -count=1 -timeout 2m ./apps/provisioner/internal/projectfs ./apps/provisioner/internal/server ./apps/provisioner/internal/runtime ./apps/provisioner/internal/compose` — PASS.
- `GOCACHE=/tmp/supabase-installer-go-cache GOMODCACHE=/tmp/supabase-installer-go-mod-cache go test -count=1 -timeout 5m ./apps/manager/... ./internal/...` — PASS.
- `GOCACHE=/tmp/supabase-installer-go-cache GOMODCACHE=/tmp/supabase-installer-go-mod-cache go test -count=1 -timeout 5m ./apps/provisioner/...` — PASS.
- `GOCACHE=/tmp/supabase-installer-go-cache GOMODCACHE=/tmp/supabase-installer-go-mod-cache go test -count=1 -timeout 5m ./...` — PASS.
- `GOCACHE=/tmp/supabase-installer-go-cache GOMODCACHE=/tmp/supabase-installer-go-mod-cache go test -count=1 -timeout 2m ./apps/provisioner/internal/render -run TestRepresentativeComposeConfig -v` — PASS; real stable-project-root `docker compose config --quiet` validation passed for lightweight, standard, and full outputs.
- `git diff --check` — PASS.

## Fix Round 5

Hardened the projectfs generation state machine. After candidate-to-generation
rename, the `generations` parent is fsynced before `current` is replaced;
current replacement is followed by a runtime-parent fsync, and a failed
pointer fsync restores the prior pointer before returning. The closure marks
the generation committed immediately after durable pointer publication and
before legacy cleanup.

Legacy migration now moves only the exact eligible root runtime names into one
temporary quarantine. A move or pre-delete sync failure restores every moved
entry and removes the quarantine; a successful migration removes the
quarantine. Cleanup failures retain committed-state semantics so the restore
closure rolls back the selected generation, including removing `current` on a
first migration with no prior pointer. Private runtime hooks provide
deterministic operation-order and second-entry move-failure fixtures without
changing the production API.

RED evidence:

- The new fsync-order and legacy-failure tests initially failed to compile
  because `Root` had no injectable runtime hooks.

GREEN verification:

- `GOCACHE=/tmp/supabase-installer-go-cache GOMODCACHE=/tmp/supabase-installer-go-mod-cache go test -count=1 -timeout 2m ./apps/provisioner/internal/projectfs` — PASS, including generation/current fsync ordering, injected generation/current fsync failures, second-entry legacy rollback, first-migration rollback, chained restore, stale CAS, candidate cleanup, and concurrent staging.
- `GOCACHE=/tmp/supabase-installer-go-cache GOMODCACHE=/tmp/supabase-installer-go-mod-cache go test -race -count=1 -timeout 2m ./apps/provisioner/internal/projectfs` — PASS.
- `GOCACHE=/tmp/supabase-installer-go-cache GOMODCACHE=/tmp/supabase-installer-go-mod-cache go test -count=1 -timeout 2m ./apps/provisioner/internal/server ./apps/provisioner/internal/runtime ./apps/provisioner/internal/compose` — PASS.
- `GOCACHE=/tmp/supabase-installer-go-cache GOMODCACHE=/tmp/supabase-installer-go-mod-cache go test -race -count=1 -timeout 2m ./apps/provisioner/internal/projectfs ./apps/provisioner/internal/server ./apps/provisioner/internal/runtime ./apps/provisioner/internal/compose` — PASS.
- `GOCACHE=/tmp/supabase-installer-go-cache GOMODCACHE=/tmp/supabase-installer-go-mod-cache go test -count=1 -timeout 5m ./...` — PASS.
- `GOCACHE=/tmp/supabase-installer-go-cache GOMODCACHE=/tmp/supabase-installer-go-mod-cache go test -count=1 -timeout 2m ./apps/provisioner/internal/render -run TestRepresentativeComposeConfig -v` — PASS; real Compose validation succeeded for lightweight, standard, and full generated outputs.
- `git diff --check` — PASS.
