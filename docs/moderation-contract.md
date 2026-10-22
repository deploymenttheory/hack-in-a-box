# Moderation huddles (#104): implementation checkpoint

This is an incomplete local backend checkpoint, not a readiness or migration
claim. Native #103 integration, HTTP/generated contracts, final normal migration
and final verification remain outstanding. The unnumbered development schema is
applied only by this checkpoint's fixtures. It must become a normal ordered
migration after actual merged #103 before the implementation is ready.

A huddle names an actual configured native checkpoint, published primary and
second reviewer, distinct second-review assignment/due time and a current named
organiser moderator. These identities form its intended panel. Replacement
reviewers use #66's existing full conflict/capacity publication flow. A huddle
never creates assignment grants, criteria or publication prerequisites.

Before opening, both exact current independent card versions must be submitted;
the second reviewer records an explicit completed review. Drafts and another
author's revision history remain private. Opening captures exact submitted card
references. Every panel/context/history request rechecks current roles,
assignment conflicts, common rubric, checkpoint/run, active team and native
card/evidence authority. Changing a submitted card invalidates that context;
it is not silently adopted. Configure a fresh explicitly reviewed context for
new source cards. A generic organiser role does not join another panel.

Operational actions remain #103-owned. A done action cannot close a formal gap
by itself. A named moderator records a separate evidence/reason decision bound
to its exact native current action version and published feedback source.
Reopened/changed/unavailable actions invalidate that completion. The recorded
decision author's current evidence access is rechecked, rather than borrowing
another moderator's broader access. An absent action-policy owner fails closed.

Proposed category changes preserve the exact original independent card,
author, value/pending state and evidence. Proposed values and their reasons and
supporting evidence are separate immutable decisions. Accept/reject never
rewrites original scorecards or fills missing independent categories. #110 may
consume explicitly signed proposals; this module calculates no totals, ranks,
finalists or awards and never averages checkpoint scores into final results.

Final sign-off calls #101 SignOffInTransaction in the same event-locked decision
transaction. It requires complete independent final cards, distinct required
authors, explicit gap/proposal decisions and native frozen post-cutoff evidence.
Disbanded or inactive output cannot finalise. A failed huddle-history write
rolls back the native sign-off; concurrent attempts create one proof/decision.
The immutable signed context retains exact source provenance. Later independent
card/source/authority changes do not create new approval or replay a decision.

## Checkpoint evidence

Real PostgreSQL/race local suite PASS 11.711s: native guarded common rubric and
scorecard journeys, intended-panel/draft/history privacy, current conflicts,
pre-live denial, distinct second reviewer, absent action owner, explicit formal
gap/proposal decisions, frozen final evidence, immutable original values,
atomic rollback, concurrent finalisation, pending-category integrity and later
card/role/disbandment denial. Scoped Go lint reports zero issues.

The operational action-policy fixture is explicitly synthetic and persisted in
PostgreSQL. It proves the typed transaction contract only. It does not claim
actual #103 records/runtime integration; that integration remains required.
