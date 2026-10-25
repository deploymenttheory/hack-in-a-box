# Fleet moderation and selection workspace — issue 111

This is the full-scope issue111 follow-up in Draft PR285, based on main
`fcbfe038f669df74a73d51ec7f160c222c859397` after the partial PR283 merge. Native
#110 qualification and #109 judging are merged real dependencies. The UI adds no
migration, publication API, native scoring policy or provider effect. Final
award confirmation/publication and feedback remain owned by #112/#113.

The current implementation is complete for independent review; acceptance stays
open until both full review axes and final-head CI pass. Historical sections
below retain their original source hashes and failures rather than relabelling
older runs as current. The final checkpoint section supersedes their open-work
notes. Local browser evidence is Chromium at desktop/narrow viewports, not a
physical-device or full issue122 accessibility certification.

## Complete acceptance ledger

| Required behavior | Implementation and evidence |
| --- | --- |
| Current actor-permitted discovery and native readiness | Real actor-permitted collection and page/team/tie capabilities drive both portal routes; no client role or repository-list inference grants a scoring action. Four original native discovery/configuration/lost-ACK/session journeys passed at e9a7a561. |
| Evidence beside reasoned final cards | Separately authorised exact huddle and original-card API reads validate team/run/rubric/frozen version/reference correspondence. Real five-team native journeys save and sign every final card with observed evidence and reasons; old originals remain unchanged. |
| Required reviews, evidence, eligibility and timing blockers | Native blockers and action capabilities control display and preflight. Common-cutoff/frozen/reconciliation policy stays with merged native owners; missing evidence never becomes a zero score. The managed-code eligibility form consumes the real native decision/evidence contract rather than inventing a SHA or approval. |
| All eligible category candidates, finalists/reserve and ties | Actual five-team native selection retains the non-finalist Technical/UX winner and multiple categories for another team. Two independent mentor ballots split UX; the organiser records the actual permitted casting decision. No unresolved tie is inferred from retained historical tie records. |
| Exact versioned private signoff | Real native review, original ACK and separately read current signoff are exercised. Pending/unknown/current GET never substitutes for an original receipt. |
| Separate finalist/category preparation | Current exact private signoff enables separate labelled local preparations; finalist preparation excludes category selections. No approval, send or publication is simulated. |
| Not-submitted/withdrawn/disbanded unused outputs | Original native cases prove not-submitted/no score; final cases use actual organiser withdrawal and explicit test-only disbanded/unused-repository facts. Current-card rendering requires the native current-source flag; historical cards cannot score retired output. |
| Current authority, history and privacy lifetimes | Structured current visibility disposes open private history before its poll; current denial removes private sources. Native cases open the exact immutable snapshot and revoke the actual session, then require real native denial and private-history disposal. Only safe matching card-ack coordinates cross disposed editors. |
| Keyboard/narrow action → review → result → next action | Bounded actual Tab traversal, native select type-ahead, keyboard text entry, exact review and next-team return run on both real portal builds. Pending controls retain focus while activation stays blocked. |
| Independent interactive production guide | Standalone/embedded production views/fields, invalid input retention, correction/cancel-to-actual-opener focus and unknown metadata after private disposal pass8/8 on919b01ef. The guide publishes pending focus, exact ACK and next-team ownership rules. |
| Final source acceptance | Local full native proof is source-attributed across e6a19b63 plus the affected919b01ef regular-desktop retest; current component suites passed26/26 each portal. The final retained-tie copy correction has RED2→GREEN2 and affected visual/native assertions are running. Independent full Standards/Spec and exact-head CI remain mandatory. |

## Source contracts and known boundary

The current real API is
`/api/v1/events/{event_id}/mentor-qualification/{run_id}` GET/POST,
`/commands/{request_id}` for exact immutable ACK metadata and `/history` for
current/original-audience history. Organiser huddle/source access is separately
rechecked; panel membership is not a grant to an entire private source document.
An organiser evidence link uses `/events/{event_id}/teams/{team_id}/build`
without `judge_run`, because that query invokes a judge-only exact context.

