# Final tally and deliberate publication (#112) — in progress

This is the full #112 backend deliverable, stacked on actual #110 checkpoint
`2abe330050f1ef8511dbe7f8626fcea5b7014c2b` (native schema 52). It is not
complete and must not close #112 or become Ready merely because CI passes.
Migration 53 is reserved for this owner. No publication endpoint, UI success,
award confirmation or external message is implied by the initial checkpoint.

## Current arithmetic and native source

The calculator consumes the exact required assignment set, immutable published
rubric reference and submitted independent cards from native judge completion.
That source now returns its public criteria/reference and required assignments in
the same transaction, retaining the original frozen-source and approval guards.
Missing/extra authors, missing/null/duplicate categories, drafts, invalid values,
duplicate entries and wrong run/rubric references fail closed.

All arithmetic uses exact base-10 stored integer score coefficients and rational
averages, preserving their integer numerator and denominator even for recurring
averages. Weighted totals and comparisons never pass through binary floating
point or rounded display text. The two-decimal display is a separate projection.
Equal values retain equal ranks; UUID ordering is a stable display order and
cannot select an award winner or resolve a tie.

A near-tie case uses 246/49 versus 251/50: both display as 5.02, but the first
retains the higher rank. A recurring weighted result retains 19/3 independently
of its displayed 6.33. The initial calculator suite passes (0.299s), including
thirteen incomplete-source subcases. A first judge package run omitted the
integration environment flag and therefore skipped its database test; it is
compile evidence only. The corrected actual command was:

```sh
HACKBOX_INTEGRATION=1 go test -race ./internal/judging \
  -run '^TestIndependentVisibilityIncompleteLockDeliberationAndReopen$' -count=1 -v
```

It passed its real PostgreSQL scenario in 2.62s (package 3.845s), checking the
transactional assignment/reference projection alongside native independent
cards, missing judges, deliberation, approval invalidation and reasoned reopen.
Its own container was stopped and removed. This is a narrow source test, not a
complete #112 PostgreSQL/API/publication proof.

## Full acceptance still to deliver

- Native, versioned result review with all-team versus fresh finalist-only judge
  source selection; all eligible competing entries remain eligible for category
  awards through their actual owning mode. Disbanded and non-submitted entries
  cannot enter scoring. Consume #110's real transactional signed-selection seam.
- Required judge completion and recorded final judge votes, with exact source
  binding and stale/current authority checks. Qualification/category tie decisions
  remain the actual native #110 signed panel source; no recomputed checkpoint sum.
- Separate reasoned award/prize confirmation and versioned immutable history,
  source invalidation, exact original-command recovery and concurrency protection.
  A team may win several awards. Amount/currency are metadata, never a payment.
- Explicit independent finalist and award publication gates; safe released
  projections for #111/#113, one durable downstream event per actual publication,
  current source/recipient checks and integration with approved automation/display.
  Feedback release remains separately owned by #113.
- Versioned OpenAPI, generated clients and actual runtime wiring. Native database,
  HTTP, authorization, privacy, receipt, missing-judge, near-tie, repeated publication
  and separate embargo tests. Normal 52→53 migration and guarded rollback proof.
- Independent full Standards/Spec review, final-head CI and equal-tree merge audit
  before any ticket/epic closure.

## Native result retention schema checkpoint

Schema 53 now adds immutable versioned decisions, original-command receipts and
separately typed finalist/award publications. Composite native source references
bind judge approvals and qualification versions to the same event. Publication
identity is unique per event, kind and exact source binding, retaining independent
finalist and award embargoes. Update, delete and direct/cascade truncate are
rejected; populated rollback is refused, while empty rollback is supported.
Generated SQL models and the required application schema version are updated.

The actual PostgreSQL/race results package passed in 5.570 seconds, including
existing exact arithmetic, populated parent-schema 52→53 upgrade, prior approved
rubric preservation, incomplete native-reference rejection, duplicate publication
source rejection, independent publication kinds, cross-event foreign keys,
retention mutation checks and both guarded/empty rollback cases. These are schema
fixtures, not proof that a service command authorised or published an award.
Configured Go lint for results/database/judging passes with zero issues. A first
source-only insertion helper command failed before writing its new assertions;
those assertions were then inserted and passed in the recorded final run.

The full native command, actual qualification handoff, HTTP/runtime/API, vote,
confirmation and downstream publication acceptance above remain required.

## Private current-source reader checkpoint

The results owner now composes the actual published current run, native required
judge completion and #110 `SelectionInTransaction` in one event transaction.
Only a current organiser can read private preparation. Tallies are withheld
until native deliberation is open; missing runs, incomplete judges and missing
current signed qualification have explicit blockers. Stale stored preparation is
redacted from the current projection, retaining its immutable storage. The public
announcement type is separate and cannot carry raw cards, mentor ranks, votes or
evidence. No result HTTP endpoint or runtime composition is wired yet.

Exact category contenders use all required native judge averages in all-team
mode, and the actual signed panel category IDs in qualification mode, including
non-finalists. Ties retain every candidate and derive their identity from exact
source binding; stable UUID order does not select a winner. The full selection
metadata (including run/version/reference/signoff) participates in source binding.

The real PostgreSQL/race results package passed in 6.764 seconds: schema and
arithmetic scenarios plus a current organiser/missing-run reader, judge/outsider
denials and foreign-event denial. Its role fixture deliberately tests the reader
boundary; it is not a full lifecycle rehearsal or a completed qualified tally.
The existing native judge scenario also passed with the new explicit deliberation
source assertion in 2.59 seconds (package 3.996s). Configured source lint passed
after replacing a category-contender allocation with bounded slice reuse.

Full actual signed-selection → result preparation → judge ballots → confirmation
→ separate publication/native HTTP/runtime/provider integration remains required.
No successful UI publication or external message is simulated by these readers.
