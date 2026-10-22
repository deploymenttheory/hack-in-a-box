# Planning and logistics verification

## Preparation tasks — #47

Migration 15 stores current task state and immutable task changes. The preparation
API requires current event authority and uses the event revision in `If-Match`.
Event locking serializes edits with role changes, schedule edits and lifecycle
actions. A competing edit returns a conflict instead of overwriting someone else's
work. Current task state and audit/history records commit together.

Unmodified template tasks appear as not-started seeds with task revision zero.
Saving materializes operational state and updates reusable task definitions by
value. Owners, evidence, progress and decision reasons are not copied into new
events. Schedule-linked due times are computed using the current event timezone,
calendar and schedule, so rescheduling preserves task progress and recalculates
relative dates. Missing schedule references are visible blockers. Fixed due times
remain fixed.

Organisers define tasks, owners, dependencies, checklists and evidence. Current
assignees can update their own progress, checklist completion and visible evidence;
they cannot change the definition or grant a waiver/override. Confirmed attendees
can access explicitly assigned preparation work; unconfirmed applicants cannot.
Organiser-only evidence and inaccessible files are omitted, including identifiers
and labels. Hidden dependencies are represented only by a count and blocker.
Assignee saves preserve hidden dependencies/evidence. Historical changes are
organiser-only and still check current file access.

Cycles, missing/cross-event dependencies and invalid owners are rejected.
Completion, blocking, waivers and overrides require reasons. Completion requires
finished dependencies and checklist items unless an organiser records an explicit
override. These operational decisions never mutate the authoritative attendance,
admission, team-freeze, challenge-publication or submission facts.

Evidence URLs and repository references are bounded HTTP(S) links, with no embedded
credentials; the server never fetches them. File links must belong to the event and
be readable by the person attaching them. Generic audit metadata contains only
identifiers and revisions; task prose and reasons stay in restricted task history.

Verification covers real PostgreSQL concurrent edits, immutable history, cycle and
cross-event rejection, reasoned waivers and overrides, current ownership and role
checks, restricted evidence, staff edits retaining hidden fields, DST-aware date
recomputation, read-only events and minimum-attendance gate independence. HTTP
tests validate generated request/response schemas, CSRF, filters, stale revisions
and immediate loss of access after role revocation.

## Organiser preparation workspace — #48

The event preparation tab provides a status board, list and chronological run
sheet. Owner, role, status and attention filters operate on the permission-filtered
task page. Overdue, blocked, unowned and scheduling problems have text labels.
The run sheet combines tasks and the event agenda, displays event-local times and
owners, and links accessible evidence. Confirmed competitors, the configured
minimum, admission cutoff, team freeze, common publication, live start and
submission cutoff remain distinct. Readiness links open the authoritative
applicant, team, setup or stage workspace for the reported blocker.

Organisers can edit task definitions, named and role owners, schedule-linked or
fixed dates, dependencies, checklist progress and evidence. Uploads use protected
file endpoints with stable retry keys; evidence defaults to organiser visibility.
Reasons are required for blocking, completion, waiver and overrides. Server
validation remains authoritative for owners, graphs, permissions and completion.
The workspace is read-only when current task/event permissions prohibit changes.
It is deliberately scoped to organisers; assignee portals are separate backlog
deliverables and use the existing permission-filtered API.

A stale event revision retains the complete in-memory draft. The organiser must
load the latest saved task, compare its fields and explicitly acknowledge applying
the draft before another write can use the newer revision. Other validation
failures do not trigger this overwrite flow. Navigation prompts protect unsaved
edits. Private drafts are not copied into browser storage. Existing fixed deadlines
retain their saved instant; edited wall times are resolved by the server, including
an explicit earlier/later choice for repeated DST times.

