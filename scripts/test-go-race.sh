#!/bin/sh
set -eu
cd "$(dirname "$0")/.."

# App and HTTP adapter native fixtures each exceed one ten-minute invocation.
# Keep the same deadline and race detector for two exhaustive batches per package.
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
go list -race ./cmd/... ./internal/... > "$race_manifest/packages"
go list -race ./internal/app ./internal/httpapi > "$race_manifest/batched-packages"
test "$(wc -l < "$race_manifest/batched-packages")" -eq 2
while IFS= read -r race_package; do
  test "$(awk -v package="$race_package" '$0 == package { count++ } END { print count+0 }' "$race_manifest/packages")" -eq 1
done < "$race_manifest/batched-packages"
awk 'NR == FNR { batched[$0] = 1; next } !($0 in batched)' \
  "$race_manifest/batched-packages" "$race_manifest/packages" > "$race_manifest/other-packages"
test -s "$race_manifest/other-packages"

# Go's -list output includes top-level tests, examples and fuzz tests. -run
# selects those parents and all their subtests; benchmarks are not run by the
# original default command either. Go identifiers contain no regex operators.
while IFS= read -r race_package; do
  race_directory="$race_manifest/${race_package##*/}"
  mkdir "$race_directory"
  go test -race "$race_package" -list '^(Test|Example|Fuzz)' > "$race_directory/list-output"
  awk '/^(Test|Example|Fuzz)/ && !/[[:space:]]/' "$race_directory/list-output" | LC_ALL=C sort > "$race_directory/all"
  test -s "$race_directory/all"
  test -z "$(uniq -d "$race_directory/all")"
  awk 'NR % 2 == 1' "$race_directory/all" > "$race_directory/batch-1"
  awk 'NR % 2 == 0' "$race_directory/all" > "$race_directory/batch-2"
  test -s "$race_directory/batch-1"
  test -s "$race_directory/batch-2"
  cat "$race_directory/batch-1" "$race_directory/batch-2" | LC_ALL=C sort > "$race_directory/union"
  cmp "$race_directory/all" "$race_directory/union"
  printf '%s race batches: %s + %s = %s top-level tests/examples/fuzz tests\n' \
    "$race_package" "$(wc -l < "$race_directory/batch-1")" "$(wc -l < "$race_directory/batch-2")" "$(wc -l < "$race_directory/all")"
  if test "${1-}" = --check; then
    for race_batch in 1 2; do
      awk -v package="$race_package" -v batch="$race_batch" '{ print package " batch=" batch " " $0 }' "$race_directory/batch-$race_batch"
    done
  fi
done < "$race_manifest/batched-packages"
test "${1-}" != --check || exit 0

# Import paths cannot contain whitespace; expand only Go's own package list.
# shellcheck disable=SC2046
go test -race -timeout 10m $(cat "$race_manifest/other-packages")
while IFS= read -r race_package; do
  for race_batch in 1 2; do
    race_names=$(paste -sd '|' "$race_manifest/${race_package##*/}/batch-$race_batch")
    printf 'Running %s race batch %s\n' "$race_package" "$race_batch"
    go test -race -timeout 10m "$race_package" -run "^($race_names)$"
  done
done < "$race_manifest/batched-packages"
