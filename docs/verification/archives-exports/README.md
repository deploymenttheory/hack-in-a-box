# Issue #119: archive/export implementation and verification

Draft PR #268 is based on main `18b23f4e3201f9ab0d10ef3875628585f0ffd74f`.
This is the complete source candidate, pending final local gates, independent
Standards/Spec review and every required exact-head CI check. The issue stays open.

## Native source and authority inventory

| Surface | Current authority and projection |
| --- | --- |
| Archive directory/detail | Native EventRead; metadata only for participants/platform administration without current StaffRead |
| Per-event/cross-event summary pages | Current organiser, bounded cursor pages; no global total or cross-event unique-person population |
| Summary CSV | Same StaffRead, deliberate export audit, no-store, every cell formula-neutralised |
| Roster JSON export | Native logistics/ProfileExport, operational field projection and existing retention deadline; no photographer consent or organiser shortcut |
| Formal deliverable export | Exact team and explicitly selected organiser/mentor/participant audience through Submissions.Report; frozen revision retained, no draft or borrowed audience |
| Own assessment export | Current assigned mentor/reviewer, own independent cards only; no organiser-wide scores/history |
| Recorded assessment context export | Exact context ID through real #104 moderation owner; context/version/binding and exact submitted cards, all current panel/evidence/author grants rechecked |
| Published project export | Event-bounded native cursor, positive deliberate publication projection, current audience/withdrawal/retention and native media-reference resolution; no old bytes or private evidence |
| Published feedback export | Existing audience/staff authorization; one frozen suppressed aggregate and current deliberately reviewed published prose, never identifiable responses |
| Objective/evidence export | Current StaffRead plus native outcome readability/retention; exact immutable objective/measure definitions and dated/source-labelled evidence |

The selected-source endpoint returns bounded JSON downloads and an explicit
source state. Recorded contexts are complete native bounded submitted sets;
`after` is invalid for that selection. Contradictory resource/context/audience
parameters are invalid rather than being recorded as misleading audit scope.
Exports audit only safe actor/event/resource-or-context references, revision and
count. No native credential, provider token, private browser storage or provider
effect is introduced.

## Reporting semantics and source gaps

Registration records, registered people, confirmed competitors, recorded
attendance, competition entries and formal frozen/not-submitted/withdrawn/unused
outcomes remain separate. `recorded_attendance_people` counts distinct recorded
presence; it has no unrelated registration denominator. Missing records and
explicit absence both mean zero recorded presence, never a physical census of
zero attendance. Physical census coverage is explicitly unavailable.

Missing formal native states are explicitly incomplete/unavailable, never
initialised or inferred open/not submitted by an archive read. Disbanded/inactive
entries remain unused/unscored and excluded from completed submissions.

Continued projects count current readable native `in_progress` records. Dated
adoption observations count explicit sourced evidence records, not unique adopted
systems or verified business quantity. Exact definition/version, baseline,
target, unit, measurement method/source/horizon, evidence basis/date/source,
nullable value/denominator and explicitly **current** follow-up status remain
available separately. Later planning revisions cannot retarget old evidence.
Expired/unavailable follow-up sources omit those metrics and carry explicit
availability; they never become a factual zero adoption/continuation claim.

