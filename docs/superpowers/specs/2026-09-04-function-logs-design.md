# Per-Function Edge Function Logs Design

## Summary

Add a Manager-owned log viewer for each managed Edge Function. The entry point
is a `View logs` item in that function row's `Actions` menu. It opens a
dedicated page that shows only events attributed to the selected function.

The feature must not infer function ownership from Docker log text. Supabase's
self-hosted Vector configuration treats Edge Runtime container output as
unstructured stderr, so substring filtering cannot provide the required
isolation. Instead, an Edge Runtime event worker sends structured runtime
events to a project-local collector. The collector persists a bounded,
redacted log history that the Provisioner and Manager expose to the web UI.

This feature is independent of the optional Logflare and Vector services.

## Goals

- Make recent logs reachable from each function's existing `Actions` menu.
- Attribute every displayed event to exactly one managed function.
- Include custom console output, uncaught exceptions, and worker lifecycle
  events when Edge Runtime emits them.
- Keep log ingestion failures out of the function request path.
- Prevent known project and function secrets from reaching stored records or
  API responses.
- Bound disk usage and retention without operator maintenance.

## Non-Goals

- A project-wide log explorer for every Supabase service.
- SQL analytics, charts, alerting, exports, or cross-project search.
- Live streaming over WebSocket or Server-Sent Events in the first release.
- Replacing Studio's optional Log Explorer or Logflare.
- Reconstructing function attribution from legacy unstructured Docker logs.

## User Experience

Each row on the existing Edge Functions page adds `View logs` to its `Actions`
menu. Selecting it navigates to:

```text
/projects/:projectId/functions/:functionName/logs
```

The dedicated page contains:

- a back link to the project's Functions list;
- the function name and collection health;
- a level filter;
- a message search field;
- a pause/resume control for automatic refresh;
- a manual refresh action;
- a newest-first log table with timestamp, level, event type, and message;
- pagination for loading older records.

The first request returns at most 200 records. While automatic refresh is
enabled, the client polls every five seconds for records newer than the newest
record it already has. Pausing stops polling but retains the current results.
Changing a filter starts a fresh query. The page preserves no filter state
outside the current browser session.

The UI distinguishes these states:

- the function has not produced any retained logs;
- the collector is offline or incompatible;
- collection is healthy but ingestion has dropped events;
- the query failed;
- the Functions service is disabled.

No new Logs item is added to either the project sidebar or the Functions
secondary navigation.

## Architecture

```text
Edge Runtime event worker
        |
        | structured, project-local HTTP batches
        v
Function log collector sidecar
        |
        | validated and redacted records
        v
Project-local log database
        |
        | bounded read-only queries
        v
Provisioner -> Manager -> Web UI
```

### Edge Runtime Event Adapter

The rendered Functions service starts Edge Runtime with a Manager-supplied
event-worker module compatible with the pinned Edge Runtime image. The adapter
normalizes supported runtime events into a versioned internal envelope. It
forwards console events, uncaught exceptions, and boot/shutdown events in
bounded batches to the collector over the project's private Compose network.
Event identity is the SHA-256 of a deterministic compact JSON serialization of
the callback object: envelope, event, and metadata fields have fixed order and
OpenTelemetry attribute keys are sorted. Whitespace and source JSON formatting
therefore do not affect deduplication, and the invocation `execution_id` is not
used as the event identifier.
Numeric timing fields are restricted to JavaScript safe integers, and
OpenTelemetry attribute names use the ASCII `[A-Za-z0-9][A-Za-z0-9._/-]*`
domain with string values. Both `Warn` and the runtime's `Warning` spelling are
accepted and normalized while retaining their original spelling in EventID.

The adapter obtains function ownership from structured runtime metadata. An
event without an exact function identifier is rejected from the per-function
store rather than assigned heuristically. Runtime-wide adapter errors remain
available through the collector health status but never appear under an
arbitrary function.

The official Edge Runtime is beta software, so the adapter has an explicit
compatibility contract tied to the image version in the bundled template. A
template upgrade must pass the event-adapter integration fixture before the
pinned image is changed. If initialization detects an unsupported event shape,
collection becomes `incompatible` without preventing Edge Runtime startup.

### Function Log Collector

Every project with Functions enabled runs one lightweight collector sidecar on
the private project network. It has no published host port. Only the event
adapter uses its ingestion endpoint; user-facing reads do not traverse the
project network.

The collector:

1. validates the envelope version and required identifiers;
2. verifies that the function belongs to the project;
3. normalizes timestamps, levels, and event types;
4. applies secret redaction and message truncation;
5. writes accepted records transactionally;
6. reports health, rejected records, and dropped batches through bounded
   counters without echoing rejected payloads.

