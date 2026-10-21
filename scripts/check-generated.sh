#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
sh scripts/generate.sh
git diff --exit-code -- internal/database/dbgen internal/httpapi/contract web/src/api/schema.gen.ts
test -z "$(git ls-files --others --exclude-standard -- internal/database/dbgen internal/httpapi/contract web/src/api/schema.gen.ts)"
