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

- Multi-file publication is implemented with staged, fsynced files plus rollback on write failure; filesystem-level crash consistency across three independent renames remains dependent on the host filesystem.
