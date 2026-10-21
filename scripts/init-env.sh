#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
if test -e .env; then echo '.env already exists; it was left unchanged.'; exit 0; fi
umask 077
password=$(openssl rand -hex 32)
metrics=$(openssl rand -hex 32)
sed -e "s/^HACKBOX_DB_PASSWORD=$/HACKBOX_DB_PASSWORD=$password/" \
    -e "s/^HACKBOX_METRICS_TOKEN=$/HACKBOX_METRICS_TOKEN=$metrics/" .env.example > .env
echo 'Created .env with private local credentials. Configure OIDC to enable sign-in.'
