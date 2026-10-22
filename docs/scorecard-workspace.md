# Independent mentor scorecard workspace

Issue #102 uses the merged #101 native assessment contract. Open a team's
**Your checkpoint scorecards** link in its mentor workspace, or visit
`/events/{event}/mentor/{team}/scorecards` directly. Both public and admin builds
use the same current-resource API authority; the page does not depend on Today.

The schedule lists only currently published checkpoint/run/rubric bindings.
Opening a card reads all own-card pages before concluding a card is new. A
current final card additionally requires the actual frozen formal submission.
Native common-publication, scheduled-live, assignment/conflict, window and
post-cutoff checks remain authoritative. GETs never start an assessment or freeze
a rubric; only the existing native write transaction does so.

Each category starts Pending/unobserved, with no numeric value. Explicit zero is
a scored observation and requires evidence on submission. The native contract
permits an independent submission containing pending categories; the button and
receipt state their count rather than calling the assessment complete. Drafts
may omit evidence. Public guidance and score anchors are available beside each
field. Live checkpoints are explicitly provisional; final qualification uses
its own rubric/run and exact frozen submission revision. No checkpoint totals
are averaged, copied to final cards, or displayed as final results.

Evidence notes and scores are memory-only. This workflow does not use mentor
text recovery, sessionStorage, localStorage, offline queues or automatic replay.
Leaving, reloading, changing event/team/session or a current native resource
denial clears the editor. Current checkpoint and own-card reads refresh every
15 seconds. A transient current-read failure hides stale saved evidence and entry
while keeping the editor mounted in memory; a successful current read restores
that unsent input. Native authority/source denial unmounts and clears it. A
network failure cannot authorize a write. Observation references
use their native exact retained-version endpoint, including its current private
read policy. Formal submission references open actual submitted artifacts via
current protected downloads or safe HTTP(S) references, never working drafts or
fabricated Git verification receipts.

Every write retains its exact original event revision, own-card version,
checkpoint version, author, run/rubric and frozen-output binding in memory.
A fresh native read precedes sending. A changed server version requires explicit
review and a deliberate new base; it does not silently overwrite a newer card.
The native API has no command-receipt or Idempotency-Key endpoint. Its immutable
card/event version transaction prevents duplicate effects. A lost response is
labelled unconfirmed. An explicit check compares the exact expected retained
version, source binding, state and full body, even if a newer version now exists;
it never resends the request. Native 400 input rejection preserves entered notes
and states that no save occurred. Unknown/transport outcomes remain unconfirmed.

Only the current caller's own cards/history are requested. There is no panel
request, invented moderation grant, sign-off, qualification outcome or shared
feedback publication. Those controls belong to #104/#105 and the native policy.

Verification:

- `scorecard-model.test.ts`: pending/zero, native bounds/UTF-8 evidence limits,
  retained input and exact source/author/version/state receipt matching.
- `scorecard-workspace.spec.ts`: desktop/mobile keyboard form, rubric guidance,
  evidence validation, draft/submit, pending count, lost accepted response with
  later retained history, explicit conflict base, native400, offline rejection,
  current revocation, cursor lookup and other-author denial, memory cleanup,
  frozen final gating and exact native observation evidence preview.
- Native PostgreSQL/race suites cover concurrent versions, event-lock assignment
  removal, independent-author privacy, current session, actual rubric/live/final
  gates and exact current observation/submission evidence ownership.
