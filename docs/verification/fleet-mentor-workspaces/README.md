# Mentor operations UI migration — issue #240

Base: merged team/coverage main `e159ebfd4f54ee8a67e5924eb216cb2bbd8372ec`.
This is an incremental draft checkpoint, not a claim that UI-07 is complete.
Accepted reference: `docs/design/fleet-reference/fleet-preview.html`, visual contract,
primitive API, task patterns and independently published style guide.

## Native source and ownership inventory

| Workflow / nested owner | Current native contract and boundary | Intended composition |
| --- | --- | --- |
| `mentor-workspace.tsx`: assigned roster, TeamSummary, TeamEvidence | Published mentor coverage identifies current own assignments. Observations require a current published team and active assigned mentor or organiser. Authorship confers no lasting access after assignment loss. | Reuse #237 roster/coverage contracts; scoped team workspace and evidence inspector. |
| ObservationEditor; `mentor-observation-api.ts`; mentor-drafts/recovery | UUID-keyset current notes; immutable per-note/version history and independent status history cursors. `can_record` requires actual common release + live start + current assignment; note author controls edits. POST takes event revision and note UUID/version, not an invented idempotency endpoint. Recovery reads exact note history/current authority, never assumes absence from a truncated list. | Focused evidence edit → exact review → native receipt/history; retain draft correction but scrub on current authority loss. |
| OperationalStatus; NoteHistory/StatusHistory | Operational state/reason/next action is private and separate from observation evidence and all scores. Independent versions plus event revision fence concurrent changes. | Separate check-in task with original-source review; no competitive status inference. |
| `help-workspace.tsx`: RequestForm; `help-attention-queue.tsx` | Complete authorised summary array, native scoped waiting/claimed/resolved/cancelled counts, no prose or cursor. Request creation requires current confirmed published membership. Setup/logistics differ from competition-output help (actual common publication + live start). | Attention queue and focused new-request flow. Initial checkpoint migrates queue only. |
| `help-reader.tsx`; `help-actions.tsx` | Native current-request capability flags and eligible transfer targets. Request revision + CSRF + Idempotency-Key, stable request/command UUIDs. Claim/release/transfer/resolve/cancel/reopen remain operational. Current GET is not an action receipt; deliberate native idempotent retry must retain exact original command. Meeting route validation stays server-owned. | Named request inspector, reviewed action and exact unknown reconciliation. |
| `clarification-workspace.tsx`: CommonAnswers, PublishedRubric, PrivateQuestionList | Common answers have server search and UUID cursor; private questions have a separate cursor without global client search. Workspace supplies `can_ask`, `can_review`, team/source, staff attachment gate and native common audience count. | Common answer collection and distinct private attention queue; never mix source questions/attachments into common feed. |
| `clarification-questions.tsx`; question-files; reader/context | Private question UUID/command identity, current team and source references; private attachment ownership/bytes gated independently. Definite denial clears context; current capability loss must also scrub mounted child drafts/late work. | Private focused question/attachment task and evidence inspector. |
| `clarification-answer-editor.tsx` | Save document, explicit approve, explicit publish; exact answer revision/version and immutable common challenge/rubric source. Answer document deliberately has no private question/attachment reference. Reviewed audience is native metadata, not publication authority. | Review/correct/confirm common answer publication with source/audience preview. |
| `scorecard-workspace.tsx`: checkpoint/card loader, editor, SavedCard, history; evidence picker/preview | Cursor-scoped checkpoint/card/history endpoints, current private author/mentor or second reviewer. Rubric digest, checkpoint version, native run and event/card versions bind scores. Pending/unobserved is absent, never zero. Submitted/locked/moderated states remain authoritative. `mentor_final` additionally requires post-cutoff native frozen submission revision. | Focused provisional/final evidence score task and exact original-source review. |
| `scorecard-api.ts` / model | CurrentCard pages current checkpoint metadata; exact reconciliation reads immutable expected card version and matches actor/team/checkpoint/rubric/run/purpose/frozen submission, full scores/evidence and event revision. No automatic version rebasing or fabricated idempotency header. | Preserve real matcher; current GET alone cannot settle an uncertain original card write. |
| `review-dashboard.tsx`: dashboard, TeamSummary, TeamReview, CheckpointCoordination | Cursor coverage projection separates operational support from assessment completeness, source availability and native final signoff. Missing/suppressed/unavailable categories remain explicit. Mentor view sees own completion before actual huddle access; metadata grants no private card authority. | Attention queue → named team review workspace with explicit next actions. |
| `review-huddles.tsx`: configuration, panel/editor, history; review-huddle-api | Current intended panel/moderator, live/assessment stage and source versions govern access/actions. Configure, second review, open, gaps/resolutions, change proposals/decisions, cancel/signoff are separate native commands. Revision+version+actor/action-specific immutable-history match is a real receipt; read-current is not. | Focused configuration/decision review preserving reason, exact source, draft state, keyboard opener and uncertain write barrier. |
| JudgeSummaryPreview | Dedicated factual backend projection; team/output state, native frozen metadata and artifact counts only. No qualification score, rank, raw private notes or comparative commentary. | Read-only factual handoff inspector. New final judging #109, finalist panels #111, result release #113 and backend freeze #96 remain separate. |

