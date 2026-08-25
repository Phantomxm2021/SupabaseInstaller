# Supabase Self-hosted Web Installer & Manager V1 Design

**Date:** 2026-08-26
**Status:** Approved
**Source:** `Docs/Supabase Self-hosted Web Installer & Manager PRD.md`

## 1. Goal

Build a server-only web product that installs and manages multiple isolated, official Supabase Self-hosted Docker runtimes. An administrator starts the product once with Docker Compose, opens one URL, and uses a React interface to create, configure, operate, update, back up, and delete Supabase projects without direct shell, filesystem, or Docker Socket access from the browser.

V1 implements the complete PRD in five independently runnable milestones. Every milestone must leave the product deployable and must preserve the rule that one project owns one isolated official Supabase runtime.

## 2. Product Boundaries

The Manager orchestrates official Supabase Self-hosted Docker assets. It does not fork Supabase services, reproduce Supabase database initialization, share project databases, or expose the Docker Socket to the browser-facing process.

V1 production targets are Linux servers on `amd64` and `arm64` with Docker Engine 27 or newer and Docker Compose v2. macOS Apple Silicon with Docker Desktop is supported as a development and integration-test environment, not as a production target. Windows Server, Podman, remote Docker hosts, Kubernetes, high availability, multi-region deployment, and Supabase Cloud platform emulation are outside V1.

The first release pins one tested official Self-hosted template and all container image tags. It never uses `latest`, GitHub `master`, or an unvalidated template at project creation time. V1's update UI can move between template versions shipped and explicitly supported by the installed Manager release.

## 3. Runtime Architecture

The product ships as two long-running containers started by one top-level Compose project:

```text
Browser
   |
   | HTTP/HTTPS, one public origin
   v
Manager container
   |- embedded React application
   |- Go REST API
   |- session and authorization boundary
   |- SQLite metadata store
   |- encrypted secret store
   |- operation coordinator and event stream
   |
   | authenticated private-network RPC
   v
Provisioner container
   |- Docker Engine API client
   |- project filesystem writer
   |- Compose/runtime lifecycle executor
   |- health, logs, backup, and rollback executor
   |
   v
Docker Socket -> isolated Supabase project runtimes
```

Only the Manager publishes a host port. It serves the built React files and owns `/api/*`, so no browser-side cross-origin configuration is required. The Provisioner has no published port, joins only the private management network, validates every request, and is the only product container with the Docker Socket mounted.

The production deployment command is:

```sh
docker compose up -d
```

The default deployment exposes a configurable HTTP port for use behind an existing reverse proxy. An optional Caddy Compose profile provides direct HTTPS. Both modes keep a single browser origin.

## 4. Repository and Technology Layout

```text
apps/
  web/                 React + TypeScript + Vite application
  manager/             Go HTTP server, domain layer, SQLite adapters
  provisioner/         Go internal RPC server and Docker adapters
internal/
  contracts/           Shared API schemas and generated clients
  templates/           Pinned official Supabase runtime bundle metadata
deploy/
  docker-compose.yml   Manager + Provisioner production deployment
  caddy/               Optional HTTPS profile
tests/
  integration/         Cross-service and Docker integration tests
docs/
  operations/          Installation, recovery, security, and upgrade guides
```

The web application uses React, TypeScript, Vite, React Router, TanStack Query, React Hook Form, Zod, and a small accessible component system built with Radix primitives. The visual language is a compact, original dashboard inspired by the information density of Supabase Studio without copying Supabase trademarks or protected assets.

The Manager and Provisioner use Go. SQLite stores metadata and operation history. Database access is migration-driven and transaction-bound. REST is used between the browser and Manager. Server-Sent Events stream operation progress and live logs because all browser updates are server-to-client and do not require WebSocket command semantics.

## 5. Manager Responsibilities

The Manager owns all public behavior:

- first-run administrator bootstrap, login, logout, password change, recovery-code rotation, and session revocation;
- project validation, persistence, service configuration, and dependency rules;
- encryption and decryption of secrets at the application boundary;
- operation creation, idempotency, cancellation eligibility, progress persistence, and SSE fan-out;
- allocation records for API, Studio, database, and pooler host ports;
- REST resources for projects, services, Auth, Storage, Functions, logs, backups, versions, and operations;
- serving the React application and enforcing authorization for every API route;
- generating redacted audit events for sensitive changes.

