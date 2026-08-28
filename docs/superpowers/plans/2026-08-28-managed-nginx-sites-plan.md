# Managed Nginx Sites Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Automatically publish, update, validate, reload, and remove per-project Nginx virtual hosts without modifying `/etc/nginx/nginx.conf` or granting the Manager container arbitrary host configuration access.

**Architecture:** Add a small native host-side Nginx proxy agent, started by systemd and reached only through an authenticated Unix socket. The Provisioner sends a typed request containing a slug, hostname, and loopback ports; the host agent validates those values and renders the only allowed server-block template into `/etc/nginx/sites-available/supabase-manager-<slug>.conf`, then maintains the matching `sites-enabled` symlink. The agent atomically swaps files, runs host `nginx -t`, reloads Nginx, and restores the prior site on validation/reload failure. Project runtime reconciliation calls the agent only after candidate services are healthy; a failed proxy apply enters the existing runtime rollback path.

**Tech Stack:** Go 1.27, Unix-domain HTTP socket, systemd socket/service units, Ubuntu Nginx `sites-available`/`sites-enabled`, Docker Compose bind-mounted socket, existing Manager/Provisioner contracts and runtime reconciliation.

---

## Fixed behavior and boundaries

- The solution never edits `/etc/nginx/nginx.conf`. It assumes the normal Ubuntu include of `/etc/nginx/sites-enabled/*`; the installer verifies that this include is active before enabling automation.
- One project owns exactly one stable file, named from its validated slug. A Domain change overwrites that same file atomically; it cannot leave an old-domain file behind.
- `sites-enabled` contains only a symlink to its `sites-available` counterpart. Deleting a project removes both paths.
- The Provisioner cannot send arbitrary Nginx text. It sends only a typed `Apply` or `Remove` request; the host agent owns template rendering, filesystem access, `nginx -t`, and `systemctl reload nginx`.
- The proxy agent is optional at deployment level. `NGINX_PROXY_MODE=disabled` preserves current external/manual-proxy behavior; `managed` requires its socket and token before a project can be reconciled.
- Cloudflare DNS and edge certificates remain external concerns. The agent manages only the origin Nginx route.

## Planned file map

| Path | Responsibility |
| --- | --- |
| `apps/nginxproxy/cmd/nginx-proxy-agent/main.go` | Native host-agent entry point, Unix listener, authenticated HTTP handler. |
| `apps/nginxproxy/internal/site/site.go` | Typed request validation, fixed server-block renderer, stable site path calculation. |
| `apps/nginxproxy/internal/site/store.go` | Atomic site-file/symlink swap, backup/restore, no stale Domain-named files. |
| `apps/nginxproxy/internal/site/runner.go` | Narrow `nginx -t` and `systemctl reload nginx` execution interface. |
| `apps/nginxproxy/internal/site/*_test.go` | Unit tests for rendering, path safety, rollback, and removal. |
| `apps/provisioner/internal/proxy/client.go` | Authenticated Unix-socket client for typed Apply/Remove calls. |
| `apps/provisioner/internal/proxy/client_test.go` | Client request, token, timeout, and response-error tests. |
| `apps/provisioner/internal/config/config.go` | `NGINX_PROXY_MODE`, socket path, and proxy token configuration. |
| `apps/provisioner/internal/runtime/backend.go` | Proxy registrar dependency injection. |
| `apps/provisioner/internal/runtime/reconcile.go` | Apply proxy after candidate health; restore prior proxy on runtime rollback. |
| `apps/provisioner/internal/runtime/backend.go` | Remove proxy after project containers stop and before deleting project data. |
| `apps/provisioner/internal/runtime/reconcile_test.go` | Reconcile ordering and rollback tests using a fake proxy registrar. |
| `deploy/Dockerfile.nginx-proxy-agent` | Produces the static native agent artifact via Docker BuildKit output. |
| `deploy/nginx-proxy-agent.service` | systemd service definition for the native agent. |
| `deploy/nginx-proxy-agent.socket` | Root-owned Unix socket definition with narrowly scoped access. |
| `deploy/install-nginx-proxy-agent.sh` | One-time host installer; verifies Ubuntu Nginx layout and installs agent/unit files. |
| `deploy/docker-compose.yml` | Mounts only the agent socket into Provisioner and passes managed-proxy settings. |
| `deploy/.env.example` | Documents managed-proxy environment values without secrets. |
| `docs/operations/project-host-nginx.md` | Replaces manual per-project `conf.d` instructions with managed sites deployment, rollback, and DNS guidance. |

