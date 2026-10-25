# Deliberate own-team feedback reports — native owner and typed API

Issue #113 owns announced-result reading and deliberate feedback report delivery.
The support feedback owner intentionally cannot carry scores. This separate
native aggregate stores explicitly authored report prose; it does not copy raw
judge/mentor assessment, infer sanitisation or publish an event result.

An organiser reads the actual current #112 confirmation and an exact own-team
score projection in the same event transaction. The source contains the current
confirmation/binding, public category labels and exact category averages/total;
it has no rank, award or private assessment prose. Saving a report captures its
source, author and explicit optional-score choice. Releasing captures that exact
draft version and current confirmed published-team audience. Saving a correction
keeps the prior release visible until a separate reviewed release; original
versions and actor-exact command acknowledgements remain immutable.

Recipient reads require both the original audience snapshot and fresh native
confirmed published-team membership. Drafts/staff history and other recipients'
identities never enter the participant projection. Result-source drift pauses
current score-bearing report reads/previews without rewriting stored originals.
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
under the exact current event confirmation. Their source explicitly has
`scores_available=false`, `total=null` and no category scores; choosing to include
nonexistent final scores is rejected. No zero, rank or other-team tally is
manufactured. Scores for a scored team still require the explicit saved choice.

The accepted greenfield Fleet UI, action/review/unknown-receipt lifetimes,
independent interactive guide, print/mobile/accessibility/browser proof and
actual room announcement rendering are being delivered in the separate frontend
lane. This native/API checkpoint alone does not complete #113. The separate #112
room parser/frame-bound correction is consumed rather than duplicated here.