The Manager never executes shell commands and never opens the Docker Socket. It sends typed, authenticated commands to the Provisioner and reconciles the returned state into SQLite.

## 6. Provisioner Responsibilities

The Provisioner accepts only an explicit allow-list of typed operations. It does not expose a general command-execution endpoint. Project IDs and slugs are validated before filesystem use, paths are resolved under the configured project root, and Compose project names are generated rather than accepted verbatim.

Its operations include:

- host capability, disk, memory, Docker, Compose, port, and directory checks;
- atomic project-directory preparation from the pinned official template;
- `.env`, `.env.functions`, structured overrides, proxy snippets, and version metadata generation;
- secret injection without returning plaintext in logs;
- Compose pull, create, start, stop, restart, recreate, remove, and inspect actions;
- dependency-ordered startup and health verification;
- per-service log streaming with sensitive-value redaction;
- `pg_dump`, configuration backup, encrypted archive creation, and restore;
- configuration snapshots and compensating rollback actions.

Docker actions use the Docker Engine API where practical. Compose lifecycle behavior uses the installed Compose v2 CLI with fixed arguments and a controlled environment because official Supabase runtime assets are Compose-native. No user input is interpolated into a shell string.

## 7. Data and Secret Model

SQLite contains at least these entities:

- `admins`, `sessions`, `recovery_codes`;
- `projects`, `project_services`, `project_configs`;
- `project_secrets` with ciphertext, nonce, key version, and secret kind;
- `operations`, `operation_steps`, `operation_events`;
- `port_allocations`, `runtime_versions`, `backups`, `audit_events`.

Project configuration is split into typed non-secret JSON and individually encrypted secret values. The Manager requires `MASTER_ENCRYPTION_KEY` at startup. Secrets use AES-256-GCM with a fresh nonce, authenticated context containing project ID and secret kind, and an explicit key version for future rotation. Administrator passwords use Argon2id. Recovery codes are random, displayed once, and stored only as hashes.

Each project has a directory beneath a configurable root, defaulting to `/opt/supabase-manager/projects/<slug>`. Writes use a staging directory followed by atomic rename. The directory contains Manager metadata, official template files, generated environment files, overrides, functions, and persistent volume directories. File modes restrict secret-bearing files to the service account.

## 8. Authentication and Session Design

On an empty database, the first browser visit enters a first-run setup flow that creates the single V1 administrator and returns recovery codes. Environment variables may preseed an administrator name and one-time password for automated deployment; that password must be changed at first login.

Sessions use opaque random identifiers stored in `HttpOnly`, `Secure` when HTTPS is active, and `SameSite=Strict` cookies. Only session hashes are persisted. State-changing requests require same-origin checks and CSRF protection. Login and sensitive operations are rate-limited and audited. Secret reveal, copy, rotate, destructive delete, restore, and version migration require recent password confirmation.

## 9. Project Configuration and Dependency Engine

The default Lightweight configuration requires only Project Name, Domain, and Site URL. It enables PostgreSQL, Envoy, Auth, PostgREST, Studio, and postgres-meta. It disables Realtime, Storage, imgproxy, Edge Functions, Supavisor, Logs/Logflare, Vector, and direct PostgreSQL exposure.

Standard adds Realtime, Storage, Functions, and Supavisor. Full enables all supported official services. Custom starts from a preset and allows valid overrides.

Dependency rules are centralized in a pure domain package used by both API validation and UI affordances:

- PostgreSQL is mandatory;
- Studio requires postgres-meta;
- Auth, PostgREST, Realtime, Storage, and Studio require PostgreSQL;
- public APIs require the selected gateway;
- imgproxy requires Storage;
- Logs requires Logflare and Vector;
- Vector cannot be independently enabled without Logs;
- R2 is an S3 backend preset;
- Storage backend and S3-compatible protocol exposure remain independent options;
- published ports must be unique across active and reserved projects.

Invalid combinations are rejected by the API even if a client bypasses UI controls. The UI explains automatically enabled dependencies and prevents impossible states.

## 10. Installation and Operation Model