Native #110 initially had no permitted run discovery/action-capability projection.
Actual producer ee980c31 now supplies that seam and is consumed by this UI. Private Proposal/Current/Signoff
are not an announcement API. The UI keeps local preparation explicit, and must
never claim an announced result from private panel state. Early read-only source
is intentionally not a complete scoring/signoff implementation.

## Initial bounded proof

On the current isolated working-tree extension of actual parent2abe330,
TypeScript, focused ESLint, both production portal builds and the independent
guide build pass. Admin production reader/privacy/native-shaped HTTP-fixture
cases pass **8/8 in 5.3s**, desktop plus Chromium narrow emulation. They exercise
source-only denial, actual organiser build destination, separate private
preparation, changed signoff/source clearing and current native denial/fresh read.
These are deterministic browser fixtures, not PostgreSQL/native acceptance.

The independent static Go guide host passes **4/4 in 13.4s**, standalone and
embedded at both widths. Actual production views are imported; no API/auth,
cookie or private-store behavior is used. These checks do not establish full
accessibility, real mobile certification, native command success or publication.
No product RED is claimed for these initially passing reader additions.

Initial tool bookkeeping: a type-check command was accidentally started from the
worktree root rather than web, then terminated only its owned handle. The actual
successful checks use this worktree's pinned `web/node_modules/.bin/tsc` and
ESLint. No dependency or global tool was installed or changed by that attempt.

The initial production reader/guide implementation was checkpointed as
`47467be94a99d25217ef360204d374666bd3e4a0`. The following housekeeping checkpoint
only ignores owned Playwright outputs and adds this source attribution; it does
not claim that the earlier working-tree executions ran on a later clean commit.

## Initial native-version metadata correction

A new actual shared-store regression failed before the correction: a version0
native panel command became unknown, survived task replacement, but lost its
`expectedVersion: 0` metadata. The whitelist now admits nonnegative safe integers;
negative, fractional, nonfinite and unsafe values remain discarded, as do private
reason/panel payloads. This changes retained metadata only, not native authority
or any command precondition. The full five-case communications suite passes
(506ms), along with TypeScript and focused ESLint. Native command/browser receipt
matching remains a separate required proof; this unit success is not that proof.

## Native receipt codec and command-owner extension

The additive real PostgreSQL/race test
`TestFrontendQualificationCodecUsesActualAcceptedOriginalReceipts` uses the
existing native receipt fixture and actual `Service.Change`/`OriginalReceipt`.
With `HACKBOX_INTEGRATION=1`, it passed in **3.991s** and produced three accepted
command/receipt vectors, including initial version0, verbatim HTML/Unicode reason,
and nil versus explicit empty evidence. The frontend fingerprint matches those
actual immutable native receipts; the shared metadata plus codec suite passes
**6/6 in 906ms**. Go lint reports zero issues. Types/focused lint and both portal/
static-guide builds pass for the codec/API/field extension.

This working-tree extension adds exact native API adapters, controlled reasoned
score fields, and a scoped reviewed-command/original-recovery owner. Its UI owner
integration and native browser command journeys remain pending the newly
implemented actual producer discovery/readiness checkpoint. The hook is not
claimed as an exercised complete workflow. The three corpus commands are panel
configuration; final-card/evidence/tie serialization still requires real native
UI command proof. No stored private body survives task disposal.

The first Go invocation omitted the explicit integration flag, so its successful
compile with skipped fixture is **not** native execution evidence. An attempted
frontend corpus run then correctly failed to resolve the absent artifact. The
explicitly enabled native run generated the real corpus; only the subsequent
six passing frontend cases are codec evidence. All owned native/browser/build
handles reached terminal status; the fixture terminated only its own container.

A bounded production command-owner test now passes **2/2 in 929ms**, covering
lost initial writes, safe version0 identity after private task teardown,
mismatched versus exact original acknowledgement, current reads remaining
separate, and deliberate byte-identical captured retry. It uses explicit API
transport doubles plus the real native receipt corpus; it is not a native browser
acceptance run. Its initial failure expected an internal exception string that
shared Failure deliberately sanitises; the corrected test checks the exact public
failure, persistent unknown metadata and absence of acknowledgement before the
subsequent genuine matching-corpus receipt. No product assertion was removed.

## Actual producer readiness and first command browser integration