Desktop and mobile browser scenarios cover assignment, filters, run-sheet links
and timezone display, blocker navigation, checklist/evidence edits, reasoned
blocking and waiver, dependency rejection and explicit completion override,
concurrent edit review, permission loss, unsaved navigation, retrying a file upload,
and ambiguous local deadlines. These use synthetic HTTP fixtures for UI behavior;
the real PostgreSQL transaction and authorization coverage remains in #47's
preparation and HTTP integration suites. Board and conflict screenshots are
captured as Playwright artifacts and inspected at both viewport sizes.

Synthetic visual evidence: [desktop board](media/preparation-board-desktop.png) and
[mobile board](media/preparation-board-mobile.png).

## Attendance and restricted logistics — #52

Migration 16 stores per-person, per-local-day arrival state, immutable correction
history and organiser-configured catering buffers. The shared roster contains
current confirmed competitors and arrival metadata only. Organisers and logistics
staff can read it and record arrivals; the server checks current event roles,
write permission and confirmation under the event lock for each request.

Arrival writes use the person's revision for the selected day. A request UUID
identifies the exact actor, person, date and command. Duplicate scans do not create
another arrival or history revision. A replay returns current state, so retrying
an old successful scan after a correction cannot restore the earlier state.
Different intent with a stale revision conflicts. Corrections require a reason.
Withdrawn people disappear from the current roster and cannot be checked in; their
restricted history remains available and a mistaken historical arrival can still
be cleared with a reason. Generic audit records carry identifiers, not correction
prose or personal fields.

The roster identifies its observation time and event timezone. Live-day choices
follow calendar dates across DST, with an exclusive end instant; an event ending
at midnight does not gain an extra day. A draft without live dates still exposes
headcounts but does not allow check-in. Configured dates outside a maximum
366-day attendance window must be corrected before arrival operations.

Headcounts report confirmed competitors, their physical check-ins for the selected
day, distinct active staff, staff also competing, buffer and catering total.
Catering counts a person with both roles once. Only the confirmed competitor count
feeds the existing event minimum; arrivals, staff and buffer never update admission
claims or lifecycle facts. Buffer edits require the current event revision and
organiser permission.

The paginated logistics query and JSON export require an explicit current logistics
role. Each page rechecks current roster membership; only confirmed competitors and
active staff appear, once each. Contact, dietary, accessibility, travel and social
fields remain separate from shared check-in. Photo consent retains the existing
photographer/self policy and is never inferred from social attendance or included
in a logistics export. Missing/unknown social answers do not mean yes. Every export
page records its actor, event and result count. Revocation takes effect on the next
page/request. New travel/social profile fields support participant correction;
older clients preserve them when omitted, and profile deletion/retention removes
them with the other operational fields.

Real PostgreSQL tests cover concurrent duplicate requests, distinct repeated scans,
reasoned corrections and immutable history, stale retry protection, independent
days, DST boundaries, invalid dates, withdrawn participants, role revocation,
cross-event access, read-only events, deduplicated catering, unchanged admission,
pagination, audited exports and sensitive-field projections. HTTP tests validate
the generated request/response schemas, CSRF, current permissions, per-day revision
conflicts, export headers and withdrawal behavior. Existing privacy retention tests
also include the new fields.

## Attendance and venue readiness screens — #53

The attendance workspace supports name/status search, keyboard check-in and
reasoned corrections for each local hack day. It shows confirmed competitors and
their configured minimum separately from physical arrivals, staff and deduplicated
catering totals. The source roster observation time and timezone remain visible.
Organisers can configure buffer places; a stale event revision preserves the
proposed value and requires loading current headcounts before saving again.

An interrupted initial arrival retains its request UUID for retry. Corrections
show history, retain drafts on errors and require explicit review of the latest
arrival after a revision conflict. Current permissions disable further edits when
revoked. Unsaved corrections cannot silently disappear during navigation.

Restricted logistics is a separate, explicitly authorised screen. It loads contact,
dietary, accessibility, travel and social details only while open, discards inactive
query pages immediately and does not write private data to browser storage. Search
is scoped to the current page. Each downloaded page uses the audited export API and
rechecks current permissions. Returning to shared check-in removes those fields.
Unknown social attendance remains visibly unconfirmed.

