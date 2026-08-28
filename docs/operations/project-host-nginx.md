# Project Supabase host behind Nginx and Cloudflare

Each managed project has one public hostname. Its Studio UI is proxied from
the root path and its Supabase API paths are proxied to the project's loopback
API Gateway port:

```text
manager.example.com              -> 127.0.0.1:8080       (Manager)
project.supabase.example.com     -> project API/Studio  (one project host)
```

The recommended deployment uses the native `nginx-proxy-agent`. It owns only
`/etc/nginx/sites-available/supabase-manager-<slug>.conf`, its matching
`sites-enabled` link, and one root-only credential file beneath
`/etc/supabase-manager/nginx-auth`. The Provisioner talks to it through an
authenticated Unix socket; neither Manager nor Provisioner has access to
`/etc/nginx`.

## Project values

Configure the following through Manager before reconciling a project:

| Setting | Example | Meaning |
| --- | --- | --- |
| `General.Domain` | `project.supabase.example.com` | Public Supabase host; hostname only |
| `General.SiteURL` | `https://app.example.com` | Application redirect URL, not the Supabase host |
| `Network.APIPort` | allocated by Manager | Loopback API Gateway port |
| `Network.StudioPort` | allocated by Manager | Loopback Studio port when Studio is enabled |

The agent renders the fixed host routing template. It does not accept raw
Nginx text from a project and it never publishes API or Studio ports outside
`127.0.0.1`.

## Cloudflare DNS and TLS

Create one record per project or a wildcard record:

```text
*.supabase.example.com  A  <host-public-ip>  Proxied
```

Use Cloudflare **Full (strict)** and install an origin certificate that covers
the wildcard hostname on the server. Cloudflare DNS and certificate issuance
remain operator-managed; the agent only uses the two installed certificate
paths to render virtual hosts.

## Install the managed agent

Use the single Ubuntu installer from the repository root. It generates a
dedicated Agent token, validates the standard Nginx layout, installs the native
Agent, explicitly restarts it on every upgrade, and starts the control plane in
managed-proxy mode.

```sh
sudo ./scripts/install-supabase-manager.sh \
  --public-origin https://manager.example.com \
  --certificate-file /etc/nginx/ssl/cloudflare-origin.pem \
  --certificate-key-file /etc/nginx/ssl/cloudflare-origin.key
```

The installer never edits unrelated Nginx sites and never removes Supabase
project data. Rerun it after upgrading the repository; no separate Agent or
Compose override command is required.

## Lifecycle behavior

In managed mode, a successful reconcile performs these steps:

1. Render and validate the project runtime.
2. Start/recreate the runtime and wait for its health checks.
3. Atomically write the slug-stable Nginx site file and the root-only Studio
   credential hash, validate with `nginx -t`, then reload Nginx.
4. Persist the project configuration.

If Nginx validation/reload fails, the agent restores its prior site file,
enabled link, and credential hash. If a later Manager metadata write fails, Provisioner restores
the prior runtime and prior proxy route. Updating `General.Domain` overwrites
the same slug-named site file, so it cannot leave an obsolete hostname file.
Deleting a project stops its runtime, removes its managed site and enabled
link, and only then removes project data.

The generated template requires the configured Studio username and password
for `/` (or returns `404` when Studio is disabled), and routes `/auth`,
`/rest`, `/graphql`, `/storage/v1/`,
`/functions`, `/mcp`, `/sso`, and `/realtime/v1/` to the API Gateway. Realtime
includes WebSocket forwarding.

## Validation and troubleshooting

```sh
sudo systemctl --no-pager status supabase-manager-nginx-proxy-agent.service
sudo journalctl -u supabase-manager-nginx-proxy-agent.service -n 100 --no-pager
sudo nginx -t
sudo ls -l /etc/nginx/sites-enabled/supabase-manager-*.conf
```

For a project named `project`, its stable managed file is
`/etc/nginx/sites-available/supabase-manager-project.conf`. Do not edit that file
manually: the next Manager reconciliation overwrites it. Put any custom Nginx
policy in a separate operator-owned site file.

The Manager host remains separate on `127.0.0.1:8080`; Provisioner port 9090
is private and must never receive a public Nginx server block. PostgreSQL and
Supavisor ports are TCP services and are not part of this HTTP(S) host.
