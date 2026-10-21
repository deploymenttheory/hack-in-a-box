# Organiser workspace verification

The approved follow-on batch is #34, #33, #39, #45, #40, #35, #37, #41, #36 and #38. Changes are delivered as dependent PRs so the policy, lifecycle, interface and privacy work can be reviewed separately.

| Issue | Implementation | Evidence |
| --- | --- | --- |
| #34 | Current event/resource policies, transactional role and owner changes, irreversible competition exclusions, registration eligibility boundary | PostgreSQL role/resource matrix, cross-event IDs, revocation, last-organiser protection, registration/grant races, generated HTTP response validation. |
| #33 | Validated local login return paths, expired-session feedback, account-scoped safe drafts and logout cleanup | Signed OIDC callback returns to its original page; desktop/mobile return-path, logout and account-switch tests. |
| #39 | Event directory APIs, configuration, optimistic revisions, explicit lifecycle transitions and transactional domain gate contracts | Real PostgreSQL checks for confirmed-only thresholds, explicit go action, withdrawal continuity, post-publication attendance exceptions, cutoff/outcome gates, reopening reasons and archive write/job protection; HTTP schema validation. |
| #45 | Event-local calendar arithmetic, business calendars, explicit DST resolution, schedule preview/approval and obsolete-work cancellation | Spring/autumn seven-calendar-day tests (167/169 elapsed hours), gap/fold handling, excluded dates, stale edits, cancelled approvals and preservation of executed actions. |
| #40 | Event directory/switching, draft setup, local-time previews, readiness blockers, stage decisions and protected unsaved edits | Desktop/mobile creation and two-event navigation, invalid fields, blocked transitions, draft cleanup and navigation protection; visually checked at 1440px and 390px. |
| #35 | Event staff roles, lasting eligibility explanations, owner transfer, revocation and assignment history | Browser grant/acknowledgment and last-owner conflict checks; real HTTP ownership/revocation checks. Idempotent role edits do not fabricate new assignment history. |

| #37 | Field-specific operational profile DTOs, audited logistics exports, self-correction and configurable retention/deletion | Role-by-role HTTP serialization, export/cross-event denial, revision conflicts, retention and safe aggregates. |
| #41 | Immutable template versions/publications and independent event configuration snapshots | Reference remapping, DST rebasing, clone retries/rollback, no private/live records and source-edit independence. |
| #36 | Separate GitHub OAuth identity verification with PKCE, encrypted tokens and invitation review | Deterministic OAuth/API fixture, forged/replayed callbacks, renamed logins, duplicate IDs, unlink/relink, HTTP/desktop/mobile checks. |
| #38 | Shared two-event authorization fixtures covering HTTP, files, exports, queued jobs and per-emission policy | Revoked/overlapping grants, active/waitlisted eligibility conflicts, transaction races and a deliberate event-filter mutation that fails the suite. |

## Domain boundaries

Registration writes `competition_claims` through `permissions.SetRegistrationState` inside its own transaction. The function shares the event row lock with role changes. #51 supplies real confirmed-attendee facts; no API in this batch grants a competitor place.

Resource owners register `(event_id, id, kind)` in `policy_resources` and maintain `resource_assignments` transactionally. Mentor/judge and credential access requires current scoped authority; platform administration does not implicitly grant private event data. Future team, content and assessment modules extend this boundary and its test matrix.

Existing foundation databases have no event ownership model. Platform administrators can explicitly assign an existing event owner through the protected ownership endpoint; the new owner receives an organiser grant and permanent event-specific competition exclusion. Acknowledgment is required. Existing events are not silently assigned to every administrator.

Event creation starts in draft. #51 marks attendance facts ready and supplies confirmed claims; #59 records frozen team facts; #60 records frozen/not-submitted/withdrawn outcomes; #69 checks publication readiness in its publication transaction and owns the exact scheduled-time enforcement. Missing facts block the relevant transition. These are internal transaction contracts, never client-editable counters.

