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
excluded from new dispatch. Paired display integration reads only the latest safe announced snapshot of each
kind after the presentation owner validates its token. Superseded releases remain
in the separately authorised results history; they do not accumulate in the room
feed. The typed room consumer retains only safe announcement fields and rejects
private/extra result properties. Its512 KiB ceiling includes the actual 391,766-byte
Go-escaped maximum of two cues, 100 finalists, 100 reserves and50 awards. Safe passive DTOs
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

## Native API, consumer and recovery proof

The versioned contract now defines six typed organiser/judge/announcement/receipt
operations, generated Go/TypeScript models and the real API composition. Private
organiser operations require the separate admin surface. Judge ballots retain
native assignment authority on both surfaces; announced snapshots use current
confirmed-attendee/staff authority. Missing private rubric sources are explicit
nulls, never invented approved defaults.

Actual HTTP/PostgreSQL/race proof passed in 45.429s. It validates successful responses
against the authoritative schemas, strict body/header/CSRF boundaries, public
organiser denials, actor-exact receipts, both-mode source/ballot behavior, 409
before all required votes and after split votes, finalist-first publication,
exact monetary confirmation and separate award embargo/release. The integrated
canonical generation check, both portal builds and repository-wide Go lint pass.
Distinct captured request IDs racing the same release passed in 6.384s: exactly one
publication/version/receipt commits and the other command conflicts.

The native publication consumer test first proved that using the real results
message purpose had no official source fact (RED in 6.210s). Publication now records
its typed official fact in the same transaction. The registered native owner
checks actual latest-kind/event/version/time and current audience; missing owners
fail closed. Actual results-purpose/in-app, supersession, revocation and injected
fact-write rollback controls passed in 17.530s. This uses the actual approved campaign,
occurrences and registered handlers. The fixture clock is 2027, so this is not a
wall-clock worker-lease or SMTP/Teams delivery claim.

A recovery regression first failed in 30.761s: private source teardown blocked the
judge's immutable original acknowledgement. A shared native receipt authority
now checks current role, assignment and conflicts without reopening raw evidence
or requiring the old deliberation to remain writable. Exact recovery and current
assignment revocation pass in 11.103s. Current-state GETs do not become original-command ACKs.

A real corrected-publication regression initially exposed three releases in a
live room (RED in 13.696s). Live rooms now select at most one latest publication per
kind; all three historical releases remain retained/readable through the results
API. The original 128 KiB client ceiling also rejected a legal escaped maximum
result payload. The Go encoder measures 391,766 bytes; the 512 KiB ceiling passes
that legal payload and retains an explicit oversize rejection. Typed protocol
coverage passes 6/6. The combined actual PostgreSQL/race consumer, original-recovery
and transactional fact rollback scenarios pass in 31.530s. These are backend/protocol
proofs; visible results presentation remains frontend #113 work.

## Remaining acceptance gates

- Independent full Standards/Spec reviews, green final-head CI and equal-tree
  merge audit remain mandatory before closing #112 or its epic child.
- Feedback report preview/release, Fleet results screens and visible room results
  remain separate frontend #113 work. Broader independently calculated lifecycle/
  privacy verification remains #114; neither is claimed complete by these tests.

Only synthetic data and test-owned PostgreSQL resources were used. No real
messages, provider effects, payments or user container changes were performed.
