# Moderation huddles (#104)

The backend records independent review, intended-panel huddles, formal evidence
gaps, proposed changes and explicit canonical final decisions. It does not
calculate totals, select finalists, make awards or average checkpoint scores.
The implementation integrates actual #103 feedback records on merged schema36.
Normal migration37 installs the private immutable records. Domain and HTTP
fixtures use Goose directly; no provisional schema application remains.

A huddle names an actual configured native checkpoint, published primary and
second reviewer, distinct second-review assignment/due time and current named
organiser moderator. These identities form its intended panel. Replacement
reviewers use #66's existing full conflict/capacity publication flow. A huddle
never creates assignment grants, criteria or publication prerequisites.

Before opening, both exact current independent card versions must be submitted;
the second reviewer records an explicit completed review. Drafts and another
author's revision history remain private. Opening captures exact submitted card
references. Every panel/context/history request rechecks current roles,
assignment conflicts, common rubric, checkpoint/run, active team and native
card/evidence authority. Changing a submitted card invalidates that context;
it is not silently adopted. A generic organiser role does not join a panel. Historical pages additionally
check each original intended panel and native source/evidence readers, filtering
inaccessible snapshots with deterministic version cursors. Legitimate old action
and submitted-card versions remain readable under current native permission;
version advancement alone does not erase history.
Checkpoint creation and review use actual server time and event-local native
checkpoint windows; no competition assessment begins before live start.

Operational actions remain #103-owned. The shared runtime uses FeedbackActions,
which delegates exact action/version/published-feedback/current-actor authority
to the native feedback owner in the same transaction. Metadata contains no
feedback prose. Immutable huddle/action links retain native action and feedback
version foreign keys. A done action cannot close a formal gap by itself. The
named moderator records a separate evidence/reason decision bound to its exact
current action and published source. Reopened/changed/unavailable actions
invalidate completion. Each reader retains the native action permission, and
the recorded decision author's current evidence access is checked separately.
An absent action owner fails closed. Native feedback withdrawal and action-owner
withdrawal prevent finalisation without fabricating a replacement decision.

Proposed category changes preserve the exact original independent card,
author, value/pending state and evidence. Proposed values, supporting evidence,
reasons and explicit accept/reject decisions remain separate immutable records.
Accepting a proposal never rewrites original cards or fills missing categories.
Conflicting accepted proposals for the same card/category are rejected. A huddle
has at most 100 formal gaps and 100 proposals; immutable version history and
lists use bounded cursors with no lifetime record cap.

Final sign-off calls #101 SignOffInTransaction in the same event-locked decision
transaction. It requires complete independent final cards, distinct required
authors, explicit gap/proposal decisions and native frozen post-cutoff evidence.
Disbanded or inactive output cannot finalise. A canonical decision is unique per
(event, team, native final run) and links the exact signed huddle history version,
its pre-sign context version and actual #101 immutable proof. A failed history
write rolls back both canonical decision and native proof. Concurrent different
huddles cannot produce competing decisions or orphan proofs. An identical retry
returns the existing decision without event/history changes. A fresh final
source requires an explicit native run restart and fresh independent cards;
old signed source/history stays retained. Separate live checkpoint huddles are
not constrained by final-decision uniqueness.

Strict private list/detail/history/decision HTTP contracts use current sessions,
CSRF, event revisions and huddle versions. Participant/logistics/export/MC/display,
notification and logging payloads exclude private huddle/card/proposal prose.
Original nil-policy #101 panel denial remains tested before installing the real
huddle owner. The shared runtime supplies the same actual scorecards↔moderation
policy graph and native feedback owner to the API and worker. No external
messages or #110 tally/award effects occur.

## Verification

Actual populated36→37 PostgreSQL/race verification passes (4.766s), comparing
canonical snapshots of every preexisting public data table except Goose state.
Fixtures contain native common challenge/rubric, independent cards, reviewed
feedback/actions, private Q&A, help, inbox, automation pause and audit records.
Both an otherwise-empty rollback and latest-schema restoration pass. Retained
planned huddles, canonical final decisions, native #101 proofs and exact #103
foreign-key evidence prevent destructive rollback at schema37; all private
moderation tables reject mutation/deletion.

Real PostgreSQL/race tests prove native lifecycle, intended-panel/draft/history
privacy, current conflicts, pre-live denial, distinct second reviewer, explicit
gap/proposal decisions, frozen final evidence and immutable original values.
Different huddles cannot create competing final decisions; identical uncertain
retries do not create new history, and explicit native restart/fresh independent
cards preserve old decisions. A forced history failure rolls back native proof.
Pending independent categories remain incomplete despite accepted proposals.

Historical panel disclosure was reproduced red (1.969s), then corrected with
per-snapshot current native-source checks and bounded permission-filtered
keysets. Real native observation→independent second card→full #66 replacement→
explicit planned huddle reconfiguration→author-role revocation proves that the
latest context stays readable while its retired source-bearing history is
filtered, without blocking earlier accessible records (3.097s). Native feedback
history allows old action versions while current withdrawal still denies access
(3.006s). Normal action reopening/re-resolution retains legitimate original
history. Lists and history also traverse 1,006 records without a lifetime cap.

Strict authenticated HTTP/current-session and actual shared-runtime checks
preserve Q&A, released-rubric, feedback, provider and Session.draft_scope contracts.
Full local checks and exact-head CI evidence are recorded in the implementation
PR; no frontend moderation UI, totals or awards are claimed by this backend work.
