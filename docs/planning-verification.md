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
