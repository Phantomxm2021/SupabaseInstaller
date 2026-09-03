# Server Supabase host behind Nginx and Cloudflare

Each managed server has one public hostname. Its Studio UI is proxied from
the root path and its Supabase API paths are proxied to the server's loopback
API Gateway port:

```text
manager.example.com              -> 127.0.0.1:8080       (Manager)
server.supabase.example.com      -> server API/Studio   (one server host)
```

The recommended deployment uses the native `nginx-proxy-agent`. It owns only
`/etc/nginx/sites-available/supabase-manager-<slug>.conf`, its matching
`sites-enabled` link, and one root-only credential file beneath
`/etc/supabase-manager/nginx-auth`. The Provisioner talks to it through an
authenticated Unix socket; neither Manager nor Provisioner has access to
`/etc/nginx`.

## Server values

Configure the following through Manager before reconciling a server:

| Setting | Example | Meaning |
| --- | --- | --- |
| `General.Domain` | `server.supabase.example.com` | Public Supabase host; hostname only |
| `General.SiteURL` | `https://app.example.com` | Application redirect URL, not the Supabase host |
| `Network.APIPort` | allocated by Manager | Loopback API Gateway port |
| `Network.StudioPort` | allocated by Manager | Loopback Studio port when Studio is enabled |

The agent renders the fixed host routing template. It does not accept raw
Nginx text from a server and it never publishes API or Studio ports outside
`127.0.0.1`.

## Migrate legacy Caddy projects

Legacy projects that still have `network.httpsMode=caddy` remain readable, but
Manager will not save an unrelated patch or render their Compose. Migrate each
project explicitly:

1. Configure the external TLS reverse proxy with a route for the project domain
   to that project's loopback API port (`127.0.0.1:<Network.APIPort>`). Keep the
   API and Studio routes on the same project host and verify both are reachable.
2. In Manager, set `network.httpsMode` to `external` and save the project.
3. Reconcile the project, then verify API and Studio reachability through the
   external proxy and inspect the generated Compose for the absence of a
   `caddy` service.

Manager never auto-switches a legacy project because changing its TLS entrypoint
without a verified external route could cause an outage. Repeat this migration
for every project before relying on the shared external proxy.

## Cloudflare DNS and TLS

Create one record per server or a wildcard record:

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
server data. Rerun it after upgrading the repository; no separate Agent or
Compose override command is required.

## Lifecycle behavior

In managed mode, a successful reconcile performs these steps:

1. Render and validate the server runtime.
2. Start/recreate the runtime and wait for its health checks.
3. Atomically write the slug-stable Nginx site file and the root-only Studio
   credential hash, validate with `nginx -t`, then reload Nginx.
4. Persist the server configuration.

If Nginx validation/reload fails, the agent restores its prior site file,
enabled link, and credential hash. If a later Manager metadata write fails, Provisioner restores
the prior runtime and prior proxy route. Updating `General.Domain` overwrites
the same slug-named site file, so it cannot leave an obsolete hostname file.
Deleting a server stops its runtime, removes its managed site and enabled
link, and only then removes server data.

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

For a server named `project`, its stable managed file is
`/etc/nginx/sites-available/supabase-manager-project.conf`. Do not edit that file
manually: the next Manager reconciliation overwrites it. Put any custom Nginx
policy in a separate operator-owned site file.

The Manager host remains separate on `127.0.0.1:8080`; Provisioner port 9090
is private and must never receive a public Nginx server block. PostgreSQL and
Supavisor ports are TCP services and are not part of this HTTP(S) host.
