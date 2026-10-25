#!/bin/sh
set -eu
cd "$(dirname "$0")/.."

# The app's cumulative native fixtures exceed one ten-minute invocation. Keep
# the same deadline and race detector for two disjoint, exhaustive app batches.
# Every other package still runs once, without a test-name filter.
case "${1-}" in
  '')
    test "${HACKBOX_INTEGRATION-}" = 1 || {
      echo 'HACKBOX_INTEGRATION=1 is required for native race verification' >&2
      exit 1
    }
    ;;
  --check) ;;
  *) echo 'usage: sh scripts/test-go-race.sh [--check]' >&2; exit 1 ;;
esac
test "$#" -le 1

race_manifest=$(mktemp -d)
trap 'rm -rf "$race_manifest"' EXIT HUP INT TERM
go list ./cmd/... ./internal/... > "$race_manifest/packages"
race_app=$(go list ./internal/app)
test "$(awk -v app="$race_app" '$0 == app { count++ } END { print count+0 }' "$race_manifest/packages")" -eq 1
awk -v app="$race_app" '$0 != app' "$race_manifest/packages" > "$race_manifest/other-packages"
test -s "$race_manifest/other-packages"

# Go's -list output includes top-level tests, examples and fuzz tests. -run
# selects those parents and all their subtests; benchmarks are not run by the
# original default command either. Go identifiers contain no regex operators.
go test ./internal/app -list '^(Test|Example|Fuzz)' > "$race_manifest/list-output"
awk '/^(Test|Example|Fuzz)/ && !/[[:space:]]/' "$race_manifest/list-output" | LC_ALL=C sort > "$race_manifest/all"
test -s "$race_manifest/all"
test -z "$(uniq -d "$race_manifest/all")"
awk 'NR % 2 == 1' "$race_manifest/all" > "$race_manifest/batch-1"
awk 'NR % 2 == 0' "$race_manifest/all" > "$race_manifest/batch-2"
test -s "$race_manifest/batch-1"
test -s "$race_manifest/batch-2"
cat "$race_manifest/batch-1" "$race_manifest/batch-2" | LC_ALL=C sort > "$race_manifest/union"
cmp "$race_manifest/all" "$race_manifest/union"
printf 'App race batches: %s + %s = %s top-level tests/examples/fuzz tests\n' \
  "$(wc -l < "$race_manifest/batch-1")" "$(wc -l < "$race_manifest/batch-2")" "$(wc -l < "$race_manifest/all")"

if test "${1-}" = --check; then
  for race_batch in 1 2; do
    awk -v batch="$race_batch" '{ print "batch=" batch " " $0 }' "$race_manifest/batch-$race_batch"
  done
  exit 0
fi

# Import paths cannot contain whitespace; expand only Go's own package list.
# shellcheck disable=SC2046
go test -race -timeout 10m $(cat "$race_manifest/other-packages")
for race_batch in 1 2; do
  race_names=$(paste -sd '|' "$race_manifest/batch-$race_batch")
  printf 'Running app race batch %s\n' "$race_batch"
  go test -race -timeout 10m ./internal/app -run "^($race_names)$"
done
