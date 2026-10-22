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
