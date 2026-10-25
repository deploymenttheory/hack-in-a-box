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
