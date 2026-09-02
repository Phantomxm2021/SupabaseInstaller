# Task 2 implementation report

## Scope

- Renderer emits a typed `STORAGE_FILE_SIZE_LIMIT` dotenv value and wires Storage's `FILE_SIZE_LIMIT` to it.
- R2 always emits `GLOBAL_S3_FORCE_PATH_STYLE=true`, overriding an unsafe requested value.
- R2 Storage emits `TUS_ALLOW_S3_TAGS: "false"`.
- Renderer rejects `network.httpsMode=caddy` at the `Project` boundary.
- Zero-value compatibility inputs retain the existing 50 MiB file-size default.

## TDD evidence

Added focused tests for R2 compatibility, typed file-size rendering, and Caddy rejection. Before implementation:

```text
go test ./apps/provisioner/internal/render -run 'TestRender(R2ForcesCompatibleStorageOptions|StorageFileSizeLimit|UsesOnlyPostgreSQL17AndGatewayChoices)$' -count=1
FAIL (three expected assertions: Caddy accepted; R2 options missing; file-size value missing)
```

After implementation:

```text
go test ./apps/provisioner/internal/render -run 'TestRender(R2ForcesCompatibleStorageOptions|StorageFileSizeLimit|UsesOnlyPostgreSQL17AndGatewayChoices)$' -count=1
ok
```

## Verification

```text
go test ./apps/provisioner/internal/render -count=1
ok
go test ./apps/provisioner/... -count=1
ok (all packages)
git diff --check
ok
```

Updated standard/full Compose goldens for the Storage interpolation.

## Legacy Caddy correction

The initial implementation rejected Caddy in `render.Project`, which incorrectly
blocked trusted legacy projects during reconcile and secret rotation. The
manager is the admission/enforcement point (`validateConfiguration` permits
Caddy only for unchanged stored legacy state); the renderer must preserve that
state. The renderer rejection was removed and the Caddy regression now asserts
the legacy PostgreSQL 17/Caddy Compose output.

Correction TDD evidence:

```text
go test ./apps/provisioner/internal/render -run TestRenderUsesOnlyPostgreSQL17AndGatewayChoices -count=1
FAIL (legacy Caddy render rejected by renderer boundary)
go test ./apps/provisioner/internal/render -run TestRenderUsesOnlyPostgreSQL17AndGatewayChoices -count=1
ok
```
