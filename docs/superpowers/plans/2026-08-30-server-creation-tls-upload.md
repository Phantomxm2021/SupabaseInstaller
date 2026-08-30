# Server-Creation TLS Upload Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (- [ ]) syntax for tracking.

**Goal:** Upload a certificate and private key while creating a Server, write /etc/nginx/ssl/<certificate-name>-<base-domain-label>.{pem,key}, and render that pair into the Server's managed Nginx site.

**Architecture:** The browser submits one bounded multipart creation request. Manager persists only certificate name and generated paths, then asks Provisioner to stage the raw pair through the authenticated private channel before it queues the existing install operation. Provisioner forwards PEM data only to the native Nginx agent; retries use stored paths and never require a private-key upload.

**Tech Stack:** React, react-hook-form, Zod, Vitest, Go net/http, Go crypto/x509, Unix sockets, Nginx.

---

### Task 1: Define the safe non-secret TLS contract

**Files:**
- Create: internal/contracts/tls.go
- Modify: internal/contracts/configuration.go
- Test: internal/contracts/tls_test.go

- [x] **Step 1: Write the failing tests**

    func TestManagedTLSPathsUseBaseDomainLabel(t *testing.T) {
        got, err := ManagedTLSPaths("cloudflare-origin", "https://beegame.studio")
        if err != nil { t.Fatal(err) }
        if got.CertificateFile != "/etc/nginx/ssl/cloudflare-origin-beegame.pem" { t.Fatal(got.CertificateFile) }
    }

    func TestManagedTLSPathsRejectUnsafeName(t *testing.T) {
        if _, err := ManagedTLSPaths("../origin", "https://beegame.studio"); err == nil { t.Fatal("accepted traversal") }
    }

- [x] **Step 2: Verify RED**

Run: go test ./internal/contracts -run TestManagedTLSPaths -count=1

Expected: compile error because ManagedTLSPaths does not exist.

- [x] **Step 3: Implement the smallest contract**

Create ManagedTLSConfig with CertificateName, CertificateFile, and PrivateKeyFile. Add ManagedTLS ManagedTLSConfig to NetworkConfig. ManagedTLSPaths must parse the configured Site URL, take the base-domain label, validate the lowercase/digit/hyphen certificate name, and derive the two /etc/nginx/ssl paths. Manager normalization overwrites any caller-provided paths.

- [x] **Step 4: Verify GREEN and commit**

Run: go test ./internal/contracts -count=1

Expected: exit code 0.

Run: git add internal/contracts/configuration.go internal/contracts/tls.go internal/contracts/tls_test.go && git commit -m "feat: add managed tls configuration contract"

### Task 2: Stage verified PEM pairs in the Nginx agent

**Files:**
- Create: apps/nginxproxy/internal/site/certificate.go
- Modify: apps/nginxproxy/internal/site/site.go
- Modify: apps/nginxproxy/internal/server/server.go
- Test: apps/nginxproxy/internal/site/certificate_test.go
- Test: apps/nginxproxy/internal/site/site_test.go
- Test: apps/nginxproxy/internal/server/server_test.go

- [ ] **Step 1: Write failing stage/render tests**

    func TestCertificateStorePublishesMatchedPair(t *testing.T) {
        result, err := NewCertificateStore(t.TempDir()).Stage(context.Background(), CertificateInput{
            Name: "cloudflare-origin", BaseDomain: "beegame.studio",
            CertificatePEM: testCertificatePEM, PrivateKeyPEM: testPrivateKeyPEM,
        })
        if err != nil { t.Fatal(err) }
        assertMode(t, result.PrivateKeyFile, 0o600)
    }

    func TestRendererUsesManagedTLSPaths(t *testing.T) {
        rendered, err := NewRenderer(TLSPaths{AuthDirectory: t.TempDir()}).RenderApply(ApplyRequest{
            Slug: "bgs", Domain: "bgs.beegame.studio", APIPort: 8000,
            CertificateFile: "/etc/nginx/ssl/cloudflare-origin-beegame.pem",
            CertificateKeyFile: "/etc/nginx/ssl/cloudflare-origin-beegame.key",
        })
        if err != nil || !strings.Contains(rendered.Contents, "cloudflare-origin-beegame.pem") { t.Fatal(err) }
    }

- [ ] **Step 2: Verify RED**

Run: go test ./apps/nginxproxy/internal/site ./apps/nginxproxy/internal/server -run 'TestCertificateStore|TestRendererUsesManagedTLSPaths' -count=1

Expected: compile error because the store and request TLS fields are absent.

- [ ] **Step 3: Implement staging**

