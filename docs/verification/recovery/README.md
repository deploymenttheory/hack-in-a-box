# Recovery verification — issue #124

The full implementation is a review candidate in Draft PR #269. Independent final
reviews and all exact-head CI checks remain required before merge. The executed
native and packaged proofs below are distinct; neither is represented as a broad
production security/release certification.

## Delivered scope

- Consistent quiesced PostgreSQL/native-files capture into private authenticated
  age ciphertext; fixed encrypted manifest binds schema, capture interval, object
  metadata and ciphertext hashes. No plaintext backup/dump/archive scratch files.
- Complete authenticated EOF/path/type/inventory/checksum validation before fresh
  database/files restore. Existing targets are refused; restored schema/file
  metadata must agree before private staged files are published.
- Durable recovery hold before API/admin/worker file opening, cleanup, provider
  wiring, maintenance, lease reconciliation and River. New restore hold commits
  with SQL. Active holds cannot be bypassed through schema rollback.
- Explicit current typed pending-action review and fingerprint-bound resume,
  immutable maintenance history, and preservation of original identities,
  approvals, attempts and ambiguous outcomes. No provider is called by the tool.
- Separate private archive identity and retained deployment credential keys;
  existing pgx keyword/URI deployment configuration and pool tuning are preserved.
- A pinned, independent operator image with bundled PostgreSQL clients, executable
  isolated rehearsal and [operator runbook](../../recovery-runbook.md).

## Native real-PostgreSQL/race proof

Executed with capped workers on the native source frozen into `294d74b`; subsequent
client-connection changes are covered separately below. The shared fixture-only
clock correction `450b9ff` was cherry-picked as `7218da6` before packaged builds.

```sh
HACKBOX_INTEGRATION=1 GOMAXPROCS=2 GOFLAGS=-p=1 \
  go test -race ./internal/recovery ./internal/credentials \
  -run 'TestRecovery|TestArchive|TestFreshEncryptedRecovery' -count=1 -v
```

Terminal: recovery **9.951s PASS**, credentials **10.726s PASS**. These are selected
regressions, not a claim that every repository test ran locally.

The fresh-server dataset has 1,000 synthetic accounts, an actually released common
challenge/rules with exact approved rubric, resolved help with three transitions,
a private question and approved/published common answer using native inbox
contracts, dated outcome evidence, an explicitly approved 30-day follow-up, an
attachment, encrypted native credential, and original pending/ambiguous operation
and attempt history. **Seventeen nonempty native history tables** compare exactly
after restoration. Native owner reads and stranger denial are exercised.

The native cases also prove:

- Missing/wrong archive identity does not expose schema or files; missing,
  wrong-material, new-only and retained historical deployment keys preserve native
  credential reveal behavior. A subsequent source replacement is absent from the
  recovered value; an existing target cannot be overwritten.
- Actual held worker composition leaves storage unopened and reconciliation empty;
  wrong/stale reviews fail. New native schedules and verification attempts change
  the safe fingerprint without exposing their bodies/destinations.
- Live native runtime prevents exclusive capture, hold identity remains stable,
  maintenance history rejects DELETE/TRUNCATE, active-hold rollback fails and
  post-review rollback succeeds. Original operation states/approvals remain.
- Ten deterministic archive scenarios cover valid input, wrong key, truncated DB
  and file streams, unsafe tar path, symlink header/artifact, wrong native checksum,
  unexpected artifact and public artifact permissions.
- Archive identity creation is private, separate and cannot overwrite old keys.
  An actual failed SQL stream rolls back its prior synthetic table. The initial
  unchanged-flags atomicity probe already passed; explicit `--file=-` documents
  stdin/single-transaction use, not a claimed formerly reproduced atomicity bug.

## Actual packaged operator proof

Source: **`c2471d85e9af0a28acb036e378bec4f0c4130f5e`**.

```sh
docker build --target recovery --tag hackbox-recovery:124-c2471d8 .
HACKBOX_RECOVERY_IMAGE=hackbox-recovery:124-c2471d8 \
  ./scripts/rehearse-recovery.sh
```

Both commands completed successfully. Image:
`sha256:5b07cf0d854b676110e800f5fe281e15f8fc0321d66b9004ddb1c2dc5c8cf7ec`.
The [machine-readable result](operator-result.json) records exact provenance and
script digest. The host provided Docker, OpenSSL, Python and shell utilities;
**no host pg_dump/psql** was used.

This exercises actual keygen → backup → verify → fresh restore → actual held
worker → known synthetic inventory review → explicit resume, using existing
**keyword pgx connection strings plus pool tuning**, with validated equivalent
operator client URIs. Native PostgreSQL/migrate/worker binaries run in the image.
The source/target PostgreSQL servers and all files/keys/archive volumes are fresh
and uniquely scoped. No deployment `.env`, real provider or existing container is
used. Read-only container/network/volume checks after completion found **zero**
remaining resources with the owned recovery prefix.

Measured dataset: **one event, one 43-byte attachment, original pending and
ambiguous intents**. Fresh restore: **484.510ms**; whole synthetic rehearsal:
**11.052379667s**. Snapshot completed at **2026-10-25 00:50:24.332992678 UTC**.
Image build/pull, real external-key retrieval and human review are excluded. This
small deterministic measurement is **not** a production RTO promise or a claim to
represent a production-size media corpus. Fresh servers/volumes establish logical
clean-host isolation; this is not a claim of a second physical machine.

The actual packaged worker stayed alive while held, with zero reconciliation and
a physically present old orphan preserved. After it was stopped, reviewed resume
left the original pending/ambiguous states and zero attempt counts unchanged.
A source mutation after backup was absent from the recovered event, demonstrating
snapshot RPO rather than pretending subsequent writes were recovered. Deployment
backup cadence determines the real loss window; this procedure adds no PITR.

## Diagnosed integration failures and final checks

The initial native fresh restore failed because `pg_dump` clears session search
path. Qualifying the appended recovery-hold tables fixed that exact observed
failure. Temporary private stderr instrumentation was removed.

The initial actual packaged capture failed because libpq does not expand a URI
supplied solely as its PGDATABASE default. An isolated probe showed the installed
binary existed and explicit libpq fields succeeded where URI-only default failed.
The corrected adapter preserves encoded credentials/TLS in documented environment
fields, keeps secrets out of argv/output, and accepts equivalent operator
configuration for existing keyword deployments. Changed host/database/credential/
TLS fails equivalence validation. Pool-only tuning is consumed by native pgx
parsing before conversion. Final focused compatibility race proof: **1.374s PASS**;
bounded native/CLI lint: **zero issues**. Earlier failed images are not attributed
as successful final packaged proof.

Canonical generation completed successfully, adding only the two unused SQLC
recovery table model types; public HTTP/TypeScript contracts are unchanged. A
read-only symlink to existing browser tooling resolved the isolated worktree's
initial missing generator. No shared dependencies were modified. ShellCheck,
Bash syntax and final `go mod tidy -diff` are clean. Native recovery/CLI/testdb lint
previously completed with zero issues. Local full-repository Go/browser campaigns
were not repeated; exact-head CI must supply those required checks before merge.

The final documentation/generated-model commit follows the executed packaged
source above. Generated additions are unused table types; no tested recovery
behavior or operator script changed after that source was frozen.
