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

The pulled image sizes were approximately 1.75 GB for PostgreSQL, 1.65 GB for Studio, 658 MB for PostgREST, 526 MB for postgres-meta, 261 MB for Envoy, and 78 MB for Auth. Keep at least 8 GB of free disk space for the first project and build cache. Docker Desktop with 8 GB RAM is sufficient for Lightweight; 12 GB is recommended when other containers run concurrently.

## Manager is healthy but the URL is unreachable

Check that Manager is attached to the `frontend` network and has a published port:

```sh
docker compose -f deploy/docker-compose.yml --env-file deploy/.env ps
docker port supabase-manager-manager-1
```

Only Manager should publish a host port. Provisioner must remain private.

## Installation fails before creating containers

Validate the generated project Compose file without printing its resolved environment:

```sh
docker compose --file "$PROJECT_ROOT/PROJECT_SLUG/docker-compose.yml" \
  --project-directory "$PROJECT_ROOT/PROJECT_SLUG" \
  --project-name "supabase-manager-PROJECT_SLUG" config --quiet
```

`PROJECT_ROOT` must be an absolute host path mounted into Provisioner at the identical absolute path. This is required because Docker Desktop resolves project bind mounts on the host daemon.

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

Use the web UI's default “Delete runtime only” action to remove containers and networks while retaining configuration, volumes, encrypted Manager secrets, and the project directory. Data deletion requires typing the exact project name.

Stopping the control plane is non-destructive:

```sh
docker compose -f deploy/docker-compose.yml --env-file deploy/.env down
```

Do not add `-v` and do not delete `PROJECT_ROOT` unless permanent data removal is intentional.
