#!/usr/bin/env bash
set -euo pipefail

REPOSITORY_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

# Nginx workers must be able to traverse this directory to read each
# project-specific htpasswd file.
grep -Fqx 'install -d -m 0755 "$AUTH_DIRECTORY"' \
  "$REPOSITORY_ROOT/scripts/install-nginx-proxy-agent.sh"

echo "nginx proxy agent installer permissions test passed"
