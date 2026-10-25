# Mentor category-tie advisory verification (#112)

Mentor views are newly authored advisory evidence. Every required judge must
record a ballot, a strict majority is required, and a split remains unresolved.
Advisory absence adds no confirmation gate. Mentor scores/ranks never enter final
judge calculations, and advisory records do not enter the decision binding.

## Implementation checkpoint

- Separate retained advisory streams bind author, team, exact category tie,
  judge run, current source binding and frozen submission revision. Corrections
  record their predecessor and reason; original commands have actor-exact receipts.
- Published mentor coverage, current active role, team eligibility and conflicts
  are checked in the event-locked transaction. An organiser role or generic
  resource grant alone does not allow contribution. Mentors read only their own
  current assigned targets/contribution; judges read current advice only for their
  independently authorised category ties. Organiser preparation remains private.
- The contribution API has no preferred-winner field, score, rank or vote weight.
  Result rounds, votes, tallies and confirmations are owned by the existing result
  owner. Advisory changes advance the event revision without replacing those facts.
- A new mentor review/write route and current judge/organiser reading components
  are implemented using production Fleet primitives. Browser verification and the
  independent interactive guide remain pending at this early Draft checkpoint.

## Actual execution so far

The first native run found the application schema target still set to 54 despite
migration 55 being present (**RED 9.536s**). The canonical target now includes the
new migration; no fixture manually creates its tables. A second run reached source
replacement and exposed a fixture omission (**RED 25.289s**): reopening a judge card
invalidates deliberation, so the corrected fixture explicitly reopens native
deliberation after resubmission before preparing the next source.

The resulting real PostgreSQL/native owner regression passed (**58.201s**, test
**56.44s**) before subsequent formatter/error-wrapping and equivalent validation
extraction. It proves scoped targets/no private score payload, unchanged tally and
round after advice, exact receipt/replay/changed-intent rejection, immutable
correction provenance, native concurrent revision fencing, required ballots,
unresolved split and explicit majority, role loss and source replacement with
safe original acknowledgement retained. Current-head reruns and stronger full
confirmation proof remain required; this engineering checkpoint is not final
acceptance evidence.

Both portal builds and TypeScript have passed. Frontend lint and configured affected Go lint have passed (zero issues).
Canonical generation is being checked at the committed checkpoint.

## Required before completion

- Current-head native regression, explicit advice-independent confirmation and
  immutable populated-parent migration/rollback proof.
- HTTP contract/current coverage/conflict/role/source denial in both portals.
- Actual native mentor → judge category ballot/correction/unknown ACK journeys in
  administration/participant portals and desktop/mobile viewports, no fake domain
  success responses or relaxed timeouts.
- Independently published interactive guide using actual production exports,
  keyboard/focus/privacy checks, and final screenshots/source attribution.
- Affected owner regressions, complete current-head CI and independent Standards
  and Spec reviews. The issue stays open and PR stays Draft until these pass.
