# Fleet moderation and selection workspace — issue 111

This is a full-scope work-in-progress UI delivery, stacked on actual native #110
PR #279 source `2abe330050f1ef8511dbe7f8626fcea5b7014c2b`. The parent owns
migration52, private qualification authority and actual FinalistPolicy. This UI
adds no migration and cannot make a local selection authoritative. #109 is a
separate real judge interface dependency; #112/#113 retain final award
confirmation/publication and feedback release. No live provider effects occur.

## Complete acceptance ledger

| Required behavior | Current implementation and remaining proof |
| --- | --- |
| Current actor-permitted discovery and native action readiness | Native producer gap reported precisely to #110 owner; actual additions are being implemented there. UI must consume generated capabilities, never infer a grant from a role or rubric listing. |
| Evidence side by side with reasoned amended final cards | Actual current panel reader and source-specific huddle reauthorization beside current moderated scores/reasons exist. Native card editors, evidence selection/save/sign, exact receipt handling and full native browser journey remain pending. |
| Required reviews, evidence, eligibility, absent judges and timing blockers | Current native team/proposal blockers and formal/code outcomes are visible. Add native readiness fields when published; current judge-completion metadata must come from its actual permitted owner. No missing value becomes a zero score. |
| All eligible category candidates versus finalists/reserve and ties | Pure production views render the native separate lists and unresolved-tie blockers. Native reasoned ballots, casting resolution and full configured-count/non-finalist category browser flow remain pending. |
| Versioned panel signoff | Native current-signoff state is displayed; prepare/review/confirm and original-ACK recovery are pending actual producer capabilities and native proof. No current GET can acknowledge an earlier command. |
| Separate finalist/category announcement preparation | Separate explicitly private local previews require current native signed source; finalist preview omits categories. This is preparation only, not approval/send/publication. No publication endpoint is invented; later owners retain that boundary. |
| Not-submitted/withdrawn/disbanded unused outputs | Current native status is shown distinctly, without a score entry action. Actual retired-source writer denial and corresponding browser/native proof remain required. |
| Current authority, history and privacy lifetimes | Actor/session/event/run keys plus parent event/private read scopes fence native panel and huddle reads. Current denial and changed-source tests clear inspector/private preview. Exact command lifetime, version0 safe metadata, original receipt/history and native session/role-loss proof remain pending. |
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

Native #110 initially had no permitted run discovery/action-capability projection;
the owner is implementing that actual read seam. Private Proposal/Current/Signoff
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
