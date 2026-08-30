# Per-Server Managed Nginx SSL Upload Design

## Goal

Allow an operator to upload a TLS certificate and private key while creating a
Server from the Manager web UI. The default certificate name is
`cloudflare-origin`. Servers under the same configured Site URL base domain use
the same managed certificate pair.

## Scope

- Add an SSL section to the Create Server workflow.
- Display the chosen certificate name and generated managed paths in review.
- Accept a certificate PEM and private-key PEM together.
- Allow a safe custom certificate name; default to `cloudflare-origin`.
- Validate PEM parsing and confirm that the certificate public key matches the
  private key before changing host state.
- Store files under `/etc/nginx/ssl/<name>-<base-domain-label>.pem` and
  `/etc/nginx/ssl/<name>-<base-domain-label>.key`; create `/etc/nginx/ssl`
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
derives the physical filename as `<certificateName>-<baseDomainLabel>`, and runs
`nginx -t` followed by reload. The agent returns only safe metadata: selected
name and absolute paths.

If that filename pair already exists, the agent reuses it only when the new
upload is cryptographically the same certificate and key. A different pair is
rejected instead of overwriting a live base-domain certificate.

Each project configuration persists the certificate name and generated paths.
Existing installations without uploaded certificate material retain the
installer-provided `cloudflare-origin` paths until their Server is recreated.

## UI and API

The Create Server workflow gains an **Nginx TLS certificate** section:

- Certificate name input, prefilled with `cloudflare-origin`.
- Certificate PEM upload input.
- Private key PEM upload input.
- Generated filename preview, for example `cloudflare-origin-beegame.pem`,
  without exposing private-key contents. The suffix is the normalized primary
  label of the configured Site URL base domain: for
  `bgs.beegame.studio`, it is `beegame`, not `bgs`.
- Upload requirement: both files must be selected before creating the Server.

The UI shows precise failures for invalid names, missing files, malformed PEM,
non-matching key pairs, Nginx validation failure, and agent unavailability.

### Front-end interaction design

The wizard remains four steps: **Server details**, **Services**, **Security &
integrations**, and **Review & install**. TLS does not create a Setup tab or a
new wizard step. It is a second card on the first step, directly below the
vertical Server details card.

```
Create a server                                      Step 1 of 4 · Server details
Configure the complete Supabase runtime before Docker resources are created.

┌ Server details ───────────────────────────────────────────────────────────┐
│ Server name                                                                │
│ [ Production API                                                        ]  │
│                                                                            │
│ Site URL                                                                   │
│ [ https:// ] [ beegame.studio                                           ]  │
│                                                                            │
│ Studio username                                                            │
│ [ supabase                                                              ]  │
│                                                                            │
│ Studio password                                                            │
│ [ •••••••••••••                                                        ]  │
│                                                                            │
│ Supabase version                                                          │
│ [ self-hosted/v0.8.0                                                    ]  │
└────────────────────────────────────────────────────────────────────────────┘

┌ Nginx TLS certificate ────────────────────────────────────────────────────┐
│ Upload the certificate used by this Server's external Nginx site.          │
│                                                                            │
│ Certificate name                                                          │
│ [ cloudflare-origin                                                     ]  │
│                                                                            │
│ Managed file names                                                        │
│ Certificate  /etc/nginx/ssl/cloudflare-origin-beegame.pem                 │
│ Private key   /etc/nginx/ssl/cloudflare-origin-beegame.key                │
│                                                                            │
│ Certificate PEM                                                           │
│ [ Choose certificate file ]  example-origin.pem                           │
│                                                                            │
│ Private key PEM                                                           │
│ [ Choose private-key file ]  example-origin.key                           │
│                                                                            │
│ The private key is uploaded once, never shown again, and is not stored in │
│ the Manager database.                                                     │
└────────────────────────────────────────────────────────────────────────────┘

                                                    Cancel       Continue →
```

- The card is always visible on **Server details**. It is disabled, with a
  single inline explanation, until Site URL has a valid hostname.
- `Certificate name` defaults to `cloudflare-origin`; it permits only lowercase
  letters, digits, and hyphens. Editing it immediately updates both path
  previews.
- The base-domain label is derived from Site URL. For
  `bgs.beegame.studio`, the preview becomes
  `cloudflare-origin-beegame.pem`; the project subdomain is never part of the
  certificate filename.
- Both upload controls are required. Selecting one file marks that control
  complete, but **Continue** remains disabled until both are valid. Neither
  field displays file contents.
- The browser sends the selected files only when **Install server** is clicked;
  moving between wizard steps retains the in-memory selections. Leaving or
  refreshing the page clears them.
- On the Review step, a compact **Nginx TLS certificate** summary shows the
  certificate name and two managed paths, plus an **Edit server details**
  action. It never includes file names from the user's workstation, file
  contents, or the private key.
- Validation messages appear below their own control: unsupported extension or
  unreadable file, malformed certificate PEM, malformed private-key PEM,
  certificate/key mismatch, and a conflict when the base-domain pair already
  exists with different cryptographic material.

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

New Servers upload their base-domain certificate pair. The default certificate
name yields files such as `/etc/nginx/ssl/cloudflare-origin-beegame.pem` for
`*.beegame.studio`; all Servers under that base domain reference that pair.

## Verification

- Unit-test TLS name validation, PEM parsing, key-pair matching,
  base-domain-label path mapping, atomic writes, and rollback.
- API-test multipart validation and redaction of private-key material.
- UI-test default name, base-domain-label filename preview, required upload fields,
  and errors.
- Agent integration-test Nginx route rendering with each base domain's custom pair.
- Regression-test the `cloudflare-origin-<base-domain-label>` default behavior.
