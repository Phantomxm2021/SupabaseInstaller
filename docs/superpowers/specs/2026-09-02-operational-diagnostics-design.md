# Operational Diagnostics Design

## Goal

Ensure every failed Manager, Provisioner, or Nginx Proxy operational action
records a specific, actionable, secret-safe diagnostic.  The public error code
and canonical user message remain stable; diagnostic detail is a separate,
explicitly redacted field.

## Scope

Covered operation families:

- Manager install, configuration reconcile/resume/retry, database-password
  rotation/compensation/confirmation, project lifecycle, function operations,
  and managed TLS staging.
- Provisioner reconcile, runtime publication, lifecycle, database-password
  rotation/rollback/confirmation, function deployment/rollback/deletion,
  managed TLS, host inspection, and Compose/Docker actions.
- Nginx Proxy site apply/remove and certificate staging.

Authentication, authorization, request decoding, field validation, and secret
reveal errors retain their current canonical messages and are not diagnostic
operation results.

## Current Failure Modes

The audit found three independent ways an actionable error is lost:

1. A runtime branch replaces a lower-level error with `errors.New("… failed")`.
   Password rotation health-check failure was one observed instance.
2. A private HTTP response has only a canonical `APIError`, so a safely
   redacted lower-level cause cannot reach Manager.
3. Manager receives a diagnostic but replaces it while writing the durable
   operation error.  Conversely, Nginx Proxy currently sometimes returns raw
   `err.Error()`, which is unsafe to preserve without a redaction boundary.

## Design

### Error layers

Every operational failure has three distinct representations:

| Layer | Representation | Purpose |
| --- | --- | --- |
| Local cause | contextual wrapped error (`%w`) or labelled `errors.Join` | preserves root cause and any failed compensation in the producing process |
| Private RPC | `code`, canonical `message`, optional `diagnostic` | transports an actionable, redacted, bounded detail to the trusted Manager |
| Durable operation | canonical operation state plus the private diagnostic | gives operators the root cause in the operation record without changing client-facing codes |

No raw error crosses a process boundary.  `diagnostic` is created at the
boundary where the complete set of request secrets is still available, then
the Manager trusts only that typed field for an allow-listed operational code.

Runtime verification also distinguishes an observed unhealthy service from an
unavailable Docker control plane. A Docker timeout is retried with bounded
backoff; if the verification window expires, the typed response sets
`runtimeOutcomeUnknown`. The Manager keeps that operation recoverable and
does not attempt speculative password compensation or configuration restore.

### Contracts and compatibility

Add `Diagnostic string \`json:"diagnostic,omitempty"\`` to the operation
response types that can be returned on failure (reconcile, lifecycle,
function result/actions, TLS stage, inspection where applicable).  Preserve
the existing `error.code` and `error.message` values.  Omitted diagnostics
remain valid for old Provisioners and are mapped to the existing canonical
message by Manager clients.

For operations that have no response body today, introduce the existing
`ErrorEnvelope` shape with an optional diagnostic rather than encoding errors
inside the message.  Manager clients decode this typed envelope and make it a
`ClientError.Message` only for an allow-listed operational code.

The Nginx Proxy API gets a matching private error envelope.  Its caller maps a
missing or malformed diagnostic to the canonical proxy error, never to the raw
HTTP body.

### Redaction and size rules

One shared redaction helper is used at each private boundary:

- Replace every non-empty supplied project secret, runtime secret, old/new
  database password, and proxy token with `[REDACTED]`.
- Preserve service names, Compose exit status, a bounded Compose output,
  filesystem operation names, health report state, and remediation-relevant
  paths under the project runtime directory.
- Collapse control characters and cap diagnostics at 4 KiB; append a fixed
  truncation marker when needed.
- Never include request bodies, certificates, private keys, function archive
  contents, or raw authorization headers.

The same sanitized string is used for the producer log and response.  Manager
does not log or persist the unsanitized cause.

### Source-preservation policy

Operational code must use these forms:

- `fmt.Errorf("action context: %w", err)` for one failed dependency.
- `errors.Join` with explicit context for a primary failure plus failed
  rollback/restore/cleanup.
- A new fixed error only when no lower-level cause exists (unsupported
  capability, invalid durable state, or an intentionally non-disclosing
  condition).

The audit will replace generic wrappers only where they discard an existing
cause.  It will not expose validation or authentication errors and will not
invent lower-level details when none exist.

### Operation coverage

| Boundary | Required behavior |
| --- | --- |
| Provisioner reconcile and password rotation | Preserve render, stage, validation, Compose, health, metadata, rollback, restore, and compensation causes; return redacted diagnostic on typed failure. |
| Provisioner lifecycle/functions/TLS/inspection | Convert operational backend errors to a redacted typed diagnostic, while retaining canonical codes. |
| Manager Provisioner client and orchestrators | Decode diagnostics only from typed, allow-listed operation responses; persist them through `operations.Fail`; fall back safely for old or malformed responses. |
| Manager lifecycle/functions/install/TLS clients | Preserve the typed downstream diagnostic through their operation failure paths. |
| Nginx Proxy and its Manager client | Sanitize errors before response/logging; propagate code plus diagnostic for apply, remove, and certificate staging. |

## Testing

Each covered family will have tests that assert:

1. a Compose, health, filesystem, or proxy failure reaches the Manager durable
   operation with its contextual detail;
2. a failed rollback/restore is retained alongside the primary cause;
3. all supplied secret sentinels, tokens, certificates, and private keys are
   absent from the diagnostic and logs;
4. older responses with no diagnostic still yield the existing canonical
   message;
5. non-operational validation/authentication responses do not gain a
   diagnostic.

An audit test will enumerate the supported operational error codes and verify
that their Manager clients use the typed diagnostic path.  This guards against
adding a new operational endpoint that silently falls back to a generic error.

## Rollout and Acceptance Criteria

This is wire-compatible: updated Managers work with older Provisioners and
vice versa, with a safe canonical fallback.  Deploy Provisioner and Nginx
Proxy before Manager to gain diagnostics immediately.

The work is accepted when every listed operational family preserves an
actionable root cause (or explicitly documents that no cause exists), no test
can recover a supplied secret from a diagnostic, and the full Go test suite
passes.
