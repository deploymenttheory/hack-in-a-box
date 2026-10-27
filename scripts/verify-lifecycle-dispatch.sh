#!/bin/sh
# Real native worker outage/restart at the agreed dispatch boundary. No SQL setup.
set -eu
umask 077

task_root=$(git -C "$(dirname "$0")/.." rev-parse --show-toplevel)
if [ -n "$(git -C "$task_root" status --porcelain)" ]; then
  printf '%s\n' 'Run from a clean checkout so the source receipt identifies the built code.' >&2
  exit 1
fi
source_revision=$(git -C "$task_root" rev-parse HEAD)
image_reference=${HACKBOX_LIFECYCLE_IMAGE:-hackbox-lifecycle:dispatch-window}
evidence_root=$(mktemp -d "${TMPDIR:-/tmp}/hackbox-dispatch.XXXXXX")
project_name=
runner_name=
case_dir=
case_captured=0

compose() {
  HACKBOX_LIFECYCLE_IMAGE="$image_reference" HACKBOX_LIFECYCLE_DISPATCH_CASE="$case_name" \
    docker compose -p "$project_name" -f "$task_root/docs/verification/lifecycle/compose.yaml" --profile dispatch-window "$@"
}

check_disk_space() {
  disk_phase=$1
  required_kib=$2
  # Read the Docker VM filesystem through a disposable pinned image. No host
  # volume or application data is mounted; --rm removes its empty probe volume.
  available_kib=$(docker run --rm --network none --read-only --cap-drop ALL \
    --security-opt no-new-privileges:true --entrypoint df \
    postgres:17.7-bookworm@sha256:86e0b703649d7a792bd9243ee28afc9d8f7c6b2b5638077c9d6882d4d472bbfd \
    -Pk / 2>> "$evidence_root/disk-space.log" | awk 'NR == 2 { print $4 }')
  case "$available_kib" in
    ''|*[!0-9]*) printf '%s\n' 'Cannot verify Docker disk capacity.' >&2; exit 1 ;;
  esac
  printf '%s available_kib=%s required_kib=%s\n' "$disk_phase" "$available_kib" "$required_kib" >> "$evidence_root/disk-space.log"
  if [ "$available_kib" -lt "$required_kib" ]; then
    printf 'Insufficient Docker disk space at %s: %s KiB available, %s KiB required.\n' "$disk_phase" "$available_kib" "$required_kib" >&2
    exit 1
  fi
}

capture_case() {
  [ -n "$project_name" ] || return 0
  [ "$case_captured" = 0 ] || return 0
  case_captured=1
  docker logs "$runner_name" > "$case_dir/runner.log" 2>&1 || true
  docker cp "$runner_name:/artifacts/." "$case_dir/" > "$case_dir/copy.log" 2>&1 || true
  docker exec "${project_name}-postgres-1" pg_dump -U lifecycle -d lifecycle -Fc > "$case_dir/final.dump" 2> "$case_dir/dump.log" || true
  compose logs --no-color > "$case_dir/services.log" 2>&1 || true
}

cleanup_case() {
  [ -n "$project_name" ] || return 0
  if ! compose down --volumes --remove-orphans > "$case_dir/cleanup.log" 2>&1; then
    printf 'Fixture cleanup failed: %s\n' "$project_name" >&2
    return 1
  fi
  if [ -n "$(docker ps -aq --filter "label=com.docker.compose.project=$project_name")" ] ||
     [ -n "$(docker volume ls -q --filter "label=com.docker.compose.project=$project_name")" ] ||
     [ -n "$(docker network ls -q --filter "label=com.docker.compose.project=$project_name")" ]; then
    printf 'Fixture resources remain after cleanup: %s\n' "$project_name" >&2
    return 1
  fi
  project_name=
}

finish() {
  result=$?
  set +e
  capture_case
  if ! cleanup_case; then result=1; fi
  printf 'Private dispatch evidence: %s\n' "$evidence_root"
  exit "$result"
}
trap finish EXIT
trap 'exit 130' INT TERM

check_disk_space before-build 8388608
docker build -f "$task_root/docs/verification/lifecycle/Dockerfile" -t "$image_reference" "$task_root" > "$evidence_root/build.log" 2>&1
image_id=$(docker image inspect --format '{{.Id}}' "$image_reference")
check_disk_space after-build 4194304

for case_name in last-on-time first-late; do
  check_disk_space "before-$case_name" 4194304
  case_dir="$evidence_root/$case_name"
  mkdir -m 700 "$case_dir"
  case_captured=0
  project_name="hackbox-dispatch-${case_name}-$(date +%s)-$$"
  runner_name="${project_name}-runner"
  printf 'Running native dispatch case: %s\n' "$case_name"
  compose up -d --wait --wait-timeout 60 --no-build public admin > "$case_dir/startup.log" 2>&1
  if [ -n "$(docker ps -aq --filter "label=com.docker.compose.project=$project_name" --filter label=com.docker.compose.service=worker)" ]; then
    printf '%s\n' 'A worker exists before the dispatch barrier; refusing a false outage proof.' >&2
    exit 1
  fi
  compose run -d --no-deps --name "$runner_name" dispatch-window > "$case_dir/runner-id.txt" 2>&1
  attempts=0
  until docker exec "$runner_name" test -s /artifacts/dispatch-ready.json >/dev/null 2>&1; do
    if [ "$(docker inspect --format '{{.State.Running}}' "$runner_name")" != true ]; then
      printf 'Native runner stopped before its ready barrier: %s\n' "$case_name" >&2
      exit 1
    fi
    attempts=$((attempts + 1))
    if [ "$attempts" -ge 60 ]; then
      printf 'Native runner did not reach its dispatch barrier: %s\n' "$case_name" >&2
      exit 1
    fi
    sleep 1
  done
  compose up -d --no-build worker > "$case_dir/worker-start.log" 2>&1
  runner_exit=$(docker wait "$runner_name")
  capture_case
  python3 - "$case_dir/source.json" "$source_revision" "$image_id" "$case_name" "$project_name" "$runner_exit" <<'PY'
import json, pathlib, sys
path, source, image, case, project, result = sys.argv[1:]
pathlib.Path(path).write_text(json.dumps({
    'source_revision': source, 'source_worktree_clean': True,
    'verification_image': image, 'case': case, 'project': project,
    'worker_absent_before_ready_barrier': True,
    'actual_runner_exit_code': int(result), 'complete_lifecycle': False,
}, indent=2) + '\n')
PY
  if [ "$runner_exit" != 0 ]; then
    printf 'Native dispatch case failed: %s (exit %s)\n' "$case_name" "$runner_exit" >&2
    exit 1
  fi
  if [ ! -s "$case_dir/dispatch-window-proof.json" ] || [ ! -s "$case_dir/final.dump" ] || [ ! -s "$case_dir/intent-journal.jsonl" ]; then
    printf 'Required native evidence is missing for passing case: %s\n' "$case_name" >&2
    exit 1
  fi
  cleanup_case
done

if [ "$(git -C "$task_root" rev-parse HEAD)" != "$source_revision" ] || [ -n "$(git -C "$task_root" status --porcelain)" ]; then
  printf '%s\n' 'Source changed while the native dispatch cases ran; do not claim a clean source receipt.' >&2
  exit 1
fi
printf '%s\n' 'Both native dispatch boundary cases passed; full lifecycle acceptance remains open.'
