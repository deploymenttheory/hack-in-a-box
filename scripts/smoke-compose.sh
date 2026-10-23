#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
# Only this unique synthetic project is removed on exit, including its volumes.
export COMPOSE_PROJECT_NAME="hackbox-smoke-$$"
export HACKBOX_DB_PASSWORD="synthetic-smoke-password"
export HACKBOX_ENV=development
export HACKBOX_PUBLIC_IMAGE="hack-in-a-box:smoke-public-$$"
export HACKBOX_ADMIN_IMAGE="hack-in-a-box:smoke-admin-$$"
export HACKBOX_PORT=18080
export HACKBOX_ADMIN_PORT=18081
export HACKBOX_ADMIN_ORIGIN=http://127.0.0.1:18081
export HACKBOX_ADMIN_OIDC_CLIENT_ID= HACKBOX_ADMIN_OIDC_CLIENT_SECRET=
export HACKBOX_PUBLIC_ORIGIN=http://127.0.0.1:18080
export HACKBOX_OIDC_ISSUER= HACKBOX_OIDC_CLIENT_ID= HACKBOX_OIDC_CLIENT_SECRET= HACKBOX_BOOTSTRAP_SUBJECT=
export HACKBOX_GITHUB_CLIENT_ID= HACKBOX_GITHUB_CLIENT_SECRET= HACKBOX_GITHUB_TOKEN_KEY=
export HACKBOX_METRICS_TOKEN=synthetic-metrics-token-32-characters
compose() { docker compose --env-file /dev/null "$@"; }
frontend() {
  page_html=$(curl --fail --silent "$1/events/8b4917c1-f27a-40c8-838b-a7fd047c4313")
  printf '%s' "$page_html" | grep -q '<div id="root">'
  script_path=$(printf '%s' "$page_html" | sed -n 's/.*src="\(\/assets\/[^"]*\)".*/\1/p' | head -n 1)
  style_path=$(printf '%s' "$page_html" | sed -n 's/.*href="\(\/assets\/[^"]*\.css\)".*/\1/p' | head -n 1)
  test -n "$script_path"
  test -n "$style_path"
  curl --fail --silent "$1$script_path" >/dev/null
  curl --fail --silent "$1$style_path" >/dev/null
}
cleanup() { compose down --volumes --remove-orphans >/dev/null 2>&1; }
trap cleanup EXIT INT TERM
compose build
compose up -d --wait postgres
compose run --rm migrate
compose up -d --wait api admin worker
test "$(compose exec -T api id -u)" = 10001
compose exec -T postgres psql -U hackbox -d hackbox -v ON_ERROR_STOP=1 -c "INSERT INTO events(name) VALUES ('Synthetic persistence fixture');" >/dev/null
compose exec -T api sh -c 'printf synthetic-file > /data/files/smoke-fixture'
curl --fail --silent http://127.0.0.1:18080/healthz >/dev/null
curl --fail --silent http://127.0.0.1:18080/readyz >/dev/null
frontend http://127.0.0.1:18080
test "$(curl --silent --output /dev/null --write-out '%{http_code}' http://127.0.0.1:18080/api/v1/missing)" = 404
# Prove the admin process, assets, database readiness and authenticated metrics
# remain available when the participant API/frontend container is stopped.
compose stop api
if curl --fail --silent --max-time 2 http://127.0.0.1:18080/healthz >/dev/null 2>&1; then
  echo 'Participant app unexpectedly remained available' >&2; exit 1
fi
curl --fail --silent http://127.0.0.1:18081/healthz >/dev/null
curl --fail --silent http://127.0.0.1:18081/readyz >/dev/null
frontend http://127.0.0.1:18081
test "$(curl --silent --output /dev/null --write-out '%{http_code}' http://127.0.0.1:18081/api/v1/session)" = 401
curl --fail --silent -H "Authorization: Bearer $HACKBOX_METRICS_TOKEN" http://127.0.0.1:18081/ops/metrics | grep -q hackbox_http_requests_total
# Prove the participant process and its own assets/session boundary likewise
# remain available when the independent admin process is stopped.
compose up -d --wait api
compose stop admin
if curl --fail --silent --max-time 2 http://127.0.0.1:18081/healthz >/dev/null 2>&1; then
  echo 'Admin app unexpectedly remained available' >&2; exit 1
fi
curl --fail --silent http://127.0.0.1:18080/healthz >/dev/null
curl --fail --silent http://127.0.0.1:18080/readyz >/dev/null
frontend http://127.0.0.1:18080
test "$(curl --silent --output /dev/null --write-out '%{http_code}' http://127.0.0.1:18080/api/v1/session)" = 401
compose stop admin worker postgres
compose up -d --wait postgres
compose up -d --wait api admin worker
test "$(compose exec -T postgres psql -U hackbox -d hackbox -Atc "SELECT count(*) FROM events WHERE name='Synthetic persistence fixture'")" = 1
test "$(compose exec -T api cat /data/files/smoke-fixture)" = synthetic-file
curl --fail --silent http://127.0.0.1:18080/readyz >/dev/null
echo 'Compose startup, independent participant/admin availability, non-root runtime, deep links and restart persistence passed.'