## First checkpoint and remaining work

Implemented: real help queue consumes shared `ActionQueue`, `CollectionToolbar`,
semantic fields and icons. Local filters operate only on the complete authorised
summary array; counts retain their native scope. Native request links and all
existing command adapters remain in place. The presentation export accepts actor ID
and native queue data, allowing the guide to import actual production composition.

Remaining before readiness: every other nested composition above; first-class
edit/review/correct/result/next action; current capability/assignment/window fences;
minimal unresolved history and payload redaction across private projection loss;
dirty/pending/unknown navigation and real responsive opener focus; actual guide
examples/guidance; synthetic before/after desktop/mobile evidence; meaningful native
browser/privacy/claim/window/receipt tests; full local checks, independent Standards
and Spec reviews and full exact-head CI. No backend/schema/provider policy change.

First-checkpoint proof: TypeScript and both production portal builds pass; targeted
ESLint for the changed source passes. Existing organiser authorised-summary filter
and private-prose isolation journey passes on desktop and mobile (2/2). These are
bounded checkpoint checks, not full issue acceptance or release certification.

## Focused help and mentor checkpoint

Help creation and current-request actions now have separate prepare/review/confirm
steps with exact request identity, expected revision, helper and operational effect.
Native idempotency keys are retained only for deliberate original-command retries.
Unknown metadata above private controls is minimal; a current GET does not establish
an ownership-action receipt. Capability replacement ends child continuations. Draft
navigation and actual keyboard source capture are explicit. Deferred successful
focus restoration now yields to a later deliberate focus instead of stealing it.
The additive shared focus option defaults to previous behaviour for #237 consumers.

The assigned mentor surface now uses authorised collection records, scoped evidence
inspection and focused observation/check-in review tasks. Existing allowlisted
30-minute local recovery remains the observation owner's contract. Unknown private
observation reconciliation binds the original immutable expected note version,
author and captured content. Check-in history remains independent from scores.
These new mentor tasks still need their updated browser proof and final review.

Checkpoint verification: both portal production builds and targeted help/mentor
ESLint pass. `npx playwright test --config playwright.mentor-operations.config.ts
help-workspace.spec.ts help-reader.spec.ts --workers 1 --max-failures 1` completes
30/30 desktop/mobile journeys. The local config changes only server port (4298) and
worker count; native assertions about privacy, authority, request idempotency,
claim races and timing are retained with explicit review actions added. The older
help-reader fixture now supplies the native session draft scope required by private
continuation fences. The remaining issue scope above is still outstanding.

## Independent assessment checkpoint