### Task 1: Define the proxy-agent request contract and renderer

**Files:**
- Create: `apps/nginxproxy/internal/site/site.go`
- Create: `apps/nginxproxy/internal/site/site_test.go`

- [ ] **Step 1: Write failing validation/rendering tests**

```go
func TestRenderApplyUsesStableSlugFileAndCurrentDomain(t *testing.T) {
    request := ApplyRequest{Slug: "bee-game", Domain: "bee.example.com", APIPort: 18001, StudioPort: 18002, StudioEnabled: true}
    site, err := RenderApply(request)
    if err != nil { t.Fatal(err) }
    if site.AvailableName != "supabase-manager-bee-game.conf" { t.Fatalf("name = %q", site.AvailableName) }
    if !strings.Contains(site.Contents, "server_name bee.example.com;") { t.Fatal("missing server name") }
    if !strings.Contains(site.Contents, "proxy_pass http://127.0.0.1:18001;") { t.Fatal("missing API upstream") }
    if !strings.Contains(site.Contents, "proxy_pass http://127.0.0.1:18002;") { t.Fatal("missing Studio upstream") }
}

func TestRenderApplyRejectsUnsafeSlugDomainAndPorts(t *testing.T) {
    for _, request := range []ApplyRequest{{Slug: "../etc", Domain: "bee.example.com", APIPort: 1}, {Slug: "bee", Domain: "bad;include", APIPort: 1}, {Slug: "bee", Domain: "bee.example.com", APIPort: 0}} {
        if _, err := RenderApply(request); err == nil { t.Fatalf("accepted %#v", request) }
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./apps/nginxproxy/internal/site -run 'TestRenderApply' -count=1`

Expected: FAIL because the `site` package does not yet exist.

- [ ] **Step 3: Implement typed requests and the fixed Nginx template**

```go
type ApplyRequest struct {
    Slug          string `json:"slug"`
    Domain        string `json:"domain"`
    APIPort       int    `json:"apiPort"`
    StudioPort    int    `json:"studioPort"`
    StudioEnabled bool   `json:"studioEnabled"`
}

type RenderedSite struct { AvailableName, Contents string }

func RenderApply(request ApplyRequest) (RenderedSite, error) {
    // validate a DNS hostname, a slug matching ^[a-z0-9][a-z0-9-]{0,62}$,
    // and each enabled loopback port in [1, 65535]; render no caller text
    // other than the validated hostname and integer ports.
}
```

The template must include the existing Studio, Auth, REST, GraphQL, Storage, Functions, MCP, SSO, and Realtime WebSocket locations. It must bind only `listen 80` and `listen 443 ssl`, use the installation-configured certificate paths, and proxy only to `127.0.0.1` ports.

- [ ] **Step 4: Run the renderer tests**

