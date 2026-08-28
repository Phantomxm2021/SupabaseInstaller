#!/usr/bin/env bash
set -euo pipefail

REPOSITORY_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
ENVIRONMENT_FILE="$REPOSITORY_ROOT/deploy/.env"
PUBLIC_ORIGIN=""
CERTIFICATE_FILE=""
CERTIFICATE_KEY_FILE=""
PROJECT_ROOT=""
NON_INTERACTIVE=false
FORCE=false
TEST_MODE=${SUPABASE_MANAGER_TEST_SKIP_HOST_CHECKS:-0}

usage() {
  cat <<'EOF'
Usage: sudo ./scripts/install-supabase-manager.sh [options]

Install or upgrade Supabase Manager, its private Provisioner, and the managed
Nginx proxy agent on Ubuntu. Existing valid secrets and project data are kept.

Options:
  --non-interactive               Require all configuration values as flags.
  --public-origin URL             Public Manager URL, such as https://manager.example.com.
  --certificate-file PATH         Cloudflare Origin Certificate PEM path.
  --certificate-key-file PATH     Cloudflare Origin Certificate private-key path.
  --project-root ABSOLUTE_PATH    Host directory for Supabase project data.
  --force                         Replace conflicting explicit non-secret settings.
  --help                          Show this help text.
EOF
}

die() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

require_value() {
  local option=$1
  local value=${2:-}
  [[ -n "$value" ]] || die "$option requires a value"
}

while (($# > 0)); do
  case "$1" in
    --non-interactive) NON_INTERACTIVE=true ;;
    --public-origin) shift; require_value --public-origin "${1:-}"; PUBLIC_ORIGIN=$1 ;;
    --certificate-file) shift; require_value --certificate-file "${1:-}"; CERTIFICATE_FILE=$1 ;;
    --certificate-key-file) shift; require_value --certificate-key-file "${1:-}"; CERTIFICATE_KEY_FILE=$1 ;;
    --project-root) shift; require_value --project-root "${1:-}"; PROJECT_ROOT=$1 ;;
    --force) FORCE=true ;;
    --help) usage; exit 0 ;;
    *) die "unknown option: $1" ;;
  esac
  shift
done

read_env_value() {
  local key=$1
  local file=${2:-$ENVIRONMENT_FILE}
  [[ -f "$file" ]] || return 0
  sed -n "s/^${key}=//p" "$file" | tail -n 1
}

is_placeholder() {
  local value=${1:-}
  [[ -z "$value" || "$value" == *replace-with* ]]
}