The scorecard surface is rebuilt as a published checkpoint collection followed by a
focused private task. Save/submit first captures an exact review of author, team,
checkpoint/rubric/run, frozen submission (where applicable), expected event/card
versions and every pending/explicit score and evidence reference. Confirmation reads
current native sources again before POST. Correcting preserves actual input; no
numeric value is inferred for an unobserved category. Existing immutable receipt
matching and history remain native. Unknown commands retain only diagnostic metadata
above the private task; a later different successful assessment cannot erase the
older unresolved attempt. A deliberately selected new base does not prove its result.

Both production portal builds and targeted mentor/scorecard lint pass. The bounded
command `npx playwright test --config playwright.mentor-operations.config.ts
mentor-workspace.spec.ts scorecard-workspace.spec.ts --workers 1 --max-failures 2`
completed 62/62 desktop/mobile journeys. It retains actual typed recovery/session
and author isolation, native byte rejection, concurrent note/check-in versions,
checkpoint/frozen-output gates, exact lost-response history, private transient-read
retention, and no offline score storage/replay. Existing journeys now explicitly
acknowledge deliberate private-task departure and confirm the new review stage.
A new journey proves a later different success keeps earlier unresolved metadata.

Current remaining work includes partial authority restoration/private-cache boundary
regressions (in addition to current denial coverage), clarifications and attachment /
common-answer review, moderation/huddles/second reviews and factual preview, actual
guide examples/guidance and before/after evidence, full dependent-consumer suites,
independent two-axis review and exact-head CI. This checkpoint is still a draft.

## Private parent restoration boundary

Parent observation/assignment and checkpoint denial now cancels/removes only that
actor/event/team's owned private child caches and clears selected inspector/card
state. Private detail readers require a successful post-mount current read before
rendering cached content. A restricted observation projection also scrubs its child
note/history references. This does not erase another team's sources or any separate
unknown-command memory. Held cancelled child queries cannot refill removed caches.

Two lifecycle unit tests pass. Both production builds and targeted lint pass. The
bounded parent/restoration/revocation/transport/earlier-unknown browser selection
completed 16/16 desktop/mobile journeys, including new restored-parent/absent-note
and restored-checkpoint/denied-old-card canaries checked again after 20 seconds.
Successful list restoration now requires an explicit current record action; it
never silently revives the previously selected private task. A confirmation
preflight failure also stays not-sent rather than inventing an uncertain write.

## Private questions and common-answer checkpoint

Common answers now use a real server-search/cursor collection; private questions
use a separate attention queue. Private question creation captures source, exact
body/attachments and its native command ID for review before sending. Uploads
recheck current team/ask authority after file reading and after event reads; a
failed preflight is not reported as an uncertain upload. Staff triage has a reviewed
question revision and checks the actor/action/revision in immutable history before
settling an uncertain decision. A current question GET alone cannot clear it.

Common-answer authoring, approval and publication each have a focused exact review
of answer version/revision, common source, document and native audience preview.
Publication rechecks a changed preview before sending. Unknown recovery requires
matching document/source/version plus its immutable actor/action/revision transition;
current approved/published flags alone are not an original receipt. The deterministic
fixture now retains those actual native history transitions. Capability loss clears
private descendants and current owners; archive keeps still-authorised read-only
text and original reconciliation input while freezing further writes. A confirmed
write remains visibly confirmed and its fresh-read action remains usable if only
the follow-up read fails.

Both production builds and targeted lint pass. Native clarification workspace and
canonical-reader journeys complete 40/40 desktop/mobile cases. Existing privacy,
UTF-8 bounds, separate approval/publication/correction, live byte gate, archive,
role/session loss and no-replay assertions remain, with explicit review steps and
shared search selectors adapted. The scoped history API resolves only a matching
local intent. Its older acknowledged unresolved metadata remains visible, without
silently clearing it or re-blocking an already deliberate new base. Existing history
immutability and pending-owner tests are retained; the focused shared history,
coverage-command and private-lifecycle unit selection completes 15/15 tests.

Still outstanding: dedicated capability-only / held-continuation clarification
regressions, huddle/moderation/second-review and review-dashboard/factual compositions,
evidence picker/preview completion, actual guide and before/after captures, all
remaining dependent-consumer/native checks, independent reviews and full exact-head CI.

