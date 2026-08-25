# Supabase Manager Configuration and shadcn Corrections Design

**Date:** 2026-08-26

**Status:** Approved design amendment

**Source:** `Docs/Supabase Self-hosted Web Installer & Manager PRD.md`, especially sections 11–53

## 1. Purpose

This amendment corrects the current Installer Core in seven areas:

1. Configuration must expose typed settings that generate the official Supabase environment and Compose configuration. A list of service switches alone is not a configuration experience.
2. Manager Settings must be an authenticated React page and must never render the raw session response or CSRF token.
3. The global sidebar must not contain a duplicate New Project action.
4. A successful installation must enter the newly created Supabase project.
5. Supported services and their settings must be changeable after installation through durable, reversible configuration operations.
6. Project deletion must refresh the project list immediately.
7. The React interface must use the shadcn design system and component conventions throughout.

The product continues to follow the earlier security decision: users edit validated, typed fields. They do not receive a raw `.env` or arbitrary Compose editor. The Manager owns translation into `.env`, `.env.functions`, and `docker-compose.override.yml`.

## 2. Configuration Information Architecture

### 2.1 Creation wizard

The New Project wizard contains these steps:

1. **Basic** — name, generated slug, domain, Site URL, pinned Supabase version.
2. **Preset & Services** — Lightweight, Standard, Full, or Custom, followed by all official service switches and dependency explanations.
3. **Authentication** — Auth enablement, Email Auth, Phone Auth, anonymous sign-in, signup/session policy, redirect URLs, OAuth providers, and SMTP.
4. **Storage & Functions** — Storage backend and credentials, S3 protocol exposure, image transformation, Functions runtime and default JWT verification.
5. **Database & Network** — direct database access, Supavisor, pool limits, gateway, and allocated ports.
6. **Review** — enabled and disabled services plus redacted summaries of Auth, Email/SMTP, OAuth, Storage, Functions, database, and network settings.

Lightweight remains a fast path: every screen has safe defaults and a user can continue without opening Advanced sections.

### 2.2 Installed-project configuration

`/projects/:projectId/configuration` contains tabs or sections for:

- General
- Services
- Authentication
- Email & SMTP
- OAuth Providers
- Storage
- Realtime
- Functions
- Database
- Connection Pooler
- Gateway & Network
- Studio & Logs
- API & Secrets

Each section loads one redacted configuration projection. Saving creates an `UPDATE_CONFIG` operation and opens the persistent operation panel. The page shows dirty state, affected services, and whether the change requires restart or recreate before confirmation.

## 3. Typed Configuration Schema

Project configuration is versioned. Non-secret values are stored as typed JSON; secret values are stored individually through the encrypted secret store. The API never returns stored secret plaintext in normal configuration responses.

### 3.1 General

- `domain`
- `siteUrl`
- `supabaseVersion` chosen only from Manager-supported pinned versions
- derived, read-only Project URL and callback base URL

Changing Domain or Site URL updates Auth URLs, generated OAuth callback URLs, gateway/proxy output, and any other official variables that derive from the public URL.

### 3.2 Services and presets

The UI includes every PRD service:

- PostgreSQL — required
- Envoy Gateway — required default; Kong remains an Advanced gateway option
- Auth — default on
- PostgREST — default on
- Studio — default on
- postgres-meta — forced on while Studio is on
- Realtime — default off
- Storage — default off
- imgproxy — default off and requires Storage
- Edge Functions — default off
- Supavisor — default off
- Logs/Logflare — default off
- Vector — managed with Logs
- direct PostgreSQL port — default off

Presets populate the same editable schema rather than selecting a separate renderer. Manual changes convert the selection to Custom.

### 3.3 Authentication

The Auth section provides:

- service enablement
- Email Auth enablement
- allow signup
- confirm email
- secure email change
- double-confirm email changes
- Phone Auth enablement and supported provider-specific settings
- anonymous sign-in
- JWT/session settings supported by the pinned official template
- Site URL, derived from General
- add/remove/validate redirect URL allow-list

The Manager maps typed fields to the pinned template's `GOTRUE_*` variables. Unsupported free-form environment names are rejected. An Advanced panel may expose additional Auth variables only through a versioned allow-list with type, validation, sensitivity, and restart metadata.

### 3.4 Email and SMTP

Email Auth settings and outbound SMTP settings are separate groups. Custom SMTP is off by default. When enabled, the form requires:

- Host
- Port
- Username
- Password
- Sender email
- Sender name