Final judging (#108/#109) and published results (#113) do not exist yet. These
sources remain explicitly unavailable; no rank, award, result, transferred
unused work, code-based performance or ROI is invented.

## Native read-only and retention corrections

A real preparation → cancelled → archived event with no formal outcomes
reproduced late maintenance creating two `not_submitted` states/changes and
altering submission gate facts. The native submission owner now fences archived
settlement and skips archived `EnsureSubmissionState`. Page/history reads return
not found for missing source. Existing pre-archive assessment settlement is
unchanged; no second outcome store or cutoff-selection rule was added.

One native privacy helper evaluates the locked event/current policy with existing
calendar-day `RetentionDeadline`. Native logistics, operational export,
own/authorised profile reads and archive composition enforce expiry before
maintenance deletes rows. Missing retention anchors remain unavailable. The
change adds no duration, deletion workflow, role or competition-write exception.
Existing approved #117 follow-up stays in its dedicated audited native owner.

## Executed meaningful proof

All PostgreSQL checks used real isolated native PostgreSQL with
`HACKBOX_INTEGRATION=1 GOMAXPROCS=2 GOFLAGS=-p=1`. Provider behaviour was local and
deterministic; no live integration effect was performed.

- Reachable archive mutation RED → GREEN: exact event/fact/outcome snapshots
  remain unchanged through late maintenance and ordinary reads/history.
- Native logistics role/field/cursor/loss and safe audit: no photo-consent export,
  no blanket organiser privilege and no private fields after expiry.
- Exact objective/evidence provenance: original description/unit/baseline/target
  survive a later objective version; explicit zero differs from nil; estimate,
  self-report and dated sourced adoption evidence remain separate. Dedicated
  archived follow-up leaves competition revision/write flags unchanged.
- Retention RED → GREEN: before/at deadline for current 90-day and legitimately
  shortened one-day policies; expired rows still physically exist and no worker
  ran. Native logistics/export/profile reads deny; archive sources report expiry
  and omit unavailable outcome counts.
- Positive formal export: exact frozen revision and selected audience, private
  organiser evidence excluded from mentor output, wrong team/audience denied.
- Positive published projects: exact event cursor, private-source field exclusion,
  current publication withdrawal and retention. No private prose/author/owner
  field is copied into the released export.
- Positive published feedback: native small-cohort prose rejection is asserted,
  then the empty suppressed snapshot is explicitly approved/published/exported;
  no raw authored response, respondent identity or suppressed respondent count.
- Real recorded huddle: exact two submitted versions/context provenance, own-only
  export, wrong context, original-author/caller grant loss and superseded native
  submitted revision denial after archival. Safe audit identifies selected context.
- Real HTTP/OpenAPI response validation: authentication/current role loss,
  metadata restrictions, required source scopes, no-store, download headers,
  source availability and hostile CSV-cell neutralisation.
- Actual event-lock waiting before role-loss commit, native lifecycle/archive
  cancellation, current queued-operation eligibility and explicitly retained
  archived follow-up execution/history were exercised with the race detector.

Selected ten-package race campaign terminal success:
`go test -race ./internal/archives ./internal/privacy ./internal/attendance
./internal/submissions ./internal/scorecards ./internal/outcomes
./internal/surveys ./internal/event ./internal/jobs ./internal/httpapi -run
'TestArchive|TestOperationalArchive|TestNeverHeldArchive|TestProfileFieldPolicies|TestRetentionRemovesFields|TestSensitiveLogistics|TestCurrentAuthorityAndRecordedPanel|TestFinalCardSubmissionEvidence|TestNativeOrganisationMemberPitch|TestReviewedProjectPositiveProjection|TestFrozenSurveyReportSuppression|TestLifecycleConfirmedGate|TestQueuedOperationsRecheck|TestActualNativeArchivedPlan'
-count=1`: archives 14.986s, privacy 4.642s, attendance 2.893s, submissions 2.952s,
scorecards 6.987s, outcomes 7.477s, surveys 3.087s, event 3.020s, jobs 2.919s,
HTTP 4.792s.

Complete positive-composition campaign terminal success:
`go test -race ./internal/archives ./internal/moderation ./internal/submissions
./internal/httpapi -run
'TestArchive|TestArchivedAssessmentExport|TestOperationalArchive' -count=1`:
archives 43.048s, moderation 15.186s, submissions 8.570s, HTTP 13.225s.
Earlier focused HTTP proof passed 4.020s; formal export passed 3.598s.
Historical local logs are `/private/tmp/hackbox119-*`; these are owner proof,
not independent review or final-head CI claims. Later narrow scope validation,
final lint/generated parity and frontend contract checks are recorded below
when terminal; no broad repository-suite pass is implied.

## Integration and remaining gates

Canonical SQL/OpenAPI and generated Go/TypeScript contracts are maintained;
actual HTTP/runtime wiring uses configured native owner dependencies. One typed
survey browser fixture includes the new mandatory native provenance fields;
existing interaction/assertion behaviour is preserved. No production frontend
component behaviour/layout changed, so no replacement visual/style-guide claim
is made for this backend deliverable.

No migration or schema change is necessary: all reporting reads existing native
immutable sources. Rollback removes additive routes/query/read composition;
it does not undo retained native facts or append-only audit rows. The shared
scorecard migration test fixture is pinned outside DST folds without changing
production scheduling or rollback assertions.

Remaining gates: terminal final scope checks, generated parity, changed-owner
lint/typecheck; independent Standards and Spec review; and every exact-head
required CI check. Keep Draft until root verifies those gates and owns merge/
issue reconciliation.

### Final native source gates

Final contradictory-scope validation and native export role/field regression
passed with PostgreSQL/race (`TestArchiveExportRejectsMisleadingScopeBeforeAudit|
TestArchiveExportsPreserveNativeRoleAndUnavailableSources`): 4.771s. Final native
lint over archives/privacy/attendance/objectives/outcomes/scorecards/moderation/
surveys/submissions/HTTP completed with `0 issues`. No implementation scope
remains; generated parity and the scheduled frontend contract check precede the
review freeze.
