# Repository access verification

Verified on actual merged `5c6c026` (webhook schema 46) plus the separate #93
implementation and normal migration 47. Released migrations 44, 45 and 46 compare
byte-for-byte unchanged. Provider effects use deterministic in-process/HTTP
fixtures; PostgreSQL fixtures are disposable testcontainers.

## Acceptance evidence

| Requirement | Fresh proof |
| --- | --- |
| Deduplicate pending, accepted, expired and failed invitations | `TestReviewedRepositoryAccessRealPostgres`: pending request retained once, accepted current-source observation, cancel-before-replace expiry, explicit policy failure and new reviewed recovery. |
| Native changes produce a reviewable plan; remove only app-managed grants | Native withdrawal creates an unapproved maintenance plan without sending a mutation. Exact approval is rechecked by the actual jobs worker. Unknown/direct existing access remains unmanaged. |
| Flag external access and provider policy | Pure proposal and actual runtime cases retain unknown collaborator flags and explicit rejection. Bounded HTTP transport observes all visible access sources and uses repository-scoped tokens. |
| Preserve freeze and prevent stale grants | Actual native team service rejects frozen reassignment. Stale verified-link revision and withdrawn-claim jobs cannot grant. Desired access uses published rather than draft membership. Repository identity/provenance is immutable. |
| Continue legitimate cleanup without changing assignments | Current claim withdrawal and published mentor window expiry revoke only proven app-managed access. Frozen team/mentor rows remain retained. Native team/repository compatibility suites include empty published group retirement and retained unused repositories. |
| Retain uncertain effects durably | Lost provider response and deferred receipt-commit failure retain the committed attempt barrier. Concurrent worker ownership snooze is expected; exactly one provider mutation is issued. Reconciliation never resends uncertain mutations. |
| Current verified account and mentor authority | Numeric ID and link revision, current confirmed claim/resource permissions and exact published mentor windows are fingerprinted and rechecked at execution. Mentor grants use read permission. |
| Actual publication and privacy | No write jobs before freeze and actual challenge publication. Real HTTP checks cover CSRF, revision, authority, withdrawal, closed writes, no-store, own-only scope, strict DTOs and exact receipt versus latest plan. |
| Current signed webhook evidence | Actual signed member ingestion and current source validation atomically enqueue the real GET-only poll. Duplicates, stale installations and reordered action evidence cannot manufacture membership state. |
| Preserve predecessor and attempt history | Actual 46→47 populated registry preservation, empty rollback, populated rollback rejection, immutable approval/identity/dispatch and DELETE/TRUNCATE protections. Historical 45/46 webhook proofs remain intact. |

## Completed checks

All commands below completed successfully, with `HACKBOX_INTEGRATION=1` for the
real PostgreSQL cases and the race detector enabled:

- Full `internal/repositoryaccess` package, including all nine runtime fault,
  coverage and invitation subcases and migration/rollback proofs: PASS (7.469s).
- Full affected `internal/githubapp` package after the cleanup correction: PASS
  (11.844s), including the native App owner and scoped transport regressions.
- Full affected `internal/githubwebhooks` package after historical fixture
  compatibility corrections: PASS (22.084s), including the actual member sink.
- Full native `repositories`, `githubidentity`, `mentors` and `teams` packages:
  PASS (17.698s, 7.052s, 29.779s and 35.499s respectively).
- All five repository/GitHub HTTP integration tests: PASS (28.612s).
- Canonical generated-output check, full Go lint, vet and build: PASS.
- Web lint and both portal production builds for the generated API client: PASS.
  Production UI source is outside this PR.

## Failures retained and resolved

The initial provisional concurrency assertion rejected the worker's normal
ownership snooze. The assertion now accepts that typed ownership result and still
requires exactly one provider mutation; no product retry behavior changed.

The first final combined native run passed six packages but failed historical
webhook fixture tails: they intentionally left PostgreSQL at version 46 and then
checked compatibility with the now-version-47 binary. Their fixed historical
version/rollback/history assertions are unchanged. The fixture tails now apply
later normal migrations before current compatibility checks and also compare the
retained history after that upgrade. The complete affected package then passed.

A new red regression exposed an unrelated template-health dependency in the App
collaborator owner. That dependency could block cleanup after template removal.
The correction separately checks current App/install/org/suspension and exact
repository-scoped permissions; disconnected, mismatched or suspended installation
checks still prevent writes. The regression and the complete App package passed.

No local full-repository PostgreSQL run or browser-suite result is claimed here.
The scoped native runtime/HTTP and full static/build checks above are fresh;
exact-head PR CI remains the complete repository gate.
