# Independent judge scoring — issue108

This is an observable backend Draft, not an acceptance or release claim. The
starting parent is reviewed PR276 source9436cf6; its immutable freeze consumer is
used for exact current-final evidence. The branch will inherit its merged main.
Migration51 is reserved with the coordinator; migration50 and recovery ordering
remain intact.

The native owner provides versioned assignment discovery, current declared
own-team/reporting conflicts, independent drafts, complete-category submission,
locked submitted cards, reasoned organiser reopening and explicit deliberation.
Per-person draft scores are never part of the round projection. Native judge
assignment is separated from an existing mentor assignment on the same assessment
resource. Losing an assignment revokes current judge/file authority while the
immutable authored history remains retained.

The brief includes current judge-visible frozen deliverables, exact native output
facts and the exact-final repository freeze consumer. It imports neither mentor
cards, qualification totals/ranks nor private raw observations. The isolated
practice endpoint accepts no real team/submission ID, uses fictional stable entry
identity and the approved configured public rubric, and writes only separate
practice storage. Practice cannot satisfy production judge completion.

`CompletionInTransaction`, `RecordResultApprovalInTransaction` and
`ResultApprovalCurrentInTransaction` are actual event-locked native records/gates
for #112. Missing judges, stale final/rubric/card versions or unresolved managed
repository cutoff evidence prevent confirmation. Reopening/reassignment/conflict
changes retain explicit approval invalidations. Award tally/ties/announcement
remain #112; this draft does not implement or claim them.

Qualification mode passes the existing exact `rubric.FinalistPolicy` through the
native assessment seam and fails closed until the real #110 selected-finalist
owner is integrated. That owner is being built independently; no placeholder
selection grants are introduced.

The current correction binds every authored production card to the exact native
repository source, disposition, decision and SHA as well as its native final and
rubric. Managed unresolved code cannot begin scoring. File/URL-only frozen
entries remain eligible. Same-final code amendments and native-final replacement
invalidate current completion rather than silently transferring existing scores.
Reasoned reopening explicitly adopts the new reviewed source and clears obsolete
scores. Hidden historical targets expose only safe stale metadata; original
stored authored bytes remain unchanged.

Current assignment discovery is `GET /api/v1/events/{event_id}/judging`; each
returned round contains only the current authorised assignment projection. The
exact team brief remains independently authorised. Actor-exact original safe
acknowledgements have a separate current-authority recovery endpoint.

Published checkpoint ae24005 had two substantive defects, both reproduced with
real PostgreSQL against its original scoring implementation: unresolved managed
repository scoring was accepted (3.290s failing package), and an explicit SHA
amendment on the same native final silently retargeted submitted cards (4.742s
failing package). The managed fixture invokes the actual native cutoff delivery
handler to retain a real cutoff source; it does not claim durable worker/provider
execution. No external provider effects are used.

The corrected expanded owner suite passed with PostgreSQL/race in 24.172s; strict
authenticated HTTP/OpenAPI/CSRF, isolated practice, actor-bound receipt recovery
and current role loss passed in 5.485s. These runs preceded the final card
transition helper extraction. Additional exact-source verification and final CI
are recorded below as they complete. No local execution is final CI.

The original published CI also found two historical test-context assumptions.
The final assessment consumer fixture stopped at migration29 despite invoking
current native judge authority; the corrected case upgrades to current schema.
The freeze rollback test correctly removed empty51 then refused populated50,
leaving a latest-schema assertion inappropriate for the guard phase. That phase
now pins50, asserts retained50, and upgrades/checks the current schema afterward.
All original frozen-final, required-judge, qualification, history and command
immutability assertions remain. Exact local RED package times were 4.709s
(rubric) and 2.934s (freeze). Independent review, genuine #110 policy integration
and all final CI remain required before acceptance.

Final correction source before rebase: the selected real PostgreSQL/race owner,
HTTP, original rubric-final consumer and migration50 guard runs passed in
22.585s, 4.826s, 5.119s and 3.853s respectively. Configured lint over judging,
access, HTTP, app, rubric and freeze reported zero issues. Canonical generation
completed. Rebase integration, full related owners and fresh exact-head CI remain
separate gates.

## Dedicated factual mentor briefing

The existing organiser-only `judge-summary-preview` contains coordination output
counts; it is not a mentor narrative publication source. The new separate
`GET/POST /api/v1/events/{event_id}/teams/{team_id}/judge-summary` owns explicitly
judge-facing factual contributions. A current published, conflict-free assigned
mentor saves at most20 facts, each referencing an actual deliverable in the
current frozen final. The schema has no qualification score, rank, private note
or observation-reference fields. It never reads/copies private mentor cards or
observations. Authored facts are editorial content, not text proven safe by a
word filter: the organiser must review and approve the exact immutable version
with a recorded reason before an assigned judge's brief includes it.

Both save and approval bind the current native final revision and exact managed
code receipt. Drafts and stale approved sources do not appear in judge briefs.
The author/reviewer endpoint rechecks current native audience and hides stale
content while retaining its version and an actor-exact original safe receipt
when requested. Assignment/conflict/role loss revokes access; immutable versions
and audit reasons remain. This separate publication does not expose private
mentor history or transfer qualification scoring.

The initial actual managed-code PostgreSQL/race summary test passed4.571s:
unapproved hiding, denied mentor self-approval and stale approval, explicit
review, approved final-linked facts, exact ACK recovery/replay after newer page,
SHA amendment hiding, assignment loss and immutable retained versions. The
expanded case adds declared reporting-conflict loss; strict authenticated HTTP
adds rejection of a qualification field and unassigned draft reads. Its first
run found an incorrect fixture column (`reason` instead of native conflict
`kind`), corrected without changing production policy; this is not a product RED.

After rebase onto merged mainb2f7, before this additive summary owner, full
PostgreSQL/race suites passed judging32.560s, rubric25.156s,
submissionfreeze30.289s and access5.705s. Selected native HTTP authority cases
passed78.384s and app clock/authority cases12.834s. Canonical parity and configured
six-owner lint passed. The initial TypeScript check found stale shared
node_modules missing checked-in dependencies; an isolated `npm ci` restored the
exact lockfile without altering other lanes, then `tsc -b` passed. These are
local owner checks, not final Linux CI or acceptance.

The complete scoring owner including the expanded factual summary case passed
with PostgreSQL/race in33.652s; strict authenticated HTTP contract/authority
passed5.229s. The summary source configured lint reported0issues and generated
TypeScript checked successfully. Fresh exact-head CI, independent reviews and
real #110 policy integration remain outstanding acceptance gates.
