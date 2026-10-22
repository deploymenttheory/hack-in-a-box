# Independent mentor scorecards (#101)

This implementation is a backend checkpoint, not a delivered issue or completed
huddle/result workflow. Its schema is intentionally provisional and unnumbered
until the preceding migrations are merged. Final ordered upgrade/rollback proof,
full affected checks and exact-head CI remain required.

## Native assessment and private authors

Organisers configure immutable checkpoint versions against an existing approved
rubric run. Windows resolve the event timezone with explicit DST fold handling;
provisional windows stay within live start and submission cutoff. Final mentor
windows start at or after cutoff. Configuration never creates a rubric, releases
criteria or becomes a prerequisite for common challenge publication. Assigned
mentors receive only actually published common criteria; organiser-only future
rubric/checkpoint drafts do not enter that projection.

Every card write calls `rubric.Service.StartAssessmentInTransaction` inside the
same event-locked PostgreSQL transaction as its immutable card and audit record.
The native owner checks current publication, run/reference, team/entry,
assignment/conflicts, actual clocks and frozen final output. A failed card insert
rolls back the first-use run freeze. Current event schedule/window changes are
rechecked rather than silently retiming or retaining cached authority.

A category has an explicit pending flag and nullable numeric value. Pending is
not zero: an explicit zero is accepted only when the referenced rubric permits
it. Omitted categories are retained as pending. Drafts preserve incomplete
entered evidence; submitted scored categories need evidence. Pending categories
may remain pending in an independent submitted review and block final sign-off.
The exact category IDs, bounds, public guidance, reference and run are retained.

Each author has an independent card identity per team/checkpoint version, with
append-only revisions. Default reads and version-cursor history expose only the
caller's own cards, including to organisers before a real huddle context exists.
Current role/assignment/conflicts and live publication are checked on every
page; final-card reads also require the current cutoff/frozen output. Currently
authorised historical cards remain readable after explicit rubric/run restart,
with their original criteria, values and author unchanged. New writes require a
current run and explicitly rebound checkpoint; they start with fresh pending
values. No code sums or averages checkpoint cards into final mentor scores.

Final cards bind the exact native frozen submission revision. A submission
reference in final evidence must identify that exact revision; an older retained
working submission cannot be presented as the assessed frozen output. Factual
observation and note references are explanatory context, not a different assessed
submission or a source of automatic points. Cross-event/team and invented native
evidence references are rejected.

## Typed moderation and sign-off seam

`ModerationPolicy.ResolveScorecardContextInTransaction` belongs to the actual
future #104 huddle/review owner. It must resolve a persisted current context,
revision/binding, intended actor, required independent reviewers, exact submitted
card references and unresolved evidence gaps inside the same transaction. A
caller-supplied boolean, opaque ID alone, organiser role or nil policy grants no
panel/signing authority. The normal application installs no fabricated owner;
its panel route fails closed.

Panel reads return only the exact current submitted card versions named by that
owning policy, not independent drafts or other authors' revision histories.
`SignOffInTransaction` is the sole finalisation entrypoint and is called inside
the owner's persisted decision transaction. It requires complete exact final
cards, all owner-required reviewers, zero unresolved gaps, current moderator and
author authority, the current native rubric run and exact frozen submission.
It retains an immutable context/run/card proof, with foreign keys to immutable
source card versions. Failed outer transactions retain no sign-off; retries of
the same unchanged source are idempotent. Changed bindings, withdrawn output,
missing reviews and revoked authority cannot replay an earlier decision.

This seam creates no moderated totals, rankings, finalists or awards. #110
consumes its exact signed source cards/proof rather than becoming a prerequisite
for #101. Tests use a typed adapter over a real PostgreSQL **synthetic owning
context** to verify this contract; they do not implement or claim actual #104
huddle integration, UI or gap decisions. #100/#102/#103 remain separate UI/support
deliverables.

## Local evidence at the first checkpoint

- Full scorecard PostgreSQL/race suite passes 11.725s: native guarded challenge
  publication, pending/zero/bounds, pre-live/cutoff/frozen gates, independent
  authors/history/concurrent edits, assignment-removal lock race, native first-use
  freeze rollback, evidence/DST/postponement and actual rubric correction/restart.
- The frozen-reference review reproduced an older submission accepted while
  binding frozen revision 2 (red 1.938s). Passing the native frozen revision into
  both card and sign-off evidence validation corrects it.
- Additional persisted-owner sign-off cases pass 3.093s, including unresolved
  gaps, required review omissions, wrong frozen source, revoked moderator,
  outer rollback, idempotence, changed binding and withdrawn output.
- HTTP/generated-contract journey passes 4.365s: sessions, CSRF, stale revision,
  actual publication/live clock, pending values, own history and cursor,
  participant/other-author denial, default nil-policy panel denial, immutable
  private score rejection and revoked-cookie access loss. Populated participant,
  logistics/export, MC and paired room payloads, and actual approved in-app
  notification persistence/list/detail exclude stored private score evidence.
- Go lint was clean and both bundles built before the final targeted evidence
  additions. Fresh lint/build/generation and final actual-main verification are
  still required after restacking and migration ordering.
