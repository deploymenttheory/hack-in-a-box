# Same-event live role boundaries

Connected R42 passed **303.00s** at clean source
`ecdfda2c1769bd3d8976d5284b52b7e62b2d4fe4`, based on merged main ee687d32.
Exact immutable verification/normal-guide images and bounded native receipts
are in `live-role-proof.json`. This source adds only test fixtures; production
result changes are already in main through #332.

The owner uses the actual Fleet staff form to grant coordinator-as-organiser,
MC and logistics on both concurrent events. Go independently checks all six
current grants, exact revision changes, unchanged configuration, live stage and
confirmed competitor count. No direct database grant or session injection is
used.

Twelve actor cases each visit both events: organiser, coordinator, current
captain, distinct current attendee, other-team attendee, mentor, judge, MC,
logistics, authenticated member without event access, revoked organiser and
anonymous stranger. Current participants are selected from actual published
team survivors after the required withdrawals. The checks cover the Staff UI,
owner-only transfer control, MC presentation, logistics-only operational data,
private help and exact native read statuses. Discovery/read phases submit zero
application writes. Go independently checks all24 event/actor contexts.

The owner then revokes mentor-a in the first event through the real staff UI.
An already-open help view loses its private context, while the same mentor's
other-event help remains readable. The owner restores the grant through the UI.
Go verifies exact current grants and ordered owner-authored revoke/restore audit
entries, unchanged prior mentor history and unchanged peer-event mentor history.
Both private-cleared and peer-retained screenshots were visually inspected.

The complete same-event native run continues through judging, feedback, archive
and configured120-day expiry on both events. Final receipts show frozen
competition hashes unchanged and all required expiry denials. All scoped
containers, volumes and networks were removed. Original private screenshots,
phase receipts, native journal, database and logs are retained outside the repo.

## Failed attempts

R40 atc28612e8 and R41 at8f8a62dd failed before revocation because the new fixture
assumed the wrong audit pagination contract. The endpoint accepts `cursor`, not
`after`; an empty terminal page retains the last cursor. A small real normal-
admin protocol probe independently confirmed empty terminal items with retained
cursor. The reader now stops on empty items and still rejects nonadvancing
nonempty pages. Neither failed run is counted as a full role journey pass.

## Remaining acceptance

This is LIVE coverage of the stated boundaries, not the complete Fleet route
inventory or every role at every stage. Event creation, registration, timed
publication, assessment/feedback/archive UI journeys, other-stage role cases,
remaining provider/live sandbox prerequisites and final provenance reconciliation
remain required by #120. `complete_lifecycle:false`; PR #316 stays draft.

Run with HACKBOX_LIFECYCLE_BROWSER_PHASES=role-matrix using the versioned
connected browser runner. Existing planning and live-recovery selections remain
supported, and required native receipt guards reject omitted phases.
