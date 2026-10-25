# Deliberate own-team feedback reports — native owner and typed API

Issue #113 owns announced-result reading and deliberate feedback report delivery.
The support feedback owner intentionally cannot carry scores. This separate
native aggregate stores explicitly authored report prose; it does not copy raw
judge/mentor assessment, infer sanitisation or publish an event result.

An organiser deliberately authors prose for a current published team. Prose-only
reports do not require an event result confirmation, a scored final entry or a
public announcement. Their source binding identifies the current event/team
scope. Saving captures the author and explicit optional-score choice; releasing
captures the exact draft version, releaser and current confirmed published-team
audience. Corrections preserve the prior release until another reviewed release.

Score inclusion is separate: the organiser must select and submit the captured
`score_binding` from the current own-team confirmed result. That narrow source
contains public category labels and exact category averages/total, with no rank,
award or private assessment prose. Its exact confirmation and judge-run identity
are retained only for score-bearing reports. Prose versions store SQL/JSON null
for those absent identities; no zero score or fabricated confirmation is used.
The native current source is revalidated before scored save, release and read.

Recipient reads require both the original audience snapshot and fresh native
confirmed published-team membership. Drafts/staff history and other recipients'
identities never enter the participant projection. Result-source drift pauses
current score-bearing report reads/previews without rewriting stored originals.
Independent prose remains available to its current original recipients through
later result confirmation, completion and unrelated judge access changes.
Releasing a report never announces finalists/awards or triggers a provider.

The versioned API exposes administrator-only GET/POST
`/api/v1/events/{event_id}/teams/{team_id}/feedback-report`, administrator
`/history`, actor-exact original `/requests/{request_id}` on either portal, and
current confirmed member `/released` on either portal. Typed save/release
commands carry request UUID, If-Match, current source binding and reason. A
current GET never creates an acknowledgement. CSRF and strict additional-field
validation apply through the canonical HTTP runtime.

A separate native read-assessment/completion path revalidates current published
rubric, frozen final/repository identity, required independent cards, current
judge roles/assignments/conflicts and genuine signed qualification. It permits
completed and archived report reads without starting a run or reopening scoring
writes. Archiving alone preserves the exact deliberately released own-team report,
including explicitly selected confirmed scores; current recipient membership,
captured audience and all native source safeguards still apply. Cancelled sources
remain unavailable; report writers retain existing
EventEdit/WritesAllowed authority and gain no competition mutation grant.

Published teams outside the final tally may receive deliberately authored prose
without a result confirmation. Their current source explicitly has
`scores_available=false` and `final_scores=null`; choosing to include
nonexistent final scores is rejected. No zero, rank or other-team tally is
manufactured. Scores for a scored team still require the explicit saved choice.

The accepted Fleet frontend consumes these typed routes in both portals. It
provides authored draft, private preview, captured review, separate release,
correction and author/audience/version history; private payloads remain local to
the current owner. It defaults optional scores off and captures the explicit
confirmed score binding only when selected. Metadata-only original request
capsules survive guarded in-app navigation, while native receipt lookup remains
separate from current reads. Current denial clears private presentation and held
continuations. Audit dates use the current event timezone.

Announced-result pages and paired room presentation consume only the #112
announcement projection. Finalist announcement, private award confirmation,
award announcement and team feedback release remain independent controls. The
assigned judge ballot workflow reads the exact native judge run and frozen
candidate evidence for both returned overall and all-team category ties.
Explicit task selection uses exact category labels from the current published
rubric in those assigned briefs; only the caller's vote and explicit replacement
are shown. Qualification categories remain with their separate native panel owner;
no judge or organiser casting authority is inferred.

See [frontend browser verification](../verification/team-reports/frontend/README.md)
for native PostgreSQL/HTTP/browser results, independent guide examples and
synthetic visual evidence. The separate #112 room parser/frame-bound correction
is consumed rather than duplicated. The parent delivery retains the category
policy, required CI and integration gates; this document does not close them.


Internal delivery gate: the owner confirmed that every judge votes on all-team
category ties and that hack mentor views are added. Whether those mentor views
are advisory or counted votes still requires clarification and a native owned
flow. This independent judge UI consumes current authorised native ties only;
it neither invents the mentor contribution nor establishes full issue113
completion. Keep the PR Draft until that dependent policy/flow is resolved.