Venue, AV and catering presets reuse preparation tasks and require a named owner
and deadline. The organiser overview displays logistics blockers with those owners
and local due times. Logistics assignees have a progress-only checklist editor,
without staff-directory access or definition/waiver controls. Progress requires a
valid owner and deadline; conflict review preserves current task structure and new
checklist items. These tasks do not override the confirmed-competitor go decision.

Local verification: production build, zero-warning ESLint, five component tests and
all 86 desktop/mobile browser scenarios passed, including 20 new attendance and
logistics cases. The browser fixtures cover keyboard use, independent days, retry
identifiers, correction history/conflicts, unsaved drafts, revoked permissions,
buffer revisions, restricted pagination/export, checklist assignment and progress,
and organiser readiness. The PostgreSQL transaction, authorisation and concurrency
coverage is supplied by the merged #47/#52 integration suites and the PR's full Go
race-test job. This ticket changes no backend API or database schema.

Synthetic screenshots were inspected on desktop and mobile, including narrow-screen
wrapping and absence of horizontal overflow. Evidence:
[desktop readiness](media/attendance-readiness-desktop.png) and
[mobile logistics checklist](media/attendance-checklist-mobile.png).

## Versioned team submissions — #60

Migration 17 extends the existing unique competition entry per event/team with
separate private working drafts, immutable formal revisions, a current final
pointer, immutable review/outcome history, and protected file references. Draft
revisions have their own concurrency counter and never count as formal evidence.
Personal preparation tasks remain in the preparation domain. Formal saves require
the current submission-state revision; replacing a final resets its completion
review to pending while preserving earlier evidence and review decisions.

Event/template deliverables define required/optional items, supported evidence
kinds and reviewer audiences. Existing templates default to all supported evidence
kinds and reviewer roles. Deadlines follow a configured agenda item or the shared
cutoff and must fall within the live submission window. Requirements become fixed
at challenge publication. Changing a linked published deadline requires an
organiser reason recorded in event decisions. Evidence labels and file names are
not hard-coded. URL/repository references accept HTTP(S) without credentials and
are never fetched or executed by the server.

Current confirmed members of the published team may save private drafts after
actual challenge publication. Organisers and reviewers cannot read those drafts.
Formal submission and organiser completion verification open at scheduled live
start; no exception permits premature acceptance or review. Assigned mentors may
read permitted formal evidence during live work; assigned judges receive only a
frozen current final after cutoff. Completion verification here is not mentor
checkpoint scoring or final judging, which have separate backlog deliverables.

The shared cutoff locks final evidence independently of lifecycle stage. Worker
maintenance, authorised submission operations and the production lifecycle service
settle outcomes under the event row lock. Each active team receives frozen evidence
or an explicit not-submitted/withdrawn outcome; retired/disbanded entries are
withdrawn and unscored. These counts feed the assessment gate in the same
transaction as stage advancement. Repeated or concurrent settlement does not
duplicate history. Application rescheduling settles the old cutoff first, so a
delayed worker or a later event postponement cannot reopen an already frozen final.

Changing evidence after an earlier deliverable deadline or shared cutoff requires
an organiser reason. Amendments append a new revision, retain any existing freeze,
record a late flag and reset review status. Old immutable evidence and review
history remain available to current team members/organisers and permitted mentors;
judges' file access follows the current frozen revision. Withdrawn entries cannot
be revived by an ordinary team save.

Uploads may specify a team scope. They reuse storage integrity checks, quotas,
idempotency and event revision checks, with current membership rechecked inside
the upload transaction. A retry cannot change its team scope. Private team files
are unavailable to organisers/reviewers until referenced by formal evidence
(organisers can also retrieve their own authorised on-behalf uploads). Generic
download URLs apply the same team, audience and time checks. Revoked members or
reviewer grants lose access on the next request. Late organiser uploads require
a recorded reason.

