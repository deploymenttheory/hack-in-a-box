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
