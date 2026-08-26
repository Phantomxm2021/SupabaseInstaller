# Project configuration operations

The Configuration page edits one aggregate revision. PATCH requests are
optimistic-concurrency checked; a stale revision is a conflict and never
silently overwrites another change. The operation panel reports validation,
runtime reconciliation, health verification, and terminal state.

## General

Domain, public Site URL, and pinned Supabase version are validated together.
Domain and URL changes recreate services that consume public URLs (gateway,
Auth, and Studio) and leave unrelated services untouched.

## Services

Service switches are dependency-aware. Studio requires postgres-meta; Storage,
Functions, and Caddy require their declared gateway/REST dependencies. Turning
off a service removes its runtime containers while retaining data. A switch
changed back to its saved value produces no operation.

## Auth

Auth settings, email/password behavior, signup policy, anonymous sign-in, and
phone provider fields are rendered into Auth. Auth changes recreate Auth only
unless a dependency also changes. Invalid nested fields are shown at the
owning control.

## SMTP

SMTP host, port, sender, and TLS settings are Auth runtime settings and normally
recreate Auth. SMTP credentials use the secret rules below.

## OAuth providers

Each provider is edited independently. Enabling, disabling, or replacing a
provider secret recreates Auth; provider credentials are never returned by GET.

## Storage

Local and S3-compatible backends validate endpoint/protocol and credentials.
Storage changes recreate Storage (and its dependency closure when required).

## Realtime

Realtime limits and log level recreate Realtime only. Its generated encryption
key remains managed by the installer.

## Functions

The Functions directory and environment variables are rendered into the
Functions runtime. Variable changes recreate Functions only. Reserved runtime
names cannot be overridden.

## Database

Database tuning and extensions are validated against the pinned template.
Changing database settings recreates the database only when the setting is
runtime-affecting; data volumes are retained across recreate/restart.

## Connection Pooler

Pool size and transaction/session ports apply only when Supavisor is enabled.
Disabled-owner ports do not participate in admission conflicts. Enabling it
reserves and validates all selected ports atomically.

## Network

Gateway mode, HTTPS mode, API/Studio/direct database/pooler ports, and public
network settings are validated as one unit. Host ports are allocated without
cross-project collisions and are released when their owner is disabled.

## Secrets and recovery

For every configured secret, `retain` keeps the encrypted value, `replace`
stores a new encrypted value, and `remove` deletes it only when the feature no
longer requires it. GET and operation history expose only `passwordSet` or
`secretSet` markers. Reveal requires recent administrator authentication and
is `no-store`; plaintext is held only in memory until the view is cleared.

An operation stages and validates a candidate runtime before publication, then
recreates only affected services and waits for the selected health set. A
render/stage failure restores the admitted desired revision and encrypted
secret snapshot. A post-publication failure attempts to restore the previous
generation and reports `ROLLED_BACK` only after health confirms recovery;
otherwise it reports `FAILED` for operator intervention. Database-password
rotation journals its phases and uses the same idempotency key on retry.

## Restart, recreate, and rollback

Restarting Manager or Provisioner resumes durable queued/running operations;
replaying an idempotency key does not double-promote a revision. Runtime
recreate keeps project volumes and applies the selected generation. Rollback
restores the prior runtime generation, secret snapshot, canonical projections,
and revision when recovery succeeds.

## Inspecting operations safely

Use the UI Operation panel or the authenticated operations API to inspect step,
status, progress, and redacted error code. Do not collect `.env`, Compose, or
secret payloads. Safe control-plane logs are:

```sh
docker compose -f deploy/docker-compose.yml --env-file deploy/.env logs --no-color --tail=200 manager provisioner
```

## Raw editing is unavailable

There is deliberately no raw `.env` editor, Compose editor, shell endpoint, or
public Provisioner endpoint. Changes must go through typed configuration
sections so validation, encrypted secret handling, service impact, and
rollback remain auditable.

## Reverse proxy expectations

Proxy only the single Manager URL. Use the exact external origin in
`PUBLIC_ORIGIN`, enable secure cookies for HTTPS, preserve forwarding headers,
and keep Provisioner on the internal Compose network. Runtime service ports are
host-bound according to the Network section and should not be independently
published by a proxy.
