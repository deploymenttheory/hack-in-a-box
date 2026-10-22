# Foundation aggregate acceptance audit

This audit evaluates epic #8 and its ten native children (#22–31) against merged
baseline `2baf40edeede530c7b58c449af794d4a1d72ae76`, on 22 October 2026. Closed
children are supporting context; the assertions and execution evidence below
establish aggregate acceptance. Publication and closure still require this
audit's own reviewed PR and successful CI at its final head.

## Requirement-to-evidence map

| Task | Inspected implementation and assertions |
| --- | --- |
| #22 API/bootstrap | `cmd/api`, `cmd/worker`, `internal/config` and `internal/httpserver` separate startup, validated configuration and bounded shutdown. `TestLoadRejectsInvalidConfigurationWithoutValues` checks safe configuration errors. Router tests cover error code/message/request ID, unknown-length body limits, panic redaction and a handler that ignores its timeout. `TestShutdownDrainsRequestsAndStopsAcceptingConnections` verifies both normal draining and forced deadline shutdown using actual TCP requests. |
| #23 application shell | The public and admin builds are separate static outputs and Go processes. `TestAssetsDeepLinksAndAPIBoundary` checks static/deep-link serving without swallowing API routes. `web/e2e/shell.spec.ts` exercises navigation, narrow layouts, safe API errors and private workspace disposal on session expiry. `session.tsx` clears private query state on identity/session loss; `api/client.ts` maps errors to fixed client messages. |
| #24 database | `TestMigrationUpgradeConcurrencyAndTransactions` starts at schema 1, retains its event across three concurrent migrators, checks current/future schema compatibility, and rolls back a domain update and operation together after a synthetic failure. `database.Within` supplies transaction-bound sqlc queries. Goose and River migrations share a dedicated migration lock. |
| #25 API contract | `api/openapi.yaml` is the versioned source; generated strict Go handlers/models and the TypeScript schema feed `openapi-fetch`. HTTP fixtures validate real response bodies against the embedded OpenAPI document. Registration/offer APIs keep registrations, offers and confirmed competition entries distinct. The generator gate rejects drift, independently demonstrated below. |
| #26 packaging | `Dockerfile` pins build/runtime images by digest, compiles the commands and both frontend surfaces, and runs as UID 10001. Compose separates migration, participant API, admin and worker services with persistent PostgreSQL/files volumes. `scripts/smoke-compose.sh` checks fresh startup, readiness, deep links, non-root execution, database/file persistence after restart, and continued admin availability while the participant API is stopped. Inherited exact-head image CI is linked below. |
| #27 durable work | `TestDurableEnqueueTwoWorkersAndRecovery` proves stable operation identity, duplicate enqueue handling, one live dispatch despite an expired lease, and crash recovery. `TestApprovalCancellationRollbackAndOutcomes` asserts atomic operation/River rollback, unapproved and cancelled work staying inactive, and no blind replay of permanent/ambiguous outcomes. `authorization_test.go` rechecks current event/role/assignment eligibility. |
| #28 private files | Storage tests reject traversal, invalid media, oversized and interrupted writes and remove incomplete objects. Service tests cover quota races, finalisation/idempotency, ownership and missing-file errors. The HTTP integration test checks exact bytes, attachment disposition, `nosniff` and authenticated access. Domain-specific file authorization remains enforced by current policy. |
| #29 audit | `TestAuditRollbackScopeAndSafeMetadata` proves transaction rollback, event-scoped queries, rejection of arbitrary confidential free text/unknown actions and database rejection of audit updates. HTTP integration covers the protected query/export and revocation through the same existing session. Domain audits use bounded IDs/actions/reasons rather than confidential content. |
| #30 operations | `TestAuthenticatedContractFilesAuditAndOperations` inserts stalled work, checks protected identifier-free lag metrics, injects synthetic secrets into headers/query/body/cookies and verifies log redaction, then closes PostgreSQL and asserts liveness 200/readiness 503. `operations.go` exposes request/error/latency, failed/ambiguous delivery and recovered-lease signals without participant labels. `docs/operations.md` defines alert thresholds and recovery actions. |
| #31 CI | Application and Go workflows fail on lint, build, race tests, browser checks, generated drift and Docker smoke failures. Actions are commit-pinned with read-only contents permission; tool/dependency versions and lockfiles are checked in. The independent negative checks below verify real nonzero failures rather than inferring them from workflow text. |