Run: `go test ./apps/nginxproxy/internal/site -run 'TestRenderApply' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit the isolated contract**

```bash
git add apps/nginxproxy/internal/site/site.go apps/nginxproxy/internal/site/site_test.go
git commit -m "feat: define managed nginx site renderer"
```

### Task 2: Implement transactional host site publication

**Files:**
- Create: `apps/nginxproxy/internal/site/store.go`
- Create: `apps/nginxproxy/internal/site/store_test.go`
- Create: `apps/nginxproxy/internal/site/runner.go`

- [ ] **Step 1: Write a failing atomic-update test**

```go
func TestApplyReplacesOneSlugFileAndRestoresOldContentsWhenNginxRejects(t *testing.T) {
    root := t.TempDir()
    store := NewStore(filepath.Join(root, "available"), filepath.Join(root, "enabled"), fakeRunner{testErr: errors.New("invalid")})
    writeExistingSite(t, store, "bee", "old.example.com")
    err := store.Apply(context.Background(), mustRender(t, "bee", "new.example.com"))
    if err == nil { t.Fatal("Apply unexpectedly succeeded") }
    requireSiteDomain(t, store, "bee", "old.example.com")
    requireEnabledLink(t, store, "bee")
}
```

- [ ] **Step 2: Run the atomic-update test to verify RED**

Run: `go test ./apps/nginxproxy/internal/site -run TestApplyReplacesOneSlugFileAndRestoresOldContentsWhenNginxRejects -count=1`

Expected: FAIL because `Store.Apply` is missing.

- [ ] **Step 3: Implement `Store.Apply` and `Store.Remove`**

```go
type Runner interface {
    Test(context.Context) error
    Reload(context.Context) error
}

func (s *Store) Apply(ctx context.Context, rendered RenderedSite) error {
    // mkdir available/enabled, write+fsync a same-directory temporary file
    // mode 0644, rename onto the stable slug file, atomically ensure the
    // enabled symlink, then Runner.Test and Runner.Reload. Restore the exact
    // prior file/link state and validate/reload it if either command fails.
}

func (s *Store) Remove(ctx context.Context, slug string) error {
    // remove the stable enabled link and source file as one reversible change;
    // only commit removal after test+reload succeeds, otherwise restore both.
}
```

The real runner must invoke only the configured absolute `nginx` binary with `-t` and the configured absolute `systemctl` binary with `reload nginx`; never use a shell, `PATH`, or caller-controlled arguments.

- [ ] **Step 4: Add removal and reload-failure coverage**

```go
func TestRemoveRestoresSiteWhenReloadFails(t *testing.T) {
    root := t.TempDir()
    store := NewStore(filepath.Join(root, "available"), filepath.Join(root, "enabled"), fakeRunner{reloadErr: errors.New("reload")})
    writeExistingSite(t, store, "bee", "bee.example.com")
    if err := store.Remove(context.Background(), "bee"); err == nil { t.Fatal("Remove unexpectedly succeeded") }
    requireSiteDomain(t, store, "bee", "bee.example.com")
    requireEnabledLink(t, store, "bee")
}

func TestApplyLeavesNoDomainNamedFilesAfterDomainChange(t *testing.T) {
    root := t.TempDir()
    store := NewStore(filepath.Join(root, "available"), filepath.Join(root, "enabled"), fakeRunner{})
    writeExistingSite(t, store, "bee", "old.example.com")
    if err := store.Apply(context.Background(), mustRender(t, "bee", "new.example.com")); err != nil { t.Fatal(err) }
    entries, err := os.ReadDir(filepath.Join(root, "available"))
    if err != nil || len(entries) != 1 || entries[0].Name() != "supabase-manager-bee.conf" { t.Fatalf("entries = %#v, %v", entries, err) }
}
```

Run: `go test ./apps/nginxproxy/internal/site -count=1`

Expected: PASS.

- [ ] **Step 5: Commit transactional host publication**

```bash
git add apps/nginxproxy/internal/site
git commit -m "feat: publish nginx sites transactionally"
```

### Task 3: Expose the authenticated native Unix-socket agent

**Files:**
- Create: `apps/nginxproxy/cmd/nginx-proxy-agent/main.go`
- Create: `apps/nginxproxy/cmd/nginx-proxy-agent/main_test.go`
- Create: `apps/nginxproxy/internal/site/http.go`
- Create: `apps/nginxproxy/internal/site/http_test.go`

- [ ] **Step 1: Write failing HTTP authorization tests**

```go
func TestApplyRejectsMissingOrIncorrectToken(t *testing.T) {
    handler := NewHandler(store, "shared-test-token")
    request := httptest.NewRequest(http.MethodPost, "/v1/sites/apply", strings.NewReader(`{"slug":"bee","domain":"bee.example.com","apiPort":18001}`))
    response := httptest.NewRecorder()
    handler.ServeHTTP(response, request)
    if response.Code != http.StatusUnauthorized { t.Fatalf("status = %d", response.Code) }
}