The password is a write-only encrypted field. An unchanged masked value means “retain the current secret”; an explicit replace action writes a new secret; an explicit remove action is allowed only when Custom SMTP is disabled or validation otherwise permits it. Saving SMTP settings recreates Auth and performs the Auth health/settings verification flow.

### 3.5 OAuth providers

The provider registry covers the PRD list: Apple, Azure/Microsoft, Bitbucket, Discord, Facebook, Figma, GitHub, GitLab, Google, Kakao, Keycloak, LinkedIn OIDC, Notion, Slack OIDC, Snapchat, Spotify, Twitch, Twitter/X, WorkOS, and Zoom.

Each registry entry declares:

- enabled state
- client ID field
- encrypted client secret field
- provider-specific fields such as Azure Tenant URL, GitHub Enterprise URL, GitLab self-hosted URL, or Keycloak Realm URL
- exact official `GOTRUE_EXTERNAL_<PROVIDER>_*` mappings for the pinned template
- validation rules

Callback URLs are generated from the project public URL, displayed read-only, and copyable. They are never hand-entered. A successful save recreates only Auth, waits for health, and verifies `/auth/v1/settings`. Failure restores the last known-good Auth configuration and recreates Auth again.

### 3.6 Storage

Storage supports these typed backends:

- Local filesystem with an automatically generated project-scoped path
- AWS S3
- Cloudflare R2 preset
- generic/custom S3-compatible provider

S3 forms include bucket, region, endpoint where applicable, access key ID, encrypted secret access key, and force-path-style. R2 derives the endpoint from Account ID. Storage backend choice and “Enable S3-compatible API” are independent settings. imgproxy is a separate switch and can be enabled only while Storage is enabled.

### 3.7 Realtime

Realtime exposes enablement and an Advanced group for maximum connections, database pool size, and log level. Values have bounded numeric/enumeration validation and map only to variables supported by the pinned template.

### 3.8 Functions

Functions exposes runtime enablement, default JWT verification, and the generated read-only functions directory. Environment variables are edited as name/value rows and rendered into `.env.functions`.

Function variable names must match the supported environment-name syntax and cannot override reserved Supabase/runtime variables. Values are encrypted at rest, masked on read, and excluded from logs and operation events. Code changes restart the Functions runtime; environment changes recreate it. V1 does not include a code editor.

### 3.9 Database and Supavisor

Database exposes the pinned Postgres version, generated database password reveal/copy/rotate flow, direct-port enablement, host port allocation, maximum connections, shared buffers, and supported extensions. Password rotation is a separate recent-auth sensitive operation.

Supavisor exposes enablement, transaction and session pool ports, pool size, and maximum client connections. The Manager allocator prevents port conflicts across projects.

### 3.10 Gateway, Studio, and Logs

- Gateway selection defaults to Envoy; Kong is Advanced.
- Studio and postgres-meta follow their dependency rule.
- Logs is a single user-facing opt-in that manages Logflare, Vector, and Studio Log Explorer together.
- Studio settings and supported log settings are exposed only when the pinned runtime has an official typed mapping.
- Network configuration exposes the project Domain, Manager-allocated internal gateway port, and HTTPS mode: External Reverse Proxy, Caddy Managed, or Manual Certificate. External mode produces safe upstream instructions; Caddy and certificate changes use typed inputs and never accept arbitrary proxy configuration text.
- Allocated API, Studio, direct database, and pooler ports are conflict-checked. Automatically allocated ports are read-only unless the relevant PRD control explicitly allows a requested host port.

### 3.11 API and secrets

The page displays Project URL and Anon Key. Service Role Key, JWT secret, and database password remain hidden until a dedicated recent-password reveal action. Copy/reveal/rotate actions are audited and never place plaintext in application logs or persistent browser storage.

## 4. API and Persistence

The Manager exposes typed endpoints rather than a generic environment-file endpoint:

```text
GET   /api/projects/:id/configuration
PATCH /api/projects/:id/configuration/general
PATCH /api/projects/:id/configuration/services
PATCH /api/projects/:id/configuration/auth
PATCH /api/projects/:id/configuration/smtp
PATCH /api/projects/:id/configuration/oauth/:provider
PATCH /api/projects/:id/configuration/storage
PATCH /api/projects/:id/configuration/realtime
PATCH /api/projects/:id/configuration/functions
PATCH /api/projects/:id/configuration/database
PATCH /api/projects/:id/configuration/pooler
PATCH /api/projects/:id/configuration/network
```

Creation accepts the same typed aggregate schema. Every successful mutation increments `configurationRevision` and returns an Operation. Requests include the expected revision; stale edits receive a conflict response and do not overwrite newer configuration.