Five owned UI commits rebased cleanly from original parent2abe onto actual
producer `ee980c31c75b10032579009323af00c431a13b21`, reaching
`eb158338afa371cbdc7bcdda4a50ba90496169d8` before the following working-tree
integration. Earlier native vectors/executions retain their original attribution;
this rebase does not relabel them as new native executions.

The actual new discovery route and generated page/team/tie capabilities now drive
both portals. Current native staff listing separately supplies panel pickers.
Reasoned final-card save/sign, unresolved-code eligibility, proposal, mentor
ballots/casting resolution and exact private signoff are wired to real command
fields and preflight readiness. These are implementation, not yet full native
browser acceptance. Reasoned final-score fields retain current references and
source versions; no client averages or selection authority are introduced.

The first new actual keyboard browser case failed on both widths while the old
eight reader cases passed: review was available before the authorised staff read
finished, so a later-rendered organiser error missed first-invalid focus. Review
now waits for that actual pending read. The unchanged focus/cancel/unknown/receipt
assertions pass **2/2 in 3.4s**, with an added deterministic held-response gate:
review is disabled before release, then enabled before normal keyboard activation.
A first post-fix attempt pressed the correctly disabled button before readiness;
it made no validation attempt and is not a product pass. No force-click, timeout
increase or global retry was used. Both portal builds/static-guide build and
TypeScript pass for the command integration. Full native commands, remaining
write browser cases, independent write-guide scenarios and inspected final
visuals remain open acceptance gates.

The final bounded controller/shared-metadata/codec checkpoint passes **9/9 in
975ms**, with TypeScript and focused ESLint clean. The additional controller
case verifies that current revision drift prevents POST, leaves no fabricated
original attempt and explains the unsent result inside the open review. This is
transport-double controller verification, not native transaction acceptance.

## History, deliberate current-base review and native browser harness

The working-tree extension of `f85c81156bd7888c60272ec78c1ac6cdc4e04010`
adds the actual current-audience paginated history endpoint and deliberate
current-base review. The earlier unknown request remains in safe history even
after selecting current facts; no original body is replayed or acknowledgement
invented. Bounded controller/metadata/codec checks pass **10/10 in 2.53s** and
actual keyboard current-base cancel/real opener return, explicit selection and
original recovery pass **2/2 in 5.9s**. JSDOM has no layout rectangles, so the
unit case verifies the enabled fallback; the actual browser independently proves
the initiating button. Both production portal builds pass; focused types/ESLint
and native app Go lint pass with zero issues.

An additive native browser harness now uses the actual app qualification HTTP
fixture, real PostgreSQL and both compiled portals with test-owned Unix IPC.
Initial scope is actual native panel configuration/lost response/original ACK,
not-submitted blockers, current history and native session expiry. Its four
portal/viewport cases are **not yet executed at this checkpoint**. It does not
claim full five-team final-card, tie/casting or private signoff acceptance. That
larger scope remains mandatory. The native owner is adding a before-cutoff test
seam for real formal URL submissions; no clock rewind or settled-fact rewrite is
used. Browser evidence records start/end commit/tree and cleanliness; no public
fixture controls or external sends are introduced.

## Executed initial native portal matrix

Actual clean source `cb41704ca5b9dce5835d0d87c7ebb0688f52bdb3`, tree
`8f4d19b4a4674fa1cf690955d4242f6d3bacd40d`, passed all four initial native
PostgreSQL/router/compiled-browser cases. Administrator desktop/mobile passed
Go/race **27.057s**; regular desktop/mobile passed **21.844s**. The preserved
`native-initial-*.json` receipts each record identical start/end commits and clean
worktree status. Every fixture terminated its own PostgreSQL container; no user
containers or provider effects were touched.

These cases execute actual permitted discovery, generated-shape configuration
POST with HTML/Unicode reason, a committed response deliberately lost, exact
original receipt recovery, independent current read, honest not-submitted/no-card/
no-signoff blockers, current history and real session-revision expiry. They do
not use canned domain responses. They remain deliberately scoped: no complete
five-team final-card/ballot/casting/signoff or full visual/manual-device acceptance
is claimed. Full write-guide and broader privacy/recovery cases remain open.

