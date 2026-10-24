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
