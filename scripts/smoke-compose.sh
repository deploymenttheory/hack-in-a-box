#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
# Only this unique synthetic project is removed on exit, including its volumes.
export COMPOSE_PROJECT_NAME="hackbox-smoke-$$"
export HACKBOX_DB_PASSWORD="synthetic-smoke-password"
export HACKBOX_ENV=development
export HACKBOX_PORT=18080
export HACKBOX_PUBLIC_ORIGIN=http://127.0.0.1:18080
export HACKBOX_OIDC_ISSUER= HACKBOX_OIDC_CLIENT_ID= HACKBOX_OIDC_CLIENT_SECRET= HACKBOX_BOOTSTRAP_SUBJECT=
export HACKBOX_METRICS_TOKEN=synthetic-metrics-token-32-characters
compose() { docker compose --env-file /dev/null "$@"; }
cleanup() { compose down --volumes --remove-orphans >/dev/null 2>&1; }
trap cleanup EXIT INT TERM
compose build
compose up -d --wait postgres
compose run --rm migrate
compose up -d --wait api worker
test "$(compose exec -T api id -u)" = 10001
compose exec -T postgres psql -U hackbox -d hackbox -v ON_ERROR_STOP=1 -c "INSERT INTO events(name) VALUES ('Synthetic persistence fixture');" >/dev/null
compose exec -T api sh -c 'printf synthetic-file > /data/files/smoke-fixture'
curl --fail --silent http://127.0.0.1:18080/healthz >/dev/null
curl --fail --silent http://127.0.0.1:18080/readyz >/dev/null
curl --fail --silent http://127.0.0.1:18080/events/8b4917c1-f27a-40c8-838b-a7fd047c4313 | grep -q '<div id="root">'
test "$(curl --silent --output /dev/null --write-out '%{http_code}' http://127.0.0.1:18080/api/v1/missing)" = 404
compose stop api worker postgres
compose up -d --wait postgres
compose up -d --wait api worker
test "$(compose exec -T postgres psql -U hackbox -d hackbox -Atc "SELECT count(*) FROM events WHERE name='Synthetic persistence fixture'")" = 1
test "$(compose exec -T api cat /data/files/smoke-fixture)" = synthetic-file
curl --fail --silent http://127.0.0.1:18080/readyz >/dev/null
echo 'Compose startup, non-root runtime, deep links and restart persistence passed.'
