# Managed Nginx Studio Authentication Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every managed project Studio host require the configured Studio username and password, while leaving Supabase API endpoints unauthenticated.

**Architecture:** The Provisioner extends its typed private Unix-socket route with the configured Studio credentials. The Nginx Agent hashes the password using Apache MD5 (`apr1`), writes one root-only `htpasswd` file per project, and renders `auth_basic` only inside `location /`. Site configuration, enabled link, and credential file are applied and removed atomically with Nginx validation and rollback.

**Tech Stack:** Go 1.27, `golang.org/x/crypto` bcrypt-compatible password primitives where required, native Nginx `auth_basic`, existing Unix-socket Nginx Agent.

---

### Task 1: Prove the missing root-path protection at rendering boundary

**Files:**
- Modify: `apps/nginxproxy/internal/site/site_test.go`
- Modify: `apps/nginxproxy/internal/site/site.go`

- [ ] **Step 1: Write the failing renderer test**

```go
func TestRenderApplyProtectsOnlyStudioRoot(t *testing.T) {
    rendered, err := renderer.RenderApply(ApplyRequest{
        Slug: "project", Domain: "project.example.com", APIPort: 18001,
        StudioPort: 18002, StudioEnabled: true, StudioUsername: "operator",
    })
    if err != nil { t.Fatal(err) }
    for _, want := range []string{
        `auth_basic "Supabase Studio";`,
        "auth_basic_user_file /etc/supabase-manager/nginx-auth/supabase-manager-project.htpasswd;",
        "proxy_pass http://127.0.0.1:18002;",
    } { if !strings.Contains(rendered.Contents, want) { t.Fatalf("missing %q", want) } }
    if strings.Contains(apiLocation(rendered.Contents), "auth_basic") { t.Fatal("API route must not be protected") }
}
```

- [ ] **Step 2: Run and verify RED**

Run: `go test ./apps/nginxproxy/internal/site -run TestRenderApplyProtectsOnlyStudioRoot -count=1`

Expected: FAIL because the generated Studio location has no `auth_basic` directive.

- [ ] **Step 3: Implement the minimal typed rendering contract**

Extend `ApplyRequest` and `RenderedSite` with the validated per-slug auth file location. Render `auth_basic` and `auth_basic_user_file` only when Studio is enabled; reject an enabled Studio request with an empty username or password.

- [ ] **Step 4: Run renderer tests**

Run: `go test ./apps/nginxproxy/internal/site -count=1`

Expected: PASS.

### Task 2: Make the Agent own secure password files transactionally

**Files:**
- Modify: `apps/nginxproxy/internal/site/store.go`
- Modify: `apps/nginxproxy/internal/site/store_test.go`
- Modify: `apps/nginxproxy/cmd/nginx-proxy-agent/main.go`
- Modify: `apps/nginxproxy/internal/config/config.go`
- Modify: `apps/nginxproxy/internal/config/config_test.go`

- [ ] **Step 1: Write failing Store tests**

Add tests that `Apply` creates `supabase-manager-<slug>.htpasswd` with mode `0600`, whose content verifies the configured password, and that `Remove` deletes it. Add a reload-failure test proving both the prior site and prior password file are restored.

- [ ] **Step 2: Run and verify RED**

Run: `go test ./apps/nginxproxy/internal/site -run 'TestStore(Apply|Remove).*Password' -count=1`

Expected: FAIL because Store currently owns only sites-available/sites-enabled files.

- [ ] **Step 3: Implement minimal secure storage**

Add an Agent-owned `NGINX_AUTH_DIRECTORY` (default `/etc/supabase-manager/nginx-auth`). Generate an Apache-compatible hash in process, never shelling out and never logging raw credentials. Extend Store snapshots/rollback to include the credential file, write it with `0600`, and remove it with the matching managed site.

- [ ] **Step 4: Run Agent test suite**

Run: `go test ./apps/nginxproxy/... -count=1`

Expected: PASS.

### Task 3: Carry the configured credentials across the private Provisioner boundary

**Files:**
- Modify: `apps/provisioner/internal/proxy/client.go`
- Modify: `apps/provisioner/internal/proxy/client_test.go`
- Modify: `apps/provisioner/internal/runtime/reconcile.go`
- Modify: `apps/provisioner/internal/runtime/reconcile_test.go`
- Modify: `apps/nginxproxy/internal/server/server_test.go`

- [ ] **Step 1: Write failing Provisioner route test**

Update `TestReconcileAppliesProxyOnlyAfterHealthyRuntime` to expect `StudioUsername` from configuration and `StudioPassword` from request secrets. Assert the previous route used for rollback contains the previous credentials only when metadata provides a usable managed route.

- [ ] **Step 2: Run and verify RED**

Run: `go test ./apps/provisioner/internal/runtime -run TestReconcileAppliesProxyOnlyAfterHealthyRuntime -count=1`

Expected: FAIL because `routeForProxy` does not receive secrets and `proxy.Route` has no credentials.

- [ ] **Step 3: Implement typed private propagation**

Add username/password fields to `proxy.Route` and Agent `ApplyRequest`. Change `routeForProxy` to accept `contracts.ProjectSecrets`, pass `request.Secrets` for the candidate, and preserve the former secret only when recovering the old metadata route. Do not add credentials to logs, errors, file names, or public APIs.

- [ ] **Step 4: Verify the private boundary**

Run: `go test ./apps/provisioner/internal/proxy ./apps/provisioner/internal/runtime ./apps/nginxproxy/internal/server -count=1`

Expected: PASS.

### Task 4: Full regression verification

**Files:**
- Modify: none unless verification finds a defect

- [ ] **Step 1: Run all backend tests**

Run: `go test ./...`

Expected: PASS.

- [ ] **Step 2: Build deployable binaries/images**

Run: `docker compose -f deploy/docker-compose.yml --env-file deploy/.env build manager provisioner nginx-proxy-agent`

Expected: all selected images build successfully when Docker is available.

- [ ] **Step 3: Commit the verified fix**

```bash
git add apps/nginxproxy apps/provisioner docs/superpowers/plans/2026-08-28-managed-nginx-studio-auth.md
git commit -m "fix: protect managed Studio routes with configured credentials"
```
