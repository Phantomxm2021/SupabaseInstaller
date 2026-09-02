# Task 3 report: safe Storage location transitions

- RED: `go test ./apps/provisioner/internal/runtime -run TestReconcileRejectsNonEmptyStorageLocationChangeBeforePublish -count=1` failed because the preflight guard was not implemented.
- GREEN: `go test ./apps/provisioner/internal/compose ./apps/provisioner/internal/runtime -count=1` passed.
- `StorageObjectCount` executes the fixed argument-vector query `docker compose exec -T db psql -v ON_ERROR_STOP=1 -U supabase_admin -d postgres -At -c "SELECT count(*) FROM storage.objects;"` against the target Compose project.
- Parsing trims outer whitespace, requires exactly one token, parses a base-10 `int64`, and rejects empty, multi-row, non-numeric, negative, overflow, or executor-error results.
- Reconcile compares Backend, Bucket, Region, Endpoint, AccountID, and LocalPath only when Storage was previously enabled and checks the previous runtime before staging. Nonzero or unavailable counts fail closed; zero permits transition.
- Tests cover exact query argv, malformed output, non-empty/unavailable rejection before recreate/publish, zero transition, unchanged location, and previously-disabled Storage.
- Commit: `fix: guard non-empty storage transitions`
