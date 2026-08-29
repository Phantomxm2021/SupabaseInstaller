#!/usr/bin/env bash
set -euo pipefail

REPOSITORY_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
TEMPORARY_DIRECTORY=$(mktemp -d)
trap 'rm -rf "$TEMPORARY_DIRECTORY"' EXIT

file_mode() {
  if stat -c '%a' "$1" >/dev/null 2>&1; then
    stat -c '%a' "$1"
  else
    stat -f '%Lp' "$1"
  fi
}

TEST_REPOSITORY="$TEMPORARY_DIRECTORY/repository"
TEST_BIN="$TEMPORARY_DIRECTORY/bin"
TEST_LOG="$TEMPORARY_DIRECTORY/commands.log"
mkdir -p "$TEST_REPOSITORY/deploy" "$TEST_REPOSITORY/scripts" "$TEST_BIN"
cp "$REPOSITORY_ROOT/deploy/.env.example" "$TEST_REPOSITORY/deploy/.env.example"
cp "$REPOSITORY_ROOT/deploy/docker-compose.yml" "$TEST_REPOSITORY/deploy/docker-compose.yml"
cp "$REPOSITORY_ROOT/scripts/install-supabase-manager.sh" "$TEST_REPOSITORY/scripts/install-supabase-manager.sh"

cat >"$TEST_REPOSITORY/scripts/install-nginx-proxy-agent.sh" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
printf 'agent %s\n' "$*" >>"$SUPABASE_MANAGER_TEST_LOG"
SCRIPT
chmod 0755 "$TEST_REPOSITORY/scripts/install-nginx-proxy-agent.sh"

HELP_OUTPUT=$(bash "$TEST_REPOSITORY/scripts/install-supabase-manager.sh" --help)
printf '%s\n' "$HELP_OUTPUT" | grep -F -- 'Existing valid secrets and server data are kept.'
printf '%s\n' "$HELP_OUTPUT" | grep -F -- '--project-root ABSOLUTE_PATH    Host directory for Supabase server data.'

cat >"$TEST_BIN/docker" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
printf 'docker %s\n' "$*" >>"$SUPABASE_MANAGER_TEST_LOG"
SCRIPT
chmod 0755 "$TEST_BIN/docker"

cat >"$TEST_BIN/systemctl" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
printf 'systemctl %s\n' "$*" >>"$SUPABASE_MANAGER_TEST_LOG"
SCRIPT
chmod 0755 "$TEST_BIN/systemctl"

cat >"$TEST_BIN/nginx" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
printf 'nginx %s\n' "$*" >>"$SUPABASE_MANAGER_TEST_LOG"
SCRIPT
chmod 0755 "$TEST_BIN/nginx"

CERTIFICATE_FILE="$TEMPORARY_DIRECTORY/origin.pem"
CERTIFICATE_KEY_FILE="$TEMPORARY_DIRECTORY/origin.key"
touch "$CERTIFICATE_FILE" "$CERTIFICATE_KEY_FILE"

PATH="$TEST_BIN:$PATH" \
SUPABASE_MANAGER_TEST_LOG="$TEST_LOG" \
SUPABASE_MANAGER_TEST_SKIP_HOST_CHECKS=1 \
bash "$TEST_REPOSITORY/scripts/install-supabase-manager.sh" \
  --non-interactive \
  --public-origin https://manager.example.test \
  --certificate-file "$CERTIFICATE_FILE" \
  --certificate-key-file "$CERTIFICATE_KEY_FILE" \
  --project-root "$TEMPORARY_DIRECTORY/projects"

ENVIRONMENT_FILE="$TEST_REPOSITORY/deploy/.env"
test -f "$ENVIRONMENT_FILE"
test "$(file_mode "$ENVIRONMENT_FILE")" = 600
grep -Eq '^MASTER_ENCRYPTION_KEY=.{32,}$' "$ENVIRONMENT_FILE"
grep -Eq '^PROVISIONER_TOKEN=.{32,}$' "$ENVIRONMENT_FILE"
grep -Eq '^NGINX_PROXY_TOKEN=.{32,}$' "$ENVIRONMENT_FILE"
grep -qx 'NGINX_PROXY_MODE=managed' "$ENVIRONMENT_FILE"
grep -q '^agent ' "$TEST_LOG"
grep -q 'docker compose .* up -d --build --wait' "$TEST_LOG"
grep -q 'systemctl is-active --quiet supabase-manager-nginx-proxy-agent.service' "$TEST_LOG"
grep -q 'nginx -t' "$TEST_LOG"
docker compose -f "$TEST_REPOSITORY/deploy/docker-compose.yml" --env-file "$ENVIRONMENT_FILE" config --quiet
docker compose -f "$TEST_REPOSITORY/deploy/docker-compose.yml" --env-file "$ENVIRONMENT_FILE" config \
  | grep -q 'NGINX_PROXY_MODE: managed'
docker compose -f "$TEST_REPOSITORY/deploy/docker-compose.yml" --env-file "$ENVIRONMENT_FILE" config \
  | grep -q 'source: /run/supabase-manager'

ORIGINAL_PROVISIONER_TOKEN=$(sed -n 's/^PROVISIONER_TOKEN=//p' "$ENVIRONMENT_FILE")
PATH="$TEST_BIN:$PATH" \
SUPABASE_MANAGER_TEST_LOG="$TEST_LOG" \
SUPABASE_MANAGER_TEST_SKIP_HOST_CHECKS=1 \
bash "$TEST_REPOSITORY/scripts/install-supabase-manager.sh" \
  --non-interactive \
  --public-origin https://manager.example.test \
  --certificate-file "$CERTIFICATE_FILE" \
  --certificate-key-file "$CERTIFICATE_KEY_FILE"
test "$ORIGINAL_PROVISIONER_TOKEN" = "$(sed -n 's/^PROVISIONER_TOKEN=//p' "$ENVIRONMENT_FILE")"

MISSING_CERTIFICATE_LOG="$TEMPORARY_DIRECTORY/missing-certificate.log"
if PATH="$TEST_BIN:$PATH" \
  SUPABASE_MANAGER_TEST_LOG="$MISSING_CERTIFICATE_LOG" \
  SUPABASE_MANAGER_TEST_SKIP_HOST_CHECKS=1 \
  bash "$TEST_REPOSITORY/scripts/install-supabase-manager.sh" \
    --non-interactive \
    --public-origin https://manager.example.test \
    --certificate-file "$TEMPORARY_DIRECTORY/missing.pem" \
    --certificate-key-file "$CERTIFICATE_KEY_FILE"; then
  echo "installer accepted a missing certificate" >&2
  exit 1
fi
test ! -e "$MISSING_CERTIFICATE_LOG"

echo "one-command installer smoke test passed"
