# Native post-event surveys

A survey retains an approved native form version, one native role audience,
opening/closing instants and optional exact objective/measure references. Its
published question version cannot be edited. Publication fixes invitations from
current eligible people; later grants do not silently add respondents. Answering
requires both a retained invitation and current eligibility, rechecked under the
event lock at the actual write. Only the author and current event organisers can
read identifiable answers. The interface explicitly states that this is not an
anonymous survey. Names are not used to infer addresses or link identities.

Responses use native bounded form validation, including pruning hidden answers.
One person has one current response with immutable retained revisions. An explicit
same-command retry returns its original persisted revision and timestamp even after
a later response edit. It is idempotent while the survey is open; it cannot bypass close,
withdrawal, account deactivation or archive. List projections omit answer bodies.
API responses are private/no-store. The inbox destination opens the native own-survey reader; the separate survey
workspace supplies authoring, explicit response commands and reviewed reports.
See [the workspace contract](survey-workspace.md) for the organiser-only current
administrative counts and private export, which never become public aggregate
denominators.
Opening, closing and submission times are displayed in the native event timezone.

Save, approval and publication recheck that the approved opening does not precede
the current native live end. Responses and new invitations also require that the
current live end has passed, in addition to the retained opening/closing window.
A later event-date edit cannot silently move the approved survey dates. If the
new end passes the retained close, the window remains missed. New report review,
approval and publication wait for the current live end; previously published
reports and legitimate private response history remain readable under their
normal current-authority/retention rules. No completed-stage gate is introduced.

Invitations use the existing reviewed campaign, exact template/audience versions
and approved message body. Native `survey.open` selects the retained invitation
cohort intersected with current eligibility and people who have not answered.
There is no direct survey publication notification. Its optional typed native
resource reference contains only survey identity/version; the inbox rechecks both
retained campaign recipient authority and current survey-reader authority.
Answering, cancellation and close stop new sends without automatically removing
legitimate own retained history. Withdrawal, inactive accounts and expired private
retention remove that reader authority. No archived-event write exception exists.

## Wider reports

Reports are unavailable before close. The first explicit report review freezes one
immutable aggregate for that survey version from its then-current eligible response
cohort. There is no live dashboard, arbitrary filter, role breakdown, denominator
endpoint or alternate snapshot. Reviewed prose corrections retain the same frozen
aggregate, with their own explicit approval/publication revision and timestamp.
An unreviewed correction never replaces published text. The reviewer must explicitly
confirm reviewed/redacted wording; a mandatory second independent organiser is not
invented by this contract.

The configurable minimum cohort is at least five. Below it the entire report,
including the respondent count, is suppressed and nonempty prose is rejected.
For each fixed-choice question, any small positive answer cell, complementary cell,
unknown count or answered cohort suppresses the whole question, its denominators
and its cells. Text is never automatically copied, counted or promoted. Suppressed
and unknown values are null, not zero; an unsuppressed genuinely known zero remains
zero. Rating distributions retain the same rules and do not expose a mean that
could reconstruct hidden cells. Multiple-choice totals may exceed respondents.

This is a bounded disclosure policy, not differential privacy or a guarantee
against external knowledge or comparisons with independently collected surveys.
The product does not prohibit all future overlapping surveys. Fixed snapshots,
whole-question complementary suppression and deliberate prose review prevent the
within-survey live/filter/correction differencing paths this API would otherwise
introduce. Organisers must avoid identifying details in deliberately published
prose. Self-reported learning/adoption is labelled as self-report and never claims
independently evidenced adoption or financial return. Exact question and measure
versions remain attached; later objective edits cannot retarget an existing report.

## Retention and removal

The existing event privacy deadline applies to identifiable invitations, answer
revisions and command identities. Reads fail closed at the deadline, before the
maintenance worker runs. The existing authenticated self/administrative removal
operation erases these records even when the person has no operational profile.
Scheduled maintenance uses the same privacy boundary under the event lock.
Reviewed suppressed aggregates remain independently retained with no respondent
mapping or raw-answer hash in the aggregate or erased survey stores. Separate,
organiser-restricted append-only audit records retain actors, survey/resource IDs,
revisions, actions and times; removal does not promise erasure of that safe
participation metadata. Answers and reviewed prose never enter audit/log metadata.

Normal migration40 follows actual39 and preserves populated native objective,
scorecard, moderation and feedback history. Empty rollback is supported; populated
survey history refuses rollback. No missing/out-of-order Goose policy is enabled.

## Independent review corrections

Native PostgreSQL regressions demonstrate that an old command returns its original
revision after a later edit, and that the first acknowledgement is identical to
the retained retry receipt with a ticking clock and database timestamp precision.
The receipt comes from the inserted native row rather than a second clock read.
Native event-service date updates exercise pre-approval/publication validation,
response/retry and invitation denial before the current end, exact-end reopening
within the retained window, missed-window behavior and unchanged published reports.
The fixture uses an event without common challenge publication; the event owner's
separate published-postponement rules are not claimed as newly tested here.

Independent native survey/privacy suites pass 25.701s/4.498s after the current-end
change; authenticated survey HTTP and actual approved-campaign/worker runtime pass
7.479s/6.891s. The real populated39→40 preservation test passes 3.492s. Before
this review's final persistence correction, the ticking-clock receipt regression
fails because initial and retry timestamps differ. With persisted receipts the
full native survey suite passes 19.870s, HTTP 5.054s, actual worker runtime 4.229s
and populated39→40 preservation 4.301s. Go lint reports zero issues and canonical
generation is stable. Reader browser verification passes eight desktop/mobile cases per portal,
including an event timezone differing from the browser timezone and a configured
viewport overflow assertion. Final actual-main composition and exact-head CI
remain merge requirements.

Composition onto actual merged #216/main ef606407 (schema39) retains both native
survey and dashboard runtime services, routes, public test entries and generated
contracts. Every pre-existing API path is unchanged. The only existing model
changes are the additive survey inbox destination in InboxDestination/InboxDetail
and the survey.open trigger in AutomationDefinition. Survey native source, SQL,
reader and regression files compare unchanged against the reviewed checkpoint.
Combined survey/dashboard/objective reader desktop/mobile checks pass 30 cases in
each portal (admin18.1s, participant19.0s), and both surfaces build. Actual
composed HTTP authority passes8.676s, app runtime4.736s and populated39→40 native
preservation3.538s. Later actual-main restacks still require explicit composition
review and fresh CI before merge.

The editing workspace's typed fixture exposed a generated nullable-reference
contract defect before merge: an unanswered native survey became a fabricated
revision-zero response in the HTTP projection. The authenticated HTTP regression
reproduces that failure. Nullable survey response, optional definition and
unlinked question-measure references now use the established nullable/allOf
pattern, preserving JSON null in both generated clients. The expanded actual
HTTP journey passes4.070s, including an empty own response and deliberately
published suppressed report with null measure provenance/counts. Both portals
still build. No missing response or measure is presented as a real zero value.

Final actual-main composition onto merged #217 dcbdaa6a retains all survey native,
HTTP, API, migration and UI source unchanged by that ancestry-only rebase. The
public test list retains both survey and mentor-support journeys. Final combined
survey/dashboard/mentor-support/objective reader checks pass32 desktop/mobile
cases per portal (admin19.8s, participant22.6s). Both builds, Go/web lint and
canonical generated parity pass. Exact-current-head CI remains required before
merge and issue acceptance reconciliation.
