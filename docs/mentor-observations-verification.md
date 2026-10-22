# Private mentor observations and operational history

Issue #99 adds private observations independently of scoring. Current assigned
primary/second mentors may create notes and revise their own notes; current
organisers and assigned mentors may inspect history. Original authorship does
not confer lasting access after assignment or role revocation. Participant,
logistics, MC and platform-admin status alone confer no observation access.

Each immutable note revision retains team, author, original observation time,
category, factual evidence, outcome and a bounded private feedback reference.
The reference is inert text: it neither publishes feedback nor grants access.
Operational on-track/at-risk/blocked status has its own versioned reason, next
action, actor and timestamp. Neither command creates scores, ranking,
notifications, participant content or presentation cues.

Formal review requires actual common challenge publication and the scheduled
live-start instant. Operational support remains available before live review.
Reads recheck the current schedule: postponing live start hides formal evidence
and its history again without disabling operational support. Event and record
revisions prevent stale edits; the shared event lock serialises writes with
current role, conflict, team and assignment changes. Archived read-only events
retain currently authorised history and deny both write paths.

Schema31 follows the concrete inbox schema30 source. Immutable history rejects
UPDATE, DELETE and TRUNCATE; populated rollback is refused. The real PostgreSQL
upgrade fixture preserves existing event, team, assignment, membership and audit
snapshots, rejects a cross-event team reference, and verifies that a failed
rollback retains the schema version and private evidence. The fixture exercises
30-to31, rather than inventing a predecessor from a fresh schema.

## Verification

- On the concrete inbox/rubric stack, the complete observations package passes
  with race detection (12.547 seconds), including simultaneous edits with one
  winner and an observed PostgreSQL writer blocked behind a revocation
  transaction. After revocation commits, the writer is denied and no note is
  retained. Both absent and future common publication fail closed.
- The complete HTTP package passes with race detection (65.849 seconds). The
  observation journey checks generated response contracts, authentication,
  wrong-CSRF rejection, the pre-live API gate, stale revision conflicts,
  organiser reads, original-author history, unknown score-field rejection and
  no-store responses. Reusing the same cookie after mentor revocation loses
  current and historical access.
- Populated participant-team, logistics page/export, MC presentation, paired
  room snapshot and notification-preview responses exclude private evidence.
  The MC-only session remains able to use its presentation console while the
  private observation API denies it. Request logs and generic audit records
  contain no private note or operational-status prose.
- A further focused HTTP race run passes in 3.634 seconds for actual durable
  inbox delivery: an organiser reviews and approves a real in-app reminder,
  the clock reaches its approved future due instant, and the current mentor
  reads its persisted list/detail through HTTP. Both payloads exclude the
  already-stored private evidence. This exercises delivery beyond a preview.

The PR is prepared on the reviewed inbox branch so it can be reviewed in
parallel. Before merging it must be rebased onto the actual merged preceding
schema, reproduce generated code, pass the ordered upgrade and required CI on
that exact final commit. This document does not mark #99 delivered before merge.