All mutating runtime actions are durable Operations. A request returns an operation ID immediately. The Manager persists its intended state and sends an idempotency key to the Provisioner. The UI receives ordered progress events through SSE and can reconnect using the last event ID.

Project installation implements the PRD's validation, secret generation, template preparation, configuration generation, port allocation, database-first startup, dependent-service startup, health verification, proxy registration, and final reconciliation steps. Operation statuses are `QUEUED`, `RUNNING`, `SUCCEEDED`, `FAILED`, `ROLLING_BACK`, `ROLLED_BACK`, and `CANCELLED`.

If installation fails, the runtime is stopped, temporary containers and networks are removed, ports are released when safe, and the project is marked failed with actionable diagnostics. Database and storage data are retained unless the administrator explicitly confirms deletion. Configuration updates retain the last known-good snapshot. A failed service recreate triggers a compensating restore and health check.

Manager restart recovery scans non-terminal operations and reconciles them with Docker state. Safe idempotent steps resume; ambiguous destructive steps halt and require administrator action rather than guessing.

## 11. Public API and Internal RPC

The public API follows the PRD resources under `/api`. It uses consistent JSON error envelopes with a machine code, user-safe message, field errors, operation ID when applicable, and correlation ID. Secrets are write-only unless a dedicated recent-auth reveal endpoint is used.

The Manager-to-Provisioner API is private and versioned. Requests contain a short-lived signed service token, operation ID, project ID, expected configuration revision, and typed payload. The Provisioner rejects stale revisions and duplicate non-idempotent requests. Health and event responses never contain raw secret values.

## 12. Web Information Architecture

The public route flow is:

```text
/setup -> /login -> /projects -> /projects/new -> /projects/:id/*
```

The project shell contains Overview, Services, Authentication, Database, Storage, Realtime, Functions, Connection Pool, Logs, Network, Secrets, Backups, and Settings. The create wizard covers Basic, Preset/Services, Authentication, Storage, Functions, Network, and Review, while preserving the three-field fast path through sensible defaults.

Long operations open a persistent operation panel showing current step, elapsed time, details, warnings, retry/rollback actions, and a link to redacted logs. The interface supports light and dark themes, keyboard navigation, accessible form labels, status badges that do not rely on color alone, and responsive desktop/tablet layouts. Mobile supports monitoring and simple actions but is not the primary configuration surface.

## 13. Service-Specific Behavior

Auth supports Email, Phone advanced overrides, Anonymous sign-in, signup policy, Site URL, redirect allow-list, SMTP, and the PRD provider list. Provider callback URLs are derived and read-only. Saving a provider recreates only Auth, verifies `/auth/v1/settings`, and restores the prior config on failure.

Storage supports local filesystem, generic S3, AWS S3, Cloudflare R2, and custom S3-compatible endpoints. Credentials are encrypted. Backend choice and S3-compatible API exposure are independent. Enabling imgproxy automatically requires Storage.

Functions supports runtime enablement, default JWT verification, encrypted environment variables, function discovery, restart after code changes, and recreate after environment changes. V1 does not include a code editor.

Realtime, Supavisor, direct PostgreSQL exposure, Gateway selection, optional Logs/Logflare/Vector, SMTP, Advanced Environment Variables, and service-level restart/recreate follow the dependency and minimal-restart rules in the PRD.

## 14. Networking and Ports

Each project receives a unique Compose project name, Docker network, internal gateway binding, and Manager-owned port allocations. Public project domains map through an external reverse proxy to loopback-bound gateway ports. Direct database and pooler ports are disabled by default and bound according to administrator policy when enabled.

The default Manager deployment assumes an external reverse proxy and emits Nginx/Caddy-compatible upstream information. The optional Caddy profile manages Manager HTTPS. Project proxy registration is adapter-based so external mode can generate configuration without modifying an unknown host proxy.

## 15. Backup, Restore, and Version Updates

V1 backups include a PostgreSQL logical dump, Manager project configuration, encrypted secrets export, Functions files, runtime/template metadata, and a manifest with checksums. Destinations are local filesystem and generic S3-compatible storage. Every archive is encrypted with a unique data key protected by the Manager master key or a supplied recovery passphrase.

Restore validates checksums and compatibility before stopping a runtime. It creates a safety snapshot, restores into controlled paths, starts in dependency order, and reports a durable operation. Failed restore attempts return to the safety snapshot when possible.