## In-flight private lifetime checkpoint

All new help, observation, check-in, scorecard and clarification owners now record
only minimal intent/resource/revision metadata immediately before the actual HTTP
write. A held response followed by current permission loss cannot erase the original
write barrier. Raw private input remains inside the currently authorised owner.
Known exact success or first-attempt definite rejection resolves only that intent;
current reads, component remounts and later unrelated writes do not settle it.

Dedicated desktop/mobile regressions cover held help, observation, scorecard and
common-answer writes, restricted answer preflights, and held attachment preflights.
The restricted answer test exposed an actual stale observer-refetch result: the
fresh denied projection cancelled its query and the observer returned older allowed
cached data. Direct authoritative preflight reads plus synchronous inherited private
scope tickets now block that continuation before its PUT. These 12 regressions and
all 40 clarification/native-reader journeys pass (52/52), with both builds and
focused lint passing. A broader help/mentor/card run passed 74 before two known-note
denial cases exposed the denial scrub removing its own active query and looping
back to loading. Retaining that active denial result while removing other private
children fixes both cases; the four missing-note/partial-restoration cases pass.
The remaining breadth must still run on the complete final source.

Review/huddle/factual composition is in progress separately from this checkpoint.
All previously listed unfinished scope and final verification requirements remain.

## Review, moderation and published-guide checkpoint

Review coverage now starts with comparable authorised team rows and a named detail
inspector. Native operational support remains separate from assessment completion;
unavailable assessment projections do not render suppressed placeholder counts as
zero. Team review uses a cursor agenda and a focused huddle canvas on narrow screens.
Configuration, second-review completion, opening, formal gaps/resolutions, proposals,
moderator decisions and native final sign-off retain their distinct native commands.
Each captures an exact source/action/reason/evidence review before confirmation.
The adapter rechecks actual current coverage/agenda, role, huddle version and native
source revision before writing; it grants no new panel/card/assignment access.

Huddle unknown history above private owners holds only minimal diagnostic metadata.
Exact immutable history matching includes team, huddle, expected version, author,
action/reason and the action-specific source. Current reads cannot settle an earlier
write. Permission loss clears selected private descendants and their owned caches,
fences held continuations, and retains the minimal barrier. A temporary parent read
failure pauses the mounted private draft and explicit recovery preserves input.
Local/route discard is deliberate, and close/confirmed actions return focus to the
actual responsive opener. A planned panel offers its named second an explicit route
to their independent scorecard without fetching either private panel card.

The factual handoff remains a dedicated backend projection and printable read-only
preview; private review is hidden in print. It excludes qualification scores/ranks,
raw notes and comparative commentary. New final judging, finalist/results release
and backend freeze work remain separate tickets. Exact evidence selection/reading
now uses production collection controls, fresh mounted reads and private lifetimes;
working drafts and unauthorised observation text stay unavailable.

The independent guide imports the real production help queue, mentoring review,
unknown-state and detail components with synthetic local callbacks, plus copyable
usage guidance. No API/session/cookie/private store is used. The new guide scenario
also exercises current-read-is-not-receipt, keyboard return and portal/reset lifetime.

Checkpoint proof (not final-head certification): both production builds and focused
lint pass; the complete help/mentor/card/review selection completes 112/112 browser
journeys. Full ESLint and all 133 unit tests (19 files, bounded two workers) pass.
The independent Go-hosted guide completes 32/32 desktop/mobile browser journeys.
The expanded native review/support suite completes 32/32, adding current capability
loss, held-write/parent restoration, dirty local/route cancellation, actual opener
focus, separate second review and transient parent recovery. Existing assertions are
preserved with explicit task/review stages; the dependent support drilldown now
selects the actual team record and scoped intended-panel help destination.

Actual synthetic baseline images were generated from merged main e159ebf using
its original native tests (10 desktop/mobile capture journeys passed). `before/`
and `after/` contain the current retained captures. Initial inspection confirms the
old mobile page mixed all panel controls in one long form; the new selected huddle
has a focused canvas, explicit task navigation and independent evidence inspection.
The final complete matching capture set, dependency/portal proof and independent
reviews/full exact-head CI are still outstanding. This PR remains a draft.


