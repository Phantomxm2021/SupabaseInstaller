#!/bin/sh
set -eu
IMAGE="supabase/edge-runtime:v1.74.0"
DIGEST="sha256:2781daf92394db91f7e94129cc3d04ec474ad16a8fe64b3fbeef6e7d557ab120"
REPO_ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
VERIFY_TMP=$(mktemp -d "${TMPDIR:-/tmp}/verify-edge-event-worker.XXXXXX")
BOUND_OUTPUT="$VERIFY_TMP/output"
CONTAINER="supabase-event-worker-verify-$$"

run_bounded() {
  seconds=$1; shift; : >"$BOUND_OUTPUT"
  "$@" >"$BOUND_OUTPUT" 2>&1 & pid=$!
  steps=$((seconds * 4)); step=0
  while kill -0 "$pid" 2>/dev/null; do
    if [ "$step" -ge "$steps" ]; then
      kill "$pid" 2>/dev/null || :; wait "$pid" 2>/dev/null || :; return 124
    fi
    step=$((step + 1)); sleep 0.25
  done
  wait "$pid"
}
cleanup() {
  if command -v docker >/dev/null 2>&1; then
    if run_bounded 5 docker rm -f "$CONTAINER"; then :; else :; fi
  fi
  rm -rf "$VERIFY_TMP"
}
trap cleanup EXIT HUP INT TERM

if ! command -v docker >/dev/null 2>&1; then echo "SKIP verify-edge-event-worker: Docker is unavailable"; exit 0; fi
if run_bounded 10 docker info; then :; else echo "SKIP verify-edge-event-worker: Docker daemon is unavailable"; exit 0; fi
if run_bounded 10 docker image inspect "$IMAGE"; then :; else
  inspect_status=$?
  if [ "$inspect_status" -eq 124 ]; then echo "timed out inspecting $IMAGE" >&2; exit 1; fi
  if run_bounded 120 docker pull "$IMAGE"; then :; else
    pull_status=$?; cat "$BOUND_OUTPUT" >&2
    if [ "$pull_status" -eq 124 ]; then echo "timed out pulling $IMAGE" >&2; else echo "failed to pull $IMAGE" >&2; fi
    exit 1
  fi
fi
if ! run_bounded 10 docker image inspect "$IMAGE" --format '{{join .RepoDigests " "}}'; then cat "$BOUND_OUTPUT" >&2; exit 1; fi
case "$(cat "$BOUND_OUTPUT")" in *"@$DIGEST"*) ;; *) echo "edge-runtime digest mismatch" >&2; exit 1;; esac

cat >"$VERIFY_TMP/index.ts" <<'EOF'
console.log("EDGE_EVENT_WORKER_MAIN_READY");
Deno.serve(() => new Response("ok"));
EOF
if run_bounded 20 docker run -d --name "$CONTAINER" -v "$VERIFY_TMP:/main:ro" -v "$REPO_ROOT/internal/templates/manager/function-logs/event-worker:/event-worker:ro" "$IMAGE" start --main-service /main --event-worker /event-worker; then :; else
  start_status=$?; cat "$BOUND_OUTPUT" >&2
  if [ "$start_status" -eq 124 ]; then echo "edge-runtime startup timed out" >&2; else echo "edge-runtime startup failed" >&2; fi
  exit 1
fi
ready=false; attempt=0
while [ "$attempt" -lt 40 ]; do
  if ! run_bounded 5 docker logs "$CONTAINER"; then exit 1; fi
  if grep -Fq EDGE_EVENT_WORKER_MAIN_READY "$BOUND_OUTPUT"; then ready=true; break; fi
  if ! run_bounded 5 docker inspect "$CONTAINER" --format '{{.State.Running}}'; then exit 1; fi
  if [ "$(cat "$BOUND_OUTPUT")" != true ]; then echo "runtime exited during startup" >&2; exit 1; fi
  attempt=$((attempt + 1)); sleep 0.25
done
if [ "$ready" != true ]; then echo "event-worker startup timed out" >&2; exit 1; fi
if ! run_bounded 10 docker rm -f "$CONTAINER"; then exit 1; fi

