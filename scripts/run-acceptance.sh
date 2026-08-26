#!/usr/bin/env bash
set -euo pipefail

# Starts an isolated disposable control plane, runs the real HTTP acceptance,
# and removes only the Compose project it created. No command tracing is used:
# generated credentials never appear in shell output.
repo_root="$(cd "$(dirname "$0")/.." && pwd)"
acceptance_root="$(mktemp -d "${TMPDIR:-/tmp}/supabase-manager-acceptance.XXXXXX")"
env_file="$acceptance_root/.env"
compose_project="${SUPABASE_MANAGER_E2E_COMPOSE_PROJECT:-supabase-manager-task10-acceptance}"
manager_port="${SUPABASE_MANAGER_E2E_MANAGER_PORT:-18081}"

cleanup() {
	if [[ -f "$env_file" ]]; then
		docker compose -p "$compose_project" -f "$repo_root/deploy/docker-compose.yml" --env-file "$env_file" down >/dev/null 2>&1 || true
	fi
	rm -rf "$acceptance_root"
}
trap cleanup EXIT

install -m 600 /dev/null "$env_file"
master_key="$(openssl rand -base64 32)"
manager_token="$(openssl rand -hex 32)"
project_root="$acceptance_root/projects"
mkdir -p "$project_root"
cat >"$env_file" <<EOF
MASTER_ENCRYPTION_KEY=$master_key
PROVISIONER_TOKEN=$manager_token
MANAGER_PORT=$manager_port
PUBLIC_ORIGIN=http://127.0.0.1:$manager_port
PROJECT_ROOT=$project_root
EOF
chmod 600 "$env_file"
unset master_key manager_token

docker compose -p "$compose_project" -f "$repo_root/deploy/docker-compose.yml" --env-file "$env_file" up -d --build --wait
export SUPABASE_MANAGER_E2E_URL="http://127.0.0.1:$manager_port"
export SUPABASE_MANAGER_E2E_USERNAME="${SUPABASE_MANAGER_E2E_USERNAME:?set administrator username for acceptance}"
export SUPABASE_MANAGER_E2E_PASSWORD="${SUPABASE_MANAGER_E2E_PASSWORD:?set administrator password for acceptance}"
export SUPABASE_MANAGER_E2E_COMPOSE_PROJECT="$compose_project"
go test -tags=integration ./tests/integration -run TestConfigurationReconcile -v -timeout 45m