func TestApplyAcceptsTypedRequestAndNeverAcceptsRawConfiguration(t *testing.T) {
    handler := NewHandler(newTestStore(t), "shared-test-token")
    valid := httptest.NewRequest(http.MethodPost, "/v1/sites/apply", strings.NewReader(`{"slug":"bee","domain":"bee.example.com","apiPort":18001}`))
    valid.Header.Set("Authorization", "Bearer shared-test-token")
    validResponse := httptest.NewRecorder()
    handler.ServeHTTP(validResponse, valid)
    if validResponse.Code != http.StatusNoContent { t.Fatalf("valid status = %d", validResponse.Code) }
    invalid := httptest.NewRequest(http.MethodPost, "/v1/sites/apply", strings.NewReader(`{"slug":"bee","domain":"bee.example.com","apiPort":18001,"rawConf":"server {}"}`))
    invalid.Header.Set("Authorization", "Bearer shared-test-token")
    invalidResponse := httptest.NewRecorder()
    handler.ServeHTTP(invalidResponse, invalid)
    if invalidResponse.Code != http.StatusBadRequest { t.Fatalf("raw configuration status = %d", invalidResponse.Code) }
}
```

- [ ] **Step 2: Run the HTTP test to verify RED**

Run: `go test ./apps/nginxproxy/... -run TestApplyRejectsMissingOrIncorrectToken -count=1`

Expected: FAIL because the agent handler does not exist.

- [ ] **Step 3: Implement the socket agent**

```go
// POST /v1/sites/apply: Authorization: Bearer <token>, JSON ApplyRequest
// POST /v1/sites/remove: Authorization: Bearer <token>, JSON {"slug":"..."}
// GET  /health/live: no project data, 204
```

`main.go` must read absolute paths and the shared token from environment, remove only a stale socket owned by the agent, `net.Listen("unix", socketPath)`, set socket permissions to `0660`, and shut down cleanly on SIGTERM.

- [ ] **Step 4: Run all agent tests and static checks**

Run: `go test ./apps/nginxproxy/... -count=1 && go vet ./apps/nginxproxy/...`

Expected: PASS.

- [ ] **Step 5: Commit the host-agent API**

```bash
git add apps/nginxproxy
git commit -m "feat: add nginx proxy host agent"
```

### Task 4: Wire the Provisioner to the host agent

**Files:**
- Create: `apps/provisioner/internal/proxy/client.go`
- Create: `apps/provisioner/internal/proxy/client_test.go`
- Modify: `apps/provisioner/internal/config/config.go`
- Modify: `apps/provisioner/cmd/provisioner/main.go`
- Modify: `apps/provisioner/internal/runtime/backend.go`
- Modify: `apps/provisioner/internal/runtime/reconcile.go`
- Modify: `apps/provisioner/internal/runtime/reconcile_test.go`

- [ ] **Step 1: Write a failing reconciliation ordering test**

```go
func TestReconcileAppliesProxyOnlyAfterCandidateRuntimeIsHealthy(t *testing.T) {
    runner, proxy := &fakeReconcileRunner{}, &fakeProxyRegistrar{}
    backend := NewBackend(root, runner, healthyInspector, proxy)
    if _, err := backend.Reconcile(context.Background(), request); err != nil { t.Fatal(err) }
    if proxy.applyCalls != 1 || proxy.applyBeforeHealthy { t.Fatalf("proxy ordering = %#v", proxy) }
}

