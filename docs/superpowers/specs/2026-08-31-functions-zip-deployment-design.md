# Manager Edge Functions ZIP deployment

## Goal

Allow a Manager administrator to deploy one self-hosted Supabase Edge Function
from `<function-name>.zip`, see its deployment state, roll back once, and
delete it. The feature preserves the existing Manager-to-Provisioner trust
boundary: Manager coordinates authenticated operations; Provisioner alone may
write server project data or run Docker Compose.

V1 deliberately excludes a source editor, arbitrary command runner, CI
integration, and automated validation of business-function responses.

## User experience

The project navigation gains **Functions** at
`/projects/:projectId/functions`. The page uses only existing UI primitives:
`Card`, `Table`, `Badge`, `Button`, `DropdownMenu`, `Dialog`,
`AlertDialog`, `Progress`, and `Sonner`.

The page has a deployment card with an upload button and a function table. A
row includes the function name, current version hash, last deployment time,
runtime availability, and an actions `DropdownMenu`. The menu contains:

- Deploy new version
- Roll back to the preceding successful version, when available
- Delete function

Delete is destructive and opens the existing `AlertDialog`; the administrator
must type the function name to continue. The page and actions are available to
the same authenticated Manager administrators who manage project settings. No
additional role system is introduced in V1.

When Edge Functions is disabled for the project, the page shows the list but
disables deploy, rollback, and delete. It links the operator to the existing
Functions service configuration.

## Archive contract

An upload represents exactly one function. The endpoint path supplies its
authoritative function name; the UI derives the name from the archive filename
and requires it to match. The archive must be named `<function-name>.zip` and
its root must contain `index.ts` directly:

```text
stripe-webhook.zip
├── index.ts
├── deno.json            # optional
└── lib/                 # optional
```

`main` is reserved for Supabase's dispatcher and cannot be deployed, rolled
back, or deleted through Manager. Existing non-Manager directories are not
changed until an administrator explicitly deploys the matching function name.

The Provisioner treats every archive as untrusted. It accepts only ZIP data
and rejects:

- compressed uploads over 20 MiB;
- archives whose extracted total exceeds 100 MiB, whose file count exceeds
  500, or whose individual file size exceeds the configured safe limit;
- absolute paths, `..` traversal, backslashes that normalize outside the
  release root, duplicate normalized paths, symbolic links, special files, and
  non-regular files;
- archives without a root-level `index.ts`, or with an enclosing top-level
  directory.

Archive MIME type is advisory only. The Provisioner validates the ZIP reader
and counts bytes while extracting. Function source is treated as sensitive
operational input: it is never placed in the Manager database, operation
events, regular application logs, or browser responses.

## Storage and versioning

For a project, Provisioner owns these paths below the existing Functions
volume:

```text
volumes/functions/
├── main/                                      # upstream dispatcher; untouched
├── <function-name>                            # Manager-controlled current pointer
└── .manager/
    └── <function-name>/
        ├── releases/<sha256>/                 # extracted source + safe manifest
        └── staging/<operation-id>/             # deleted on terminal outcome
```

The root function entry is a Manager-controlled pointer to a release in the
same filesystem, so changing the current version is atomic. The exact pointer
representation must be supported by the pinned Edge Runtime (a relative
directory symlink is the intended implementation) and is covered by an
integration test before the release is enabled. The Dispatcher passes the
function directory to Edge Runtime, which resolves that directory at request
time.

Each release has a Manager-generated manifest containing only function name,
SHA-256, deployment operation ID, and timestamp. It contains no archive body,
secrets, or user-provided metadata. At most two successful releases are
retained: current and immediately previous. A successful third deployment
removes the older release only after the new version has been activated and
the Functions runtime restarted.

The Manager accepts uploads into a private, mode-restricted durable spool
outside every project directory. The spool is named by operation ID and keeps
the operation resumable across a Manager restart. It is removed after a
terminal deployment outcome and must not be served by HTTP or mounted into any
Supabase service.

## APIs

Public APIs are protected by the existing Manager session and CSRF middleware:

| Method | Path | Contract |
| --- | --- | --- |
| `GET` | `/api/projects/:id/functions` | Return safe function/version metadata and runtime availability. |
| `POST` | `/api/projects/:id/functions/:name/deploy` | Accept one multipart ZIP and return `202` with `operationId`. |
| `POST` | `/api/projects/:id/functions/:name/rollback` | Queue a rollback to the prior release and return `202`. |
| `DELETE` | `/api/projects/:id/functions/:name` | Require exact-name confirmation, queue deletion, and return `202`. |

The Manager adds corresponding narrow authenticated Provisioner endpoints.
They receive only project identity, validated function name, operation ID, and
a package stream or operation metadata. The Provisioner validates all values
again; no generic filesystem path, Docker command, compose service name, or
shell argument is accepted from either request.

Operations add `DEPLOY_FUNCTION`, `ROLLBACK_FUNCTION`, and
`DELETE_FUNCTION` types. Operation events contain named lifecycle steps and
safe error codes, never archive contents or paths. These actions use the same
per-project admission/fencing mechanism as configuration changes so a
Functions deploy cannot race a Functions configuration recreate.

## Deployment and recovery

The deployment state machine is:

```text
QUEUED
→ VALIDATING_ARCHIVE
→ STAGING_RELEASE
→ ACTIVATING_RELEASE
→ RESTARTING_FUNCTIONS
→ SUCCEEDED
```

Manager records the queued operation, stores the incoming package in its
private spool, and hands work to Provisioner. Provisioner validates and stages
the archive under the project Functions volume; Manager does not extract
archives or invoke Docker.

After staging, Provisioner activates the pointer and invokes the existing
Compose Runner only as `restart functions`. A successful container restart
means the source is published and the Functions runtime restarted; it does not
mean an unknown user function's own HTTP logic has been exercised. The UI must
make that distinction clear.

If validation or staging fails, Provisioner removes staging data and retains
the live version. If restart fails after activation, the operation enters
`ROLLING_BACK`, restores the previous pointer, and retries only the Functions
restart. A completed compensation is recorded as `ROLLED_BACK`; an
unsuccessful compensation remains `FAILED` with a redacted, bounded
diagnostic.

On Manager or Provisioner restart, operation ID, private spool state, and
release manifests are used to resume an incomplete action or complete safe
cleanup. Retried work is idempotent: an already staged release with the same
operation ID and SHA-256 is reused, and a completed terminal operation is not
replayed.

Rollback activates the previous release and restarts Functions. Delete confirms
the exact name, removes the Manager-controlled pointer and its retained
releases, then restarts Functions. Both honor the same project operation lock.

## Validation

Unit and API tests cover:

- Manager session/CSRF enforcement, multipart size handling, filename/name
  validation, operation admission, and safe API responses;
- Provisioner ZIP traversal, symbolic-link, duplicate-path, size-limit, root
  `index.ts`, staging cleanup, version retention, and pointer activation;
- only the `functions` Compose service is restarted;
- restart failure compensation, rollback failure reporting, and idempotent
  recovery at every state-machine boundary;
- the Functions UI's existing-component composition, disabled service state,
  dropdown options, delete confirmation, upload feedback, and operation view.

Integration coverage deploys a sample ZIP and invokes
`/functions/v1/<function-name>`, deploys a deliberately bad release to verify
the previous version survives the failed deployment, then verifies a manual
rollback restores the prior sample release.