The independent109 correction review exposed applicable hardening to complete in
this111 owner: HTTP408 must remain unknown, and sent-command departure barriers
must survive private task denial in the metadata owner. Those are explicit
remaining work, not waived by this passing initial native matrix.

## Timeout uncertainty and surviving departure protection

On the unchanged compiled production owner, six new actual browser cases failed:
HTTP408 discarded original identity on both widths, and both pending/ACK and
pending/408 planning barriers disappeared after private reader denial. The
correction keeps408 unknown and moves sent-command route/planning/before-unload
protection into the surviving metadata history, leaving unsent/preflight guards
with private editors. The same six assertions pass **6/6 in 4.8s**. They cover
late exact ACK barrier release, refused/acknowledged unknown departure, retained
metadata on actual return and absence of extra POST; no case timeout or global
retry changed. The brand selector uses each actual portal's full accessible name.

TypeScript/focused ESLint and the controller/shared-metadata/codec suite pass
**10/10 in 1.10s** after adapting its harness to a real in-memory router. Native
initial-panel evidence remains attributed to cleancb417, not relabelled as this
later correction. Full native final-card/tie/signoff and write-guide acceptance
remain open. The new-proposal review now explicitly states its actual native
reset of current ballots/resolutions while preserving immutable history.

## Independent interactive write guide

The guide now imports actual production final-card fields/validation and the
production confirmation dialog, using explicitly fictional memory callbacks.
Initial new standalone/embedded cases failed all four real-opener return
assertions while the four inherited read-guide cases passed. The guide owner now
restores the actual enabled opener after the correction render; unchanged cases
pass **8/8 in 4.0s** across both widths. They cover retained invalid values and
first-invalid focus, captured review/correction, pending dismissal lock, unknown
versus current facts, private scope disposal and safe synthetic original callback.
Cookie/storage traps and the no-account/API boundary stay active. This is guide
behavior, not real native domain acknowledgement or a device certification.

The dedicated production browser configuration now supports a bounded regular
portal run as well as administrator, preserving actual portal accessible names.
No full ordinary browser suite is multiplied by the new configuration.

## Corrected producer and merged judging integration

Ten owned UI commits rebased from parent6897630 onto corrected native producer
`414d8036d3e8894537595683abcb4fd6c72fa186`, which includes merged109 on main29948.
Clean UI source `214c624b4bea2f79e1f3bea2606902f1fe20223a` preserves both portals'
judging and panel routes, both guide examples, the merged guide pointer correction,
all browser registrations and separate native proof steps/artifacts. No production
backend, OpenAPI or generated-contract change is introduced by the UI rebase.

TypeScript/focused ESLint and both production builds pass on this clean source.
The complete bounded moderation browser suite passes administrator **16/16 in
8.9s** and regular **16/16 in 7.3s**, each including both widths. These deterministic
transport cases retain their separate scope from the cb417 native four-case
matrix. The full five-team frozen-source final-card/tie/signoff proof, completed
visual captures and final reviews/current CI remain mandatory open gates.

## Issue 111 follow-up after the merged checkpoint

PR283 was merged as `fcbfe038f669df74a73d51ec7f160c222c859397`; that checkpoint
was not full issue111 acceptance. The follow-up branch starts from that actual
main and applies only the two preserved privacy edits, without replaying merged
implementation commits. Original native/source receipts above remain historical.

The previously unpublished excluded-target regression failed both widths before
the correction: an open history retained private decision/round prose after an
authoritative current panel read removed its target, until the history poll.
The lifetime fence now includes structured decision/reference visibility and
presence flags for withheld prose; private text is never placed in scope keys.
Against the current main working-tree extension, unchanged cases pass **2/2 in
4.6s**, with TypeScript/focused lint and both production builds passing. The
independent write guide passes **8/8 in4.6s**, including unchanged standalone and
embedded cancel-to-actual-opener assertions, retained validation and unknown
metadata after private disposal. This is not new native transaction evidence.

Remaining mandatory scope stays in one follow-up PR for existing111: real
five-team frozen-source card/eligibility/tie/signoff journeys, complete current
history/capability/privacy/recovery hardening, native/visual proof across both
portals, independent reviews and final current CI. Final award confirmation,
publication and feedback release remain separate112/113 responsibilities. No
local preview or UI receipt invents those APIs or released results.

