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


## Final author candidate and visual evidence

At exact source `0b1b70014ce0d196a90758bedded2bce3600c60e`, the complete native
four-case browser matrix passed **89.231s** (test **87.60s**): administration
desktop **35.63s**, administration mobile **16.78s**, participant desktop
**16.57s**, participant mobile **18.62s**. All four cases ran once, without
retries, skips, flaky results or changed timers. The explicit status assertions
prove that current reads retain uncertainty and exact original acknowledgement
removes the obsolete transport error. Native role/source fencing, actual retained
versions and exactly one real judge ballot remain part of every case.

The immediately preceding attempt at `a110358c` is retained as **RED 174.148s**:
three cases passed, and participant mobile reached the final read action after
automatic current-source polling had already correctly denied its source. The
old locator expected only the active-state read button. The final test accepts
the actual active or already-denied read-only action, while retaining the exact
denied heading, private-content absence and unchanged write-count assertions.
No product authority or timing was altered for that test correction.

The independent guide then passed **2/2 (2.2s)** at the same source. Its actual
focus, correction, unknown acknowledgement separation and absence of private
API/storage calls remain required assertions. The final improved shared-hook
regression passed **3/3 (2.44s)**. Both portal builds/TypeScript, affected frontend
and Go lint, independent guide build and canonical generation are clean.

All Go, canonical API and database query bytes remain identical to `a85f8d84`,
so the complete current native owner/schema proof above remains attributed to
that exact source. Production frontend bytes remain identical to `a110358c`;
`0b1b7001` adds only the final test-state locator and stronger unit-test seam.

The sixteen unedited final synthetic screenshots were visually inspected across
both portals, widths and light/dark modes. Mentor captures show exact ACK
acknowledgement without the stale red failure; judge captures keep advisory
provenance separate from the required judge ballot. Event-brand canvas colours
remain authoritative within either shell theme. Files, SHA-256 hashes and
normalised successful per-case results are indexed in
[`native-browser-evidence.json`](native-browser-evidence.json). No raw authenticated
traces, cookies or live user data are published.

The author candidate is frozen for independent full Standards and Spec reviews
and current-head CI. It remains Draft until those final gates pass.


## Independent review correction: authoritative successful context changes

Full independent review of `8e596734` found one P1: a successful preflight
returning narrower mentor targets or a newly prepared source could reject the
write while retaining the old private editor until an unrelated poll. The two
actual-reader/shared-history unit regressions reproduced this (**RED 1.03s**,
two failures; the three earlier regressions still passed).

At `01192604eca5ff44b4088b6a094ef7b905eeaec9`, successful preflight pages
reach their owning reader before the generic revision/version guard. A changed
existing context signature synchronously renews the private authority and
publishes the new page, remounting its child editor. This covers confirm, fresh
base read and base selection; it does not fabricate an HTTP denial. Ordinary
revision/version changes preserve authorised input and conflict recovery. The
independent original-request history remains untouched, with no inferred ACK.
All **6/6 unit regressions pass (1.42s)**, including both positive200 changes,
ordinary revision conflict and the existing exact-ACK/cross-owner protections.

The strengthened actual native four-case browser matrix passes **83.006s**
(test **81.37s**): administration desktop **21.75s**, administration mobile
**20.27s**, participant desktop **19.83s**, participant mobile **19.52s**.
Each case now proves removal of published mentor coverage while the mentor role
remains active returns real **200 with no targets**, then clears held private
views/contribution/review with no POST. Restored coverage starts with blank
inputs. Both judge and mentor also hold reviews while an actual judge card is
reopened, resubmitted, deliberation reopened and results prepared again. The new
valid native source returns **200**; both old owners clear private inputs/reviews
and never retarget or POST them. Existing role403, unprepared-source409, lost
committed-response, exact receipt and non-counted advice cases remain intact.
The immutable prior result version still contains exactly one actual judge
ballot; the explicit re-prepare adds exactly one version and resets its current
ballots to zero. Advisory history still contains exactly three versions.

