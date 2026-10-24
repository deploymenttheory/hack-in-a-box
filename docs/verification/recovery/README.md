# Recovery implementation checkpoint

Issue #124 is in progress. This first slice implements durable startup isolation;
it is not yet a complete backup or restore procedure.

- Migration 48 owns the singleton recovery hold and immutable operator maintenance
  history. An active hold refuses rollback to a binary without this startup gate.
- Both API surfaces and the worker wait before opening files, constructing provider
  runtimes, cleaning storage, reconciling intents or starting River.
- A shared process-lifetime database lease prevents a quiesced capture from racing
  native process startup. Operators must stop their services deliberately; this
  tool never stops containers.
- `go run ./cmd/recovery -action hold|status` uses the deployment database connection
  from `HACKBOX_DATABASE_URL`. `-action resume -hold UUID -review FINGERPRINT`
  requires the current hold and unresolved-operation metadata fingerprint. It does
  not approve, resolve, renumber or resend an original operation.

The first regression exercises real PostgreSQL and actual worker composition:
held startup must leave the storage path absent and reconciliation history empty;
wrong/stale reviews and rollback are rejected; reviewed resume preserves pending
and ambiguous operation identities/states. Executed: `HACKBOX_INTEGRATION=1 GOMAXPROCS=2 GOFLAGS=-p=1 go test -race
./internal/recovery -run TestRecoveryHold -count=1` passed against isolated real
PostgreSQL (6.775s). The fixture includes failed active-hold rollback and successful
post-review rollback. This proves this bounded slice, not the remaining restore scope.

Remaining full issue scope: private authenticated streaming backups; consistent
PostgreSQL plus files capture; complete authenticated validation and fresh-target
restore; separate archive identities/deployment credential keys; safe review of all
native pending/ambiguous external actions; malicious archive tests; fresh isolated
PostgreSQL/files recovery of native histories and attachments; missing/wrong/retained
credential-key proof; measured synthetic RTO/RPO and the final operator runbook.
No production backups, real provider actions or existing user container changes
are part of the rehearsal.

Bounded compile/lint: `GOMAXPROCS=2 GOFLAGS=-p=1 golangci-lint run
./internal/recovery/... ./cmd/recovery/... ./internal/app/... ./internal/database/...`
completed with zero issues. An initial CLI exit/defer lint finding was corrected
before this checkpoint. No full-suite, encrypted-backup or timed-restore proof is
claimed yet.