The versioned API exposes current submissions, private drafts, bounded change
history, organiser completion reviews and explicit outcomes below
`/api/v1/events/{event_id}/teams/{team_id}/submission`. Mutation requests require
CSRF and the appropriate quoted state/draft revision, including `"0"` for a new
record. The generated Go adapter and TypeScript client include these contracts.

PostgreSQL tests exercise required evidence/kinds, private drafts, live-start and
cutoff boundaries, early deadlines, immutable replacement/review history, concurrent
saves and settlement, current team/role authority, restricted file audiences,
late amendments, disbanding, rescheduling and the assessment gate. HTTP integration
tests validate responses against OpenAPI, CSRF, revision conflicts and protected
downloads. Repository URLs remain references to mutable external content: #96 owns
immutable SHA receipts and unresolved provider evidence, and this implementation
does not claim a referenced branch head was captured at cutoff.

Local verification passed: the full command/package PostgreSQL suite with `-race`
(serial package execution after a Docker reaper startup failure in the parallel
run), zero-issue Go lint, and the browser production build with regenerated types.

## #69 — versioned content and guarded publication

Migration 18 adds immutable native page versions, attachment references and
workflow/release history. Each event has one canonical challenge item whose type
cannot be changed. Current drafts, review and approval state are separate from the
last released version. Editing text, attachments, audiences or schedules creates a
new version and invalidates approval; readers retain the previous released version
until a new one is published or the item is withdrawn. Approval binds the exact
content revision and schedule. Event-date changes invalidate pending challenge
approval in the event update transaction.

The challenge uses seven calendar days before live start at the same event-local
clock time, including DST changes. The user explicitly agreed a maximum **60-second
scheduler dispatch allowance** on 22 October 2026. It cannot publish early. A
release attempted later than that allowance is recorded as missed; blocked or
missed initial publication requires postponement preserving the preparation week
or cancellation. Reapproval after the due time and generic manual/team-audience
controls cannot bypass the rule. Initial challenge publication rechecks current
preparation stage, confirmed-attendance minimum, organiser-confirmed frozen teams,
current approving-organiser authority and event writability under the event lock.
Planned and actual publication instants are both retained.

Approved content is its durable scheduled job. The worker checks due releases at
startup and every five seconds; the database prioritises common challenges. A job
carries the approved revision, so a stale job cannot publish a later draft. The
released pointer, immutable release event and first challenge publication timestamp
commit atomically. A unique constraint prevents repeated release events for a
version. Cancel/edit/release races serialize on the event lock. A blocked job does
not silently retry after its gate changes.

Current confirmed attendees, event roles and published team membership determine
released audiences. Explicit privileged previews are limited to current organisers
and, for challenge/brief content, current early-brief-access grants. The existing
staff-grant API remains the sole grant path: active/waitlisted competitors must
withdraw first, and a grant causes lasting competition ineligibility even if never
opened or later revoked. Platform administration confers no preview access. Briefs
cannot be released through a generic page action before the common challenge.

Both metadata and generic direct-download URLs enforce these audiences. Content
attachments must be organiser-owned event uploads, never private team evidence or
cross-event files. Each attachment is bound to one content item, preventing aliases
through a different audience/release policy. Removing a grant or current membership
revokes copied links on the next request. Withdrawal stops future content and file
retrieval and explicitly states that downloaded copies cannot be recalled. The
published common challenge cannot be withdrawn; approved exceptional postponement
retains it and permits continued preparation.

The revised-date notices already persisted by the event domain are now readable by
all current confirmed attendees and staff through `/api/v1/events/{event_id}/notices`.
This makes revised dates/deadlines available in the application; actual email/chat
notification delivery remains the separate #82 implementation. The organiser and
attendee screens are #70/#71, including safe Markdown rendering and version-bound
required read acknowledgements. No arbitrary URLs are fetched by content APIs.

`/api/v1/events/{event_id}/content` exposes audience-filtered listings, immutable
version saves, explicit preview/history and review/approve/release/cancel/withdraw
actions. Mutations require CSRF and the quoted content-state revision (`"0"` for a
new item). Ordinary libraries expose only released versions and never private
version metadata. Go and TypeScript contracts are regenerated from OpenAPI.

