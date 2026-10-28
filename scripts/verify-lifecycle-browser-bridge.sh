#!/bin/sh
set -eu
umask 077
browser_root=$(git -C "$(dirname "$0")/.." rev-parse --show-toplevel)
cd "$browser_root"
test -z "$(git status --porcelain)"
browser_source=$(git rev-parse HEAD)
browser_evidence=$(mktemp -d "${TMPDIR:-/tmp}/hackbox-browser-bridge.XXXXXX")
browser_project=hackbox-browser-bridge-$(date +%s)
export HACKBOX_LIFECYCLE_IMAGE=${HACKBOX_LIFECYCLE_IMAGE:-hackbox-lifecycle:browser-bridge}
browser_guide_supplied=${HACKBOX_LIFECYCLE_GUIDE_IMAGE:-}
export HACKBOX_LIFECYCLE_GUIDE_IMAGE=${HACKBOX_LIFECYCLE_GUIDE_IMAGE:-hackbox-lifecycle:browser-guide}
browser_compose(){ docker compose -p "$browser_project" -f "$browser_root/docs/verification/lifecycle/compose.yaml" --profile browser "$@"; }
browser_finish(){
 browser_exit=$?
 set +e
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
browser_host_free_kib=$(df -Pk "$browser_root" | awk 'NR==2 {print $4}')
if [ "$browser_host_free_kib" -lt 8388608 ]; then
 printf 'Insufficient host disk space: need at least 8388608 KiB free before lifecycle verification.\n' >&2
 exit 1
fi
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
browser_host_free_kib=$(df -Pk "$browser_root" | awk 'NR==2 {print $4}')
if [ "$browser_host_free_kib" -lt 4194304 ]; then
 printf 'Insufficient host disk space: need at least 4194304 KiB free before lifecycle verification.\n' >&2
 exit 1
fi
browser_free_kib=$(docker run --rm --network none --read-only --cap-drop ALL --entrypoint df postgres:17.7-bookworm -Pk / | awk 'NR==2 {print $4}')
test "$browser_free_kib" -ge 4194304
browser_compose up -d --wait --wait-timeout 60 --no-build public admin worker styleguide browser-proxy > "$browser_evidence/startup.log" 2>&1
curl --silent --fail --max-time 3 http://127.0.0.1:18088/readyz --output /dev/null
set +e
HACKBOX_LIFECYCLE_BROWSER_ARTIFACTS="$browser_evidence" HACKBOX_LIFECYCLE_BROWSER_SOURCE="$browser_source" node "$browser_root/docs/verification/lifecycle/browser-bridge.mjs" > "$browser_evidence/browser.log" 2>&1
browser_result=$?
set -e
docker exec "$browser_project-postgres-1" pg_dump -U lifecycle -d lifecycle -Fc > "$browser_evidence/final.dump"
python3 - "$browser_evidence/source.json" "$browser_source" "$browser_image" "$browser_guide_image" "$browser_guide_source" "$browser_project" "$browser_result" <<'PY'
from pathlib import Path
import json,sys
path,source,image,guide,guide_source,project,result=sys.argv[1:]
Path(path).write_text(json.dumps({'source_revision':source,'verification_image':image,'normal_guide_image':guide,'normal_guide_source':guide_source,'guide_web_source_matches':True,'project':project,'actual_browser_exit_code':int(result),'complete_lifecycle':False},indent=2)+'\n')
PY
printf 'Native browser bridge exit: %s\n' "$browser_result"
test "$browser_result" -eq 0
test -s "$browser_evidence/browser-bridge-proof.json"
test "$browser_source" = "$(git rev-parse HEAD)"
test -z "$(git status --porcelain)"