## Complete matching visual evidence

All images below are actual deterministic native browser captures at the configured
desktop and iPhone 13/Chromium viewports, with synthetic identities and source data.
The before set is merged main e159ebf; the after set is this UI branch. No PNG was
edited or composed. Existing source and privacy assertions run before each capture.

| Current workflow | Before desktop / mobile | After desktop / mobile |
| --- | --- | --- |
| Mentor private observation / separate check-in | [Desktop](before/mentor-private-mobile-entry-desktop.png) · [Mobile](before/mentor-private-mobile-entry-mobile.png) | [Desktop](after/mentor-private-mobile-entry-desktop.png) · [Mobile](after/mentor-private-mobile-entry-mobile.png) |
| Independent provisional scorecard | [Desktop](before/private-checkpoint-scorecard-desktop.png) · [Mobile](before/private-checkpoint-scorecard-mobile.png) | [Desktop](after/private-checkpoint-scorecard-desktop.png) · [Mobile](after/private-checkpoint-scorecard-mobile.png) |
| Current team help and operational history | [Desktop](before/team-help-current-history-desktop.png) · [Mobile](before/team-help-current-history-mobile.png) | [Desktop](after/team-help-current-history-desktop.png) · [Mobile](after/team-help-current-history-mobile.png) |
| Support and assessment coverage | [Desktop](before/support-and-completeness-desktop.png) · [Mobile](before/support-and-completeness-mobile.png) | [Desktop](after/support-and-completeness-desktop.png) · [Mobile](after/support-and-completeness-mobile.png) |
| Intended private panel and exact evidence | [Desktop](before/private-panel-evidence-desktop.png) · [Mobile](before/private-panel-evidence-mobile.png) | [Desktop](after/private-panel-evidence-desktop.png) · [Mobile](after/private-panel-evidence-mobile.png) |

Visual comparison against the accepted Fleet reference: old repeated boxed note,
recovery, check-in, category and panel forms are replaced with flat source headings,
shared controls and separator rows. Team comparison uses a real table/mobile list;
selected work owns the narrow canvas. Named Lucide/text actions separate evidence
inspection, gaps, proposals and formal decisions instead of exposing every edit at
once. Existing exact evidence and pending/unobserved semantics remain visible.

## Final local proof and precise limits

The broad participant selection completes **234/234** desktop/mobile cases, covering
all migrated native owners plus dependent help readers, mentor-support drilldown,
team resources, inbox, participant guidance, connection recovery and portal shell.
The final cursor audit found a selected huddle capability observation restricted to
page one. The new real later-page test fails against the published pre-fix bundle
on both widths, retaining a panel after current capability loss. Inspecting the
matching record on every returned page fixes it without inferring absence from an
incomplete page. Both production builds pass and the complete huddle/support suite
then completes **34/34**, including both red-to-green counterproofs. Full ESLint and
all **133 unit tests / 19 files** pass. The final independently Go-hosted guide
completes **32/32**, with API/cookie/private-storage access forbidden in every case.

The rebuilt expanded administrator/shared team-and-coverage selection completes
**218/218** desktop/mobile cases, including shared roster/focus/planning consumers. The first
pre-fix run passed198 cases with only the two intended later-page failures and18
unrun after its failure limit. No pre-fix run is claimed as final proof. Independent
Standards/Spec reviews and full exact-head GitHub CI remain mandatory before ready
or merge; the parent delivery agent owns those steps. No backend transaction or real
provider behaviour changed, so this migration adds no PostgreSQL/provider operation.


A final factual-reader regression also reproduces a current-authority gap: returning
through client routes while the15-second query cache is fresh can show the former
organiser projection without starting a new native read. The dedicated factual
query now requires a fresh mounted GET, gates presentation until that read completes,
and removes its cache when unmounted. This does not change its factual allowlist or
printability. Its direct red-to-green proof and all post-main integration checks are
required after restacking onto newly merged resource main c91c52e.
