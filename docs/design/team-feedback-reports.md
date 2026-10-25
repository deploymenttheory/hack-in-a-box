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
completed-event reports without starting a run or reopening scoring writes.
Cancelled/archived sources remain unavailable; report writers retain existing
EventEdit/WritesAllowed authority and gain no competition mutation grant.

Published teams outside the final tally may receive deliberately authored prose
under the exact current event confirmation. Their current source explicitly has
`scores_available=false` and `final_scores=null`; choosing to include
nonexistent final scores is rejected. No zero, rank or other-team tally is
manufactured. Scores for a scored team still require the explicit saved choice.

The accepted greenfield Fleet UI, action/review/unknown-receipt lifetimes,
independent interactive guide, print/mobile/accessibility/browser proof and
actual room announcement rendering are being delivered in the separate frontend
lane. This native/API checkpoint alone does not complete #113. The separate #112
room parser/frame-bound correction is consumed rather than duplicated here.
