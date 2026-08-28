# Install Supabase Manager on Ubuntu

Supabase Manager runs Manager and Provisioner as containers and uses a native
host Agent for per-project Nginx sites. Run the installer from the repository
root. It is the only supported production deployment command.

## Prerequisites

- Ubuntu with systemd and outbound access to Ubuntu packages and container images
- At least 8 GB RAM available to Docker (12 GB recommended for concurrent Supabase runtimes)
- A Cloudflare Origin Certificate and private key already installed on the host
- Cloudflare DNS records that point the Manager and project hostnames to this server

The installer installs Docker Engine/Compose v2, Nginx, and OpenSSL only when
they are absent. It does not create Cloudflare DNS records or certificates.

## Install or upgrade

```sh
sudo ./scripts/install-supabase-manager.sh \
  --public-origin https://manager.example.com \
  --certificate-file /etc/nginx/ssl/cloudflare-origin.pem \
  --certificate-key-file /etc/nginx/ssl/cloudflare-origin.key
```

For non-default project storage, add `--project-root /opt/supabase-manager/projects`.
The script creates `deploy/.env` mode `0600`, preserves valid existing secrets,
sets managed Nginx mode, installs and explicitly restarts the Agent, then runs
the control-plane Compose stack with `--build --wait`.

Interactive execution may be used when flags are omitted:

```sh
sudo ./scripts/install-supabase-manager.sh
```

Rerun the same command after pulling a new release. It updates Manager,
Provisioner, and the Nginx Agent without deleting Manager volumes, generated
project data, or unrelated Nginx sites.

Open the configured `PUBLIC_ORIGIN`. The first visit creates the administrator
and displays one-time recovery codes.

For public project hosts, configure the Manager hostname separately from each
project's `General.Domain`. The installed native Agent creates and removes only
safe per-project `sites-available`/`sites-enabled` files; `PUBLIC_ORIGIN` alone
does not publish project runtimes. See [Project Supabase host behind Nginx and
Cloudflare](project-host-nginx.md).

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
