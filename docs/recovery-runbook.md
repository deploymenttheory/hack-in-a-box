# PostgreSQL, files and key recovery

This is the deployment operator procedure for issue #124. It restores a trusted,
quiesced logical snapshot into a **new** PostgreSQL database and **new** storage
path. The recovered API, admin API and worker remain held until an operator
reviews the current pending-action inventory. It never resets original operation
IDs, treats unknown effects as successful, approves old commands, or contacts a
provider as part of backup/verify/restore/resume.

## Build and prerequisites

Build the reviewed application revision's independent operator target:

```sh
docker build --target recovery -t hackbox-recovery:REVIEWED_REVISION .
```

The image includes the recovery command, existing migrate/worker entrypoints and
pinned PostgreSQL 17.7 clients. It does not need Node, either web portal or host
`pg_dump`/`psql`. Match the database major version and the exact native schema required by that
reviewed application revision; the schema recorded by a historical rehearsal is
not the schema of every later release. No
application service migrates itself on startup. Keep the reviewed image digest
with the backup inventory; the encrypted manifest records its backup UUID, schema,
UTC capture interval, object identities/sizes/checksums and ciphertext hashes.

Run the command as UID/GID 10001 with private writable directories owned by that
identity (0700 directories/0600 artifacts). Use a dedicated private environment
file containing `HACKBOX_DATABASE_URL`, supplied with Docker `--env-file`; never
put a credential value in command arguments, shell history or a report. Mount the
correct existing native files volume, archive destination and recipient file
explicitly. The existing `HACKBOX_DATABASE_URL` remains the pgx deployment connection string;
ordinary keyword-style DSNs and pgx pool tuning remain supported. For a keyword
DSN, put an explicit equivalent PostgreSQL client URI in the separate private
`HACKBOX_RECOVERY_POSTGRES_URI` environment setting. Native pgx parsing compares
host, database, user, password, TLS, fallback and startup configuration before any
client is launched; a mismatch fails closed. Do not modify the deployed application
connection merely for recovery. For existing URI deployments no second setting is
needed; pgx-consumed pool-only parameters are validated and omitted from the libpq
client environment. Unsupported/ambiguous client options fail closed, and no TLS
mode or credential is approximated. URI fields are decoded into documented
PostgreSQL client variables through
[libpq's environment](https://www.postgresql.org/docs/17/libpq-envars.html), not
arguments; raw client stderr is not printed.

## Keys and custody

Generate a separate archive identity once, outside the backup destination:

```sh
/app/recovery -action keygen \
  -identity /identity/archive.key -recipients /recipients/archive.pub
```

Run that inside the operator image with separately mounted private identity and
public-recipient directories. It refuses existing files and prints only the public
recipient. Keep the identity in a separate access-controlled recovery store; a
backup operator needs only the public recipient. Restore operators need the exact
archive identity. Losing it makes that backup unusable; neither the application,
manifest nor PostgreSQL can recreate it. Test key custody independently of a
source-host failure. Retain old identities until all backups encrypted to them
expire under the deployment's existing backup-retention policy.

Archive encryption and **deployment credential encryption** are different keys.
Do not copy `.env`, SMTP/OIDC/GitHub secrets or `HACKBOX_CREDENTIAL_KEYRING` into a
bundle. Reprovision them separately. Retain the exact historical credential key
IDs **and material** needed by the selected backup, plus the active deployment key;
a new active key alone cannot decrypt an older envelope. See
[team credential recovery](team-credential-recovery.md). Missing/wrong deployment
keys leave safe metadata available but native reveal/create/rotate fails closed;
a missing/wrong archive identity prevents any restoration.

## Consistent capture

1. Choose an operator-controlled private archive directory and the exact native
   database/files pair. Stop **that deployment's** API, admin API and worker using
   its existing deployment controls. Also stop direct writers/importers. The tool
   never stops a container for you. Keep the source quiesced throughout capture.
2. Capture with the operator image on the deployment's database network, source
   files mounted read-only, public recipient mounted read-only, and only the
   archive directory writable:

   ```sh
   /app/recovery -action backup -files /data/files \
     -bundle /archives/BACKUP_NAME -recipients /recipients/archive.pub
   ```

   `BACKUP_NAME` must not exist. A dedicated exclusive database lease refuses a
   currently leased native API/worker and prevents new native startup during
   capture. This supplements the explicit quiescence requirement; it is not a
   substitute for stopping independent/direct database writers or checking that
   the intended deployment is stopped. Existing processes from older binaries do
   not implement this lease and must also be stopped.
3. The database dump and standard file tar stream go directly into maintained
   [age authenticated encryption](https://pkg.go.dev/filippo.io/age#Encrypt). Only
   ciphertext is staged. Referenced native files must match their recorded sizes
   and SHA-256 values; missing/changed objects abort capture. Unreferenced orphans
   are excluded, never silently substituted for missing references. The completed
   three-artifact directory is published atomically after final authentication and
   filesystem sync. An incomplete temporary directory is not a backup.
4. Independently verify using the separately held identity:

   ```sh
   /app/recovery -action verify -bundle /archives/BACKUP_NAME \
     -identity /identity/archive.key
   ```

   Verify reads every encrypted stream through authenticated EOF, validates the
   exact inventory and rejects unexpected entries, symlinks, unsafe tar paths,
   extra records, oversized/inconsistent entries and checksum mismatches. A
   successful JSON manifest reports the selected capture; it is not proof of any
   provider delivery. Resume the original deployment only after capture succeeds
   or the failure is understood; capture does not change its approvals or hold.

Backups contain private application data even though encrypted. Store completed
bundles privately, copy them through the existing approved backup process, and
verify the copied bundle. There is no new deletion schedule or retention duration
in this change. Trust the backup's origin: age authenticates the encrypted stream,
not the identity of its author. PostgreSQL documents that
[restoring SQL executes code from the source database](https://www.postgresql.org/docs/17/app-pgdump.html).
Only restore a trusted capture; hashes are not a signature from a known operator.

## Fresh restore, still held

1. Provision a fresh PostgreSQL 17 server/database and new durable files volume on
   an isolated recovery deployment/network. Do **not** run migrations or start any
   API/admin/worker yet. Supply a suitable fresh database owner through the
   recovery environment file; global PostgreSQL roles/passwords are not in this
   logical dump. Do not target a running deployment.
2. Mount the trusted encrypted bundle and archive identity read-only. Mount the
   new files volume at `/data/files`, and choose the absent path
   `/data/files/restored` as its destination:

   ```sh
   /app/recovery -action restore -bundle /archives/BACKUP_NAME \
     -identity /identity/archive.key -files /data/files/restored
   ```

   The tool seals its own private ciphertext copy and verifies all authentication,
   paths and hashes before consuming SQL or extracting objects. It refuses an
   existing database or destination. Files are extracted into a private staging
   directory with exclusive regular-file creation. The database stream and a new
   hold incarnation are committed together using
   [psql stop-on-error/single-transaction mode](https://www.postgresql.org/docs/17/app-psql.html).
   Schema and exact restored file-table metadata must match before the files path
   is published. A failed restore is not a usable installation; keep it isolated
   and held, inspect the failure, and prepare a fresh target for another attempt.
3. Configure **all three** recovered runtime services to use that exact restored
   path. For Compose, a recovery override must set `HACKBOX_STORAGE_PATH` to
   `/data/files/restored` in `api`, `admin` and `worker`, and select the fresh
   database/files volumes/project. The default Compose path is `/data/files`;
   do not accidentally open that parent instead. Restore the separate deployment
   configuration/keyring without printing its values.
4. Check native safe metadata and current field/resource permissions, expected
   attachment bytes, exact rubric/rules references, private help/clarification
   history, outcome evidence and approved follow-up schedules. The rehearsal below
   proves these against native owners. Competition outcomes/assessments are not
   recomputed or rewritten by recovery. Archived competition stays read-only;
   dedicated retained follow-up eligibility stays with its existing owner.

A recovered worker may be started to observe the hold: it waits **before** opening
or cleaning files, constructing provider runtimes, maintenance, lease recovery,
reconciliation and River startup. The APIs also wait before files/provider/auth
wiring. A River queue pause or event automation pause alone is insufficient.
Restarting a process does not remove this database hold. Schema rollback refuses
an active hold, so an older binary cannot silently bypass it.

## Review and deliberate resume

```sh
/app/recovery -action status
```

The safe typed inventory includes generic pending/running/ambiguous/permanent
operations; unexecuted schedules; SMTP test/campaign state; prepared/retryable/
ambiguous email verifications; unresolved repository/access effects; communication
occurrences; approved native outcome-review schedules; and scheduled content
release. It contains exact native IDs, revisions, source references, state, timing
and attempts, never message bodies, mailboxes, private notes, credential values or
provider secrets. `review_entries` counts metadata records, **not** distinct effects
or people. Multiple records can describe one original effect. Its fingerprint
changes when the reviewed inventory changes.

Review pending/ambiguous work and retained native receipts against the selected
snapshot and any subsequently observed provider history before enabling runtime.
A restored `pending` row can predate a later real-world delivery. Preserve unknown
status unless its native owner has actual evidence; a current provider object or
an ordinary GET does not prove the original command completed. Do not reset IDs,
mark ambiguous rows delivered, grant blanket approval or manually replay them.
Use the existing native reconciliation/approval contract; if the evidence is not
sufficient to resume safely, leave the recovery held.

Once deliberately reviewed, use the **current** hold ID and fingerprint:

```sh
/app/recovery -action resume -hold EXACT_HOLD_UUID -review EXACT_FINGERPRINT
```

This writes immutable operator maintenance history and clears that hold only if
the locked current inventory still matches. It does not change business approvals,
receipts, original operation IDs, attempt history or unknown states. Stale reviews,
a different hold incarnation, and concurrent repeat resumes fail closed. Then
start the independently configured API/admin/worker as appropriate; native current
roles, event locks, source versions, retention windows and idempotency/reconciliation
rules continue to govern execution. An operationally reversible hold can be placed
again only after explicitly quiescing the deployment; it does not undo an external
effect already performed after a previous resume.

## Rehearsal and measured recovery bounds

```sh
HACKBOX_INTEGRATION=1 GOMAXPROCS=2 GOFLAGS=-p=1 \
  go test -race ./internal/recovery ./internal/credentials \
  -run 'TestRecovery|TestArchive|TestFreshEncryptedRecovery' -count=1 -v

HACKBOX_RECOVERY_IMAGE=hackbox-recovery:REVIEWED_REVISION \
  ./scripts/rehearse-recovery.sh
```

The first command needs Go and Docker. The second needs Docker, POSIX utilities,
OpenSSL and Python 3 **only** on its host; PostgreSQL clients and application
commands run in the operator image. The script creates a unique isolated network,
source/target servers and private named volumes; it cleans up only those resources.
It never loads the deployment `.env`, reads a user project or stops its containers.
Its known synthetic pending/ambiguous inventory is checked before automated test
resume; never copy that automatic approval into a production procedure.

The native real-PG dataset uses 1,000 synthetic active accounts, one released
competition event and a separate completed follow-up event, exact approved rubric
and released rules, resolved help with three transitions, a private question and
approved/published common answer with native inbox references, dated outcome
adoption evidence, an explicitly approved 30-day follow-up, an encrypted native
credential, an attachment and original pending/ambiguous operation/attempt history.
Seventeen nonempty native history tables are compared exactly after fresh-server
restore. Native reads recheck current authority, including stranger denial.

See the [executed verification record](verification/recovery/README.md) for the
actual measured capture/restore intervals, dataset sizes, packaged-command result
and exact source provenance. These are measured rehearsal results, **not** a
promised production SLA or a claim to cover a production-size media corpus. Image
build/pull, infrastructure provisioning, obtaining external keys and human review
are stated separately. The encrypted manifest's quiesced capture interval defines
the recoverable snapshot. Subsequent source changes are intentionally absent from
restore: data after that capture is the observed loss window. Backup cadence
therefore bounds RPO; this logical procedure adds no PITR or zero-loss guarantee.