Post-publication postponement stores an immutable reason and an exact revised-date notice with a durable `event.rescheduled` operation. Communication delivery is owned by #82; a pending operation is not evidence that anyone has been notified. Schedule approvals similarly become delivery inputs when automation is installed. Replanning invalidates approvals and cancels linked unsent operations; executed actions retain their execution state.

Publication uses seven calendar days at the same event-local clock time. A DST gap requires choosing a different time; a repeated time requires an explicit earlier/later choice. Business-day rules apply to relative agenda/offer calculations and never change the challenge-publication interval.

## Verification commands

```sh
HACKBOX_INTEGRATION=1 go test -race -timeout 5m ./cmd/... ./internal/...
golangci-lint run ./cmd/... ./internal/...
npm --prefix web run lint
npm --prefix web run build
npm --prefix web test
npm --prefix web run test:e2e
sh scripts/check-generated.sh
```

## Privacy and reusable templates (#37, #41)

`internal/privacy/service_test.go` and the HTTP contracts verify self-correction,
optimistic revisions, field-level payloads (logistics contact/dietary/accessibility;
photographers consent only), scoped exports and revocation. Retention defaults to
90 days after completion/cancellation, with seven-day private drafts; organisers
can configure both. A bounded worker sweep deletes fields/drafts and keeps only
anonymous removal totals plus safe decision audit records. Administrators can
remove a profile without gaining permission to read its operational fields.

`internal/templates/service_test.go` verifies immutable versions/publications,
independent event snapshots, ID/reference remapping, inline raster branding by
value, draft schedules without approval/delivery state, transactional failure and
idempotent clone retries. The London DST fixture keeps challenge publication seven
calendar days before live start at the same local time. Clone definitions are an
explicit allowlist; participant, assessment, credential and delivery records never
enter the copy. Existing events remain independent of subsequent publications.

Current event organisers can create events/templates; installation administrators
bootstrap the first event. The session exposes this capability for the creation
screen. Creating a new event still requires the lasting competition-exclusion
acknowledgment and grants its creator the organiser role. Template editing APIs are
included here; the template editor and domain execution remain in their own issues.

Verification: full Go suite with `HACKBOX_INTEGRATION=1` and `-race`, HTTP response
schema validation, zero Go lint findings, frontend build/unit/browser checks.


## GitHub identity and authorization regression checks (#36, #38)

See [GitHub setup](github-identity.md) for provider configuration and the separate
repository-invitation contract. The account screen explains verification and
unlink/relink impact; it does not treat a typed login as verified ownership.

The shared `testdb.NewAuthorization` fixture supplies two events and an overlapping
person. File tests prove that a grant in one event does not authorize another
event's bytes and that revocation is checked on the next download. Background
fixtures queue approved work, revoke the judge grant or resource assignment, and
verify that delivery does not occur. The stream test is a **per-emission policy
contract**, not an SSE implementation: #85 still owns the live transport and must
call the same current policy before reading/serializing every emission.

The permission suite covers received, waitlisted and confirmed registrations
against organiser/advance-access grants, persistent exclusions after revocation
without opening a brief, eligibility in a different event, and concurrent
registration/grant attempts. Profile HTTP tests use a logistics person with
legitimate grants in both events and a profile present in only one event.

Mutation verification changed only the operational-profile query's event filter
from `p.event_id=$1` to a non-filtering UUID check, regenerated sqlc, and ran
`TestPrivacyAndTemplateHTTPContracts`. The suite failed with
`event-scope predicate missing: 200` instead of the required 404. The original SQL
and generated binding were restored byte-for-byte, and the unmodified suite was
rerun successfully. The probe was temporary and is not part of the implementation.

Final checks: real PostgreSQL race suite; OpenAPI request/response validation;
Go lint and module advisory scan; frontend lint/build, five component tests and
26 desktop/mobile browser tests; Docker Compose migration, startup, non-root
runtime and restart persistence. Visual screenshots use synthetic identities only.
