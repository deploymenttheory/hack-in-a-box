# Reviewed shared feedback and support actions

Issue #103 adds normal schema36 after the actual merged automation schema35.
Shared feedback, private mentor observations, private questions, help requests,
operational task status and assessment decisions remain separate records.

## Publication and current access

`PUT /api/v1/events/{event}/feedback/{id}` takes an explicit `save`, `review`,
`publish` or `withdraw` command. The team/category identity is immutable. A save
creates a new draft version without replacing the previous published pointer.
Review binds the exact hand-written title/body/source references. Publication
requires that review, current publisher/reviewer authority and current native
source access in the same event-locked transaction. There is no two-person rule.
Changing a reviewed draft requires another explicit review.

Sources are optional metadata-only observation/question/help references; no
source fetch copies prose or attachments into feedback. Question provenance is
organiser-only through the real private question owner. Mentor/help/scoring
roles cannot confer question access. Unknown source kinds/owners fail closed.
Unreleased source-derived bodies and history retain their native source read
policy. Deliberately published prose is released to its team's current members;
private provenance never inherits that release. Team/logistics DTOs omit private
observation/question references unless their actual owner separately authorises
the reader. A same-team help link is included only while the native help owner
permits reading it. No private scores or assessment records enter this model.

Current active people, current published team membership, confirmed competition
place and current helper/organiser/conflict/availability policy are rechecked on
reads and mutations. Setup/logistics helper scope reuses the help owner;
logistics never gains competition-output access. Competition-output feedback
and actions require actual common release and scheduled live start. Operational
preparation does not start or complete a formal observation/assessment.
Archived events retain currently authorised published/history reads and reject
all new publication, action, acknowledgement, configuration and maintenance writes.

## Independent actions and acknowledgements

Published feedback can have independently versioned support actions. Details
commands require an explicit `reviewed: true` team-facing payload. Owners are
current own-team members or eligible native helper/staff candidates, obtained
from a per-feedback authorised candidate endpoint rather than a people directory.
The current owner is rechecked on mutation and marked unavailable when authority
is lost; ownership is never silently transferred.

A team member may update only their own assigned task from open/in-progress to
in-progress/done. Current scoped helpers/organisers can review details, reassign,
reopen or cancel. History records exact actor, permission basis (`team_owner`,
`helper`, `organiser`), change kind and immutable action version. A team member's
acknowledgement binds an exact published feedback version in its own history;
it does not complete a task or sign off a review. A new published version needs
its own acknowledgement.

All publication/action writes and acknowledgements use current native authority
before checking a stable actor/command ID and payload digest. Changed payloads
cannot reuse a command ID. `If-Match` binds the relevant record version, not a
silently refreshed event version. Responses include `command_version` for the
original retained result; the currently authorised view may already be newer.
This prevents a retry from claiming that a newer release was acknowledged.

`ResolveActionInTransaction` is the metadata-only seam for #104. It returns exact
current action/version, published feedback version, operational status and owner
eligibility under native current access. Done is necessary operational evidence,
not a formal gap-resolution decision: #104 owns explicit moderator review and
its own source binding. Reassignment/reopen/new publication changes that binding.

## Overdue follow-up and actual destinations

An organiser sets separate setup/logistics/output thresholds (60–86,400 seconds).
An open/in-progress action becomes overdue at due time plus its category threshold.
Append-only recipient-specific escalation records deduplicate action cycle,
settings version and person; due/owner changes and reopening start a new cycle.
Targets are current organisers, plus current logistics helpers only for
setup/logistics. No private prose, source references, scores or arbitrary URLs
enter escalation previews, generic audit or logs.

The real shared runtime runs native support maintenance after guarded content
publication and before automation. It scans events independently of campaign
bindings, so a zero-campaign event still receives due follow-ups. Publication
notices and staff follow-up notices have distinct native owners and exact typed
source/destination identity. Current membership/resource/target category is
rechecked for inbox list/count/open/read and the actual destination. A completed
action does not erase an authorised historical follow-up; withdrawal/current
resource or recipient-authority loss hides it.

Minimal read-only routes `/events/{event}/feedback/{id}` and
`/events/{event}/feedback-followups/{id}` exist in both builds. They show released
prose/actions and publication history, with 15-second current-access refresh,
keyboard controls, safe text rendering and no original source/private draft UI.
Their cache keys include the resource-owner kind as well as event and ID. Full
team/organiser support controls remain separate from this backend's reader;
no Today/dashboard, moderation, scorecard or shared feedback grant is fabricated.

## Schema and verification

The populated35→36 fixture preserves prior event/team/membership/help/inbox,
automation settings and audit data. Empty36 rollback is supported. Once support
or settings history/native notices exist, rollback is blocked pending an explicit
preservation plan. Immutable feedback/action/command/acknowledgement/escalation
history cannot be mutated.

Tests cover real PostgreSQL concurrent task versions, an event-lock writer behind
role removal, independent acknowledgements, source-private drafts/history, actual
native question/observation/help policies, request/task/review/score separation,
category recipient deduplication, zero-campaign runtime notifications, current
revocation, strict authenticated HTTP and ordered upgrade/rollback. Browser
fixtures use fresh local builds and actual reader/inbox links on desktop/mobile;
no real provider messages are sent.

Final composition is based on actual merged schema35 (`8ac6a9c`). Current native
PostgreSQL/race suites passed for feedback (14.753s), the real shared runtime
(2.944s), and authenticated HTTP (3.962s), including populated35→36 and guarded
rollback. Fresh participant/browser composition passed106 cases (1.7m); isolated
admin reader/inbox/Q&A composition passed46 cases (33.2s). Both builds, web lint,
full Go lint and generated-contract parity passed. Desktop/mobile captures were
inspected for safe literal text, keyboard focus, wrapped identifiers and overflow.
Archived write hints are false without removing authorised retained reads; a
source-hidden draft does not expose its newer private revision counter.