Collection is best effort. A timeout, full queue, unavailable database, or
collector restart may drop logs, but it must not delay or fail a function
invocation. The adapter queues at most 1,000 events, sends batches of at most
100 events every 250 milliseconds, and abandons a collector request after 500
milliseconds. When the queue is full it drops the oldest queued event and
increments a counter; it does not create an unbounded retry backlog.

### Storage

Function logs use a project-local database separate from the Manager control
database. Records are indexed by function identifier and descending timestamp,
with a stable record ID as a pagination tiebreaker. Stored fields are:

- record ID;
- event timestamp and ingestion timestamp;
- project ID;
- function identifier and display name;
- execution ID when supplied by Edge Runtime;
- normalized level;
- normalized event type;
- redacted message;
- truncation marker.

Records are retained for seven days. Each project's database is also limited
to 512 MiB. Scheduled maintenance removes expired records first and then the
oldest remaining records until the size is under the cap. Maintenance runs at
startup and hourly, and deletes at most 10,000 records per transaction so it
cannot monopolize the database. Deleting a project removes its log database.
Deleting a function does not immediately erase its records, but removes their
normal UI/API entry point; the records remain inaccessible until normal expiry.

## API Contracts

The Provisioner owns log-file access and exposes internal, authenticated read
operations to Manager. Manager exposes an authenticated project endpoint with
this conceptual shape:

```text
GET /api/projects/:projectId/functions/:functionName/logs
    ?limit=200
    &before=<opaque-cursor>
    &after=<opaque-cursor>
    &level=error
    &search=<bounded-text>
```

`before` and `after` are mutually exclusive opaque cursors. `limit` is capped
at 200. Search is a case-insensitive message substring query capped at 256
UTF-8 bytes. The response includes records, older/newer cursors, collection
health, and the server timestamp used by the client to schedule its next poll.

All existing Manager authentication and project authorization rules apply.
The API returns not found for a function outside the project's managed
function set and never allows a caller-provided filesystem path, container
name, or database path.

## Redaction and Data Handling

Redaction occurs before persistence. The collector builds its redactor from
the project's known secret values and the existing diagnostic redaction rules.
It covers at least Authorization values, API keys, JWT material, database
passwords, service-role credentials, OAuth and SMTP secrets, storage
credentials, and Functions environment secrets.

Control characters are normalized. A stored message is capped at 10 KiB and
marked when truncated. API responses apply the same output length bound as a
defense in depth. Raw rejected events and unredacted payloads are never written
to application logs, operation events, or health diagnostics.

Because user function code is trusted project code rather than an isolation
boundary, the private collector endpoint prevents accidental external access
but is not presented as protection from a malicious function deliberately
forging its own logs. The collector still rejects attempts to attribute an
event to another project or an unknown function.

## Failure Behavior

- Edge Runtime starts even when its event adapter or collector is unavailable.
- Collector incompatibility is reported explicitly and does not fall back to
  Docker-text matching.
- A failed page query keeps currently rendered records and exposes a retry
  action.
- Dropped-event counters indicate incomplete history without revealing event
  bodies.
- Disabling Functions stops the adapter and collector; retained data remains
  queryable until normal expiry, while the page states that collection is
  stopped.
- Enabling optional Logflare/Vector does not duplicate records in this store,
  because only the Edge Runtime event adapter feeds it.

## Testing

### Unit Tests

- Parse every supported Edge Runtime event fixture.
- Reject missing, malformed, runtime-wide, and unknown-function attribution.
- Normalize levels and event types.
- Redact every configured secret category before persistence.
- Normalize control characters and enforce the 10 KiB limit.
- Enforce seven-day expiry and the 512 MiB capacity policy.
- Encode and validate pagination cursors and query bounds.

### API Tests

- Enforce authentication and project/function isolation.
- Cover newest-first pagination, incremental `after` queries, level filtering,
  search, and invalid parameter combinations.
- Return stable empty, offline, incompatible, dropped-event, and disabled
  states without leaking internal paths or raw errors.

### Web Tests

- Expose `View logs` in each function's `Actions` menu.
- Navigate to the correct project/function route.
- Render records, filters, empty/error/health states, and older-page loading.
- Poll every five seconds while active, stop while paused or unmounted, and
  merge incremental records without duplicates.

### Integration and Template Tests

- Run two functions concurrently and prove that their console, exception, and
  lifecycle events never cross result sets.
- Verify that collection failure does not change function responses or
  latency beyond the adapter timeout budget.
- Exercise Functions with optional Logs/Logflare both disabled and enabled.
- Gate every pinned Edge Runtime template update on the adapter compatibility
  fixture.
- Verify project deletion removes the project log database.

## Rollout

The feature ships with the bundled template and is applied during project
creation or configuration reconciliation when Functions is enabled. Existing
projects receive the collector service and event-worker configuration on their
next successful reconciliation. Until that happens, the log page reports that
collection has not been installed; it does not show unfiltered legacy Docker
logs.
