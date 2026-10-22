# Mentor observations: implementation checkpoint

Issue #99 is in progress. The versioned HTTP contract, adapters and production
composition compile and focused HTTP privacy verification passes. This
checkpoint does not claim the issue is delivered.

The domain stores immutable observation revisions with original authorship and
observation time. A current assigned primary or second mentor can record their
own observations; another mentor cannot rewrite the author's evidence. Current
assigned mentors and current organisers may inspect the team's private history.
Platform administration, judging, MC, logistics and participant access alone do
not confer this access. Removing an assignment or introducing a current conflict
also removes historical access; being an original author is not a lasting grant.

Formal observations require actual common challenge publication and the
scheduled live start. Operational on-track, at-risk and blocked status lives in
a separate versioned table, with reason and next action, and may support
preparation logistics before formal review opens. Neither write path touches
scores, ranking, notifications, participant projections or presentation cues.
Generic audit entries retain actor, event, team and revision references only.

The event lock serialises these operations with role, team and mentor assignment
changes. Both event revision and observation/status version must match. An
observation edit preserves its author's original timestamp and all earlier
versions; status history also retains each decision. Database triggers reject
updates, deletions and truncation of the stored history. A populated-history rollback is
blocked.

Evidence includes a private, bounded human feedback reference. This is inert
text: storing a reference does not dereference a resource, publish feedback or
grant access to feedback owned by another workflow.

At this checkpoint, real PostgreSQL/race tests pass (5.337 seconds) for the exact
live-start boundary, independent preparation status, primary/second mentor
access, authorship and revision conflicts, simultaneous edits with one winner,
current conflict/assignment/role revocation, two-event isolation, unrelated role
and platform-admin denial, disbanded teams, append-only history and absence of
private prose in generic audit records. Expanded domain tests pass with race
detection (11.555 seconds), including a PostgreSQL-observed writer blocked behind
a revocation transaction: after that transaction commits, the writer is denied
and no observation is retained. Archived read-only events retain authorised
history but deny both observation and operational-status writes.

The focused real HTTP/PostgreSQL test passes with race detection (7.145 seconds).
It validates the generated response contract, authentication, CSRF, stale event
revisions, the pre-live API gate, current-author history, organiser read access,
unknown score-field rejection and no-store responses. The same authenticated
cookie loses both current and historical access after mentor-role revocation.
Populated participant team, logistics page/export, approved room snapshot and
rendered mentor reminder-preview responses exclude the stored private evidence.
The room check uses a real paired capability and an activated synthetic cue;
the notification preview resolves a real current mentor. Request logs contain
neither private evidence nor outcome text. This does not claim integration with
the separately pending inbox/automation implementation.

Remaining before a PR: final schema ordering, upgrade
and rollback verification; generated-code reproduction and full CI. Migration
32 and the local schema constant are provisional until the preceding delivery
batch lands; they are not a deployment-order commitment.

After rebasing onto merged main `990f490`, focused domain/HTTP PostgreSQL race
tests pass in 8.905/8.193 seconds, scoped lint reports zero issues, and both portal
builds pass with the rebased lockfile dependencies. A provisional schema fixture
passes in 2.842 seconds: it upgrades actual schema26, preserves event/team/mentor
assignment/membership/audit snapshots, rejects a cross-event status reference,
and refuses rollback after private history is populated, retaining the schema
version and evidence. The predecessor and migration number still need updating
and rerunning against the actual immediately preceding merged schema before
publication; this is not an ordered31-to32 deployment proof.

The branch is now rebased onto actual SMTP merge `6d7ae5f` (schema27), preserving
the shared communications runtime and test validator. The entire HTTP package
passes with race detection in 64.636 seconds, and the entire observations package
passes in 13.750 seconds, including the provisional upgrade fixture now starting
from actual schema27. The new publication test rejects absent/future common
publication even on the live date and hides previously readable formal evidence
when the current live-start schedule moves into the future. Operational support
remains available. The final immediately preceding-schema upgrade remains a
publication gate, pending the ready Teams/rubric/inbox changes; schema32 is still
provisional.

An additional focused HTTP race run passes in 4.561 seconds with an actual MC-only
session: the private observation API denies that session, while the populated MC
presentation response remains usable and excludes private evidence. The existing
room capability, logistics/export, participant team and reminder-preview checks
remain in the same passing journey.
