# Native notification action sequencing (#329)

CI run40507781737 reached a successful native notification configuration save,
but the following pointer action landed while the preview-read button was
disabled during refresh. Its pointerdown/up were captured without a click,
there was no second notification GET, and approval never appeared. A repeated
local run found the same pattern after the initial session configuration save.

Both transitions now wait for the original acknowledgement and the review to
close, then use the existing finite Tab/Enter keyboard journey to reach the
actual enabled preview control. The server preview must still enable the exact
notification decision. No forced click, sleeps, retries of writes or budget
changes are added. Application code and authority remain unchanged.

The wait-for-acknowledgement-only proposal still failed one of3mobile runs.
Correcting only the later transition passed5of6quiet desktop/mobile journeys,
with the remaining failure at the initial pointer read. These failed proposals
are not counted as passing evidence. Concurrent heavy native checks also hit
the existing90-second scenario budget; those runs are retained privately.

The final correction passed all6affected native desktop/mobile journeys in a
quiet run (3each,57.755s total), with real PostgreSQL and the race detector.
The complete selected scenario includes concurrent bookings, own original
receipts, cancellation/promotion, scoped roster and withdrawal. Final-run
source-clean receipts are in proof.json; original per-run logs/traces remain
private. The two other unchanged native session scenarios were not selected
in this repetition. Full native browser CI still runs its ordinary selection.

Frontend artifacts were copied unchanged from the normal main-ef676fea public
and admin images; the source rebase to6c52e4e1 changes Compose/docs only. This
is native booking-test evidence, not the full #120 lifecycle/role acceptance.