Version updates support only template/image sets bundled in and approved by the Manager release. The UI shows release notes and blocking migration warnings. An update backs up configuration, pulls pinned images, applies the supported template transition, recreates affected services, and verifies health. Image rollback is supported; irreversible database migrations require explicit confirmation and cannot claim automatic rollback.

## 16. Error Handling and Observability

Every request and operation has a correlation ID. Manager and Provisioner logs are structured JSON. User-visible diagnostics omit environment dumps and pass through redaction for authorization headers, API keys, JWTs, database passwords, OAuth/SMTP/S3 credentials, and Function secrets.

Health states are `HEALTHY`, `DEGRADED`, `STARTING`, `STOPPED`, `UNHEALTHY`, and `UNKNOWN`. Project health derives from the database, gateway, and all enabled services. An optional-service failure produces `DEGRADED` when core service health remains intact.

The dashboard reports host CPU, memory, and disk plus per-project container usage. Resource thresholds warn but do not block installation unless a hard safety requirement such as insufficient disk for staging is violated.

## 17. Delivery Milestones

### Milestone 1: Installer Core

Deliver the deployment Compose file, first-run login, project model, encrypted secrets, port allocation, pinned Lightweight template, create wizard, operation stream, install/start/stop/restart/delete, health, Overview, and failure cleanup.

### Milestone 2: Complete Service Manager

Add Realtime, Storage shell, imgproxy, Functions shell, Supavisor, Logs/Logflare/Vector, gateway options, service configuration, service restart/recreate, resource display, and dependency enforcement.

### Milestone 3: Auth Manager

Add Auth general settings, URLs, SMTP, all listed OAuth providers, provider-specific fields, Auth-only recreation, settings verification, and rollback.

### Milestone 4: Storage and Functions

Complete local/S3/AWS/R2/custom Storage, S3 protocol exposure, upload verification, encrypted Function secrets, function discovery, and restart/recreate controls.

### Milestone 5: Operations

Complete redacted live logs, metrics, supported version updates, local/S3 encrypted backup and restore, configuration import/export, operational documentation, and full acceptance testing.

Each milestone includes database migrations, API contracts, UI states, unit tests, integration tests, security checks, documentation, and a deployable top-level Compose configuration.

## 18. Testing and Acceptance

Domain behavior is developed test-first. Pure unit tests cover slug normalization, dependency closure, invalid combinations, port allocation, configuration precedence, redaction, encryption, operation transitions, and rollback planning. API tests use a temporary SQLite database and a fake Provisioner. Provisioner contract tests use a fake Docker client and temporary project root.

Docker integration tests run on the available Apple Silicon Docker Desktop and in Linux CI. They verify the two-container Manager deployment, private Provisioner network, lack of a published Provisioner port, generated Compose validity, isolated project names/networks/volumes/secrets, lifecycle idempotency, failure cleanup, and SSE reconnect behavior.

Milestone acceptance uses a real pinned official Supabase Runtime. Final V1 acceptance creates three concurrent projects and validates project isolation, Lightweight installation, Google/GitHub/Apple Auth changes, local and R2-compatible Storage, Functions execution and secrets, Realtime WebSocket availability, Logs stack behavior, backups, restore, update warnings, and the rule that a change to one project never restarts or changes another.

## 19. Security Invariants

- The browser and Manager container never access the Docker Socket.
- The Provisioner is not exposed on a host port and has no general shell endpoint.
- Project identifiers cannot escape the configured project root.
- Every project has unique database data, Auth data, JWT/API credentials, Storage, Function secrets, network, and Compose identity.
- Secret plaintext is never stored in SQLite, operation events, logs, backups without encryption, or browser storage.
- Official Supabase initialization SQL and role/schema creation logic are never copied or modified by the Manager.
- All production image tags and template versions are pinned.
- Optional services remain disabled in the default Lightweight preset.
- Destructive deletion defaults to retaining data and requires exact project-name confirmation for data removal.

## 20. Definition of Ready for Implementation

Implementation can begin when this design is accepted. The implementation plan must map every PRD Definition-of-Done item to a concrete test-first task, preserve milestone-level deployability, and identify the exact pinned official template version during Milestone 1 dependency setup.
