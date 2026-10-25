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


## Complete native browser and current owner checkpoint

At `a85f8d847af0960cedbe9e9be88c077b412eeb2f`, all four native browser cases
passed on a quiet restored engine, **140.110s** (test **138.27s**): administration
Desktop **34.78s**, administration mobile **35.65s**, participant desktop **35.43s**,
participant mobile **32.41s**. No retries, skipped or flaky cases. Each fresh native
fixture proved three retained advisory versions and exactly one actual judge
ballot, current role/source disposal and real committed-response-loss recovery.
Historical strict-locator failures were corrected to the actual named reading
region, separating current native records from same-page controlled editor text.

The earlier focused pending-POST failure at `5d6aaeda` is retained as failed
execution (**124.817s**): GETs took 2.61s/5.26s and POST had no completed response.
Docker Desktop subsequently exited, but that timing does not establish the cause.
After authoritative restore, the unchanged save/receipt path at `b682f665` returned
first POST 200 in **493ms**, correction in **453ms**, judge ballot in **1.185s**,
and recovered original ACK in **344ms**. The original pending-save symptom did not
recur in that baseline or the complete quiet matrix; no application fix or proven
Docker root cause is inferred from this alone.

The final current native owner/schema batch at the same `a85f8d84` source passed:
`results` **4.232s**, `teamreports` **3.296s**, composed native/API/confirmation
**42.123s** (test **40.52s**). It additionally proves that an actually assigned
second mentor receives their own empty contribution stream while neither reading
nor replacing another author's views or original acknowledgement.

## Visual status correction before final freeze

Visual inspection of the passing matrix found a real status defect: an exact
original ACK was known, but the private hook retained an obsolete red transport
failure. The unedited before screenshot and hash are retained under `history/`.
A two-consumer shared-store regression reproduced that exact seam (**RED 1.05s**).
The bounded UI correction tracks the uncertain attempt identity and hides its
transport failure only after that exact metadata attempt is completed while the
private scope remains current. Current reads, selected bases, an unrelated older
ACK and session disposal cannot stand in for that completion. The corrected unit
regression passes **2/2 (1.20s)**; no API call, domain record, source binding, private
payload lifetime or successful-write inference was added.

Both portal builds/TypeScript and ESLint pass with the UI correction. The complete
latest native matrix with an explicit error-before-ACK/error-cleared-after-ACK
assertion and final clean screenshots remains pending at this checkpoint. The
previous four-case pass stays attributed to its actual source above.


The same unit seam also exposed premature private-scope invalidation after a
write-time 409 while the existing refresh confirmed current authority. Its
regression failed in **1.26s** and the combined **3/3** suite passed in **1.98s**.
A failed current preflight still ends its source context; an ordinary rejected
write performs the same existing refresh without permanently disabling its
still-authorised editor. If that current read proves source loss, the scoped
reader still disposes its payload. No new network call, domain state or permission
was introduced. TypeScript, both portal builds and ESLint pass with both bounded
UI corrections; the latest native matrix remains required before final freeze.
