# Configuration Remediation Design

## Goal

Repair the confirmed configuration defects in Supamanager without silently
changing existing project data, then add a separately reviewed migration to
Supabase's current opaque API-key and asymmetric-JWT model.

## Scope and sequencing

### Phase 1: configuration correctness

Phase 1 covers CFG-001 through CFG-004 and CFG-006 through CFG-010 from the
[configuration audit](../../audits/2026-09-03-configuration-official-alignment.md),
except CFG-005. It must preserve existing project credentials and running
projects unless an administrator explicitly changes a relevant setting.

1. **R2 Storage:** force path-style and disable S3 object tags for TUS. The
   backend selector must set these values; server-side validation must reject
   invalid persisted/API requests so clients cannot bypass the UI.
2. **Storage transitions:** do not permit a change to backend, bucket,
   endpoint, region, account ID, or local path while objects exist. The
   Provisioner will count `storage.objects` through the project-local database
   container before publishing a changed configuration; it returns an
   actionable error rather than attempting implicit migration. Empty backends
   remain switchable. A future migration job is out of scope.
3. **Phone MFA:** enabling either Phone MFA flag requires a configured Phone
   Auth provider and retained/replaced provider secret. This keeps the shared
   SMS dependency explicit and works for both initial create and later patch.
4. **Supavisor:** reconcile must update the persistent tenant settings, rather
   than recreating only the container. The pinned `pooler.exs` startup script
   will upsert the tenant's client limit and pool size by its fixed tenant ID.
5. **Caddy:** Manager is multi-project, so per-project Caddy is disabled. The
   API rejects `httpsMode=caddy`, UI presents only external reverse proxy, and
   the renderer fails closed if a legacy configuration requests Caddy. Existing
   Caddy configurations require a manual migration to external proxy mode;
   they are not silently rewritten.
6. **Storage upload limit:** add a bounded `uploadFileSizeLimit` field to
   Storage configuration, defaulting to the current 50 MiB. Render it to
   `FILE_SIZE_LIMIT`. The value is bytes, because the downstream variable is
   bytes; UI labels it in MiB while preserving an exact integer value.
7. **R2 Account ID:** accept only a 32-character lowercase hexadecimal Cloudflare
   Account ID and validate the constructed HTTPS endpoint before rendering.
8. **Functions directory:** remove the unused `FunctionsConfig.Directory`
   field from the API contract, UI, normalization, and reconciliation diff.
   Function releases continue to use Manager's fixed, project-local managed
   directory.

### Phase 2: new API keys and asymmetric JWTs

Phase 2 is intentionally not implemented in Phase 1. It adds encrypted
project-secret fields for the publishable/secret opaque keys and asymmetric
signing material, emits all required Auth/Gateway/Realtime/Storage/Functions
variables, and exposes an explicit migration/rotation operation.

New projects may opt into the current key model only after all consumers are
rendered and tested. Existing projects remain legacy-key projects until an
administrator starts migration. A failed migration must retain the old active
key set and restore the previous Compose generation.

## Architecture and data flow

The Manager remains the authority for typed configuration, validation, secret
mutation, and admission. The Provisioner remains the authority for generated
Compose, Storage inspection, and runtime reconciliation.

```
UI/API patch
  -> Manager normalization + validation
  -> encrypted desired configuration
  -> Provisioner preflight (Storage transition check)
  -> atomic generation publish + targeted recreate
  -> service-specific startup/upsert
```

The Storage preflight occurs before the new generation is published. A
non-empty `storage.objects` result returns an error naming the incompatible
transition; the existing runtime is left untouched. The storage schema is the
authoritative catalog for objects addressable through this self-hosted stack,
so its rows are deliberately used as the conservative transition gate.

Supavisor's durable tenant settings are updated by its own pinned startup
script at every container start. The script finds the tenant by external ID,
creates it if absent, otherwise updates precisely `default_pool_size`,
`default_max_clients`, and its bootstrapped user pool setting. The tenant ID,
database endpoint, and credentials remain stable.

## Compatibility and error handling

- Storage configurations persisted before Phase 1 are normalized for R2 during
  read/patch/render: path-style is forced and the R2-only TUS behavior is
  emitted. No secret is modified.
- A legacy Caddy project is not made unstartable by an unrelated configuration
  patch. Its configuration can be read, but changing it requires migration to
  external mode; an explicit lifecycle migration can be designed later.
- Invalid Phone MFA or R2 values are rejected by both Manager validation and
  renderer validation. UI validation mirrors the same restrictions for early
  feedback but is not the security boundary.
- Removing the Functions directory field is backward compatible because JSON
  decoders ignore the historical field and runtime behavior has always used
  the fixed managed path.
- Upload limit must be positive and bounded to prevent integer overflow and
  accidental unlimited uploads. The accepted range is 1 MiB through 5 GiB.

## Test strategy

Each behavior is implemented test-first and observed failing before production
code changes.

- Manager unit tests: reject Phone MFA without provider; reject invalid R2
  Account ID; reject Caddy; validate upload-limit bounds; preserve API patches
  without Functions directory.
- Renderer tests: R2 Compose/env always contain path-style and
  `TUS_ALLOW_S3_TAGS=false`; upload limit reaches Storage; Caddy requests fail;
  no Functions directory output exists.
- Runtime/reconcile tests: non-empty Storage rejects backend transition before
  publish/recreate; empty Storage permits it; Supavisor render/startup contains
  update-or-create tenant behavior.
- Web tests: Storage limit control and R2 restrictions; Caddy option removed;
  Functions directory is absent.
- End-to-end smoke tests: R2 normal and resumable upload path where credentials
  are available; Supavisor tenant update verified through its management state.

## Non-goals

- Object copying, cross-bucket migration, or rollback of object bytes.
- Per-function runtime configuration or arbitrary raw `.env` access.
- Replacing the host-level external reverse proxy with a multi-domain Caddy
  control plane.
- Phase 2 API-key/JWK implementation in the Phase 1 change set.