### Follow-up native checkpoints and current open gates

The five-team source fixture passed real PostgreSQL/race at clean `23478c4e`
(6.934s), using formal live submissions, a monotonic cutoff, independent submitted
final cards, real second-review completion and exact signed huddles. Its first
attempt correctly failed invalid mentor capacity; the fixture now declares the
actual five-team coverage and configures a native panel rather than bypassing
that policy. No production authority was changed.

The real save-card journey exposed lost inspector/next-action continuity in both
admin layouts at `f9f565d7`. It passed both at clean `b12ac3d3` (Go/race68.592s)
after carrying only exact acknowledgement/navigation metadata to a matching fresh
native card. A separate test-only `Draft` versus native `draft` capitalization
correction is not a product fix. A second actual keyboard regression identified
the inspector heading stealing next-action focus; explicit owner focus preserves
the shared default and is documented in the interactive guide.

At clean `dc52ad54c463beadd70281a9cec338b712d94417`, the full admin desktop
browser passed in35.8s: original authorised evidence, five reasoned save/sign
pairs, split ballots from two independent mentors, organiser casting, exact
versioned signoff, a non-finalist Technical/UX category winner, and separate
private preparations. `native-complete-admin-desktop-dc52.json` records its exact
source/tree. Its mobile attempt stopped at a test trying to press the hidden
collection without Back to panel. The follow-up now uses the real return action
and verifies same-row keyboard focus through the existing coverage focus owner.

A real production-component regression also showed a retained historical card
as current after native `card_source_current=false`/withdrawal (RED2/2). The
current inspector now requires the native current-source flag and labels
withdrawn/disbanded/inactive outcomes distinctly; the affected retired-card and
back-focus cases passed4/4 in3.6s. Source was the working-tree extension on dc52;
these are component checks, not native outcome proof.

Clean `f619f9e5dc4f26a06d26a4921b70afb191018891` extends the native journey
with an actual organiser withdrawal, explicitly test-only disbanded/unused-repo
lifecycle facts, no current score/actions after retirement, and light/dark
captures. The full admin mobile journey PASSED (2.1m browser). Admin desktop
stopped at its second card confirmation: a real native request became unknown
and the dialog remained; the UI retained the unknown barrier. This is an OPEN
proof failure, not a pass or evidence that the command failed. No retry,
assertion weakening, timing-policy change or fabricated receipt was introduced.
The complete run therefore FAILED (Go/race222.116s). Independent guide build and
all8 real standalone/embedded guide checks passed in8.8s on the same source.

Still mandatory: diagnose and resolve the desktop native failure, complete both
regular-portal native layouts and current visual inspection, remaining
privacy/recovery cases, independent full reviews and final CI. All issue111
acceptance stays open until those gates complete; #112/#113 still own final
award publication and feedback release.


## Final follow-up checkpoint

The safe late-original-card acknowledgement race was reproduced in both widths:
a current poll disposed the private sender before its matching reply and lost
the result/next action. RED2 became GREEN2; the complete affected denial,
HTTP408, history-visibility and pending checks passed10/10 in6.8s. No captured
scores/reason/evidence crosses that boundary. A one-time navigation handoff
requires current panel/card/freeze/version authority and never reopens a manual
selection after departure. Signed cards expose the explicit **Review another
team** action.

The full original/final matrix at e9a7a561 passed the four original native
journeys and exposed only a new history-test locator ambiguity in final cases:
the same retained reason legitimately existed in multiple immutable snapshots.
The selector now binds to the exact explicitly opened panel version. Actual Tab
traversal and native-select type-ahead replaced implicit locator focus; browser
pages activate once when changing actors. Native session expiry now asserts the
actual denied response and immediate disposal, allowing the real current check
to win the race rather than trying to activate an already removed control.

At clean e6a19b63 the final native admin desktop, admin narrow and regular narrow
journeys passed. The aggregate four-case run failed only regular-desktop saved
confirmation focus during a pending poll. That real focus defect was reproduced
RED2 and fixed GREEN2 while activation stayed disabled. Related regressions
passed6/6 in12.3s. The affected complete native regular-desktop journey then
passed at clean919b01ef (Go/race172.462s). Neither the earlier genuine unknown
HTTP outcome nor failed aggregate runs are reclassified as passes.

