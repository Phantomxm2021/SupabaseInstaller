#!/usr/bin/env bash
set -euo pipefail

if [[ ${EUID} -ne 0 ]]; then
  echo "run this installer with sudo" >&2
  exit 1
fi

REPOSITORY_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
ENV_FILE=${1:-"$REPOSITORY_ROOT/deploy/.env"}
CERTIFICATE_FILE=${NGINX_CERTIFICATE_FILE:-/etc/nginx/ssl/cloudflare-origin.pem}
CERTIFICATE_KEY_FILE=${NGINX_CERTIFICATE_KEY_FILE:-/etc/nginx/ssl/cloudflare-origin.key}
SOCKET_PATH=/run/supabase-manager/nginx-proxy-agent.sock
AUTH_DIRECTORY=${NGINX_AUTH_DIRECTORY:-/etc/supabase-manager/nginx-auth}

if [[ ! -f "$ENV_FILE" ]]; then
  echo "environment file not found: $ENV_FILE" >&2
  exit 1
fi

read_env_value() {
  local key=$1
  sed -n "s/^${key}=//p" "$ENV_FILE" | tail -n 1
}

TOKEN=$(read_env_value NGINX_PROXY_TOKEN)
if [[ -z "$TOKEN" || ${#TOKEN} -lt 32 || "$TOKEN" == *replace-with* ]]; then
  echo "NGINX_PROXY_TOKEN in $ENV_FILE must be a generated 32+ byte secret" >&2
  exit 1
fi
if [[ ! -f "$CERTIFICATE_FILE" || ! -f "$CERTIFICATE_KEY_FILE" ]]; then
  echo "NGINX certificate or key file is missing; set NGINX_CERTIFICATE_FILE and NGINX_CERTIFICATE_KEY_FILE before installation" >&2
  exit 1
fi
if [[ "$AUTH_DIRECTORY" != /* ]]; then
  echo "NGINX_AUTH_DIRECTORY must be an absolute path" >&2
  exit 1
fi
if ! grep -Eq 'include[[:space:]]+/etc/nginx/sites-enabled/\*;' /etc/nginx/nginx.conf; then
  echo "/etc/nginx/nginx.conf does not include /etc/nginx/sites-enabled/*; configure that standard Nginx include first (this installer will not modify nginx.conf)" >&2
  exit 1
fi

TEMPORARY_DIRECTORY=$(mktemp -d)
trap 'rm -rf "$TEMPORARY_DIRECTORY"' EXIT
docker build \
  --file "$REPOSITORY_ROOT/deploy/Dockerfile.nginx-proxy-agent" \
  --target export \
  --output "type=local,dest=$TEMPORARY_DIRECTORY" \
  "$REPOSITORY_ROOT"

install -d -m 0750 /etc/supabase-manager /run/supabase-manager
install -d -m 0755 "$AUTH_DIRECTORY"
install -m 0755 "$TEMPORARY_DIRECTORY/nginx-proxy-agent" /usr/local/bin/nginx-proxy-agent
install -m 0644 "$REPOSITORY_ROOT/deploy/systemd/supabase-manager-nginx-proxy-agent.service" /etc/systemd/system/supabase-manager-nginx-proxy-agent.service

umask 077
cat > /etc/supabase-manager/nginx-proxy-agent.env <<EOF
NGINX_PROXY_SOCKET=$SOCKET_PATH
NGINX_PROXY_TOKEN=$TOKEN
NGINX_SITES_AVAILABLE=/etc/nginx/sites-available
NGINX_SITES_ENABLED=/etc/nginx/sites-enabled
NGINX_AUTH_DIRECTORY=$AUTH_DIRECTORY
NGINX_CERTIFICATE_FILE=$CERTIFICATE_FILE
NGINX_CERTIFICATE_KEY_FILE=$CERTIFICATE_KEY_FILE
NGINX_BINARY=/usr/sbin/nginx
SYSTEMCTL_BINARY=/bin/systemctl
EOF
chmod 0600 /etc/supabase-manager/nginx-proxy-agent.env

systemctl daemon-reload
systemctl enable supabase-manager-nginx-proxy-agent.service
# `enable --now` does not replace an already-running process. Explicitly
# restart so upgrades always activate the binary just installed above.
systemctl restart supabase-manager-nginx-proxy-agent.service
systemctl --no-pager --full status supabase-manager-nginx-proxy-agent.service
