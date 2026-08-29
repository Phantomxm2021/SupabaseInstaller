# Troubleshooting

## Baseline observed on Apple Silicon

The Installer Core acceptance run completed on Docker Desktop for Apple Silicon (`aarch64`) with 10 virtual CPUs and 7.75 GiB available memory. A cold Lightweight installation took about two minutes once the control-plane images were built; network speed and registry cache dominate this time.

The healthy six-service Lightweight stack used about 504 MiB at idle in this run:

| Service | Observed memory |
| --- | ---: |
| PostgreSQL | 63 MiB |
| Envoy API gateway | 30 MiB |
| Auth | 10 MiB |
| PostgREST | 131 MiB |
| postgres-meta | 81 MiB |
| Studio | 190 MiB |

The pulled image sizes were approximately 1.75 GB for PostgreSQL, 1.65 GB for Studio, 658 MB for PostgREST, 526 MB for postgres-meta, 261 MB for Envoy, and 78 MB for Auth. Keep at least 8 GB of free disk space for the first server and build cache. Docker Desktop with 8 GB RAM is sufficient for Lightweight; 12 GB is recommended when other containers run concurrently.

## Manager is healthy but the URL is unreachable

Check that Manager is attached to the `frontend` network and has a published port:

```sh
docker compose -f deploy/docker-compose.yml --env-file deploy/.env ps
docker port supabase-manager-manager-1
```

Only Manager should publish a host port. Provisioner must remain private.

## Installation fails before creating containers

Validate the generated server Compose file without printing its resolved environment:

```sh
docker compose --file "$PROJECT_ROOT/PROJECT_SLUG/docker-compose.yml" \
  --project-directory "$PROJECT_ROOT/PROJECT_SLUG" \
  --project-name "supabase-manager-PROJECT_SLUG" config --quiet
```

`PROJECT_ROOT` must be an absolute host path mounted into Provisioner at the identical absolute path. This is required because Docker Desktop resolves server bind mounts on the host daemon.

### `check host port ...: provisioner returned 404 Not Found`

Manager checks the Docker host's published ports through the private
Provisioner endpoint before allocating a server port. A `404` on
`/internal/v1/host/ports/<port>` means the Manager and Provisioner images were
built from different revisions (typically only `manager` was rebuilt). It does
not mean that the configured port was changed or that port `8000` is invalid.

After updating the repository, rebuild and recreate both control-plane
services together; keep `PORT_RANGE_START` and `PORT_RANGE_END` unchanged:

```sh
docker compose -f deploy/docker-compose.yml --env-file deploy/.env build manager provisioner
docker compose -f deploy/docker-compose.yml --env-file deploy/.env up -d --force-recreate --wait manager provisioner
```

The private endpoint can be checked from the Manager container (Provisioner is
intentionally not published on a host port):

```sh
docker compose -f deploy/docker-compose.yml --env-file deploy/.env exec -T manager \
  sh -lc 'wget -qO- --header="Authorization: Bearer ${PROVISIONER_TOKEN}" \
  http://provisioner:9090/internal/v1/host/ports/8000'
```

Expected output is JSON containing `"port":8000` and an `"available"` boolean.

## Safe log collection

Control-plane logs avoid rendering request bodies and generated credentials:

```sh
docker compose -f deploy/docker-compose.yml --env-file deploy/.env logs --no-color --tail=200 manager provisioner
```

Runtime logs can be collected for a named service:

```sh
docker compose --file "$PROJECT_ROOT/PROJECT_SLUG/docker-compose.yml" \
  --project-directory "$PROJECT_ROOT/PROJECT_SLUG" \
  --project-name "supabase-manager-PROJECT_SLUG" logs --no-color --tail=200 auth
```

Review runtime logs before sharing them. Supabase services can include URLs or request metadata even though the Manager/Provisioner redactor removes known generated secrets from managed log paths.

## Safe cleanup

Use the web UI's default “Delete runtime only” action to remove containers and networks while retaining configuration, volumes, encrypted Manager secrets, and the server directory. Data deletion requires typing the exact server name.

Stopping the control plane is non-destructive:

```sh
docker compose -f deploy/docker-compose.yml --env-file deploy/.env down
```

Do not add `-v` and do not delete `PROJECT_ROOT` unless permanent data removal is intentional.
