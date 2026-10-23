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
same-command retry is idempotent while the survey is open; it cannot bypass close,
withdrawal, account deactivation or archive. List projections omit answer bodies.
API responses are private/no-store. The minimal inbox destination is a read-only
own-survey reader; the separate #116 workspace owns authoring and response controls.

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
mapping or raw-answer hash. Audit records contain actors, resource IDs, revisions,
actions and times; answers and reviewed prose never enter audit/log metadata.

Normal migration40 follows actual39 and preserves populated native objective,
scorecard, moderation and feedback history. Empty rollback is supported; populated
survey history refuses rollback. No missing/out-of-order Goose policy is enabled.
