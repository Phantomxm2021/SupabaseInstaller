# Supabase Manager Installer

Supabase Manager is a two-container control plane for isolated, pinned
Self-hosted Supabase servers. Manager is the only public HTTP endpoint;
Provisioner is private and is the only process with Docker Socket access.

## Ubuntu installation

For an Ubuntu server with managed Nginx hosts for servers, use the single supported
deployment command. It installs missing host prerequisites, creates and
preserves deployment secrets, starts Manager and Provisioner, and installs then
restarts the native Nginx Agent.

```sh
sudo ./scripts/install-supabase-manager.sh \
  --public-origin https://manager.example.com \
  --certificate-file /etc/nginx/ssl/cloudflare-origin.pem \
  --certificate-key-file /etc/nginx/ssl/cloudflare-origin.key
```

Cloudflare DNS records and a valid Cloudflare Origin Certificate are operator
prerequisites. The installer does not issue certificates or create DNS records.
Rerun the same command after pulling an upgrade; it keeps valid secrets and
server data while rebuilding the control plane and explicitly restarting the
Nginx Agent.

## Docker Desktop development

Use Docker Engine 27+ and Docker Compose v2. Allocate at least 8 GB to Docker
Desktop for a Lightweight server (12 GB is recommended for concurrent work).
On Docker Desktop, `PROJECT_ROOT` must be an absolute shared host path.

## Start manually

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
Compose environment on port 18081, creates a complete Custom server with SMTP
and Functions, exercises OAuth and Functions updates, and removes only that
disposable server on exit. It never prints generated secrets.

## Security boundary

The browser receives redacted configuration and opaque session cookies. Secret
plaintext is decrypted only for a typed private Provisioner request and is not
stored in SQLite, operation events, logs, browser storage, or backups. Raw
`.env` and Compose editing are intentionally unavailable.

## Server operations

Create a server from the Custom wizard or Lightweight preset, then use the
Server Configuration workspace for safe changes. Every change is an audited
operation with revision checks, health verification, and rollback where
possible. See [server configuration](docs/operations/project-configuration.md)
for section semantics, restart impact, and recovery behavior.

## Inspect and stop

```sh
docker compose -f deploy/docker-compose.yml --env-file deploy/.env ps
docker compose -f deploy/docker-compose.yml --env-file deploy/.env logs --tail=200 manager provisioner
docker compose -f deploy/docker-compose.yml --env-file deploy/.env down
```

`down` does not delete Manager data or server data. Delete runtime/data from
the UI after confirming the exact server name; do not use `down -v` unless
removing the control-plane database is intentional.

## Reverse proxy

Publish only Manager through the reverse proxy. Set `PUBLIC_ORIGIN` to the
external HTTPS origin and `SECURE_COOKIES=true`; preserve Host and X-Forwarded-
Proto, and do not proxy the private Provisioner network.

For managed per-server Nginx hosts on Ubuntu, use the one-command installer
above rather than a separate Compose override or Agent command.
