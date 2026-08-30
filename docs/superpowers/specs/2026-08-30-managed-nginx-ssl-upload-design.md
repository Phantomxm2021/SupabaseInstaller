# Managed Nginx SSL Upload Design

## Goal

Allow an operator to upload and activate a host-wide Nginx TLS certificate and
private key from the Manager web UI. The default certificate name is
`cloudflare-origin`. Every managed project site uses the active certificate
pair.

## Scope

- Add a global SSL card to Manager Project Settings.
- Display the active certificate name and its managed paths.
- Accept a certificate PEM and private-key PEM together.
- Allow a safe custom certificate name; default to `cloudflare-origin`.
- Validate PEM parsing and confirm that the certificate public key matches the
  private key before changing host state.
- Store files under `/etc/nginx/ssl/<name>.pem` and
  `/etc/nginx/ssl/<name>.key`; create `/etc/nginx/ssl` automatically when it
  does not exist.
- Atomically activate the selected pair, validate Nginx, reload Nginx, and
  roll back both files and active selection on failure.
- Re-render existing managed sites so their `ssl_certificate` and
  `ssl_certificate_key` directives use the active pair; no project runtime is
  recreated.

## Non-goals

- Per-project certificate pairs.
- ACME or Let's Encrypt issuance and renewal.
- Returning private-key material through any Manager API.
- Accepting arbitrary host paths from the browser.

## Architecture

The Manager API receives a bounded multipart upload containing `name`,
`certificate`, and `privateKey`. It forwards the bytes over the existing
authenticated Unix-socket channel to the native Nginx proxy agent.

The agent is the sole writer for `/etc/nginx/ssl`. It creates that directory
when necessary, then validates the
requested name against a restricted identifier pattern, parses the PEM inputs,
verifies their key pair, writes both files with root-owned restrictive modes,
updates the active TLS selection, and runs `nginx -t` followed by reload. The
agent returns only safe metadata: active name and absolute paths.

The active pair is persisted in an agent-owned metadata file. On startup, the
agent loads it; if it is absent, it uses the installer-provided default paths
for `cloudflare-origin`. Existing routes are re-applied using the active pair.

## UI and API

Project Settings gains an **Nginx TLS certificate** card:

- Certificate name input, prefilled with `cloudflare-origin`.
- Certificate PEM upload input.
- Private key PEM upload input.
- Active certificate metadata, without exposing private-key contents.
- Upload and activate action, disabled until both files are selected.

The UI shows precise failures for invalid names, missing files, malformed PEM,
non-matching key pairs, Nginx validation failure, and agent unavailability.

## Security

- Certificate names accept lowercase letters, digits, and hyphens only.
- Files are never stored in the Manager database or browser local storage.
- The Manager accepts bounded upload sizes and does not log file contents.
- The private key is written only by the host agent with mode `0600`.
- Certificates use mode `0644` and the directory is root-owned with Nginx
  traversal access only.
- Replacement is atomic and rollback restores the prior active files and
  generated Nginx site configuration if validation or reload fails.

## Compatibility

Existing installations retain the current installer paths:

- `/etc/nginx/ssl/cloudflare-origin.pem`
- `/etc/nginx/ssl/cloudflare-origin.key`

The first UI upload imports a new managed pair and becomes the active source
for all generated sites. No existing project has to be deleted or recreated.

## Verification

- Unit-test TLS name validation, PEM parsing, key-pair matching, atomic writes,
  active-selection persistence, and rollback.
- API-test multipart validation and redaction of private-key material.
- UI-test default name, required upload fields, success metadata, and errors.
- Agent integration-test Nginx route rendering with the active custom pair.
- Regression-test the `cloudflare-origin` default behavior.