Real PostgreSQL tests with race detection cover DST calendar arithmetic, early,
exact, last-allowed and missed publication times; each readiness gate; concurrent
workers and cancellation; stale jobs, edits and rescheduling; immutable history;
private previews; current role/team authority; direct file retrieval; cross-event
and attachment-alias denial; permanent early-access exclusion; and approved
postponement with revised-date notices. Authenticated HTTP tests also validate
OpenAPI responses, CSRF, optimistic concurrency and withdrawal of copied links.

Local verification passed: the full command/package suite against real PostgreSQL
with `-race` and serial package execution; the affected content suite after the
final dispatch-order refinement; zero-issue Go lint; and the web production build
with regenerated API types. The existing web bundle-size advisory remains.

A focused regression caught the event overview reporting a missed publication at
T−7+0 seconds while the worker correctly allowed dispatch through +60 seconds.
Both now use the same scheduling constant. The test fails before the correction
and passes at 0, 60 and just over 60 seconds afterward; the content, event and
schedule PostgreSQL/race suites and final Go lint pass with the correction.


## #70 — organiser content authoring and release workspace

The event Content tab now supports native Markdown with safe preview, reusable
challenge/role/FAQ/resource starters, event-template role briefs and copies of
existing pages. Audience preview names the selected role or team and current
release state without issuing grants or publication actions. Markdown omits raw
HTML and remote images; links allow only HTTP(S), mailto and local anchors, reject
embedded credentials, and isolate opened tabs.

Organisers can upload protected attachments, compare immutable versions, review,
approve, schedule, cancel and withdraw content. Saving an approved page creates a
new private draft. Release controls require the saved, approved version; stale
revision conflicts retain local edits and require explicit review of the latest
version. Event-local schedules use the shared server time resolver with an explicit
DST choice. Placeholders, missing attachments and past/invalid schedules block
approval. Storage integrity and current access are checked on both approval and
actual release, including when a file disappears after approval. Verification
uses the current transaction connection; a one-connection-pool regression prevents
lock/pool starvation during attachment approval and scheduled release.

Initial challenge authoring displays its fixed seven-calendar-day publication
instant, confirmed competitor minimum and current team-freeze gates. The read-only
readiness endpoint uses the same domain gate as the worker. Approval cannot waive
those gates; there is no initial manual challenge release. The UI explains the
60-second dispatch allowance and the postponement/cancellation outcome. Privileged
access links to the existing staff workflow and explains lasting competition
exclusion and the active/waitlisted-registration conflict.

The public contract adds authorised attachment metadata and organiser-only
publication readiness. No database migration is required. The released reader
library and required acknowledgements remain #71.

Verification covers desktop/mobile editing of approved pages, role/team preview,
unsafe Markdown, reusable starters, attachments, placeholder and schedule blockers,
optimistic conflicts, revoked permissions, unsaved navigation and withdrawal.
Real PostgreSQL regressions cover current readiness, protected metadata and missing
storage at approval/dispatch; authenticated HTTP tests validate the new responses.
Local build, component tests, non-database Go race tests and both linters pass.
The full local browser run hit host disk exhaustion; local database tests could
not start because Docker stopped responding. The PR CI must pass the full browser,
PostgreSQL race, generated-contract and runtime-image checks before merge.

## #71 — participant resource library and version acknowledgements

The Resources tab lists and searches only currently authorised released pages.
Search runs on the server after audience/version checks, so hidden titles, draft
text and unauthorised team/role pages cannot influence matches or pagination.
The reader shows the released version and time, protected attachment links and the
same safe Markdown renderer used for authoring. It never requests privileged
preview. Current access is refreshed on focus, periodically and on demand; a
revoked or conflicting acknowledgement hides the reader until current access and
content have been reloaded. The released challenge explains that teams may build
immediately while formal submissions and mentor/judge review start at live start.