CONTAINER="supabase-event-worker-fixtures-$$"
if run_bounded 20 docker run -d --name "$CONTAINER" -e FUNCTION_LOG_VERIFY_FIXTURES=/fixtures -v "$REPO_ROOT/internal/templates/manager/function-logs/event-worker:/adapter:ro" -v "$REPO_ROOT/internal/templates/manager/function-logs/event-worker/fixtures:/fixtures:ro" "$IMAGE" start --main-service /adapter; then :; else
  fixture_start_status=$?
  if [ "$fixture_start_status" -eq 124 ]; then echo "fixture runtime startup timed out" >&2; else echo "fixture runtime failed to start" >&2; fi
  exit 1
fi
attempt=0; records=""
while [ "$attempt" -lt 40 ]; do
  if ! run_bounded 5 docker logs "$CONTAINER"; then echo "failed to read fixture logs" >&2; exit 1; fi
  records=$(cat "$BOUND_OUTPUT")
  if printf '%s\n' "$records" | grep -Fq FUNCTION_LOG_FIXTURE_RECORDS=; then break; fi
  if ! run_bounded 5 docker inspect "$CONTAINER" --format '{{.State.Running}}'; then exit 1; fi
  if [ "$(cat "$BOUND_OUTPUT")" != true ]; then echo "fixture runtime failed before records" >&2; exit 1; fi
  attempt=$((attempt + 1)); sleep 0.25
done
if ! printf '%s\n' "$records" | grep -Fq FUNCTION_LOG_FIXTURE_RECORDS=; then echo "fixture run returned no records" >&2; exit 1; fi
for expected in \
  277b9cfd8b07e451a7d27656ca57416ae743d1d0c0c17b63c639b921a4844288 \
  ed79c198aa6af2159f0b449644d4333a441ce8a2f8f0237fc8157c70e224bae6 \
  00647ff3976a9d1e0f7ec41a6b44eae4095aa21c71c1dc86d43a2c7c60b738c1 \
  '"name":"warn-u2028","eventId":"9f053d87d5ee7e902d32a426ef041c8d4d2a58279914038543256b58821f4dea","level":"warn"' \
  '"name":"warning-u2028","eventId":"d12a29e976e73eca17fcbb45adc2aa6db63c6967bdfc88ffe77a04b152fcf458","level":"warn"' \
  '"name":"fractional","error":"incompatible"' '"name":"unsafe","error":"incompatible"' \
  '"name":"unicode-attribute","error":"incompatible"' '"name":"nonstring-attribute","error":"incompatible"'
do
  if ! printf '%s\n' "$records" | grep -Fq "$expected"; then echo "fixture mismatch: $expected" >&2; exit 1; fi
done
if ! run_bounded 10 docker rm -f "$CONTAINER"; then exit 1; fi

for mode in constructor iterator; do
  CONTAINER="supabase-event-worker-$mode-$$"
  if ! run_bounded 20 docker run -d --name "$CONTAINER" -e FUNCTION_LOG_VERIFY_EVENT_MANAGER_FAILURE="$mode" -v "$REPO_ROOT/internal/templates/manager/function-logs/event-worker:/adapter:ro" "$IMAGE" start --main-service /adapter; then exit 1; fi
  inert=false; attempt=0
  while [ "$attempt" -lt 20 ]; do
    if ! run_bounded 5 docker logs "$CONTAINER"; then exit 1; fi
    if grep -Fq 'FUNCTION_LOG_EVENT_MANAGER_INERT timers=0 ticks=0' "$BOUND_OUTPUT"; then inert=true; break; fi
    attempt=$((attempt + 1)); sleep 0.25
  done
  if [ "$inert" != true ]; then echo "$mode failure escaped compatibility boundary" >&2; exit 1; fi
  if ! run_bounded 5 docker inspect "$CONTAINER" --format '{{.State.Running}}'; then exit 1; fi
  if [ "$(cat "$BOUND_OUTPUT")" != true ]; then echo "$mode failure stopped runtime" >&2; exit 1; fi
  if ! run_bounded 10 docker rm -f "$CONTAINER"; then exit 1; fi
done
echo "verified Edge Runtime v1.74.0 startup, canonical parity, and inert failures"
