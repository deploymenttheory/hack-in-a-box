#!/bin/sh
set -eu
umask 077
browser_root=$(git -C "$(dirname "$0")/.." rev-parse --show-toplevel)
cd "$browser_root"
test -z "$(git status --porcelain)"
browser_source=$(git rev-parse HEAD)
browser_evidence=$(mktemp -d "${TMPDIR:-/tmp}/hackbox-connected-browser.XXXXXX")
browser_project=hackbox-connected-browser-$(date +%s)
export HACKBOX_LIFECYCLE_BROWSER_PHASES=${HACKBOX_LIFECYCLE_BROWSER_PHASES:-planning}
export HACKBOX_LIFECYCLE_IMAGE=${HACKBOX_LIFECYCLE_IMAGE:-hackbox-lifecycle:connected-browser}
browser_guide_supplied=${HACKBOX_LIFECYCLE_GUIDE_IMAGE:-}
export HACKBOX_LIFECYCLE_GUIDE_IMAGE=${HACKBOX_LIFECYCLE_GUIDE_IMAGE:-hackbox-lifecycle:browser-guide}
browser_compose(){ docker compose -p "$browser_project" -f "$browser_root/docs/verification/lifecycle/compose.yaml" --profile browser --profile journey "$@"; }
browser_finish(){
 browser_exit=$?
 set +e
 if [ -n "${browser_runner:-}" ]; then
  docker logs "$browser_runner" > "$browser_evidence/native-runner.log" 2>&1
  docker cp "$browser_runner:/artifacts/." "$browser_evidence/native/" > "$browser_evidence/copy.log" 2>&1
 fi
 browser_compose logs --no-color > "$browser_evidence/services.log" 2>&1
 browser_compose down --volumes --remove-orphans > "$browser_evidence/cleanup.log" 2>&1 || browser_exit=1
 for resource in container volume network; do
  case "$resource" in
   container) browser_remaining=$(docker ps -aq --filter "label=com.docker.compose.project=$browser_project");;
   volume) browser_remaining=$(docker volume ls -q --filter "label=com.docker.compose.project=$browser_project");;
   network) browser_remaining=$(docker network ls -q --filter "label=com.docker.compose.project=$browser_project");;
  esac
  if [ -n "$browser_remaining" ];then browser_exit=1;fi
 done
 printf 'Private browser evidence: %s\n' "$browser_evidence"
 exit "$browser_exit"
}
trap browser_finish EXIT
trap 'exit 130' INT TERM
browser_free_kib=$(docker run --rm --network none --read-only --cap-drop ALL --entrypoint df postgres:17.7-bookworm -Pk / | awk 'NR==2 {print $4}')
test "$browser_free_kib" -ge 8388608
npm --prefix "$browser_root/web" ci > "$browser_evidence/browser-dependencies.log" 2>&1
"$browser_root/web/node_modules/.bin/playwright" install chromium > "$browser_evidence/browser-install.log" 2>&1
if [ -z "$browser_guide_supplied" ]; then
 docker build --target styleguide --label "org.opencontainers.image.revision=$browser_source" --build-arg "STYLEGUIDE_REVISION=$browser_source" -t "$HACKBOX_LIFECYCLE_GUIDE_IMAGE" "$browser_root" > "$browser_evidence/guide-build.log" 2>&1
fi
browser_guide_source=$(docker image inspect --format '{{index .Config.Labels "org.opencontainers.image.revision"}}' "$HACKBOX_LIFECYCLE_GUIDE_IMAGE")
test "${#browser_guide_source}" -eq 40
git diff --exit-code "$browser_guide_source" "$browser_source" -- web > "$browser_evidence/guide-source-comparison.log"
browser_guide_image=$(docker image inspect --format '{{.Id}}' "$HACKBOX_LIFECYCLE_GUIDE_IMAGE")
docker build -f "$browser_root/docs/verification/lifecycle/Dockerfile" -t "$HACKBOX_LIFECYCLE_IMAGE" "$browser_root" > "$browser_evidence/build.log" 2>&1
browser_image=$(docker image inspect --format '{{.Id}}' "$HACKBOX_LIFECYCLE_IMAGE")
browser_free_kib=$(docker run --rm --network none --read-only --cap-drop ALL --entrypoint df postgres:17.7-bookworm -Pk / | awk 'NR==2 {print $4}')
test "$browser_free_kib" -ge 4194304
browser_compose up -d --wait --wait-timeout 60 --no-build public admin worker styleguide browser-proxy > "$browser_evidence/startup.log" 2>&1
curl --silent --fail --max-time 3 http://127.0.0.1:18088/readyz --output /dev/null
browser_runner=$browser_project-runner
mkdir -m 700 "$browser_evidence/native"
browser_compose run -d --no-deps --name "$browser_runner" journey > "$browser_evidence/runner-start.log" 2>&1
set +e
HACKBOX_LIFECYCLE_BROWSER_RUNNER="$browser_runner" HACKBOX_LIFECYCLE_BROWSER_ARTIFACTS="$browser_evidence" HACKBOX_LIFECYCLE_BROWSER_SOURCE="$browser_source" node "$browser_root/docs/verification/lifecycle/connected-browser.mjs" > "$browser_evidence/browser.log" 2>&1
browser_result=$?
set -e
docker exec "$browser_project-postgres-1" pg_dump -U lifecycle -d lifecycle -Fc > "$browser_evidence/final.dump"
python3 - "$browser_evidence/source.json" "$browser_source" "$browser_image" "$browser_guide_image" "$browser_guide_source" "$browser_project" "$browser_result" <<'PY'
from pathlib import Path
import json,sys
path,source,image,guide,guide_source,project,result=sys.argv[1:]
Path(path).write_text(json.dumps({'source_revision':source,'verification_image':image,'normal_guide_image':guide,'normal_guide_source':guide_source,'guide_web_source_matches':True,'project':project,'actual_browser_exit_code':int(result),'complete_lifecycle':False,'browser_phases':__import__('os').environ['HACKBOX_LIFECYCLE_BROWSER_PHASES']},indent=2)+'\n')
PY
printf 'Connected native browser exit: %s\n' "$browser_result"
test "$browser_result" -eq 0
test -s "$browser_evidence/connected-browser-proof.json"
test "$browser_source" = "$(git rev-parse HEAD)"
test -z "$(git status --porcelain)"