func TestReconcileRestoresPreviousProxyWhenApplyFails(t *testing.T) {
    runner, proxy := &fakeReconcileRunner{}, &fakeProxyRegistrar{applyErr: errors.New("nginx rejected candidate")}
    backend := NewBackend(root, runner, healthyInspector, proxy)
    if _, err := backend.Reconcile(context.Background(), changedDomainRequest); err == nil { t.Fatal("Reconcile unexpectedly succeeded") }
    if !runner.recreatedPreviousServices { t.Fatal("previous runtime was not restored") }
    if proxy.restoreCalls != 1 { t.Fatalf("proxy restore calls = %d", proxy.restoreCalls) }
}
```

- [ ] **Step 2: Run the reconciliation tests to verify RED**

Run: `go test ./apps/provisioner/internal/runtime -run 'TestReconcile(AppliesProxyOnlyAfterCandidateRuntimeIsHealthy|RestoresPreviousProxyWhenApplyFails)' -count=1`

Expected: FAIL because `ProxyRegistrar` is not a dependency yet.

- [ ] **Step 3: Add the narrow Provisioner client and configuration**

```go
type Registrar interface {
    Apply(context.Context, ApplyRequest) error
    Remove(context.Context, string) error
}

// mode=disabled returns a no-op registrar; mode=managed requires both
// NGINX_PROXY_SOCKET and NGINX_PROXY_TOKEN and dials only that Unix socket.
```

The client timeout must be bounded (15 seconds), attach the bearer token, reject non-2xx responses without returning agent internals to Manager, and never send project secrets.

- [ ] **Step 4: Integrate lifecycle ordering**

```go
// Reconcile: render + health check candidate -> proxy.Apply(candidate) ->
// publish metadata. If proxy.Apply fails, call existing runtime rollback; the
// agent has already restored the old site on its own failed validation/reload.
// If a later metadata publication fails after proxy success, runtime rollback
// must call proxy.Apply(previous configuration) before returning failure.
// DELETE_DATA: stop the project runtime -> proxy.Remove(slug) -> delete data.
```

Pass `General.Domain`, `Network.APIPort`, `Network.StudioPort`, and `Services.Studio` only. Do not infer ports from hard-coded 8000/8001 values.

- [ ] **Step 5: Run focused Provisioner tests**

Run: `go test ./apps/provisioner/internal/proxy ./apps/provisioner/internal/runtime -count=1`

Expected: PASS.

- [ ] **Step 6: Commit Provisioner integration**

```bash
git add apps/provisioner
git commit -m "feat: register project nginx sites during reconcile"
```

### Task 5: Package the host agent and deployment wiring

**Files:**
- Create: `deploy/Dockerfile.nginx-proxy-agent`
- Create: `deploy/nginx-proxy-agent.service`
- Create: `deploy/nginx-proxy-agent.socket`
- Create: `deploy/install-nginx-proxy-agent.sh`
- Modify: `deploy/docker-compose.yml`
- Modify: `deploy/.env.example`

- [ ] **Step 1: Write installer smoke checks as shell tests**

Create: `deploy/test-nginx-proxy-agent-installer.sh`

```sh
set -eu
test -f "$DEST/lib/supabase-manager/nginx-proxy-agent"
grep -F 'include /etc/nginx/sites-enabled/*;' "$FAKE_NGINX_CONF"
test -f "$DEST/systemd/supabase-manager-nginx-proxy-agent.service"
test -f "$DEST/systemd/supabase-manager-nginx-proxy-agent.socket"
```

- [ ] **Step 2: Run installer smoke check to verify RED**

Run: `sh deploy/test-nginx-proxy-agent-installer.sh`

Expected: FAIL because the installer and units do not exist.

- [ ] **Step 3: Build and install the native agent without host Go**

`deploy/Dockerfile.nginx-proxy-agent` must use the existing Go Alpine builder and `--output type=local` so the installer can build a static binary using Docker already required by this product. The installer must:

```sh
# verify /etc/nginx/nginx.conf (or supplied NGINX_MAIN_CONF) includes
# /etc/nginx/sites-enabled/*; create the two site directories if absent;
# build/copy the agent binary; create a root-owned environment file containing
# NGINX_PROXY_TOKEN; install systemd socket/service; daemon-reload; enable and
# start the socket. It must not edit nginx.conf.
```

The service uses `/run/supabase-manager/nginx-proxy.sock`, `/etc/nginx/sites-available`, `/etc/nginx/sites-enabled`, `/usr/sbin/nginx`, and `/bin/systemctl` as explicit arguments/environment values.

- [ ] **Step 4: Mount only the socket into Provisioner**

```yaml
provisioner:
  environment:
    NGINX_PROXY_MODE: ${NGINX_PROXY_MODE:-disabled}
    NGINX_PROXY_SOCKET: ${NGINX_PROXY_SOCKET:-/run/supabase-manager/nginx-proxy.sock}
    NGINX_PROXY_TOKEN: ${NGINX_PROXY_TOKEN:-}
  volumes:
    - /run/supabase-manager:/run/supabase-manager
