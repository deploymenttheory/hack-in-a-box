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

Initial owner/API/router/composition compilation and canonical code generation
are being checked. Real PostgreSQL transaction/concurrency tests, authenticated
HTTP tests, configured lint, generated parity, independent review and all final
CI remain pending. No local execution is represented as final CI.
