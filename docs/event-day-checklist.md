# Event operator checklist

Use this with the current event workspace and [operator handbook](operator-handbook.md).
Record the organiser, event, timezone, current revision and reasons for exceptional
actions. An enabled action still needs its native review and recorded response.
The [glossary](../GLOSSARY.md) and [ADRs 0001–0005](adr/) define the policy.

## Before registration closes

- Confirm branded event details, timezone, live dates, submission cutoff, rules,
  reporting contacts, privacy/retention, venue/connectivity and support ownership.
  Separate dietary/accessibility details from general attendee and room displays.
- Configure a minimum of confirmed competitors appropriate to the event. Offers,
  waitlisted people, staff and spectators do not count toward that threshold.
  Confirming the threshold enables an organiser decision; it does not advance
  the event automatically. Advancing to preparation closes new registration.
  Each team has one competition entry per event; working revisions do not create
  additional entries.
- Explain event-lifetime eligibility before granting organiser or unreleased-theme
  access. Anyone who has held that role/access cannot compete in that event after
  revocation. An active attendee, including a waitlisted person, must explicitly
  withdraw before that grant; do not silently remove a place to add privileges.
- Review offers and the FIFO admission policy. Admission cutoff is no later than
  team freeze, and team freeze precedes common challenge publication. Existing
  offers may continue under the approved policy until cutoff; a vacant place
  afterward does not reopen admission.
- Form teams around five people, preferably four to six. Before freeze, reassign
  members of teams below three through an organiser-reviewed allocation. Review
  staff, mentor and judging coverage and conflicts before publication.

## Before the common challenge release

- Freeze team membership before the challenge is released. After freeze there is
  no reassignment: a smaller team can continue or withdraw. A disbanded team's
  repository is simply unused and unscored; no work-transfer process is required.
- Check preparation stage, minimum confirmed attendance, every confirmed
  competitor's published/frozen team, provider readiness, current resource
  permissions and exact approved automation bindings.
- Release the challenge seven calendar days before live start, at the same local
  clock time in the event timezone. Inspect timezone/DST resolution explicitly;
  seven calendar days can differ from 168 elapsed hours. Use the current native
  schedule/readiness view rather than an independently calculated reminder.
  The worker may dispatch within the accepted 60-second window and records the
  actual publication instant; later dispatch is missed, not a catch-up release.
- If publication is blocked or missed, postpone to preserve the preparation week
  or cancel. Do not bypass the gate with manual early access. A shortfall during
  preparation does not automatically cancel or roll back the event.
- After common publication, teams may plan, set up repositories and start building.
  Working drafts/code are permitted; formal submission, review and scoring open
  only at live start. Publish the common rubric/version and give judges their
  synthetic practice before real assessment. Practice scores are not competition
  scores. See [rubric contract](rubric-contract.md).
- A disruption after publication can justify an explicit reasoned postponement:
  keep the challenge available, communicate revised live/cutoff dates, and record
  the exception. If withdrawals later put attendance below the minimum, a
  reasoned organiser exception is required to enter live below that minimum;
  the invested work does not itself record that approval.

## At live start and throughout the hack

- Check both portal readiness, real sign-in, worker health, venue/network access,
  protected resources, repository access and the current live-start gate. Verify
  actual collaborator acceptance; an invitation or queued plan is not access.
- Confirm the published rubric, judging coverage, rules and reporting contacts.
  Keep the MC/room display on its intended public projection; do not project
  private support requests, dietary/accessibility data or assessments.
- Triage help using the real claim/current-assignee workflow. A claim race needs
  a fresh authoritative read; do not assume the first button click won.
- Use the canonical clarification owner for common questions. Distinguish a
  proposed answer, an approved/published answer and a material change needing
  its explicit review/acknowledgment process. Update the official resource and
  audience rather than spreading conflicting copies through chat.
- Mentors may give feedback and provisional checkpoint assessments during live
  work. Keep mentor/private evidence separate from final judge votes and published
  results. Preserve current role/team/conflict checks when coverage changes.
- If connectivity fails, use the documented connection/recovery state and share
  the approved operational contact route. There is no full offline mode. Downloaded
  run sheets do not acknowledge later writes, grant new authority or replace
  current submission/cutoff checks. Do not promise an automatic offline sync.
- For paused automation, inspect why it is paused and which exact actions remain
  approved/current. Resume through the owning reviewed control after checking
  provider state. A missed dispatch, unknown effect or changed source is not
  repaired by bulk resending or editing the database.

## At cutoff and final judging

- The shared deadline freezes submissions even if the organiser advances stage
  later. Inspect every team's frozen submission or explicit not-submitted/withdrawn
  outcome. A private working draft is not an accepted formal entry.
- Resolve repository freeze exceptions through the current evidence/reconciliation
  workflow. Record what commit/evidence was actually observed and when. A late
  current branch head does not prove what existed at cutoff. Retain uncertainty;
  do not invent a historical SHA or silently extend one team's deadline.
- Move to assessment only after cutoff and required outcomes are recorded. Check
  judge assignments/conflicts, frozen build access, current rubric version and
  required scoring sign-offs. A saved draft score is not a signed final score.
- For final ties, require every judge's vote and a strict majority. A split remains
  unresolved until a replacement vote produces that majority. Mentor views are
  advisory evidence; they do not become extra votes or reduce the denominator.
  Preserve the distinction between qualification-mode panel decisions and the
  all-team final judge decision. Follow the current owner before confirming.
- Review final rankings, category awards and evidence; publish results explicitly.
  Tally completion, an award draft or an archive action does not publish results.
  Retain the original command/outcome when a response is lost; do not repeat an
  award-publication write under a new identity to find out whether it happened.

## Outcomes, publication and archive

- Review team feedback, attendance records and media consent under their own
  current policies. A submitted project or an award does not publish a showcase
  entry. Publish only the reviewed project projection and permitted evidence;
  remove/restrict it when current authority or retention requires that.
- Record objectives, owners and follow-up dates. Label estimated/self-reported
  benefits separately from independently evidenced adoption/value; keep sources
  and observation dates. Unknown value remains unknown.
- Review current recipients, exact reminder plan/approval, pause status and privacy
  deadline before follow-up dispatch. Outcome-specific audited updates/reminders
  can continue after archive under their registered exception; generic archived
  competition/communication writes stay frozen. This is not a blanket archive
  unlock. See [outcome follow-through](outcome-follow-through-design.md).
- Export the authorised archive/run sheet and verify retained protected references
  through their current audience/retention policy. Schedule the installation's
  consistent backup and verify recoverability with separately held keys.
- Certificates and session booking are optional M5 capabilities. They do not block
  a first-release hack; open certificate deliverables must not be described as
  completed. A booking or event registration alone does not prove attendance or
  establish an award. Confirm the installed capability before promising it.

Use [operations](operations.md) and [the recovery runbook](recovery-runbook.md)
for incidents. Preserve original operation IDs, attempts, source versions and
provider receipts. Record any manual decision and its reason; do not clear an
alert by deleting history, forcing a successful state or bypassing a native gate.