Both complete portal component suites passed26/26 (admin41.4s, regular40.3s).
Controller/codec/shared metadata tests passed10/10 in3.91s. Types, focused lint,
both portal builds and native app Go lint passed. The independently hosted guide
build and all8 standalone/embedded checks passed at919b01ef in6.0s.

Visual inspection found and corrected one final contradiction: a signed panel
still warned of an unresolved tie by counting retained tie contexts. Native
`unresolved_tie:` blockers now exclusively control that message; tie history is
unchanged. This was RED2→GREEN2 in4.0s. Current native visual/assertion proof and
source-attributed captures will be appended without altering the frozen
production implementation. All8 native cases remain required in CI; its10m
aggregate budget accounts for four full five-team journeys plus the original
four, with no global retry, assertion relaxation or native request-timeout change.

Remaining acceptance gates: independent full Standards/Spec, final-head CI and
review of the final source-attributed guide/visual record. Issue111 remains open
until the parent completes those gates. #112/#113 publication and feedback are
separate; no live provider effect or external acceptance is claimed here.

### Frozen implementation: final native and guide evidence

At exact clean `a6f5e447dff0726f3f709eb9741131f4e9a25f5e`, **all four final
native portal/width journeys PASSED in one Go/race run (289.107s)**. This includes
the resolved-tie warning assertion, actual keyboard/native-select flow, every
card/ballot/casting/signoff step, separate preparations, retired-output checks,
exact history and real denied-session disposal. This supersedes the earlier
partial final runs without changing their historical outcomes. The original
four initial cases remain attributed to e9a7a561 and all eight are mandatory in
current-head CI.

The independent production guide build and all8 checks passed on that frozen
implementation in4.1s. `final-native/artifacts.json` records exact source/tree
and SHA256 for four successful native records and12 genuine PNG captures.
All8 signed light/dark portal/width captures and four comparison captures were
visually inspected: no horizontal clipping, labels/actions remain readable,
separate category preparation stays private and the false tie warning is absent.
The event's configured light canvas remains its actual branding in a dark shell;
these images reflect that existing branding contract.

One comparison element capture could clip its lower boundary if its independently
loaded original-evidence panel grew after screenshot measurement. A test-only
readiness assertion now waits for all10 actual independent observations before
measuring that capture. Its affected regular-narrow visual retest is recorded
separately; no production implementation, authority or assertion is weakened.

Only independent full reviews and final-head CI remain acceptance gates. No
physical-device, screen-reader or full issue122 certification is claimed.

The screenshot-readiness-only retest completed at exact clean
`f7681392f0a319178d52dcc310f3de8e2d17f4e0`: the actual regular-narrow native
journey passed (70.562s browser, zero retries/skips). Its fixture records matching
start/end commit and successful browser exit. The original tool's terminal output
was truncated, so no separate Go duration is asserted for this retest; subsequent
process inspection confirmed no live owned native/browser process. Only this
scope's four artifacts were refreshed with their actual source/tree and hashes;
the other three retain a6f5 attribution. The refreshed comparison was visually
inspected with its complete original-evidence and moderated-card contents.
Production source is unchanged from the four-scope a6f5 native pass.

### Post-review manual inspector focus correction (not in merged PR285)

The independent Standards review found that retained original acknowledgement
metadata disabled normal inspector focus even after manually opening another
team. The complete actual UI sequence (matched sign ACK → Review another team
→ inspect another team → revisit that team → reopen the original team) reproduced
**RED2** at both layouts: the newly displayed inspector heading was inactive.
The correction separates the current opening's focus provenance from retained
safe ACK metadata. The ACK transition keeps its one-shot explicit next-action
focus; a manual opening restores the normal inspector heading focus. Pending
actions remain disabled and original unknown requests retain their barriers.

At original correction checkpoint `7fd3c8a1217859a52ff214622b52daebedcfc5c5`,
the complete sequence and related pending/private-denial/HTTP408 checks passed
**12/12 admin (4.5s) and 12/12 regular (5.2s)**, both layouts. The correction
transplanted cleanly as `b186fe241ce4122dd9d068666187b1c3567d11fc` onto the
rebased PR head `10ee498923647ffeb108325bbb958a5e4c7cf638`; types, focused ESLint
and both production portal builds passed on b186. Only the three owned
UI/browser files changed; the shared native fixture union is untouched.