CertificateStore.Stage decodes PEM with crypto/x509, compares public keys, calls os.MkdirAll on /etc/nginx/ssl with 0755, and writes temp files in that directory before fsync+rename. Certificates have 0644 mode and private keys 0600. A pair may be reused only when the existing certificate/key cryptographically match; different material returns conflict. Invalid PEM or mismatched keys leaves no target pair.

Add CertificateFile and CertificateKeyFile to site.ApplyRequest and permit only absolute paths within /etc/nginx/ssl. Add an authenticated POST /v1/certificates/stage endpoint, capped at 1 MiB, returning only safe path metadata.

- [ ] **Step 4: Verify GREEN and commit**

Run: go test ./apps/nginxproxy/internal/site ./apps/nginxproxy/internal/server -count=1

Expected: exit code 0.

Run: git add apps/nginxproxy/internal/site apps/nginxproxy/internal/server && git commit -m "feat: stage managed tls certificates in nginx agent"

### Task 3: Relay staging through Provisioner and route the persisted paths

**Files:**
- Modify: internal/contracts/provisioner.go
- Modify: apps/provisioner/internal/proxy/client.go
- Modify: apps/provisioner/internal/runtime/backend.go
- Modify: apps/provisioner/internal/runtime/reconcile.go
- Modify: apps/provisioner/internal/server/server.go
- Test: apps/provisioner/internal/proxy/client_test.go
- Test: apps/provisioner/internal/runtime/reconcile_test.go
- Test: apps/provisioner/internal/server/server_test.go

- [ ] **Step 1: Write failing tests**

    func TestManagedClientStagesCertificateWithoutKeyInResponse(t *testing.T) {
        response := httptest.NewRecorder()
        // The test handler decodes StageManagedTLSRequest, asserts non-empty PEM
        // input, and responds with ManagedTLSConfig only.
        if strings.Contains(response.Body.String(), "PRIVATE KEY") { t.Fatal("private key leaked") }
    }

    func TestRouteForProxyUsesPersistedTLSPaths(t *testing.T) {
        route, managed := routeForProxy("bgs", configurationWithManagedTLS(), contracts.ProjectSecrets{})
        if !managed || route.CertificateFile != "/etc/nginx/ssl/cloudflare-origin-beegame.pem" { t.Fatal(route) }
    }

- [ ] **Step 2: Verify RED**

Run: go test ./apps/provisioner/internal/proxy ./apps/provisioner/internal/runtime ./apps/provisioner/internal/server -run 'TestManagedClientStagesCertificate|TestRouteForProxyUsesPersistedTLSPaths' -count=1

Expected: compile error for the absent staging RPC and route fields.

- [ ] **Step 3: Implement private RPC**

Add StageManagedTLSRequest (name, base domain, certificate PEM, private-key PEM) and a redacted result to internal contracts. Add StageManagedTLS to the Provisioner proxy/backend and a Manager-authenticated POST /internal/v1/certificates/stage handler. The proxy forwards only to the agent Unix socket. routeForProxy copies only Network.ManagedTLS paths. Legacy metadata without ManagedTLS continues to use installer certificate paths.

- [ ] **Step 4: Verify GREEN and commit**

Run: go test ./apps/provisioner/internal/proxy ./apps/provisioner/internal/runtime ./apps/provisioner/internal/server -count=1

Expected: exit code 0.

Run: git add internal/contracts/provisioner.go apps/provisioner/internal/proxy apps/provisioner/internal/runtime apps/provisioner/internal/server && git commit -m "feat: relay tls staging through provisioner"

### Task 4: Accept multipart create requests in Manager

**Files:**
- Modify: apps/manager/internal/provisioner/client.go
- Modify: apps/manager/internal/httpapi/projects.go
- Modify: apps/manager/internal/httpapi/projects_test.go
- Modify: apps/manager/internal/project/service.go
- Test: apps/manager/internal/project/service_test.go

- [ ] **Step 1: Write failing HTTP tests**

    func TestCreateProjectStagesTLSThenQueuesInstall(t *testing.T) {
        request := multipartCreateRequest(t, projectDraftFixture(), testCertificatePEM, testPrivateKeyPEM)
        response := httptest.NewRecorder()
        mux.ServeHTTP(response, request)
        if response.Code != http.StatusAccepted || strings.Contains(response.Body.String(), "PRIVATE KEY") { t.Fatal(response.Body.String()) }
    }

    func TestCreateProjectRejectsMissingPrivateKey(t *testing.T) {
        request := multipartCreateRequest(t, projectDraftFixture(), testCertificatePEM, nil)
        response := httptest.NewRecorder()
        mux.ServeHTTP(response, request)
        if response.Code != http.StatusUnprocessableEntity { t.Fatal(response.Code) }
    }

