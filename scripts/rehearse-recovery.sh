#!/usr/bin/env bash
# Synthetic, isolated operator proof. Never reads the deployment .env or Compose project.
set -euo pipefail
: "${HACKBOX_RECOVERY_IMAGE:?Build the Docker recovery target and set HACKBOX_RECOVERY_IMAGE}"
command -v docker >/dev/null
command -v openssl >/dev/null
command -v python3 >/dev/null
recovery_project="hackbox-recovery-$(openssl rand -hex 8)"
recovery_pg='postgres:17.7-bookworm@sha256:86e0b703649d7a792bd9243ee28afc9d8f7c6b2b5638077c9d6882d4d472bbfd'
recovery_source=''
recovery_target=''
recovery_worker=''
recovery_network=''
recovery_volumes=()
recovery_started=$(python3 -c 'import time; print(time.monotonic())')
cleanup() {
  for recovery_container in "$recovery_worker" "$recovery_target" "$recovery_source"; do
    if [ -n "$recovery_container" ]; then docker rm -fv "$recovery_container" >/dev/null 2>&1 || true; fi
  done
  for recovery_volume in "${recovery_volumes[@]-}"; do
    if [ -n "$recovery_volume" ]; then docker volume rm "$recovery_volume" >/dev/null 2>&1 || true; fi
  done
  if [ -n "$recovery_network" ]; then docker network rm "$recovery_network" >/dev/null 2>&1 || true; fi
}
trap cleanup EXIT INT TERM
recovery_network=$(docker network create "$recovery_project")
for recovery_suffix in source-files target-files archives identity recipients; do
  recovery_name="$recovery_project-$recovery_suffix"
  if docker volume inspect "$recovery_name" >/dev/null 2>&1; then exit 1; fi
  docker volume create "$recovery_name" >/dev/null
  recovery_volumes+=("$recovery_name")
done
recovery_source=$(docker run --detach --network "$recovery_project" --name "$recovery_project-source" -e POSTGRES_DB=hackbox_test -e POSTGRES_USER=test -e POSTGRES_PASSWORD=synthetic-recovery-only "$recovery_pg")
recovery_target=$(docker run --detach --network "$recovery_project" --name "$recovery_project-target" -e POSTGRES_DB=hackbox_test -e POSTGRES_USER=test -e POSTGRES_PASSWORD=synthetic-recovery-only "$recovery_pg")
for recovery_database in "$recovery_source" "$recovery_target"; do
  recovery_ready=0
  for ((recovery_attempt=0; recovery_attempt<60; recovery_attempt++)); do
    if docker exec "$recovery_database" pg_isready -U test -d hackbox_test >/dev/null 2>&1; then recovery_ready=1; break; fi
    sleep 1
  done
  test "$recovery_ready" = 1
