# Supabase Manager Installer

Supabase Manager is a two-container control plane for isolated, pinned
Self-hosted Supabase projects. Manager is the only public HTTP endpoint;
Provisioner is private and is the only process with Docker Socket access.

## Requirements

Use Docker Engine 27+ and Docker Compose v2. Allocate at least 8 GB to Docker
Desktop for a Lightweight project (12 GB is recommended for concurrent work).
On Docker Desktop, `PROJECT_ROOT` must be an absolute shared host path.

## Start

```sh
mkdir -p /Users/Shared/supabase-manager/projects
umask 077
MASTER_ENCRYPTION_KEY="$(openssl rand -base64 32)"
PROVISIONER_TOKEN="$(openssl rand -hex 32)"
install -m 600 /dev/null deploy/.env
sed -e "s#^MASTER_ENCRYPTION_KEY=.*#MASTER_ENCRYPTION_KEY=$MASTER_ENCRYPTION_KEY#" -e "s#^PROVISIONER_TOKEN=.*#PROVISIONER_TOKEN=$PROVISIONER_TOKEN#" deploy/.env.example > deploy/.env.tmp
chmod 600 deploy/.env.tmp
mv deploy/.env.tmp deploy/.env
unset MASTER_ENCRYPTION_KEY PROVISIONER_TOKEN
docker compose -f deploy/docker-compose.yml --env-file deploy/.env up -d --build --wait
```

Open `PUBLIC_ORIGIN`. On the first visit, choose an administrator username and
password in the setup page; the password must contain at least 12 characters.
Save the recovery codes shown by setup because they are displayed only once.
`MASTER_ENCRYPTION_KEY` and `PROVISIONER_TOKEN` are required; the master key
must decode to exactly 32 bytes and the token must contain at least 32 bytes.
The checked-in placeholders are rejected at startup.

For a disposable end-to-end run, execute `scripts/run-acceptance.sh`. It
creates a fresh control-plane volume and temporary administrator automatically
(or uses `SUPABASE_MANAGER_E2E_USERNAME` and
`SUPABASE_MANAGER_E2E_PASSWORD` when explicitly provided), starts an isolated
Compose project on port 18081, creates a complete Custom project with SMTP and
Functions, exercises OAuth and Functions updates, and removes only that
disposable project on exit. It never prints generated secrets.

## Security boundary

The browser receives redacted configuration and opaque session cookies. Secret
plaintext is decrypted only for a typed private Provisioner request and is not
stored in SQLite, operation events, logs, browser storage, or backups. Raw
`.env` and Compose editing are intentionally unavailable.

## Project operations

Create a project from the Custom wizard or Lightweight preset, then use the
project Configuration workspace for safe changes. Every change is an audited
operation with revision checks, health verification, and rollback where
possible. See [project configuration](docs/operations/project-configuration.md)
for section semantics, restart impact, and recovery behavior.

## Inspect and stop

```sh
docker compose -f deploy/docker-compose.yml --env-file deploy/.env ps
docker compose -f deploy/docker-compose.yml --env-file deploy/.env logs --tail=200 manager provisioner
docker compose -f deploy/docker-compose.yml --env-file deploy/.env down
```

`down` does not delete Manager data or project data. Delete runtime/data from
the UI after confirming the exact project name; do not use `down -v` unless
removing the control-plane database is intentional.

## Reverse proxy

Publish only Manager through the reverse proxy. Set `PUBLIC_ORIGIN` to the
external HTTPS origin and `SECURE_COOKIES=true`; preserve Host and X-Forwarded-
Proto, and do not proxy the private Provisioner network.