is_valid_origin() {
  [[ "$1" =~ ^https?://[^[:space:]/]+([/][^[:space:]]*)?$ ]]
}

detected_origin() {
  local address
  address=$(hostname -I 2>/dev/null | awk '{print $1}')
  if [[ -n "$address" ]]; then
    printf 'http://%s:8080' "$address"
  else
    printf 'http://localhost:8080'
  fi
}

prompt_value() {
  local label=$1
  local current=$2
  local answer
  read -r -p "$label [$current]: " answer
  printf '%s' "${answer:-$current}"
}

check_host() {
  if [[ "$TEST_MODE" == 1 ]]; then
    return 0
  fi
  [[ ${EUID} -eq 0 ]] || die "run this installer with sudo"
  [[ -f /etc/os-release ]] || die "Ubuntu is required"
  . /etc/os-release
  [[ ${ID:-} == ubuntu ]] || die "Ubuntu is required"
  command -v systemctl >/dev/null || die "systemd is required"
  systemctl --version >/dev/null || die "systemd is required"
}

install_prerequisites() {
  if [[ "$TEST_MODE" == 1 ]]; then
    return 0
  fi
  local -a packages=()
  command -v docker >/dev/null || packages+=(docker.io)
  command -v nginx >/dev/null || packages+=(nginx)
  command -v openssl >/dev/null || packages+=(openssl)
  if ! docker compose version >/dev/null 2>&1; then
    if apt-cache show docker-compose-v2 >/dev/null 2>&1; then
      packages+=(docker-compose-v2)
    elif apt-cache show docker-compose-plugin >/dev/null 2>&1; then
      packages+=(docker-compose-plugin)
    else
      die "Docker Compose v2 is unavailable from configured apt repositories"
    fi
  fi
  if ((${#packages[@]} > 0)); then
    apt-get update
    apt-get install -y "${packages[@]}"
  fi
  command -v docker >/dev/null || die "Docker installation failed"
  docker compose version >/dev/null || die "Docker Compose v2 installation failed"
  command -v nginx >/dev/null || die "Nginx installation failed"
  command -v openssl >/dev/null || die "OpenSSL installation failed"
  if systemctl list-unit-files docker.service --no-legend 2>/dev/null | grep -q '^docker\.service'; then
    systemctl enable --now docker.service
  fi
  docker info >/dev/null || die "Docker daemon is unavailable"
}

choose_value() {
  local key=$1
  local requested=$2
  local fallback=$3
  local existing
  existing=$(read_env_value "$key")
  if [[ -n "$requested" ]]; then
    if [[ -n "$existing" && "$existing" != "$requested" && "$FORCE" != true ]]; then
      die "$key already has a different value; rerun with --force to replace it"
    fi
    printf '%s' "$requested"
  elif [[ -n "$existing" ]]; then
    printf '%s' "$existing"
  else
    printf '%s' "$fallback"
  fi
}

replace_env_value() {
  local file=$1
  local key=$2
  local value=$3
  local temporary_file="$file.next"
  awk -v key="$key" -v value="$value" '
    index($0, key "=") == 1 {
      if (!seen) print key "=" value
      seen = 1
      next
    }
    { print }
    END { if (!seen) print key "=" value }
  ' "$file" >"$temporary_file"
  chmod 0600 "$temporary_file"
  mv "$temporary_file" "$file"
}

ensure_secret() {
  local file=$1
  local key=$2
  local generator=$3
  local current
  current=$(read_env_value "$key" "$file")
  if is_placeholder "$current"; then
    replace_env_value "$file" "$key" "$($generator)"
  fi
}

prepare_environment() {
  local temporary_file
  temporary_file=$(mktemp "$REPOSITORY_ROOT/deploy/.env.XXXXXX")
  trap 'rm -f "$temporary_file" "$temporary_file.next"' RETURN
  if [[ -f "$ENVIRONMENT_FILE" ]]; then
    cp "$ENVIRONMENT_FILE" "$temporary_file"
  else
    cp "$REPOSITORY_ROOT/deploy/.env.example" "$temporary_file"
  fi
  chmod 0600 "$temporary_file"
  ensure_secret "$temporary_file" MASTER_ENCRYPTION_KEY 'openssl rand -base64 32'
  ensure_secret "$temporary_file" PROVISIONER_TOKEN 'openssl rand -hex 32'
  ensure_secret "$temporary_file" NGINX_PROXY_TOKEN 'openssl rand -hex 32'
  replace_env_value "$temporary_file" PUBLIC_ORIGIN "$PUBLIC_ORIGIN"
  replace_env_value "$temporary_file" SECURE_COOKIES "$([[ "$PUBLIC_ORIGIN" == https://* ]] && printf true || printf false)"
  replace_env_value "$temporary_file" PROJECT_ROOT "$PROJECT_ROOT"
  replace_env_value "$temporary_file" NGINX_PROXY_MODE managed
  replace_env_value "$temporary_file" NGINX_PROXY_SOCKET /run/supabase-manager/nginx-proxy-agent.sock
  replace_env_value "$temporary_file" NGINX_PROXY_SOCKET_DIRECTORY /run/supabase-manager
  mv "$temporary_file" "$ENVIRONMENT_FILE"
  chmod 0600 "$ENVIRONMENT_FILE"
  trap - RETURN
}

resolve_inputs() {
  local previous_origin previous_project_root
  previous_origin=$(read_env_value PUBLIC_ORIGIN)
  previous_project_root=$(read_env_value PROJECT_ROOT)
  if [[ "$NON_INTERACTIVE" == false ]]; then
    PUBLIC_ORIGIN=$(prompt_value 'Manager public origin' "${PUBLIC_ORIGIN:-${previous_origin:-$(detected_origin)}}")
    CERTIFICATE_FILE=$(prompt_value 'Cloudflare Origin Certificate file' "${CERTIFICATE_FILE:-/etc/nginx/ssl/cloudflare-origin.pem}")
    CERTIFICATE_KEY_FILE=$(prompt_value 'Cloudflare Origin Certificate key file' "${CERTIFICATE_KEY_FILE:-/etc/nginx/ssl/cloudflare-origin.key}")
  fi
  PUBLIC_ORIGIN=$(choose_value PUBLIC_ORIGIN "$PUBLIC_ORIGIN" "${previous_origin:-}")
  PROJECT_ROOT=$(choose_value PROJECT_ROOT "$PROJECT_ROOT" "${previous_project_root:-/opt/supabase-manager/projects}")
  is_valid_origin "$PUBLIC_ORIGIN" || die "--public-origin must be an absolute http:// or https:// URL"
  [[ "$PROJECT_ROOT" == /* ]] || die "--project-root must be an absolute path"
  [[ -n "$CERTIFICATE_FILE" && -n "$CERTIFICATE_KEY_FILE" ]] || die "certificate file and key file are required"
  [[ -f "$CERTIFICATE_FILE" ]] || die "certificate file not found: $CERTIFICATE_FILE"
  [[ -f "$CERTIFICATE_KEY_FILE" ]] || die "certificate key file not found: $CERTIFICATE_KEY_FILE"
}

install_agent() {
  NGINX_CERTIFICATE_FILE="$CERTIFICATE_FILE" \
  NGINX_CERTIFICATE_KEY_FILE="$CERTIFICATE_KEY_FILE" \
  "$REPOSITORY_ROOT/scripts/install-nginx-proxy-agent.sh" "$ENVIRONMENT_FILE"
}

verify_deployment() {
  systemctl is-active --quiet supabase-manager-nginx-proxy-agent.service
  if [[ "$TEST_MODE" != 1 ]]; then
    test -S /run/supabase-manager/nginx-proxy-agent.sock
  fi
  nginx -t
  docker compose -f "$REPOSITORY_ROOT/deploy/docker-compose.yml" --env-file "$ENVIRONMENT_FILE" ps --status running
}

main() {
  check_host
  install_prerequisites
  resolve_inputs
  mkdir -p "$PROJECT_ROOT"
  prepare_environment
  install_agent
  docker compose -f "$REPOSITORY_ROOT/deploy/docker-compose.yml" --env-file "$ENVIRONMENT_FILE" up -d --build --wait
  verify_deployment
  printf 'Supabase Manager is ready at %s\n' "$PUBLIC_ORIGIN"
}

main
