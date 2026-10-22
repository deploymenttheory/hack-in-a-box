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

Current notes use an ascending UUID cursor; observation and status histories use
independent descending version cursors. Pages default to50 with a maximum of100.
Every page rechecks the same current team/assignment/role and live-review policy.
Cursors are navigation positions, never authority. Current-state and write paths
load only the latest status/note version, independently of history length.
An absent history cursor starts that collection. When continuing the other
collection after one is exhausted, retain the exhausted collection's last
returned version as its exclusive bound; clearing it to null restarts it.

## Verification

- On the concrete inbox/rubric stack, the complete observations package passes
  with race detection (11.926 seconds), including simultaneous edits with one
  winner and an observed PostgreSQL writer blocked behind a revocation
  transaction. After revocation commits, the writer is denied and no note is
  retained. Both absent and future common publication fail closed.
- The complete HTTP package passes with race detection (65.305 seconds). The
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
- Independent review reproduced a valid1,001st note committing while its
  response and subsequent reads failed (3.043-second red proof). Bounded keyset
  reads replace that permanent cap. Real PostgreSQL/race tests now traverse all
  1,006 current notes and1,006 revisions of both histories without duplicates or
  omissions, preserve original authorship, accept the next valid write, return
  empty exhausted pages, and deny continued cursors after revocation (4.007s).
  The focused HTTP path also exercises current/history cursor parsing and
  response contracts, page bounds and invalid cursor rejection (3.602s).
  Independent re-review additionally passes the adapted original1,001-note case
  and unequal/empty observation/status histories (3/1,001,1,001/3,1,000/0),
  including exhaustion and revoked assignments, in9.759 seconds.
- A further focused HTTP race run passes in 3.634 seconds for actual durable
  inbox delivery: an organiser reviews and approves a real in-app reminder,
  the clock reaches its approved future due instant, and the current mentor
  reads its persisted list/detail through HTTP. Both payloads exclude the
  already-stored private evidence. This exercises delivery beyond a preview.
- The concrete30-to31 populated upgrade and guarded rollback pass with race
  detection (3.637 seconds). Historical inbox and rubric rollback fixtures also
  pass under schema31 (3.791/3.856 seconds). Full application Go lint and web
  lint report zero issues, both portal builds pass, and generated code reproduces.
  The historical observation rollback fixture now asserts protected schema31,
  verifies retained evidence, then restores the deployment's latest schema;
  later empty migrations may legitimately roll down before the retained guard.

The final PR is rebased onto actual inbox merge `2ec98b3` (schema30), following
parallel review. Generated code, the ordered upgrade and required CI must pass
on its exact final commit. This document does not mark #99 delivered before merge.
