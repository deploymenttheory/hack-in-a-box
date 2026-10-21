#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
go tool sqlc generate
go tool oapi-codegen --config api/oapi-codegen.yaml api/openapi.yaml
npm --prefix web run generate
