# Manager configuration migration

Manager stores the canonical configuration in SQLite. Provisioner renders the
canonical value into each server’s `.manager-runtime/current` Compose/env
artifact and Docker reads those generated files. The `project_configs` rows,
revision leases, and operation snapshots are historical upgrade data; normal
Dashboard reads and writes do not use them.

## Upgrade procedure

1. Back up `manager.db` and each server’s `volumes/` directory.
2. Stop Manager and Provisioner during the maintenance window.
3. Start the new Manager once. Migration `014_single_source_configuration.sql`
   creates one `project_configuration` row per server from the newest valid
   aggregate.
4. Inspect the migration log for servers with missing or invalid aggregate
   data. Do not invent defaults; repair those servers manually before
   starting reconciliation.
5. Start Provisioner and use Retry from the server page. It reads the
   canonical row and regenerates the runtime. Never delete
   `volumes/db/data` during a configuration migration.

## Verification

```sh
sqlite3 /path/to/manager.db \
  "SELECT COUNT(*) FROM projects p JOIN project_configuration c ON c.project_id=p.id;"

docker compose -f deploy/docker-compose.yml --env-file deploy/.env \
  logs --no-color --tail=200 manager provisioner
```

For a server, validate the generated Compose file from its server root:

```sh
docker compose \
  --project-directory /home/supabase-manager/projects/<slug> \
  --file /home/supabase-manager/projects/<slug>/.manager-runtime/current/docker-compose.yml \
  --env-file /home/supabase-manager/projects/<slug>/.manager-runtime/current/.env \
  config --quiet
```

If Compose or a health check fails, the operation remains `FAILED` with the
exact phase and redacted service output. Fix the reported runtime issue and
retry; do not edit `project.json` or manually change a revision number.
