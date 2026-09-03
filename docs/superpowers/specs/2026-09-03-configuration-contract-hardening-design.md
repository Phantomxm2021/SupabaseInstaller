# Configuration Contract Hardening Design

## Goal

Make Supamanager's typed project configuration match the documented Supabase
self-hosted behavior while preserving the runtime behavior of existing projects.
This change closes CFG-011 through CFG-017 from the configuration audit.

## Scope

The implementation covers four related configuration boundaries:

1. Auth session expiry and the Auth default redirect URL.
2. PostgreSQL, Supavisor, and Realtime connection capacity.
3. Safe Postgres `shared_buffers` input and Supavisor internal metadata pool
   configuration.
4. Removal of the unsafe legacy per-project Caddy render path.

It does not expose arbitrary raw `.env` settings, automatically install an
external proxy, alter existing project domains, or migrate application OAuth
providers.

## Public configuration model

### Auth Site URL

`GeneralConfig` gains `AuthSiteURL` (`authSiteUrl` on the wire). It is the
absolute HTTP(S) URL rendered as GoTrue's `SITE_URL` and represents the
application's default post-auth redirect URL.

The existing `GeneralConfig.SiteURL` remains the manager-owned base hostname
used to derive the immutable project public domain (`<slug>.<base-host>`). It
is retained to avoid changing project addressing, managed TLS file derivation,
or existing API requests. User-facing labels must call it **Server base domain**
and call the new field **Auth site URL**.

For backward compatibility, an empty `AuthSiteURL` renders as
`https://<General.Domain>`, exactly matching the prior runtime behavior. New
project defaults set it to that same derived project URL before installation.
When supplied, it is validated as an absolute HTTP(S) URL and rendered without
derivation. `SUPABASE_PUBLIC_URL` and `API_EXTERNAL_URL` continue to derive
only from the project domain.

### JWT expiry

`AuthConfig.JWTExpiry` is valid only in the official range 1–604800 seconds.
The default is 3600 seconds. Rendering a legacy zero value must emit 3600 so a
stored pre-migration project never starts Auth with an invalid value. Values
above 604800 continue to be rejected by manager validation rather than silently
clamped.

### Connection budget and pool configuration

`PoolerConfig` gains `InternalDBPoolSize` (`internalDbPoolSize`), which maps
only to `POOLER_DB_POOL_SIZE`; `PoolSize` continues mapping only to
`POOLER_DEFAULT_POOL_SIZE`. The new field defaults to 5.

Manager aggregate validation calculates the enabled pool consumers:

```
required = fixedReserve
         + realtime.databasePoolSize (when Realtime is enabled)
         + pooler.poolSize (when Supavisor is enabled)
         + pooler.internalDbPoolSize (when Supavisor is enabled)
```

`fixedReserve` is 10 connections for Postgres/Auth/REST/Storage/metadata
system consumers. The precise constant is documented beside the validator and
displayed in its validation error. A configuration is invalid if any enabled
pool exceeds `database.maxConnections` or if `required >= database.maxConnections`.
The strict inequality leaves at least one connection beyond the modeled budget.
Disabled services do not consume the corresponding budget.

Existing persisted configurations with zero `InternalDBPoolSize` are hydrated
to 5 before validation/rendering. No existing nonzero persisted pool field is
rewritten.

### PostgreSQL shared buffers

`DatabaseConfig.SharedBuffers` remains optional. A nonempty value must be a
positive integer followed by a Postgres memory unit: `B`, `kB`, `MB`, `GB`,
`TB`, `KiB`, `MiB`, `GiB`, or `TiB` (case-sensitive according to Postgres
examples). Manager validation is authoritative; the web schema mirrors it.
The renderer only appends the Postgres `-c shared_buffers=...` argument after
that validation has passed. The UI states that saving causes a database
recreation and therefore dependent services briefly restart.

### Legacy Caddy

No project configuration may render the per-project upstream Caddy overlay.
New configurations already reject Caddy; stored legacy Caddy configurations
must now fail preparation and reconciliation with a clear migration-required
error before Compose changes are attempted. The error directs administrators to
place an external reverse proxy in front of the loopback API gateway and save
`httpsMode=external`. There is no automatic mode switch because removing Caddy
before an external proxy is live would cause an outage.

The manager continues to read legacy state so it can display and update the
project after the operator explicitly changes it to `external`; it simply no
longer treats Caddy as runnable. The Caddy template may remain embedded because
it belongs to the pinned upstream source, but the renderer must never select it.

## Data flow and compatibility

1. Configuration-store hydration normalizes legacy zero values: `JWTExpiry=0`
   becomes 3600 for read/prepare behavior, `InternalDBPoolSize=0` becomes 5,
   and empty `AuthSiteURL` is retained as an explicit backward-compatible
   fallback rather than being persisted automatically.
2. Incoming configuration patches must provide values within the current
   contract. The web client sends `authSiteUrl`, 3600 JWT expiry, and the
   internal DB pool value.
3. Manager validates the aggregate before it is encrypted or queued.
4. Provisioner renders `SITE_URL` from `AuthSiteURL` (or the compatible
   project-domain fallback), renders distinct Supavisor values, and receives
   no Caddy overlay.
5. Existing Caddy projects have a safe, actionable failure at their next
   reconciliation until changed to external mode; no silent mode conversion is
   permitted.

## Tests and acceptance criteria

- Manager tests prove JWT default/range behavior, legacy zero fallback,
  `AuthSiteURL` validity, explicit Auth URL rendering, connection-budget
  rejection, independent pool mapping, shared-buffer parsing, and Caddy
  migration-required failures.
- Renderer tests prove output environment values and absence of Caddy service.
- Web tests prove validation defaults/limits and the new labels/field wiring.
- Full Go tests, web tests, web production build, Docker Compose render tests,
  and `git diff --check` must pass.

## Non-goals and constraints

- Do not expose arbitrary PostgREST, Realtime, or raw GoTrue variables.
- Do not auto-migrate a live Caddy project to external proxy mode.
- Preserve encrypted secret semantics and optimistic configuration revision
  behavior.
- Keep the pinned `self-hosted/v0.8.0` upstream template unchanged unless a
  renderer-only safety guard cannot express the required behavior.
