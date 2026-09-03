# Runtime Image Manifests and Upgrades

## Goal

New servers default to the newest official Supabase Compose image manifest that
this Manager has tested and bundled. Administrators can choose another
supported manifest. Existing servers stay on their recorded manifest until an
administrator performs a safe, explicit runtime upgrade.

## Terminology and invariants

`self-hosted/v0.8.0` identifies an upstream Docker Compose template snapshot.
It is not a Supabase runtime or Docker image version, and it must not be
presented as one in the product. Runtime version selection is a complete image
manifest: the exact tags for Studio, Auth, Postgres, PostgREST, Realtime,
Storage, postgres-meta, Edge Runtime, Supavisor, gateways, and every enabled
dependency.

`Latest official` is a Manager-owned alias, resolved before persistence to an
immutable manifest ID. It uses the image matrix from the official upstream
`docker/docker-compose.yml` and official overlays as imported and tested by the
Manager release. The initial manifest derives from the bundled upstream
template and includes, for example, Studio `2026.08.03-sha-022b374`, Auth
`v2.189.0`, and Postgres `17.6.1.136`. It is not the mutable Docker `latest`
tag.

Docker Hub's individual image tags advance independently of the official
Compose matrix. A separately labeled `Docker Hub latest (experimental)`
manifest may be offered only after its complete service matrix has been
resolved, persisted, compatibility-tested, and marked experimental. It is
never substituted silently for `Latest official`.

A supported manifest consists of an embedded official Compose template and
overlays, its immutable service-image matrix, a compatibility validator,
release notes, and a migration classification. Adding an official upstream
matrix means shipping a new Manager release after the complete template has
passed the existing render and real-runtime test suites. The application must
not fetch or execute an arbitrary upstream Compose file at runtime.

The `general.supabaseVersion` field is migrated to a concrete manifest ID;
legacy values of `self-hosted/v0.8.0` map to the original bundled manifest.
The Manager must reject an unknown manifest before it changes configuration,
stages files, pulls images, or stops a container.

## Version catalog

Introduce one shared runtime-manifest catalog in `internal/templates`. It owns:

- `LatestSupported()` and `SupportedVersions()` in newest-first order.
- a manifest descriptor containing ID, label, channel (`OFFICIAL` or
  `EXPERIMENTAL`), embedded template root, service-image tags and digests,
  release notes URL/text, and migration class;
- lookup and validation functions used by Manager validation, Provisioner
  rendering, and Web API responses.

The initial catalog contains one official image manifest derived from the
bundled upstream Compose template. The UI presents it as `Latest official`,
shows its imported date and representative component tags, and never claims
that the template snapshot name is a runtime image version. The catalog becomes
a picker as soon as a second verified manifest is shipped, without accepting
user-provided tags.

The embedded-template package becomes version-addressable. Rendering selects
the exact template root from the configuration instead of hard-coding
`self-hosted-v0.8.0`. Release-specific renderer rules live beside the catalog
descriptor rather than growing a chain of version comparisons through the
renderer.

## New-server behavior

`DefaultConfiguration` selects `LatestSupported()`. The create wizard requests
the catalog from Manager, renders a default `Latest official` choice plus other
concrete manifests, and explains their channels and component images.
Submission contains the resolved concrete manifest ID, not the alias.

Server details and configuration pages display both the selected release and,
when applicable, an "Update available" state. They do not silently change a
server's recorded version when Manager itself is upgraded.

## Existing-server upgrade workflow

Changing runtime manifest is not a normal general-configuration PATCH. Add a
dedicated `UPDATE_VERSION` operation and endpoint carrying a target supported
manifest plus the server's exact-name confirmation. The request is accepted
only for an installed, non-deleting server with no active operation.

Before staging a candidate runtime, the Manager validates that the target is a
supported forward transition and retrieves its migration classification:

- **Compatible:** configuration is translated by the target manifest adapter;
  a configuration and runtime-generation backup is retained, candidate images
  are pulled, the candidate is reconciled, restarted, and health-checked.
- **Manual migration required:** the endpoint returns a structured blocking
  warning and does not stop or alter the runtime. The UI links to the bundled
  runbook and requires a later explicit migration workflow. PostgreSQL major
  upgrades always use this class until a tested, dedicated migration executor
  exists.
- **Unsupported or downgrade:** rejected. A failed compatible upgrade restores
  the prior runtime generation and pins, but a successful upgrade is not a
  general-purpose database downgrade facility.

The existing runtime-generation mechanism is the configuration-file rollback
boundary. The orchestration records the previous version and generation before
publication. On candidate image pull, reconcile, restart, or health failure it
selects the previous generation, recreates the old pinned services, verifies
health, and marks the operation `ROLLED_BACK` only after that verification.
It must surface `RuntimeOutcomeUnknown` when Docker state cannot be observed,
instead of claiming rollback success.

An upgrade operation reports the phases `PRECHECK`, `BACKUP_RUNTIME`,
`PULL_IMAGES`, `STAGE_TARGET`, `RECREATE_RUNTIME`, `FINAL_HEALTH_CHECK`, and
either `COMPLETE` or `ROLLBACK`. Logs and events contain version IDs and
redacted diagnostics only; they never include rendered environment values or
secret material.

## Manager upgrade and discovery

Manager/Provisioner images remain independently versioned and upgraded via a
released Manager image, not by changing managed Supabase image tags. A Manager
upgrade introduces catalog entries but does not mutate existing projects.

Release discovery is advisory: a scheduled or CI job checks official
official Compose changes and Docker Hub tag changes, then opens a candidate
update for maintainers. The candidate must vendor the complete upstream
template, record the exact image matrix, define compatibility and migration
rules, run template/render/integration tests, and ship in a Manager release
before users see it as supported. There is no unattended production
auto-upgrade in this feature.

## API and UI

Add a read-only runtime catalog endpoint returning the current latest concrete
manifest and descriptors safe for browsers. Extend project details with an
upgrade eligibility summary: current manifest, newest supported manifest,
availability, target migration class, and user-facing reason when blocked.

Add a project `Upgrade runtime` dialog. It has a manifest selector, concise
component/migration summary, backup/rollback statement, and exact server-name
confirmation. The primary action is disabled for a manual-migration target or
until confirmation matches. It queues the dedicated operation and reuses the
existing operation event view for progress and failure feedback.

## Testing and acceptance

- Catalog tests prove newest-first ordering, exact alias resolution, template
  lookup, legacy mapping, and rejection of `latest`, `master`, empty, or
  unknown manifests.
- Manager create/configuration tests prove new servers persist the resolved
  latest manifest and existing template-version rows map safely.
- Renderer tests prove each supported manifest selects its own image matrix and
  every emitted image is pinned.
- Upgrade orchestration tests cover compatible success, unavailable target,
  active-operation conflict, required exact confirmation, image/reconcile/
  health failures with verified rollback, and unknown runtime outcome.
- HTTP and Web tests cover catalog presentation, default selection, update
  availability, confirmation, manual-migration blocking, and operation UI.
- A real Docker acceptance case upgrades a disposable project between two
  compatible bundled releases once such a pair is available. Until then it
  exercises catalog resolution and rejects same-version/no-target upgrades;
  it must not manufacture a fake upstream release.

## Non-goals

This change does not auto-update production projects, import arbitrary
externally-created Compose projects, permit arbitrary individual-service image
overrides, or automate a PostgreSQL major-version migration. Those require
separate ownership, compatibility, and recovery contracts.
