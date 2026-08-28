# One-command Ubuntu deployment design

## Goal

Provide one supported installer for a clean Ubuntu host:

```sh
sudo ./scripts/install-supabase-manager.sh
```

It installs the Supabase Manager control plane and enables managed project
Nginx sites without requiring operators to create secrets, invoke a secondary
Compose override, or manually install the Nginx agent.

## Scope and prerequisites

The installer runs from an existing repository checkout on Ubuntu with
`systemd` and outbound package/image access. It supports `amd64` and `arm64`.
It installs Docker Engine, Docker Compose v2, Nginx, and OpenSSL when missing.

The operator supplies only values that cannot safely be invented:

- Manager public origin, or accepts the detected HTTP server address.
- Existing Cloudflare Origin Certificate and private-key paths when managed
  HTTPS project hosts are required.

The installer does not create Cloudflare DNS records or certificates. It
validates supplied certificate files before making changes.

## Interface

Interactive mode is the default and asks only for the public origin and
certificate paths. Non-interactive automation uses explicit flags:

```sh
sudo ./scripts/install-supabase-manager.sh \
  --non-interactive \
  --public-origin https://manager.example.com \
  --certificate-file /etc/nginx/ssl/cloudflare-origin.pem \
  --certificate-key-file /etc/nginx/ssl/cloudflare-origin.key
```

Optional `--project-root` defaults to `/opt/supabase-manager/projects`.
`--force` is required before replacing an existing managed configuration; it
never removes existing project data.

## Installation flow

1. Verify Ubuntu, root privileges, repository layout, `systemd`, disk space,
   and that the configured project root is absolute.
2. Install missing Docker Engine/Compose, Nginx, OpenSSL, and required package
   transport tools. Existing supported installations are left intact.
3. Create the project root and secure deployment directories.
4. Create `deploy/.env` with mode `0600` if absent. Preserve valid existing
   values; generate missing `MASTER_ENCRYPTION_KEY`, `PROVISIONER_TOKEN`, and
   `NGINX_PROXY_TOKEN` with OpenSSL.
5. Set `PUBLIC_ORIGIN`, secure-cookie policy, project root, and managed-Nginx
   variables in the environment file.
6. Install or update the native Nginx proxy agent through the existing internal
   installer. The agent exclusively owns its files under
   `/etc/nginx/sites-available` and `/etc/nginx/sites-enabled`.
7. Start the control plane through the base Compose file. Managed Nginx socket
   wiring is part of the base Compose definition; no secondary Compose file is
   required.
8. Verify Manager and Provisioner health, Nginx agent activity, Unix socket
   availability, Compose managed mode, and `nginx -t`.

## Safety and failure behavior

- The installer fails before mutation for unsupported hosts, missing required
  certificates, invalid origins, or unavailable package managers.
- Environment updates are atomic: write a mode-`0600` temporary file, validate
  mandatory values, then rename it into place.
- Existing non-placeholder secrets are preserved. `--force` is required to
  replace conflicting explicit settings.
- Nginx site files unrelated to Manager are never read, altered, or deleted.
- A failure after the control plane starts leaves it running and prints the
  exact failed verification. It does not delete Docker volumes or project data.

## Result

After success, operators access the Manager at the printed public origin.
New project reconciliations automatically create, update, and remove their
typed Nginx virtual-host files through the authenticated host agent.

## Tests

- Shell smoke tests run against command and filesystem doubles, asserting
  secret generation, preservation, atomic environment writes, and the exact
  Compose invocation.
- Compose configuration tests assert the base file provides the managed proxy
  socket and variables without an override.
- Existing Go tests continue to cover typed Nginx route rendering, agent
  authentication, transactional site writes, and Provisioner reconciliation.