done
recovery_source_dsn="postgres://test:synthetic-recovery-only@$recovery_project-source:5432/hackbox_test?sslmode=disable"
recovery_target_dsn="postgres://test:synthetic-recovery-only@$recovery_project-target:5432/hackbox_test?sslmode=disable"
HACKBOX_DATABASE_URL="$recovery_source_dsn" docker run --rm --network "$recovery_project" --env HACKBOX_DATABASE_URL -e HACKBOX_ENV=test --entrypoint /app/migrate "$HACKBOX_RECOVERY_IMAGE" >/dev/null
recovery_key='aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
recovery_checksum=$(printf 'Synthetic operator recovery attachment 124\n' | openssl dgst -sha256 | awk '{print $NF}')
recovery_size=$(printf 'Synthetic operator recovery attachment 124\n' | wc -c | tr -d ' ')
docker exec -i "$recovery_source" psql -X -U test -d hackbox_test -v ON_ERROR_STOP=1 -v checksum="$recovery_checksum" -v size="$recovery_size" >/dev/null <<'SQL'
INSERT INTO people(id,issuer,subject,display_name) VALUES('00000000-0000-0000-0000-000000000101','synthetic','operator-recovery','Synthetic operator fixture');
INSERT INTO events(id,name) VALUES('00000000-0000-0000-0000-000000000102','Synthetic operator source snapshot');
INSERT INTO files(id,event_id,owner_id,idempotency_key,object_key,filename,media_type,checksum,size_bytes) VALUES('00000000-0000-0000-0000-000000000103','00000000-0000-0000-0000-000000000102','00000000-0000-0000-0000-000000000101','00000000-0000-0000-0000-000000000103','aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','synthetic.txt','text/plain',:'checksum',:'size');
INSERT INTO operations(id,event_id,operation_key,kind,target_id,due_at,approved,state) VALUES('00000000-0000-0000-0000-000000000104','00000000-0000-0000-0000-000000000102','original-pending','synthetic_recovery','00000000-0000-0000-0000-000000000102',now(),true,'pending'),('00000000-0000-0000-0000-000000000105','00000000-0000-0000-0000-000000000102','original-unknown','synthetic_recovery','00000000-0000-0000-0000-000000000102',now(),true,'ambiguous');
SQL
docker run --rm -v "$recovery_project-source-files:/data/files" --entrypoint sh "$HACKBOX_RECOVERY_IMAGE" -c 'printf "Synthetic operator recovery attachment 124\n" > /data/files/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa; chmod 0600 /data/files/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
docker run --rm -v "$recovery_project-identity:/identity" -v "$recovery_project-recipients:/recipients" "$HACKBOX_RECOVERY_IMAGE" -action keygen -identity /identity/archive.key -recipients /recipients/archive.pub >/dev/null
HACKBOX_DATABASE_URL="$recovery_source_dsn" docker run --rm --read-only --tmpfs /tmp --network "$recovery_project" --env HACKBOX_DATABASE_URL -v "$recovery_project-source-files:/data/files:ro" -v "$recovery_project-archives:/archives" -v "$recovery_project-recipients:/recipients:ro" "$HACKBOX_RECOVERY_IMAGE" -action backup -files /data/files -bundle /archives/capture -recipients /recipients/archive.pub >/dev/null
# Prove RPO is the captured snapshot, never later source changes.
docker exec "$recovery_source" psql -X -U test -d hackbox_test -v ON_ERROR_STOP=1 -c "UPDATE events SET name='Synthetic source after snapshot'" >/dev/null
docker run --rm --read-only --tmpfs /tmp -v "$recovery_project-archives:/archives:ro" -v "$recovery_project-identity:/identity:ro" "$HACKBOX_RECOVERY_IMAGE" -action verify -bundle /archives/capture -identity /identity/archive.key >/dev/null
recovery_restore=$(HACKBOX_DATABASE_URL="$recovery_target_dsn" docker run --rm --read-only --tmpfs /tmp --network "$recovery_project" --env HACKBOX_DATABASE_URL -v "$recovery_project-archives:/archives:ro" -v "$recovery_project-identity:/identity:ro" -v "$recovery_project-target-files:/data/files" "$HACKBOX_RECOVERY_IMAGE" -action restore -bundle /archives/capture -identity /identity/archive.key -files /data/files/restored)
recovery_name=$(docker exec "$recovery_target" psql -X -A -t -U test -d hackbox_test -c 'SELECT name FROM events')
test "$recovery_name" = 'Synthetic operator source snapshot'
recovery_restored_hash=$(docker run --rm --read-only -v "$recovery_project-target-files:/data/files:ro" --entrypoint sha256sum "$HACKBOX_RECOVERY_IMAGE" "/data/files/restored/$recovery_key" | awk '{print $1}')
test "$recovery_restored_hash" = "$recovery_checksum"
# Held actual worker must not even clean a physically present old orphan.
docker run --rm -v "$recovery_project-target-files:/data/files" --entrypoint sh "$HACKBOX_RECOVERY_IMAGE" -c 'touch /data/files/restored/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb; touch -d 2000-01-01 /data/files/restored/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb'
recovery_worker=$(HACKBOX_DATABASE_URL="$recovery_target_dsn" docker run --detach --read-only --tmpfs /tmp --network "$recovery_project" --env HACKBOX_DATABASE_URL -e HACKBOX_ENV=test -e HACKBOX_STORAGE_PATH=/data/files/restored -v "$recovery_project-target-files:/data/files" --entrypoint /app/worker "$HACKBOX_RECOVERY_IMAGE")
sleep 2
test "$(docker inspect --format '{{.State.Running}}' "$recovery_worker")" = true
test "$(docker exec "$recovery_target" psql -X -A -t -U test -d hackbox_test -c 'SELECT count(*) FROM reconciliation_runs')" = 0
docker run --rm --read-only -v "$recovery_project-target-files:/data/files:ro" --entrypoint test "$HACKBOX_RECOVERY_IMAGE" -f /data/files/restored/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
docker stop --time 5 "$recovery_worker" >/dev/null
docker rm -v "$recovery_worker" >/dev/null
recovery_worker=''
recovery_status=$(HACKBOX_DATABASE_URL="$recovery_target_dsn" docker run --rm --read-only --network "$recovery_project" --env HACKBOX_DATABASE_URL "$HACKBOX_RECOVERY_IMAGE" -action status)
# Deliberately review this known synthetic inventory; never use this automatic
# approval in production. Unknown state is checked, not changed or resolved.
recovery_review=$(python3 -c 'import json,sys; s=json.load(sys.stdin); assert s["hold_id"]; assert sorted((i["id"],i["state"]) for i in s["intents"])==[("00000000-0000-0000-0000-000000000104","pending"),("00000000-0000-0000-0000-000000000105","ambiguous")]; print(s["hold_id"],s["fingerprint"])' <<<"$recovery_status")
read -r recovery_hold recovery_fingerprint <<<"$recovery_review"
HACKBOX_DATABASE_URL="$recovery_target_dsn" docker run --rm --read-only --network "$recovery_project" --env HACKBOX_DATABASE_URL "$HACKBOX_RECOVERY_IMAGE" -action resume -hold "$recovery_hold" -review "$recovery_fingerprint" >/dev/null
test "$(docker exec "$recovery_target" psql -X -A -t -U test -d hackbox_test -c 'SELECT count(*) FROM operations WHERE approved AND state IN('\''pending'\'','\''ambiguous'\'') AND attempts=0')" = 2
recovery_finished=$(python3 -c 'import time; print(time.monotonic())')
python3 -c 'import json,sys; r=json.load(sys.stdin); print(json.dumps({"result":"pass","dataset":"one event, one "+sys.argv[3]+"-byte attachment, original pending and ambiguous intents","restore_started_at":r["started_at"],"restore_completed_at":r["completed_at"],"capture_snapshot_completed_at":r["manifest"]["completed_at"],"total_rehearsal_seconds":float(sys.argv[2])-float(sys.argv[1]),"held_worker_reconciliations":0,"old_orphan_preserved_while_held":True,"original_intent_states_preserved_after_reviewed_resume":True,"image_build_and_operator_key_retrieval_excluded":True}))' "$recovery_started" "$recovery_finished" "$recovery_size" <<<"$recovery_restore"