The native final journey now explicitly asserts actual heading focus on each
manual opening and a same-team revisit after a real sign acknowledgement.
**That updated native assertion is NOT RUN locally yet.** Root requested holding
heavy native verification while integrating PR286 after the user merged PR285.
No source was pushed to the merged branch. This correction remains a local
follow-up candidate; native proof, independent correction reviews and follow-up
CI remain required. Earlier native matrix evidence retains its exact sources
and does not establish this new focus assertion.

The separate follow-up starts from authoritative merged main
`56d00be40e098f9ab71b2387377d2ad7cd81b63c`. Only the correction and ledger were
transplanted; exact clean implementation/checkpoint
`09d471884ffeda7b0c2f754527587bf6481bb75f` passed types, both production portal
builds and focused ESLint. Bounded affected browser suites passed **12/12 admin
(8.4s) and 12/12 regular (9.6s)** at that exact source, covering both layouts,
manual other/same/original-team openings after the matched late ACK, pending
read locks, denied-private-reader departure and original HTTP408 unknown state.
No native fixture was started while the root-owned PR286 matrix was active.
The follow-up remains draft; updated native focus proof and independent bounded
correction reviews/final CI remain required.

### CI2093: read-interrupted confirmation and obsolete focus restoration

CI run40218455895 at PR head `2093b0c3919de8b389e5a6e94526c39242c206e3`
failed only admin/desktop/final (seven other native cases passed;468.72s total).
This does not invalidate the earlier f559 eight-case PASS499.16s, and neither
result is relabelled as current correction proof. The failed artifact shows the
reader GET beginning at76828.133ms between the last Confirm focus check
(76815.434ms) and trusted Enter (76837.948ms). The Enter snapshot has Confirm
natively disabled without command aria-busy, enabled Cancel, no stale error and
no casting POST. The recorded prior acknowledged owner request was `propose`;
it was not a casting acknowledgement. The same cause reproduced deterministically
RED2 at both widths by starting a genuine read at that input boundary: zero
POSTs and the exact review stayed open after the disabled key.

The test-only reviewed-confirmation helper now observes actual trusted keydown
and click acceptance. Only positive disabled-key evidence, zero accepted clicks
and zero POSTs permits one explicit new gesture after the same captured review
is enabled. There is no loop, sent-command replay, DOM focus/click mutation,
blanket polling suppression, test retry or timeout increase. Other focus
anomalies fail. Exactly one accepted click and one POST are required; the real
response must match the exact original command/fingerprint before the unchanged
dialog-disappearance assertion. Lost, HTTP408 or mismatched receipts remain
unknown and never trigger another gesture. The native journey's final signoff
uses this same reviewed-confirmation seam.

The broader affected checks exposed another real owner race: a delayed closing
collection-return frame could steal focus after a newer manual inspector opened.
A deterministic scheduling test holds that real frame, opens the inspector using
trusted Enter and observes its real autofocus, then releases the obsolete frame.
The2093 production baseline failed RED2 at both widths (5.9s each). The only new
production change opts this closing caller into the existing
`navigation.restore(target,true)` guard, yielding to a newer connected focus
owner. Generic restoration, ACK next-action focus and normal collection return
are unchanged. Setup cases now await the actual inspector heading owner before
attempting a later input. Earlier broader attempts genuinely failed
(admin22/24, regular23/24) and are not treated as passes.

With these corrections, bounded affected suites passed **28/28 admin(11.3s)**
and **28/28 regular(11.6s)** at both layouts. They include normal acceptance,
read interruption/resumption, changed source, current denial, HTTP408, lost
response, mismatched receipt, wrong focus target, original unknown barriers,
ACK continuation and the deterministic obsolete-frame sequence. Types, focused
ESLint and both production builds passed on the owned correction overlay.
No native policy, receipt matcher, API or schema changed; all eight native CI
cases and the40m Go CI budget remain required. Affected clean-source native
proof and new independent correction reviews/current-head CI remain pending.
