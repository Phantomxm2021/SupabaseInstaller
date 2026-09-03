# Supported Runtime Versions and Upgrades

## Goal

New servers default to the newest Supabase self-hosted release that this
Manager has tested and bundled. Administrators can choose any other supported
pinned release. Existing servers stay on their recorded release until an
administrator performs a safe, explicit runtime upgrade.

## Terminology and invariants

`Latest supported` is a Manager-owned alias, resolved before persistence to a
concrete release such as `self-hosted/v0.8.0`. It is never the mutable Docker
`latest` tag and no rendered image may use that tag. The initial supported
release is `self-hosted/v0.8.0`, which is also the current upstream stable
self-hosted snapshot at the time this feature is designed.

A supported release consists of an embedded official Compose template,
overlays, a compatibility validator, image-pin verification, release notes,
and a migration classification. Adding a new upstream release therefore means
shipping a new Manager release after the complete template has passed the
existing render and real-runtime test suites. The application must not fetch or
execute an arbitrary upstream Compose file at runtime.

The `general.supabaseVersion` field always stores a concrete supported release.
Legacy rows with `self-hosted/v0.8.0` remain valid without a data migration.
The Manager must reject an unknown release before it changes configuration,
stages files, pulls images, or stops a container.

## Version catalog

Introduce one shared runtime-version catalog in `internal/templates`. It owns:

- `LatestSupported()` and `SupportedVersions()` in newest-first order.
- a release descriptor containing ID, label, embedded template root, status,
  release notes URL/text, and migration class;
- lookup and validation functions used by Manager validation, Provisioner
  rendering, and Web API responses.

The initial catalog contains only `self-hosted/v0.8.0`; the UI must still show
it as `Latest supported — self-hosted/v0.8.0` rather than pretending a newer
release exists. The catalog design makes the version picker useful as soon as a
second verified template is shipped, without accepting user-provided tags.

The embedded-template package becomes version-addressable. Rendering selects
the exact template root from the configuration instead of hard-coding
`self-hosted-v0.8.0`. Release-specific renderer rules live beside the catalog
descriptor rather than growing a chain of version comparisons through the
renderer.

## New-server behavior

`DefaultConfiguration` selects `LatestSupported()`. The create wizard requests
the catalog from Manager, renders a default `Latest supported` choice plus the
other concrete releases, and explains that all choices are pinned release
sets. Submission contains the resolved concrete ID, not the alias.

Server details and configuration pages display both the selected release and,
when applicable, an "Update available" state. They do not silently change a
server's recorded version when Manager itself is upgraded.

## Existing-server upgrade workflow

Changing runtime version is not a normal general-configuration PATCH. Add a
dedicated `UPDATE_VERSION` operation and endpoint carrying a target supported
version plus the server's exact-name confirmation. The request is accepted
only for an installed, non-deleting server with no active operation.

Before staging a candidate runtime, the Manager validates that the target is a
supported forward transition and retrieves its migration classification:

- **Compatible:** configuration is translated by the target release adapter;
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
`self-hosted/v*` releases and opens a candidate update for maintainers. The
candidate must vendor the complete upstream template, define its compatibility
and migration rules, run template/render/integration tests, and ship in a
Manager release before users see it as supported. There is no unattended
production auto-upgrade in this feature.

## API and UI

Add a read-only runtime catalog endpoint returning the current latest concrete
version and descriptors safe for browsers. Extend project details with an
upgrade eligibility summary: current version, newest supported version,
availability, target migration class, and user-facing reason when blocked.

Add a project `Upgrade runtime` dialog. It has a version selector, concise
release/migration summary, backup/rollback statement, and exact server-name
confirmation. The primary action is disabled for a manual-migration target or
until confirmation matches. It queues the dedicated operation and reuses the
existing operation event view for progress and failure feedback.

## Testing and acceptance

- Catalog tests prove newest-first ordering, exact alias resolution, template
  lookup, and rejection of `latest`, `master`, empty, or unknown versions.
- Manager create/configuration tests prove new servers persist the resolved
  latest version and existing `v0.8.0` rows remain readable.
- Renderer tests prove each supported release selects its own templates and
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
externally-created Compose projects, permit individual-service image overrides,
or automate a PostgreSQL major-version migration. Those require separate
ownership, compatibility, and recovery contracts.
