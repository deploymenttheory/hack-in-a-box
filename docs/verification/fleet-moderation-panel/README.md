# Fleet moderation and selection workspace — issue 111

This is a full-scope work-in-progress UI delivery, stacked on actual native #110
PR #279. The initial source was `2abe330050f1ef8511dbe7f8626fcea5b7014c2b`; current integration consumes actual producer `ee980c31c75b10032579009323af00c431a13b21`. The parent owns
migration52, private qualification authority and actual FinalistPolicy. This UI
adds no migration and cannot make a local selection authoritative. #109 is a
separate real judge interface dependency; #112/#113 retain final award
confirmation/publication and feedback release. No live provider effects occur.

## Complete acceptance ledger

| Required behavior | Current implementation and remaining proof |
| --- | --- |
| Current actor-permitted discovery and native action readiness | Actual actor-permitted collection discovery and generated native page/team/tie readiness are consumed from ee980c31. Both portal entry routes exist; deterministic keyboard discovery/configuration proof passes. Full native discovery/role-loss journey remains pending. |
| Evidence side by side with reasoned amended final cards | Actual current panel reader and source-specific huddle reauthorization beside current moderated scores/reasons exist. Native-gated reasoned final-card fields, separately authorised evidence selection and exact save/sign review are implemented. Real native card browser proof remains pending. |
| Required reviews, evidence, eligibility, absent judges and timing blockers | Current native team/proposal blockers and formal/code outcomes are visible. Actual native readiness/source/card version fields are consumed; independent judge-completion metadata must come from its actual permitted owner. No missing value becomes a zero score. |
| All eligible category candidates versus finalists/reserve and ties | Pure production views render the native separate lists and unresolved-tie blockers. Native reasoned ballot/casting commands consume exact native candidate sets/readiness. Complete tie/configured-count/non-finalist category browser and native proof remain pending. |
| Versioned panel signoff | Native current-signoff state and exact prepare/review/confirm use actual readiness; original-ACK recovery is implemented. Full signoff native browser proof remains pending. No current GET can acknowledge an earlier command. |
| Separate finalist/category announcement preparation | Separate explicitly private local previews require current native signed source; finalist preview omits categories. This is preparation only, not approval/send/publication. No publication endpoint is invented; later owners retain that boundary. |
| Not-submitted/withdrawn/disbanded unused outputs | Current native status is shown distinctly, without a score entry action. Actual retired-source writer denial and corresponding browser/native proof remain required. |
| Current authority, history and privacy lifetimes | Actor/session/event/run keys plus parent event/private read scopes fence native panel and huddle reads. Current denial and changed-source tests clear inspector/private preview. Exact command identity, safe version0 metadata, unknown history and original-receipt recovery have bounded actual controller/browser proof. Full native session/role-loss and command history presentation proof remain pending. |
| Keyboard/mobile action → review → result → next action | Existing Fleet primitives, flat responsive side-by-side composition, actual organiser submitted-build link (without judge_run), source/review next actions and production guide views exist. Full command keyboard/correction/focus/native narrow journey and final inspected visuals remain pending. |
| Independently hosted published production guide | Actual pure production view example runs standalone/embedded, with storage/cookie/API traps. Editors/review/unknown lifecycle examples and usage/accessibility documentation must follow in the same delivery. |
| Integration with real predecessors and final source verification | Existing #105 review/evidence routes are retained. Actual #109 routes are `/events/{event_id}/judging` and `/judging/practice`; link only against real integrated sources. Parent merge/rebase, full independent reviews and final CI remain gates. |

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
