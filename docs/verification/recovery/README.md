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

## Encrypted archive and fresh-target checkpoint

`filippo.io/age` v1.3.2 is pinned after checking the current Go module proxy and
[maintained release](https://github.com/FiloSottile/age/releases/tag/v1.3.2).
The [age streaming API](https://pkg.go.dev/filippo.io/age#Encrypt) owns encryption
and authenticated framing; this implementation does not design its own framing.
A private bundle contains `database.age`, `files.age` and `manifest.age` only.
Native file identities/checksums and the two ciphertext hashes are bound by the
encrypted manifest. There are no plaintext dump/archive scratch artifacts.

Verification consumes all authenticated streams to EOF, validates the exact
inventory and rejects paths/symlinks/extra objects/checksum inconsistencies.
Restore first copies ciphertext into an owned private directory and verifies the
whole bundle; it refuses existing database relations/storage, streams into a
fresh target and commits a new hold with the SQL restore. Raw keys remain outside
both source database and archive. An operator-only Docker target includes the
existing pinned PostgreSQL 17.7 clients.

Executed terminal proof after correction:

- Bounded archive/CLI/testdb lint: zero issues.
- `GOMAXPROCS=2 GOFLAGS=-p=1 go test -race ./internal/recovery
  ./internal/credentials -run 'TestArchive|TestFreshEncryptedRecovery' -count=1`:
  recovery passed 1.556s; native credentials passed 7.362s (integration enabled).
- Ten deterministic archive cases cover valid data, wrong identity, truncated DB
  and file ciphertext, unsafe tar path, symlink header/artifact, wrong native
  checksum, unexpected artifact and public artifact permissions.
- Fresh-server proof creates separate source/target PostgreSQL containers and a
  fresh native storage directory. A wrong archive identity exposes neither
  database schema nor restored files. Missing, wrong-material, new-only and retained
  old deployment keys retain native credential semantics. A subsequent source
  replacement is absent from the recovered credential; existing target overwrite
  is refused. All containers are fixture-owned and removed afterward.

The original exact rehearsal failed because `pg_dump` clears the session search
path; the appended hold statement was unqualified. Qualifying its two native
recovery tables fixed that observed failure. Temporary stderr instrumentation was
removed; diagnostics never included the plaintext dump or deployment keys.

An earlier successful measured 34-byte/one-attachment rehearsal captured in
316.381ms and restored in 928.502ms. This excludes image retrieval, target startup,
operator review and external-key retrieval; it is **not** a production RTO claim.
A larger native-history dataset and final explicit RTO/RPO runbook remain pending.

## Native history, review and custody checkpoint

The current native dataset now includes 1,000 synthetic accounts, approved rubric
and actually released common challenge/rules, resolved help with three transitions,
a private question and native-inbox-backed approved/published answer, dated outcome
evidence, an explicitly approved 30-day follow-up, original pending/ambiguous
operations and attempts, plus the credential and attachment. Seventeen nonempty
native history tables match exactly after fresh-server restoration; native owner
reads and stranger denial are checked. This is not an empty-schema comparison.

The typed safe review inventory now includes native unmaterialized schedules and
approved outcome follow-ups as well as generic jobs, SMTP/verification/repository/
communication/content states. Newly appearing schedules or verification attempts
invalidate an old review. Private command text/destinations are excluded.
`review_entries` counts records, not people or distinct effects.

Executed terminal capped proof (before the packaged-image rehearsal):

- Owned recovery/CLI/testdb lint: zero issues.
- `HACKBOX_INTEGRATION=1 GOMAXPROCS=2 GOFLAGS=-p=1 go test -race
  ./internal/recovery ./internal/credentials -run
  'TestRecovery|TestArchive|TestFreshEncryptedRecovery' -count=1 -v`:
  recovery passed 9.951s; credentials passed 10.726s.
- Recovery cases include ten archive scenarios, private archive-key custody,
  actual held worker/no cleanup/reconciliation, stale typed review, immutable
  history including TRUNCATE rejection, migration rollback protection and a
  real PostgreSQL failed-stream rollback.
- An unchanged-flags atomicity probe already passed; adding explicit `--file=-`
  clarifies PostgreSQL's documented stdin/single-transaction invocation. It is not
  evidence of a formerly reproduced atomicity defect.
- `go mod tidy -diff` produced zero bytes after canonical normalization.
- The isolated operator script passes Bash syntax and ShellCheck. Its actual
  packaged execution remains pending and is not inferred from native callbacks.

Full operator procedure: [recovery runbook](../../recovery-runbook.md).