Organisers can explicitly require acknowledgement on an immutable content version.
Existing documents default to optional reading. Migration 19 adds minimal immutable
receipts keyed by event, item, version and person, with the acknowledgement time.
A receipt exposes only the current reader's acknowledgement of the returned
version. Personal acknowledgements do not alter the page or event revision.
Editing a private draft preserves the released receipt; publishing a new required
version requests a fresh acknowledgement. The POST requires CSRF and the quoted
released version. It rechecks current identity/audience and writability under the
event lock, serialising with release and withdrawal. Duplicate/concurrent retries
produce one receipt and one narrow audit event. Previewing or reading a page alone
never acknowledges it, and optional or unreleased pages reject the action.

Real PostgreSQL regressions cover concurrent retries, material updates, release
races, immutable receipts, cross-event attempts, revoked attendance, withdrawal,
read-only events, the initial challenge embargo, role-specific search and cursor
pagination. HTTP checks cover CSRF, stale versions, authorisation and generated
response validation. Desktop/mobile browser tests cover safe reading, protected
attachment links, explicit acknowledgements, updates, search, roles, team revocation
and challenge publication; the authoring regression also sets the required flag.
Both reader layouts were visually inspected. Local production build, focused
browser tests, component tests, non-database Go race tests and linters pass; the
full PostgreSQL and container checks run in CI because local Docker is unresponsive.

## #61 — personal preparation and team deliverable screens

The event Deliverables tab exposes the person's assigned preparation separately
from team competition evidence. Personal tasks show ownership, deadline, progress,
checklist and references. Participants can update only the progress permitted by
current task authority; they cannot change assignments, deadlines or dependency
rules, grant waivers or fetch restricted staff/logistics data. Role-assigned
attendee tasks are labelled as shared progress rather than personal completions.
Conflicts load the current assignment and preserve progress without overwriting
changed organiser definitions.

Published teams see required/optional deliverables, their evidence kinds, due and
late status, captain/ownership, current formal evidence and completion feedback.
Private drafts and formal submissions have separate revision counters and actions.
Nothing is accepted or reviewed formally before live start. The UI uses current
server permissions, including an explicit `can_read_draft` projection: organisers
can amend or review formal evidence but never request a team's private draft.
Before actual challenge publication, no draft/evidence input is solicited.

Uploads are scoped to the team and use a stable key for each selected file. A
retry reads the latest event revision and reuses that key, so a lost response does
not create another upload or discard completed references. Formal saves use the
submission-state revision, preventing duplicate acceptance after a lost response
or conflicting save. Repeating identical formal evidence is disabled. Draft and
formal conflicts require explicit review of latest evidence before retrying; local
fields remain intact. URL/repository evidence uses HTTP(S) references without
embedded credentials, and downloaded files retain current access checks.

At cutoff the screen displays the authoritative frozen, not-submitted or withdrawn
outcome, preserves unsaved fields in a locked editor and explains that private
preparation is not a submission. Disbanded teams solicit no evidence. An exception
panel names current active organisers and provides the team reference, deadline
and recorded outcome to give them through the event contact channel. Only public
display names are projected; private contact/logistics fields are not exposed.
Organiser amendments and completion reviews require recorded reasons, preserve
history and never enable premature review. Completion feedback remains separate
from mentor checkpoint scoring and final judging.

Desktop/mobile browser coverage includes labelled file/repository evidence,
required work, a committed upload whose response fails and is retried idempotently,
private pre-live drafts, initial challenge embargo, cutoff locks, current feedback,
formal/draft/personal conflicts, disbanding, revoked membership and reasoned
organiser review/amendment. Both layouts were visually inspected. PostgreSQL and
HTTP contract regressions check draft-read flags, current active organiser names,
revocation and the pre-live/cutoff states, extending the boundary and race tests
already delivered in #60. No schema migration is needed. Local build, linters,
component and browser checks pass; real PostgreSQL and runtime container checks
must pass in CI before merge because local Docker is unresponsive.
