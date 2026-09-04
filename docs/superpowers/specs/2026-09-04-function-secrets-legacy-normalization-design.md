# Function Secrets legacy normalization and runtime template diagnostics

## Problem

Function secret updates merge the submitted Functions section into the stored project configuration and then validate the complete aggregate. Projects created before newer configuration fields existed may store zero values such as `auth.jwtExpiry = 0` or `storage.uploadFileSizeLimit = 0`. A valid Functions secret update is therefore rejected with HTTP 422 by an unrelated legacy field.

Runtime Sync also maps every official-template retrieval failure to a generic network message. On 2026-09-04 we reproduced the actual failure from both the host and the Provisioner container: GitHub returned HTTP 403 with unauthenticated core API quota `60/60`, while `raw.githubusercontent.com` returned HTTP 200.

## Design

### Compatibility normalization

Introduce one server-side compatibility normalization step immediately after loading stored configuration and before merging or validating a section update. It supplies only historical defaults that old persisted records could not contain:

- `auth.jwtExpiry`: `3600` when zero.
- `storage.uploadFileSizeLimit`: `50 MiB` when zero.

The normalization applies to stored state only. It must not rewrite non-zero values and must not weaken validation of a value explicitly supplied by a current request. An explicit invalid Storage update continues to return 422.

All section updates use the same normalized aggregate, so the behavior is not special-cased to Functions and future unrelated edits cannot encounter these two legacy gaps.

### Official-template failure behavior

Classify official-template HTTP failures without exposing secrets. GitHub HTTP 403 responses carrying rate-limit headers are reported as a GitHub API rate-limit diagnostic, including the reset timestamp when present. Other HTTP and transport failures retain a safe, actionable diagnostic.

During explicit Runtime Sync, if latest-tag resolution or download fails and the currently applied immutable template snapshot is available in the verified cache, the Provisioner may continue with that cached current template. If no verified cached snapshot exists, reconciliation fails without changing runtime state. Cache integrity checks remain mandatory.

### UI behavior

The existing field-error mapping remains unchanged. After server compatibility normalization, adding a valid Function secret no longer surfaces an unrelated `uploadFileSizeLimit` error. Runtime Sync displays the new sanitized diagnostic returned by the Provisioner.

## Verification

- A Functions PATCH against stored `uploadFileSizeLimit = 0` is accepted and persists the 50 MiB compatibility default.
- A current Storage PATCH explicitly sending `uploadFileSizeLimit = 0` remains rejected.
- Existing `jwtExpiry = 0` compatibility remains covered.
- GitHub rate-limit responses produce the specific sanitized diagnostic.
- Runtime Sync falls back only to a verified current cached snapshot; no-cache retrieval failures remain failures.
- Full Go tests, web tests, type checking, and production build pass.
