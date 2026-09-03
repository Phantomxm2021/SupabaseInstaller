#!/bin/sh
set -eu

IMAGE="supabase/edge-runtime:v1.74.0"
DIGEST="sha256:2781daf92394db91f7e94129cc3d04ec474ad16a8fe64b3fbeef6e7d557ab120"

if ! command -v docker >/dev/null 2>&1 || ! docker info >/dev/null 2>&1; then
  echo "SKIP verify-edge-event-worker: Docker is unavailable"
  exit 0
fi

if ! docker image inspect "$IMAGE" >/dev/null 2>&1; then
  docker pull "$IMAGE" >/dev/null
fi
case "$(docker image inspect "$IMAGE" --format '{{join .RepoDigests " "}}')" in
  *"@$DIGEST"*) ;;
  *) echo "edge-runtime image digest does not match $DIGEST" >&2; exit 1 ;;
esac

REPO_ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
VERIFY_TMP=$(mktemp -d "${TMPDIR:-/tmp}/verify-edge-event-worker.XXXXXX")
CONTAINER="supabase-event-worker-verify-$$"
cleanup() {
  docker rm -f "$CONTAINER" >/dev/null 2>&1 || true
  rm -rf "$VERIFY_TMP"
}
trap cleanup EXIT HUP INT TERM

cat >"$VERIFY_TMP/index.ts" <<'EOF'
console.log("EDGE_EVENT_WORKER_MAIN_READY");
Deno.serve(() => new Response("ok"));
EOF

docker run -d --name "$CONTAINER" \
  -v "$VERIFY_TMP:/main:ro" \
  -v "$REPO_ROOT/internal/templates/manager/function-logs/event-worker:/event-worker:ro" \
  "$IMAGE" start --main-service /main --event-worker /event-worker >/dev/null

ready=false
attempt=0
while [ "$attempt" -lt 40 ]; do
  logs=$(docker logs "$CONTAINER" 2>&1 || true)
  if printf '%s\n' "$logs" | grep -Fq 'EDGE_EVENT_WORKER_MAIN_READY'; then
    ready=true
    break
  fi
  if [ "$(docker inspect "$CONTAINER" --format '{{.State.Running}}')" != "true" ]; then
    printf '%s\n' "$logs" >&2
    echo "edge-runtime exited before event-worker startup completed" >&2
    exit 1
  fi
  attempt=$((attempt + 1))
  sleep 0.25
done
if [ "$ready" != true ]; then
  docker logs "$CONTAINER" >&2 || true
  echo "timed out waiting for pinned event-worker startup" >&2
  exit 1
fi
docker rm -f "$CONTAINER" >/dev/null

records=$(docker run --rm \
  -e FUNCTION_LOG_VERIFY_FIXTURES=/fixtures \
  -v "$REPO_ROOT/internal/templates/manager/function-logs/event-worker:/adapter:ro" \
  -v "$REPO_ROOT/internal/templates/manager/function-logs/event-worker/fixtures:/fixtures:ro" \
  "$IMAGE" start --main-service /adapter 2>&1 || true)
printf '%s\n' "$records" | grep -Fq 'FUNCTION_LOG_FIXTURE_RECORDS='
for expected in \
  '"eventId":"277b9cfd8b07e451a7d27656ca57416ae743d1d0c0c17b63c639b921a4844288","functionName":"contract-log","executionId":"e8a9d201-c618-4051-be3c-721a97fee216","eventType":"Boot","message":"","timestamp":"2026-09-03T20:51:33.319Z","level":"info"' \
  '"eventId":"ed79c198aa6af2159f0b449644d4333a441ce8a2f8f0237fc8157c70e224bae6","functionName":"contract-log","executionId":"e8a9d201-c618-4051-be3c-721a97fee216","eventType":"Log","message":"FUNCTION_LOG_FIXTURE_MESSAGE\n","timestamp":"2026-09-03T20:51:33.326Z","level":"info"' \
  '"eventId":"00647ff3976a9d1e0f7ec41a6b44eae4095aa21c71c1dc86d43a2c7c60b738c1","functionName":"contract-throw","executionId":"f7cb1a48-200e-4586-9ec2-5baa4250af84","eventType":"UncaughtException","message":"event loop error:' \
  '"timestamp":"2026-09-03T20:51:33.426Z","level":"error"'
do
  if ! printf '%s\n' "$records" | grep -Fq "$expected"; then
    printf '%s\n' "$records" >&2
    echo "adapter fixture output did not match expected Go contract: $expected" >&2
    exit 1
  fi
done

echo "verified Edge Runtime v1.74.0 event-worker startup and canonical fixture records"
