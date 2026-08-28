# One-command Ubuntu deployment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver one idempotent Ubuntu installer which builds and starts the Supabase Manager control plane, installs and restarts the managed Nginx Agent, and verifies the complete deployment.

**Architecture:** `scripts/install-supabase-manager.sh` will be the only public deployment command. It validates its host and inputs, safely creates or updates `deploy/.env`, delegates the Agent binary/systemd work to `scripts/install-nginx-proxy-agent.sh`, and starts the base Compose configuration. The base Compose file will own the managed-proxy socket wiring, so an overlay is not needed.

**Tech Stack:** Bash, Ubuntu apt, Docker Compose v2, systemd, Nginx, OpenSSL.

---

## File map

| File | Responsibility |
| --- | --- |
| `scripts/install-supabase-manager.sh` | New public installer: CLI, prerequisites, atomic environment handling, orchestration, and verification. |
| `scripts/install-nginx-proxy-agent.sh` | Focused privileged Agent installer called by the public installer; replaces and explicitly restarts the binary. |
| `deploy/docker-compose.yml` | Single Compose definition, including managed-proxy variables and socket mount. |
| `deploy/docker-compose.nginx-proxy.yml` | Delete; no longer a supported deployment path. |
| `deploy/.env.example` | Document required configuration, including managed Nginx mode. |
| `scripts/test-install-supabase-manager.sh` | New installer regression/smoke test using command doubles. |
| `README.md`, `docs/operations/install.md`, `docs/operations/project-host-nginx.md` | Document one command; retain Cloudflare DNS/certificate prerequisites. |

### Task 1: Fold managed Nginx into base Compose

**Files:**
- Modify: `deploy/docker-compose.yml`
- Delete: `deploy/docker-compose.nginx-proxy.yml`
- Test: `scripts/test-install-supabase-manager.sh`

- [ ] Write a Compose smoke case with generated secrets and `NGINX_PROXY_MODE=managed`.
- [ ] Add this Provisioner configuration to the base Compose file:

```yaml
NGINX_PROXY_MODE: ${NGINX_PROXY_MODE:-disabled}
NGINX_PROXY_SOCKET: ${NGINX_PROXY_SOCKET:-/run/supabase-manager/nginx-proxy-agent.sock}
NGINX_PROXY_TOKEN: ${NGINX_PROXY_TOKEN:-}
```

and mount `/run/supabase-manager:/run/supabase-manager` on Provisioner.

- [ ] Delete the overlay.
- [ ] Verify:

```sh
docker compose -f deploy/docker-compose.yml --env-file "$TEMP_ENV" config --quiet
docker compose -f deploy/docker-compose.yml --env-file "$TEMP_ENV" config | rg 'NGINX_PROXY_MODE|/run/supabase-manager'
```

Expected: a valid managed configuration, with a single socket mount.

### Task 2: Implement the one-command installer

**Files:**
- Create: `scripts/install-supabase-manager.sh`
- Modify: `scripts/install-nginx-proxy-agent.sh`
- Modify: `deploy/.env.example`
- Test: `scripts/test-install-supabase-manager.sh`

- [ ] Write a failing shell smoke test using a temporary repository and doubles for `apt-get`, `docker`, `systemctl`, `nginx`, and the focused Agent installer. Assert it creates mode-0600 `deploy/.env`, generates non-placeholder secrets, invokes Agent installation, invokes `docker compose ... up -d --build --wait`, and does not call Compose when certificates are missing.
- [ ] Implement flags exactly as follows:

```text
--non-interactive
--public-origin URL
--certificate-file PATH
--certificate-key-file PATH
--project-root ABSOLUTE_PATH
--force
--help
```

- [ ] Require root, Ubuntu, systemd, an absolute project root, a valid `http://` or `https://` public origin, and existing certificate/key paths. Interactive mode prompts only for origin and certificate paths; non-interactive mode requires them.
- [ ] Install only absent prerequisites: Docker Engine/Compose v2, Nginx, and OpenSSL. Never stop, remove, or downgrade an existing Docker daemon.
- [ ] Create `deploy/.env` atomically with `umask 077`; preserve valid existing secrets and generate only missing/placeholder values with:

```sh
MASTER_ENCRYPTION_KEY="$(openssl rand -base64 32)"
PROVISIONER_TOKEN="$(openssl rand -hex 32)"
NGINX_PROXY_TOKEN="$(openssl rand -hex 32)"
```

Set `PUBLIC_ORIGIN`, `SECURE_COOKIES=true` for HTTPS origins, `PROJECT_ROOT`, `NGINX_PROXY_MODE=managed`, and the fixed socket path. Only `--force` can replace an explicit conflicting non-secret setting.

- [ ] Call the existing focused installer with certificate variables, then run:

```sh
docker compose -f "$REPOSITORY_ROOT/deploy/docker-compose.yml" \
  --env-file "$ENV_FILE" up -d --build --wait
```

- [ ] Verify each of these after startup:

```sh
systemctl is-active --quiet supabase-manager-nginx-proxy-agent.service
test -S /run/supabase-manager/nginx-proxy-agent.sock
nginx -t
docker compose -f "$REPOSITORY_ROOT/deploy/docker-compose.yml" --env-file "$ENV_FILE" ps --status running
```

On any late failure, retain running containers, volumes, sites, and project data; print the exact failed command and never run `down -v`.

- [ ] Run:

```sh
bash -n scripts/install-supabase-manager.sh scripts/install-nginx-proxy-agent.sh
scripts/test-install-supabase-manager.sh
```

Expected: pass with no real package, Docker, systemd, or Nginx mutation.

### Task 3: Replace split deployment documentation

**Files:**
- Modify: `README.md`
- Modify: `docs/operations/install.md`
- Modify: `docs/operations/project-host-nginx.md`

- [ ] Replace manual secret creation, secondary Agent execution, and Compose overlay commands with:

```sh
sudo ./scripts/install-supabase-manager.sh \
  --public-origin https://manager.example.com \
  --certificate-file /etc/nginx/ssl/cloudflare-origin.pem \
  --certificate-key-file /etc/nginx/ssl/cloudflare-origin.key
```

- [ ] Explain that Cloudflare wildcard DNS and an Origin Certificate are external prerequisites, while rerunning the same installer upgrades Manager, Provisioner, and Agent, and explicitly restarts Agent without a manual command.
- [ ] Confirm no obsolete deployment path remains:

```sh
rg -n 'docker-compose\.nginx-proxy|install-nginx-proxy-agent\.sh deploy/\.env' README.md docs
```

Expected: no supported setup instruction references the removed overlay or a separate Agent launch.

### Task 4: Audit and release checkpoint

**Files:**
- Verify only.

- [ ] Run:

```sh
git diff --check
go test ./...
docker compose -f deploy/docker-compose.yml --env-file "$TEMP_ENV" config --quiet
docker build --file deploy/Dockerfile.nginx-proxy-agent --target export --output type=tar,dest=/dev/null .
```

Expected: all exit successfully. Commit each coherent task and a final verification correction only if one is needed.

## Self-review

- The plan covers the user-facing one-command interface, prerequisite installation, secret generation/preservation, explicit Agent restart, managed socket wiring, documentation, and verification.
- The script owns orchestration; the Agent installer remains the sole owner of its host binary/systemd artifacts.
- No task deletes Docker volumes, existing Nginx sites, or Supabase project data.