- [ ] **Step 2: Verify RED**

Run: go test ./apps/manager/internal/httpapi ./apps/manager/internal/project -run 'TestCreateProjectStagesTLS|TestCreateProjectRejectsMissingPrivateKey' -count=1

Expected: the current JSON-only handler rejects multipart input.

- [ ] **Step 3: Implement ordered creation**

Create parseCreateServerMultipart using ParseMultipartForm(1 << 20). It requires exactly draft, certificate, and privateKey fields, rejects unknown parts, and never logs bytes. Create the project record, stage the pair through Provisioner, create the operation, then launch the existing install goroutine. If staging fails, delete the project record. If operation creation fails, remove only a pair first created for this project, then delete the record. Store only certificate name and paths; never put PEM in SQLite, operation events, configuration responses, or logs.

- [ ] **Step 4: Verify GREEN and commit**

Run: go test ./apps/manager/internal/httpapi ./apps/manager/internal/project ./apps/manager/internal/provisioner -count=1

Expected: exit code 0.

Run: git add apps/manager/internal/httpapi/projects.go apps/manager/internal/httpapi/projects_test.go apps/manager/internal/project apps/manager/internal/provisioner && git commit -m "feat: accept tls uploads during server creation"

### Task 5: Build the approved first-step UI and FormData submission

**Files:**
- Create: apps/web/src/features/projects/ManagedTLSCard.tsx
- Modify: apps/web/src/features/projects/projectSchema.ts
- Modify: apps/web/src/features/projects/BasicStep.tsx
- Modify: apps/web/src/features/projects/NewProjectPage.tsx
- Modify: apps/web/src/features/projects/ReviewStep.tsx
- Modify: apps/web/src/api/client.ts
- Test: apps/web/src/features/projects/ManagedTLSCard.test.tsx
- Test: apps/web/src/features/projects/NewProjectPage.test.tsx

- [ ] **Step 1: Write failing UI tests**

    it("previews the base-domain certificate path", async () => {
      render(<NewProjectPage />)
      await user.type(screen.getByLabelText("Site URL hostname"), "beegame.studio")
      expect(screen.getByText("/etc/nginx/ssl/cloudflare-origin-beegame.pem")).toBeVisible()
    })

    it("requires both TLS files before continuing", async () => {
      render(<NewProjectPage />)
      await user.upload(screen.getByLabelText("Certificate PEM"), certificateFile)
      expect(screen.getByRole("button", { name: "Continue" })).toBeDisabled()
    })

- [ ] **Step 2: Verify RED**

Run: npm run test --workspace apps/web -- --run src/features/projects/ManagedTLSCard.test.tsx src/features/projects/NewProjectPage.test.tsx

Expected: absent-card query failures.

- [ ] **Step 3: Implement only the approved design**

Create ManagedTLSCard below the vertical Server details card. It has a certificate-name input, two standard file controls, and the readonly generated path previews. Do not add tabs, a global Project Settings card, or a new wizard step. Keep files only in React state. NewProjectPage submits FormData fields draft, certificate, and privateKey; apiFetch must not assign application/json to FormData. ReviewStep shows only the certificate name and safe managed paths.

- [ ] **Step 4: Verify GREEN and commit**

Run: npm run test --workspace apps/web -- --run src/features/projects/ManagedTLSCard.test.tsx src/features/projects/NewProjectPage.test.tsx src/features/projects/projectSchema.test.ts && npm run build --workspace apps/web

Expected: exit code 0.

Run: git add apps/web/src/features/projects apps/web/src/api/client.ts && git commit -m "feat: add tls upload card to server creation"

### Task 6: Verify and document the whole chain

**Files:**
- Create: scripts/acceptance/managed_tls_upload.sh
- Modify: the existing managed-Nginx deployment guide under docs/

- [ ] **Step 1: Write the failing acceptance script**

The script creates an ephemeral self-signed pair with openssl, stages it through
the agent, applies bgs.beegame.studio, asserts the rendered Nginx site
references cloudflare-origin-beegame.pem, and fails if private-key bytes reach
output. It removes its temporary directory with trap before exit.

- [ ] **Step 2: Verify RED and finish the acceptance script**

Run: bash scripts/acceptance/managed_tls_upload.sh

Expected before wiring: non-zero. Expected after wiring: exit code 0.

- [ ] **Step 3: Run full verification and commit**

Run: go test ./... && npm run test && npm run build && bash scripts/acceptance/managed_tls_upload.sh

Expected: all commands exit 0 and output is free of private-key material.

Run: git add docs scripts/acceptance && git commit -m "test: cover managed tls server creation"
