#!/usr/bin/env bash
set -euo pipefail

# Starts an isolated disposable control plane, runs the real HTTP acceptance,
# and removes only the Compose project it created. No command tracing is used:
# generated credentials never appear in shell output.
repo_root="$(cd "$(dirname "$0")/.." && pwd)"
acceptance_root="$(mktemp -d "${TMPDIR:-/tmp}/supabase-manager-acceptance.XXXXXX")"
env_file="$acceptance_root/.env"
compose_project="${SUPABASE_MANAGER_E2E_COMPOSE_PROJECT:-supabase-manager-task10-acceptance-$(date +%s)-$$}"
manager_port="${SUPABASE_MANAGER_E2E_MANAGER_PORT:-18081}"
runtime_project_file="$acceptance_root/runtime-project"

assert_no_compose_resources() {
	local project="$1"
	local containers networks volumes
	containers="$(docker ps -aq --filter "label=com.docker.compose.project=$project")"
	networks="$(docker network ls -q --filter "label=com.docker.compose.project=$project")"
	volumes="$(docker volume ls -q --filter "label=com.docker.compose.project=$project")"
	if [[ -n "$containers" || -n "$networks" || -n "$volumes" ]]; then
		echo "acceptance cleanup left resources for Compose project $project" >&2
		return 1
	fi
}

cleanup() {
	if [[ -f "$env_file" ]]; then
		docker compose -p "$compose_project" -f "$repo_root/deploy/docker-compose.yml" --env-file "$env_file" down -v --remove-orphans >/dev/null 2>&1 || true
	fi
	assert_no_compose_resources "$compose_project" || true
	if [[ -f "$runtime_project_file" ]]; then
		runtime_project="$(<"$runtime_project_file")"
		while IFS= read -r container; do
			[[ -z "$container" ]] || docker rm -f "$container" >/dev/null 2>&1 || true
		done < <(docker ps -aq --filter "label=com.docker.compose.project=$runtime_project")
		while IFS= read -r network; do
			[[ -z "$network" ]] || docker network rm "$network" >/dev/null 2>&1 || true
		done < <(docker network ls -q --filter "label=com.docker.compose.project=$runtime_project")
		while IFS= read -r volume; do
			[[ -z "$volume" ]] || docker volume rm "$volume" >/dev/null 2>&1 || true
		done < <(docker volume ls -q --filter "label=com.docker.compose.project=$runtime_project")
		assert_no_compose_resources "$runtime_project" || true
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

compose_up_args=(up -d --wait)
if [[ "${SUPABASE_MANAGER_E2E_BUILD:-1}" == "1" ]]; then
	compose_up_args+=(--build)
fi
docker compose -p "$compose_project" -f "$repo_root/deploy/docker-compose.yml" --env-file "$env_file" "${compose_up_args[@]}"
export SUPABASE_MANAGER_E2E_URL="http://127.0.0.1:$manager_port"
if [[ -z "${SUPABASE_MANAGER_E2E_USERNAME:-}" ]]; then
	export SUPABASE_MANAGER_E2E_USERNAME="task10-acceptance-$(date +%s)-$$"
fi
if [[ -z "${SUPABASE_MANAGER_E2E_PASSWORD:-}" ]]; then
	export SUPABASE_MANAGER_E2E_PASSWORD="$(openssl rand -hex 32)"
fi
export SUPABASE_MANAGER_E2E_COMPOSE_PROJECT="$compose_project"
export SUPABASE_MANAGER_E2E_RUNTIME_PROJECT_FILE="$runtime_project_file"
go test -tags=integration ./tests/integration -run TestConfigurationReconcile -v -timeout 45m
