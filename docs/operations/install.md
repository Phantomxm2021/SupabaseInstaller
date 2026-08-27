# Install Supabase Manager

Supabase Manager runs as two containers but exposes one browser URL. `manager` serves the React application and API. The private `provisioner` owns the Docker socket and has no published port.

## Prerequisites

- Docker Engine 24+ with Docker Compose v2
- At least 8 GB RAM assigned to Docker (12 GB recommended for Supabase plus builds)
- An absolute project directory that Docker can share with containers

## Configure and start

From the repository root:

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
```

The command writes a mode-0600 `deploy/.env` without printing either secret. The
master key must decode to exactly 32 bytes and `PROVISIONER_TOKEN` must contain
at least 32 bytes; startup rejects placeholders and zero/fixed example values.
Set `PROJECT_ROOT` to the absolute directory you created. For a remote HTTPS
URL, set `PUBLIC_ORIGIN` to that exact origin and `SECURE_COOKIES=true`.

```sh
docker compose -f deploy/docker-compose.yml --env-file deploy/.env up -d --build --wait
```

Open `PUBLIC_ORIGIN` (by default `http://localhost:8080`). The first visit creates the administrator and displays one-time recovery codes.

For a public project host behind a host-level Nginx and Cloudflare, configure
the Manager hostname separately from each project's `General.Domain`. The
project host requires Studio/API path routing to its allocated loopback ports;
`PUBLIC_ORIGIN` alone does not publish project runtimes. See
[Project Supabase host behind Nginx and Cloudflare](project-host-nginx.md).

To run the disposable HTTP acceptance against an isolated Compose project,
provide administrator credentials through the environment and run
`SUPABASE_MANAGER_E2E_USERNAME=... SUPABASE_MANAGER_E2E_PASSWORD=... scripts/run-acceptance.sh`.
The script creates Custom + SMTP + Functions, checks OAuth/Functions update
isolation, and cleans up only its own Compose project.

## Operations

```sh
docker compose -f deploy/docker-compose.yml --env-file deploy/.env ps
docker compose -f deploy/docker-compose.yml --env-file deploy/.env logs --tail=200 manager provisioner
docker compose -f deploy/docker-compose.yml --env-file deploy/.env down
```

The last command stops the control plane without deleting Manager data or any Supabase project data. Do not add `-v` unless deleting the Manager database is intentional.