The sequential independent guide passes **2/2 (1.6s)** on that same source. Both
portal builds/TypeScript and frontend ESLint pass. All sixteen fresh captures
were visually inspected; the evidence index now names `01192604`. Earlier
`0b1b7001`/`8e596734` screenshots and proof remain historically available in
that Git revision. Production Go/API/SQL bytes remain identical to the native
owner/schema proof at `a85f8d84`; only the browser fixture Go test changed.

This corrected complete candidate awaits independent review of the P1 fix in
full context and latest-head CI before Ready or merge.

### Native qualification CI: rejected local inspection

The final `d36dce65` native CI run `40247746151` failed the public desktop
final-panel journey when reopening its first team: the required heading was
absent, rather than present with incorrect focus. The other seven qualification
cases passed. This remains a failed historical run, separate from the passing
local advisory and reports matrices at that exact source.

The retained private trace shows the Inspect control passed its enabled check
at 16791 ms, then a real current-panel GET began at 16827 ms. The button had
`aria-disabled="true"` in the 16844 ms snapshot before trusted Enter at 16859 ms.
The GET returned 200 approximately 687 ms after starting. Its response bytes
were identical to the preceding current read, excluding a changed source as the
cause. Installed Base UI preserves this control's focus while disabled and
prevents the native click's default without invoking the supplied action. The
correct authority pause therefore rejected local navigation.

A tight actual-Chromium fixture reproduces the missing inspector with the old
helper, then exercises the same disabled/preventDefault event contract. The
explicit read-only `inspectCurrent` helper permits one deliberate local key
re-entry only after observing the rejected key, zero accepted activation, and
an unchanged connected DOM element and record identity. A prevented click
while positively paused is rejected; an unprevented click is accepted and is
never repeated. The exact destination heading must still receive focus.
Replacement, unexpected input and unobserved prevention fail. Default command
activation remains single-shot; no domain write, unknown acknowledgement or
retargeted action is retried. Source reads and polling remain active.

The complete keyboard browser controls pass **18/18 (2.9s)** across desktop and
mobile, including the causal local-navigation case and accepted-click,
no-observed-pause, replacement and single-command negative controls. This is a
synthetic helper proof, separate from the subsequent full native qualification
matrix. Production code, domain/API/SQL and advisory/report fixture bytes are
unchanged by this correction; their earlier source-attributed proofs remain
historical evidence rather than relabelled current-head runs.


## Local-step activation after the ada CI failure

CI run `40249603943` at `ada5a8fcd20560771cda83b14c2dea68152b29c6`
passed seven qualification scopes and failed the public/mobile/final journey.
The reason field did not appear after the local saved-card review opener. The
private trace shows the same opener enabled at relative time 31428.156 ms, a
current GET beginning at 31430.987 ms, and `aria-disabled=true` from 31437.040 ms
through trusted Enter at 31439.243 ms. Base UI's disabled-button callback prevents
the click default and returns before the local editor's `open(sign_card)`. The
reason form never mounted; this was not a failed POST or a missing receipt.

A tight real Chromium fixture reproduced the same missing visible reason field
with the old single-key helper (750 ms assertion, exit 1). It models the actual
Base UI `focusableWhenDisabled` / prevented-click semantics and the rejected-key
window, without changing production code. The corrected local-step helper and
negative controls passed 26 desktop/mobile cases in 4.8 seconds. The raw trace
and logs remain private; they are not checked in.

The audited local callbacks are editor `open`, unsent form `review`/command
`prepare`, inspector/next-team selection, history/details opening, preparation
preview selection, and explicit current-source/original-receipt GETs. These may
use `openLocal`, with an exact visible destination or focus assertion. It captures
the original DOM node, URL, control identity and rendered private workspace; only
a focused trusted key positively rejected while paused with zero unprevented
accepted clicks permits one re-entry after the same context becomes enabled. A
second rejection, changed context, replaced target or unobserved prevention fails.
The helper also rejects HTTP mutation requests during the local opening.

Router links remain single-shot: their successful navigation legitimately
prevents the click default, and they have no local disabled-button callback.
Actual write confirmation retains `confirmReviewedDialog` and its exact-one-POST,
original-receipt checks; the initial deliberately lost-ACK write and session-denial
gestures retain their original single-key behavior. No product guard, polling,
timeout, domain API, majority rule or mentor-advisory authority changed. Current
full native qualification execution and current-head CI are still required.
