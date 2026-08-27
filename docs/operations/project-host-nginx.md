# Project Supabase host behind Nginx and Cloudflare

This guide configures the public host of one Supabase project. It is separate
from the Manager control-plane host:

```text
manager.example.com              -> 127.0.0.1:8080       (Manager)
bee.supabase.example.com         -> project API/Studio  (one project host)
```

The project host uses one HTTPS hostname. Nginx sends the Studio root page to
the project's Studio port and sends Supabase API paths to the project's API
Gateway port. Auth, REST, Storage, Realtime, and Functions remain private
containers behind that Gateway.

## 1. Project values

Configure these values in Manager's typed project settings before installing or
reconciling the project:

| Setting | Example | Meaning |
| --- | --- | --- |
| `General.Domain` | `bee.supabase.example.com` | Public Supabase host; do not include a scheme or path |
| `General.SiteURL` | `https://app.example.com` | Application redirect URL; this is not the Supabase host |
| `Network.APIPort` | `8000`* | Host loopback port for the project API Gateway |
| `Network.StudioPort` | `8001`* | Host loopback port for the project Studio |

\* These are the current defaults when `deploy/.env` uses
`PORT_RANGE_START=8000` and `PORT_RANGE_END=8999`. They are not fixed ports;
the allocator may assign different values for another project or another
range. Always use the ports written to that project's generated Compose file.

On the host, read the actual bindings before writing Nginx configuration:

```sh
PROJECT_DIR=/home/supabase-manager/projects/beegamestudio
RUNTIME_DIR="$PROJECT_DIR/.manager-runtime/current"
sudo grep -nE '127\.0\.0\.1:[0-9]+:(8000|3000)' "$RUNTIME_DIR/docker-compose.yml"
sudo docker compose --file "$RUNTIME_DIR/docker-compose.yml" \
  --env-file "$RUNTIME_DIR/.env" \
  --project-directory "$PROJECT_DIR" ps api-gw studio
```

The API port is the host-side port mapped to container port `8000`; the Studio
port is the host-side port mapped to container port `3000`. Do not substitute
the Manager port (`8080`) or Provisioner port (`9090`).

The renderer derives the runtime URLs from `General.Domain`:

```text
SUPABASE_PUBLIC_URL=https://bee.supabase.example.com
API_EXTERNAL_URL=https://bee.supabase.example.com/auth/v1
```

The API and Studio ports are bound to `127.0.0.1`; they must not be published
directly to the Internet.

## 2. Cloudflare DNS and TLS

Use either one DNS record per project or a wildcard record:

```text
*.supabase.example.com  A  <host-public-ip>  Proxied
```

The Manager hostname needs its own record, for example
`manager.example.com -> <host-public-ip>`. Keep Cloudflare's proxy enabled
for HTTP(S) traffic and use **Full (strict)** with a valid certificate on the
origin. A wildcard origin certificate must cover
`*.supabase.example.com`; include the Manager hostname separately if the same
certificate is used there.

Cloudflare configuration references:

- [Proxy status](https://developers.cloudflare.com/dns/proxy-status/)
- [Full (strict) SSL mode](https://developers.cloudflare.com/ssl/origin-configuration/ssl-modes/full-strict/)
- [Cloudflare IP ranges](https://developers.cloudflare.com/fundamentals/concepts/cloudflare-ip-addresses/)

## 3. Nginx include

Keep project files in a dedicated include loaded from the Nginx `http` block.
The exact path is host-specific; this example uses
`/etc/nginx/conf.d/supabase-projects.conf`:

```nginx
# /etc/nginx/nginx.conf, inside http { ... }
include /etc/nginx/conf.d/supabase-projects.conf;
```

Do not put the project file inside the Manager Docker container. It belongs on
the host where Nginx runs.

## 4. Per-project server block

Create one block per project. The example below uses the current default
bindings (`8000` for API Gateway and `8001` for Studio). Replace both ports
with the actual values from the generated Compose file before reloading Nginx.

```nginx
# /etc/nginx/conf.d/supabase-projects.conf

server {
    listen 80;
    listen [::]:80;
    server_name bee.supabase.example.com;
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl;
    listen [::]:443 ssl;
    http2 on;
    server_name bee.supabase.example.com;

    ssl_certificate     /etc/ssl/supabase/wildcard.supabase.example.com.crt;
    ssl_certificate_key /etc/ssl/supabase/wildcard.supabase.example.com.key;

    server_tokens off;
    client_max_body_size 0;
    proxy_http_version 1.1;

    proxy_set_header Host              $host;
    proxy_set_header X-Real-IP         $remote_addr;
    proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header X-Forwarded-Host  $host;
    proxy_set_header X-Forwarded-Port  $server_port;

    # Studio UI. Protect this location with Cloudflare Access or an additional
    # Nginx auth policy when Studio is not intended to be public.
    location / {
        proxy_pass http://127.0.0.1:8001;
    }

    # Supabase API Gateway paths.
    location /auth {
        proxy_pass http://127.0.0.1:8000;
    }

    location /rest {
        proxy_pass http://127.0.0.1:8000;
    }

    location /graphql {
        proxy_pass http://127.0.0.1:8000;
    }

    location /storage/v1/ {
        proxy_pass http://127.0.0.1:8000;
        proxy_buffering off;
        proxy_request_buffering off;
    }

    location /functions {
        proxy_pass http://127.0.0.1:8000;
    }

    location /mcp {
        proxy_pass http://127.0.0.1:8000;
    }

    location /sso {
        proxy_pass http://127.0.0.1:8000;
    }

    # Realtime requires WebSocket forwarding.
    location /realtime/v1/ {
        proxy_pass http://127.0.0.1:8000;
        proxy_set_header Upgrade    $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
    }
}
```

The `location /` block is for Studio only. The API locations must stay ahead of
it semantically (the more specific prefixes win) so requests are not sent to
Studio accidentally.

If Studio is disabled for a project, remove the Studio upstream and replace
`location /` with an intentional `404` or a separate application route. Do not
point the project host at Manager or Provisioner.

## 5. Manager host remains separate

The Manager control plane uses its own server block and upstream:

```nginx
server {
    listen 443 ssl;
    server_name manager.example.com;

    ssl_certificate     /etc/ssl/supabase/manager.crt;
    ssl_certificate_key /etc/ssl/supabase/manager.key;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host              $host;
        proxy_set_header X-Real-IP         $remote_addr;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

Set Manager's `PUBLIC_ORIGIN` to `https://manager.example.com` and
`SECURE_COOKIES=true`. Provisioner port `9090` must remain on the private
Compose network and must not receive an Nginx server block.

## 6. Ports that are not part of this HTTPS host

PostgreSQL, Supavisor session/transaction ports, and any explicitly enabled
direct database port are TCP services. They are not covered by the HTTP server
block above and should remain private by default. If a controlled external TCP
connection is required, use a separate private-network, Nginx `stream`, or
Cloudflare Spectrum/Tunnel design with its own authentication and firewall
policy; never expose these ports through the normal orange-cloud HTTP record.

## 7. Applying and validating changes

After creating or changing a project's domain or allocated ports:

```sh
sudo nginx -t
sudo systemctl reload nginx

curl -I https://bee.supabase.example.com/
curl -fsS https://bee.supabase.example.com/auth/v1/settings
```

For Realtime, verify the browser WebSocket connection to
`wss://bee.supabase.example.com/realtime/v1/websocket`.

When a project is deleted, remove its server block and reload Nginx only after
the project containers are stopped. When a port changes, update the upstream
before publishing the new project generation.

## Current Manager limitation

The current Manager renderer generates the project domain and loopback port
bindings, but it does not yet write or reload the host's Nginx configuration.
Until a proxy-registration adapter is added, this file must be maintained by
the host operator (or by an external deployment script). The project is not
publicly reachable merely because `General.Domain` was saved in Manager.
