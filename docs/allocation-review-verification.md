# Allocation review and publication

Issue #65 adds organiser review over #64's read-only proposals. The Teams tab
shows declared allocation features, seed and optional proposal constraints,
explanations and unresolved people. Organisers refine a proposal through explicit
manual controls; generating or dismissing one never applies its assignments.
New proposed groups receive an organiser-selected name and captain.

Native selects and buttons support keyboard moves. Locked source cohorts require
explicit unlocking, and full destinations explain their hard capacity. Confirmed
people already in unlocked drafts can captain a reviewed replacement group;
competitors cannot use team creation to bypass an existing assignment. Failed
creation rolls back the team, entry and member move together.

Published and draft memberships remain visible side by side. Empty unlocked
teams retire atomically at publication, so draft rebalancing does not retire a
participant's published team early. Empty unpublished groups can be explicitly
disbanded. The publication preview identifies member moves, captain changes,
missing captains, undersized teams, unassigned people and proposed retirements.
Cancelling the preview performs no write. A changed event revision invalidates
the preview and any proposal; organisers must review again.

Publishing and publishing-with-freeze are distinct explicit confirmations. The
server locks the current event, checks current authority, write permission,
revision, confirmed places, team locks/capacities, captains and minimum sizes,
then copies the reviewed draft. Freeze also requires every confirmed competitor
and permanently prevents competitor reassignment. A browser clock hint disables
controls at the deadline; the server's clock and lock remain authoritative.

A changed publication inserts exactly one durable `teams.published` operation in
the same transaction. Its target is the event and its unique key is
`teams-published:<committed revision>`. Communication and repository consumers
reconcile the latest published roster, never draft data. Draft commands and
repeated unchanged publication produce no such operation. This records a change
intent; provider delivery belongs to the communication/repository backlog owners.
No provider delivery is claimed by this interface.

Verification:

- Real PostgreSQL tests cover one publication event, no draft events, unchanged
  publication, stale review, cancelled-event rejection, current confirmed-place
  checks, queued publication at the hard cutoff, replacement-group rollback and
  publication-only retirement.
- Desktop/mobile browser scenarios cover keyboard unlock/move, explanations and
  unresolved warnings, cancellation without writes, stale preview invalidation,
  cancellation during review, and explicit hard freeze with server blockers.
- Existing allocation and registration lifecycle/race coverage remains in place.