Database migrations extend `project_configs` and `project_services` and reuse `project_secrets`. Stored configuration keeps both desired state and last-known-good revision. Audit records contain changed field names and redacted before/after metadata, never secret values.

## 5. Rendering and Runtime Reconciliation

The Provisioner receives a typed, revisioned configuration snapshot. Rendering has one authoritative mapping layer per pinned official template version. It:

1. calculates service dependency closure;
2. renders `.env` and `.env.functions` with strict escaping and restrictive file modes;
3. renders the Compose override and includes only selected optional services;
4. validates the resulting Compose model before replacing active files;
5. atomically installs the candidate files;
6. recreates only affected services and their required dependents;
7. runs service-specific health checks;
8. marks the revision last-known-good on success.

On failure the Provisioner restores the prior files, recreates the prior affected services, verifies recovery, and reports whether rollback succeeded. Disabling a service removes its container but preserves project volumes and encrypted stored configuration by default. It never runs `down -v` during configuration reconciliation.

The initial creation path uses this same renderer. The current fixed Lightweight renderer must not remain as a second source of truth.

## 6. Navigation and Cache Corrections

- The global shadcn Sidebar contains Projects as navigation. New Project remains the primary button on the Projects page and is removed from the global sidebar.
- Manager Settings links to `/settings`, an authenticated React route for administrator account, security, control-plane status, and safe system information. It never navigates to `/api/session` and never renders a CSRF token.
- Creation retains both `projectId` and `operationId`. When the create operation reaches `SUCCEEDED`, the router replaces the progress route with `/projects/:projectId/overview`.
- Successful deletion cancels/removes stale detail queries, invalidates and refetches the projects query, and then navigates to `/projects`. The deleted project must disappear without a page reload.

## 7. shadcn UI System

The existing Vite app adopts Tailwind CSS and shadcn using the documented Vite setup. The application uses the actual generated shadcn components and composition patterns rather than visually imitating them with one-off CSS.

Primary mappings are:

- `Sidebar` for global and project navigation
- `Card`, `Tabs`, `Separator`, and `ScrollArea` for configuration layout
- `Form`/`Field`, `Input`, `Textarea`, `Select`, `Checkbox`, and `Switch` for typed configuration
- `Collapsible` for Advanced groups
- `AlertDialog` for destructive or runtime-impacting confirmation
- `Progress`, `Skeleton`, `Badge`, and `Table` for operations and status
- `Sonner` for mutation success/failure feedback
- `DropdownMenu` for account actions

The design retains the existing dark neutral and emerald product identity through shadcn theme tokens. Keyboard behavior, focus state, labels, descriptions, field errors, reduced motion, and responsive layouts are acceptance requirements.

## 8. Validation and Error Semantics

Validation occurs in shared domain rules on the Manager and is reflected by UI schemas. The API remains authoritative. Validation includes URL formats, unique ports, bounded numeric values, provider-required fields, email addresses, redirect patterns, environment variable names, service dependencies, and pinned-template support.

An update preview identifies affected services and operation type. Field validation errors remain attached to fields. Runtime failures use the operation panel and Sonner summary, with the rollback result stated explicitly. No error response contains rendered environment text or secret material.

## 9. Test and Acceptance Matrix

Implementation is test-driven and must cover:

- configuration schema defaults, serialization, redaction, revision conflicts, and migrations;
- every service dependency and preset override;
- Auth/Email/SMTP mappings and masked-secret retain/replace/remove semantics;
- every OAuth registry entry, provider-specific fields, callback generation, Auth-only recreation, `/auth/v1/settings` verification, and rollback;
- Storage local/S3/R2 mapping, independent S3 protocol, and imgproxy dependency;
- Functions reserved names, encryption, `.env.functions` generation, and recreate behavior;
- database and pooler port conflict checks;
- official Compose config validation for representative Lightweight, Standard, Full, and Custom projects;
- update failure rollback without volume deletion;
- Manager Settings routing with no CSRF rendering;
- create-success navigation and delete-success list refresh;
- component-level accessibility for forms/dialogs/navigation and production web build;
- Go unit/API tests, race tests, vet, frontend unit tests, and real Docker acceptance on Apple Silicon.

Acceptance requires demonstrating through the browser that an administrator can create a project with custom Auth and SMTP, edit those settings after installation, observe only Auth being recreated, and see the previous working configuration restored after an intentionally invalid update.

## 10. Delivery Ownership

Per the approved collaboration split, Luna High owns implementation. Sol owns specification audit, code review, security review, test verification, and narrowly scoped corrections found through that audit. Implementation may be delivered in internally reviewable slices, but the user-facing result is one coherent configuration and shadcn correction release.