```

Do not mount `/etc/nginx`, `sites-available`, `sites-enabled`, or `/run/systemd` into the Manager or Provisioner container.

- [ ] **Step 5: Run packaging checks**

Run: `docker build -f deploy/Dockerfile.nginx-proxy-agent --target export -o type=local,dest=/tmp/supabase-manager-nginx-agent . && test -x /tmp/supabase-manager-nginx-agent/nginx-proxy-agent`

Expected: a static executable exists; no host Nginx files are changed by the build.

- [ ] **Step 6: Commit deployment package**

```bash
git add deploy
git commit -m "feat: package managed nginx proxy agent"
```

### Task 6: Replace manual Nginx documentation and complete verification

**Files:**
- Modify: `docs/operations/project-host-nginx.md`
- Modify: `README.md` if it links to manual reverse-proxy setup

- [ ] **Step 1: Replace manual project-file instructions**

Document the one-time host installation:

```sh
cd /home/SupabaseInstaller
sudo NGINX_PROXY_TOKEN='generate-a-long-random-value' \
  ./deploy/install-nginx-proxy-agent.sh
```

Document matching `deploy/.env` values:

```dotenv
NGINX_PROXY_MODE=managed
NGINX_PROXY_SOCKET=/run/supabase-manager/nginx-proxy.sock
NGINX_PROXY_TOKEN=<same-long-random-value>
```

Explicitly state that Domain changes overwrite the slug-named site file; project deletion removes its available file and enabled link; Cloudflare DNS/certificates remain the operator's responsibility.

- [ ] **Step 2: Add an end-to-end acceptance procedure**

```sh
# Create project -> assert sites-available and sites-enabled names.
# Change Domain -> assert the same filename now has the new server_name and
# no Domain-named stale files exist.
# Force nginx -t failure in an isolated test root -> assert previous file and
# symlink are restored.
# Delete project -> assert both project paths are absent.
```

- [ ] **Step 3: Run complete verification**

Run:

```sh
go test ./...
go vet ./...
docker build -f deploy/Dockerfile.manager -t supabase-manager:nginx-automation .
docker build -f deploy/Dockerfile.provisioner -t supabase-provisioner:nginx-automation .
docker build -f deploy/Dockerfile.nginx-proxy-agent --target export -o type=local,dest=/tmp/supabase-manager-nginx-agent .
git diff --check
```

Expected: all commands pass and the generated agent binary exists.

- [ ] **Step 4: Commit documentation and final verification**

```bash
git add docs README.md
git commit -m "docs: document managed nginx project sites"
```