## Independent execution

All database tests used isolated real PostgreSQL with synthetic identities, files,
operations and keys. No production/provider credentials were configured.

- `go test -race ./internal/config ./internal/httpserver ./internal/database ./internal/files ./internal/audit ./internal/jobs`
  passed: config 1.224s, shutdown 1.295s, database 4.396s, files 6.414s, audit
  3.868s and jobs 8.774s.
- The targeted HTTP race run passed in 7.440s: authenticated contract/files/audit/
  operations, assets/deep links, health/routing errors, unknown-length body limits,
  panic redaction, uncooperative-handler timeout and private session-cookie cache
  behavior.
- A locked `npm ci`, frontend lint, both production builds, all six component
  tests, and `go build ./cmd/... ./internal/...` passed in the isolated checkout.
- The config package's normal Go lint command passed. Adding a temporary unused
  function made the same lint command exit 1 with an `unused` diagnostic.
- Adding a temporary unused frontend variable made the normal frontend lint
  command exit 1 with `@typescript-eslint/no-unused-vars`.
- Changing only the OpenAPI title and running `scripts/check-generated.sh` made
  it regenerate the embedded contract and exit 1 on generated-code drift. Every
  source/generated file and probe was restored afterward; the checkout was clean.
- A separate synthetic error-field type change made the same gate exit 1 and
  produce a verified diff in `web/src/api/schema.gen.ts`, specifically proving
  client type drift detection. That contract and all generated files were also
  restored before committing this audit.

The complete browser and Compose verification is inherited from PR #185 at
reviewed source `942bb65fb347aa54d31d3eab71b3b7c12f0fc741`, merged as the audited
baseline. Its exact commit check records were read back as completed/success:
[browser/build checks](https://github.com/deploymenttheory/hack-in-a-box/actions/runs/39823029509/job/119978976481),
[Docker startup/persistence smoke](https://github.com/deploymenttheory/hack-in-a-box/actions/runs/39823029509/job/119978975678),
and [complete Go race/generated checks](https://github.com/deploymenttheory/hack-in-a-box/actions/runs/39823029571/job/119978975867).
These are explicitly inherited results, not a claim that the smoke script was
rerun during this audit. This documentation PR still needs its own final-head CI.

## Aggregate conclusion and limits

The audit found and corrected one worker-boundary defect: when `Execute` deferred
an unapproved or currently ineligible operation, `Work` returned success and
River completed its job. Retained completed-job uniqueness then obstructed later
reconciliation. The real-worker regression reproduced both paths as
`deferred work became completed instead of remaining scheduled; deliveries=0`.
`Work` now snoozes while the operation remains pending and uncancelled, preserving
the same operation key and current-authority checks without consuming retries or
changing River uniqueness. Approval/eligibility restoration resumes one delivery;
cancellation completes the deferred job without an effect. The complete jobs
race suite passed in 20.502s, with zero scoped lint findings. The Teams adapter's
independent real-worker 429/cooldown reproduction also caught this same defect:
it failed before the correction in 4.77s and passed with fix commit `14f1489` in
35.768s. That local TLS fixture observed one rate-limited request followed by two
accepted sends, including a second operation queued during the shared cooldown.
The complete Teams integration acceptance remains with #80.

The foundation has an explicit generated API boundary with current server-side
resource policies, durable transaction/worker behavior, private persistent
storage, safe audit/operations views and exercised failure/recovery paths. The
evidence uses synthetic collateral; no live attendee data, real private keys or
provider tokens were introduced by this audit.

This is foundation acceptance, not acceptance of the later communication,
mentoring, judging or production-release epics. Live provider verification,
release load/security/accessibility rehearsals and complete installation recovery
remain with their own open tickets. The independently deployed admin process
still shares PostgreSQL and the file volume; its availability test establishes
independence from the participant process, not immunity to shared dependency
outages.
