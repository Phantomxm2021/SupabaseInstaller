# Server configuration operations

The Configuration page edits one canonical server configuration. Manager
SQLite is the source of truth; a PATCH writes that value, then queues one
runtime apply operation. The Provisioner renders the complete configuration
and runs Docker Compose against the server directory. The operation panel
reports validation, Compose, health verification, and the concrete terminal
error when an apply fails.

Older clients may still include a revision field in their request body. It is
ignored by the current API and is not sent to Provisioner. A retry always reads
the current canonical row; it does not reuse a stale candidate or restore a
last-good value.

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
cross-server collisions and are released when their owner is disabled.

## Secrets and recovery

For every configured secret, `retain` keeps the encrypted value, `replace`
stores a new encrypted value, and `remove` deletes it only when the feature no
longer requires it. GET and operation history expose only `passwordSet` or
`secretSet` markers. Reveal requires recent administrator authentication and
is `no-store`; plaintext is held only in memory until the view is cleared.

An operation validates the canonical value, renders the runtime files, runs
`docker compose config --quiet`, and applies them with `docker compose up -d
--remove-orphans`. A failure leaves the canonical value intact and records the
phase, service, exit status, and redacted Compose/health detail. It never
silently restores another configuration. Database-password rotation remains a
narrow compatibility command because it must coordinate an old and new
credential; ordinary settings do not use that protocol.

## Restart, recreate, and rollback

Restarting Manager or Provisioner resumes durable queued/running operations by
re-reading the canonical configuration. Runtime recreate keeps server volumes
and only changes the services affected by the rendered configuration. Retry is
safe because no numeric revision comparison is used by the normal apply path.

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

The Manager control plane and each server Supabase host use separate proxy
entries. Set `PUBLIC_ORIGIN` to the Manager URL, keep Provisioner private, and
route each server's public domain to its loopback-bound Studio and API Gateway
ports. Runtime service ports must not be independently published. See the
[server host Nginx and Cloudflare guide](project-host-nginx.md) for the exact
path routing, WebSocket headers, TLS, DNS, and validation procedure.
