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


## Strengthened native checkpoint and main integration

At `d4f129823438d6f1682c4e1e7f2aabc1d4e39c1c`, the serial native batch passed:

| Native package / proof | Result |
| --- | --- |
| `results`: populated 54→55 upgrade, empty rollback, retained corrections/commands, cross-event FK and populated rollback protection | PASS 3.519s |
| `teamreports`: original schema upgrade/empty rollback, then restore actual latest schema | PASS 3.054s |
| Composed advisory native/API test | PASS 32.302s (test 30.76s) |

The composed test now validates canonical HTTP responses and no-store/CSRF in both
independent portals, current conflict and published coverage withdrawal, all
required judge ballots, and actual award confirmation while other category ties
have no advisory. A later advisory correction preserves the entire confirmed round
and decision binding; corrections remain current attributed input, never a rewrite
of what earlier ballots/confirmation considered. A first expanded HTTP assertion
expected a missing CSRF header to return 403; the canonical required-header
validator correctly returned 400. The final proof separately asserts missing
header 400 and an invalid 32-byte token 403, with no accepted-status fallback.

The standalone guide passed **2/2**, desktop/mobile (**5.4s**) at `af538077`,
using production exports, actual opener restoration, retained local corrections,
pending dismissal locks, unknown callback separation and no private APIs/storage.

The branch then rebased onto merged main `076f6162558df2ac3d7fb1ff7c23b3d35dc21784`.
Generated conflicts were resolved from canonical sources; both independent guide
sections and the common native two-team/non-winner fixture were preserved. The
original proof references above remain historically attributed; the rebase is not
a retrospective claim that those runs tested a different source. Canonical
regeneration and both portal builds pass after integration. The dedicated required
CI native-browser step preserves all existing steps and timers and retains its
own artifacts.

The first actual browser attempt at `4c2956f3` stopped in native fixture setup,
before Playwright: a synthetic independent-event role used the unsupported name
“facilitator”; the real role constraint rejected it (RED 16.671s). The fixture now
uses the supported logistics role, proving that losing mentor capability clears
its narrower private resource while independent event reading remains. No native
authority rule was changed. Complete current-source browser and owner reruns remain
pending at this checkpoint.
