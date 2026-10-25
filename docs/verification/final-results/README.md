# Final tally and deliberate publication (#112) — in progress

This PR owns the complete backend deliverable in #112. It is rebased onto
merged main `fcbfe038` and adds native schema 53 above the merged qualification
owner's schema 52. Passing a checkpoint's CI does not complete this issue.

## Implemented behavior

The calculator consumes native published criteria, current frozen submissions,
required assignments and independently submitted final judge cards in one event
transaction. Missing cards, invalid categories, revoked/conflicted assignments,
stale rubric/source references and unresolved repository freezes block results.
Disbanded and non-submitted entries do not enter the native completion source.
Qualification mode uses the actual signed #110 selection and fresh finalist-only
judge assessments; mentor totals and ranks never enter final judge arithmetic.
Category contenders retain the native signed all-entry qualification selection,
including non-finalists; all-team mode uses exact judge category means.

Arithmetic preserves integer numerator/denominator pairs with `math/big.Rat`.
Full precision determines ranking; two-decimal display text cannot create or
resolve a tie. Cases include recurring 19/3 and a near tie of 246/49 versus
251/50, whose distinct values both display as 5.02.

Preparation, judge ballots, confirmation, finalist publication and award
publication are separate commands. Every required final judge must vote; a
strict majority is required after all ballots are recorded. A split stays
unresolved until an explicit replacement vote. Partial assignment sets never
silently reduce the panel. Each vote checks the caller's current native target
and frozen-evidence authority. Judge views contain their own evidence only.
Previous ballots remain in immutable versions.

Confirmation records a real native judge result approval after source, tie and
award validation. Reopening an independent card invalidates the current source
and approval. Stale current projections withhold private scores, ballots,
confirmation and decision prose while immutable storage remains intact.

Awards match every configured prize's identity, name and category. Category-less
prizes target the overall winner; neither array order nor prize names invent a
placement. One team can receive several prizes. Amount/currency are explicit
exact decimal metadata, including noncash awards; no payment is executed.

Finalists can be announced while the fresh final is pending. Confirmation alone
announces nothing. A separate award command releases only reviewed award/team
metadata. Released DTOs cannot carry raw cards, mentor rankings, ballots,
private evidence, preparation reasons or unannounced awards. Current audience
checks exclude outsiders and withdrawn competitors. Original-command receipts
remain actor/event/request scoped and exact retries do not repeat publications.
Distinct request IDs cannot announce an identical reviewed source twice.

One immutable publication row is the authoritative communication/display event.
The actual `results.published` automation adapter discovers only announced
metadata and rechecks current recipients, with superseded same-kind releases
excluded from new dispatch. Paired display integration reads only safe announced
snapshots after the presentation owner validates its token. Safe passive DTOs
live in `internal/resultpublication`, keeping consumer imports independent from
private assessment services.

Schema 53 retains immutable versioned decisions, exact request receipts and
separate typed announcements. Same-event foreign keys bind native qualification
versions and judge approvals. Direct/cascade truncate and updates/deletes fail;
populated rollback is refused and empty rollback is supported.

## Verification evidence and limits

- Initial source/arithmetic/schema proof is recorded in `source-read-proof.json`.
  Native storage proof includes populated 52→53 upgrade, retained rubric data,
  composite keys, independent kinds, duplicate source and rollback guards.
- Rebased preparation/finalist publication proof passed in 26.340s: current
  confirmed audience and withdrawal, private preparation, explicit release,
  exact original receipt, duplicate-source rejection and concurrent same-key
  requests. Pure arithmetic/decision tests also passed.
- Native result evidence helper proof passed in 25.865s: actual visible versus
  hidden frozen references, note/reference bounds, current assignment, conflict,
  role loss, withdrawal and open-deliberation checks in both modes.
- Native composed decisions passed in 24.625s: both modes' private confirmation,
  separate finalist/award embargoes, exact money/multiple awards, repeated
  release and approval invalidation; actual two-team split, missing ballots,
  explicit revote, original receipts and immutable vote history.
- After handler extraction and stale-prose redaction, the composed app tests
  passed in 181.860s. The complete results package with PostgreSQL enabled passed
  in 16.353s. Database tests use `HACKBOX_INTEGRATION=1` and `-race`; source-only
  runs that omit the integration flag are unit/compile evidence only.
- All affected backend packages passed configured lint. The subsequent exact
  repository-wide CI command exposed a test import cycle through moderation,
  archives, outcomes and automation. Publication DTOs were moved to the passive
  package; every application/test package now compiles. The exact whole-repository CI
  lint command passes with zero issues; new-head CI remains required.

## Remaining acceptance gates

- Versioned OpenAPI, generated Go/TypeScript clients, actual runtime composition
  and native HTTP proof are being completed in the API lane. They are not yet
  part of the published checkpoint.
- Native approved automation/in-app delivery, recipient revocation and paired
  display privacy proofs are in progress. Adapter existence alone is not proof
  of delivery, worker leases or provider behavior.
- Full independent Standards/Spec reviews, green final-head CI and equal-tree
  merge audit remain mandatory before closing #112 or its epic child.
- Feedback report preview/release and Fleet results screens remain separate
  frontend #113 work. Broader independently calculated lifecycle/privacy
  verification remains #114; neither is claimed complete by these tests.

Only synthetic data and test-owned PostgreSQL resources were used. No real
messages, provider effects, payments or user container changes were performed.
