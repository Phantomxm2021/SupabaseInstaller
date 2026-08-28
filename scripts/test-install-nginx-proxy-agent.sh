#!/usr/bin/env bash
set -euo pipefail

REPOSITORY_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

# Nginx workers must be able to traverse these directories to read each
# project-specific htpasswd file.
grep -Fqx 'install -d -m 0711 /etc/supabase-manager' \
  "$REPOSITORY_ROOT/scripts/install-nginx-proxy-agent.sh"
grep -Fqx 'install -d -m 0755 "$AUTH_DIRECTORY"' \
  "$REPOSITORY_ROOT/scripts/install-nginx-proxy-agent.sh"
grep -Fqx 'chmod 0711 /etc/supabase-manager' \
  "$REPOSITORY_ROOT/scripts/install-nginx-proxy-agent.sh"
grep -Fqx 'chmod 0755 "$AUTH_DIRECTORY"' \
  "$REPOSITORY_ROOT/scripts/install-nginx-proxy-agent.sh"
grep -Fqx 'find "$AUTH_DIRECTORY" -maxdepth 1 -type f -name '\''supabase-manager-*.htpasswd'\'' -exec chmod 0644 {} +' \
  "$REPOSITORY_ROOT/scripts/install-nginx-proxy-agent.sh"

echo "nginx proxy agent installer permissions test passed"
