# Per-Server Managed Nginx SSL Upload Design

## Goal

Allow an operator to upload a TLS certificate and private key while creating a
Server from the Manager web UI. The default certificate name is
`cloudflare-origin`. Each Server uses its own managed certificate pair.

## Scope

- Add an SSL section to the Create Server workflow.
- Display the chosen certificate name and generated managed paths in review.
- Accept a certificate PEM and private-key PEM together.
- Allow a safe custom certificate name; default to `cloudflare-origin`.
- Validate PEM parsing and confirm that the certificate public key matches the
  private key before changing host state.
- Store files under `/etc/nginx/ssl/<name>-<domain-label>.pem` and
  `/etc/nginx/ssl/<name>-<domain-label>.key`; create `/etc/nginx/ssl`
  automatically when it does not exist.
- Render the newly created Server's Nginx site with those paths, validate
  Nginx, reload Nginx, and roll back both files and site activation on failure.

## Non-goals

- Replacing certificate files after Server creation in this first version.
- ACME or Let's Encrypt issuance and renewal.
- Returning private-key material through any Manager API.
- Accepting arbitrary host paths from the browser.

## Architecture

The Create Server request accepts a bounded multipart upload containing
`certificateName`, `certificate`, and `privateKey`. It creates the project and
forwards the certificate bytes over the existing authenticated Unix-socket
channel to the native Nginx proxy agent as part of reconciliation.

The agent is the sole writer for `/etc/nginx/ssl`. It creates that directory
when necessary, then validates the
requested name against a restricted identifier pattern, parses the PEM inputs,
verifies their key pair, writes both files with root-owned restrictive modes,
derives the physical filename as `<certificateName>-<domainLabel>`, and runs
`nginx -t` followed by reload. The agent returns only safe metadata: selected
name and absolute paths.

Each project configuration persists the certificate name and generated paths.
Existing installations without uploaded certificate material retain the
installer-provided `cloudflare-origin` paths until their Server is recreated.

## UI and API

The Create Server workflow gains an **Nginx TLS certificate** section:

- Certificate name input, prefilled with `cloudflare-origin`.
- Certificate PEM upload input.
- Private key PEM upload input.
- Generated filename preview, for example `cloudflare-origin-tet.pem`, without
  exposing private-key contents. The suffix is the normalized first DNS label
  from the Server domain; it excludes the URL scheme, port, and parent domain.
- Upload requirement: both files must be selected before creating the Server.

The UI shows precise failures for invalid names, missing files, malformed PEM,
non-matching key pairs, Nginx validation failure, and agent unavailability.

## Security

- Certificate names accept lowercase letters, digits, and hyphens only.
- Files are never stored in the Manager database or browser local storage.
- The Manager accepts bounded upload sizes and does not log file contents.
- The private key is written only by the host agent with mode `0600`.
- Certificates use mode `0644` and the directory is root-owned with Nginx
  traversal access only.
- Upload writes a staged pair and publishes it atomically only after validation.
  A failed Server creation removes staged files and restores the prior Nginx
  site state.

## Compatibility

Existing installations retain the current installer paths:

- `/etc/nginx/ssl/cloudflare-origin.pem`
- `/etc/nginx/ssl/cloudflare-origin.key`

New Servers upload their own pair. The default certificate name yields files
such as `/etc/nginx/ssl/cloudflare-origin-tet.pem`; the first domain label
identifies the Server's certificate pair.

## Verification

- Unit-test TLS name validation, PEM parsing, key-pair matching,
  first-domain-label path mapping, atomic writes, and rollback.
- API-test multipart validation and redaction of private-key material.
- UI-test default name, first-domain-label filename preview, required upload fields,
  and errors.
- Agent integration-test Nginx route rendering with each Server's custom pair.
- Regression-test the `cloudflare-origin-<domain-label>` default behavior.
