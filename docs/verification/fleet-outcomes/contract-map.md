# Ideas and outcome follow-through — #242 / UI-09

The draft is rebased onto main `e9d7c4665a582feca6543718861ff56308d82fe9`. Private ideas and reviewed-project discovery, released project detail, owner-review destinations and definition fields now use Fleet production primitives. Full nested forms and final evidence below remain owned by #242; this is not ready for merge or a completion claim.

## Complete present UI ownership inventory

| Existing owner | Nested consumers and native boundaries |
| --- | --- |
| `outcome-workspace.tsx` | `OutcomeWorkspace` / `OutcomeDirectory`: own and selected-event organiser-review projections, native UUID cursor, no invented global search/count. `NewIdea` / `DefinitionFields`: independent organisation pitch versus explicitly selected published source team, actual challenge publication/live-start checks; proposal source/private reason remains private. `OutcomePage` / `OutcomeReader` / `OutcomeSummary`: current source, exact version, owner and due date. `OutcomeReviewPage`: current native review destination, including ordinary archived-owner access without granting event-directory/configuration access. `PublishedProjects` / `PublishedProjectPage`: existing native positive-field search/cursor and released project detail, current approved audience and consent-aware URLs. |
| `outcome-controls.tsx` | `ActionFrame`, `DefinitionEditor`, `Adoption`, `Moderation`, `Assignment`, `Progress`; generated commands preserve current capability, original expected version and reason. Event adoption grants only native outcome authority. Owner choices use current native candidates; assignment does not approve reminders. `OutcomeEvidence` / `EvidenceForm`: estimate, self-report and demonstrated adoption remain distinct; exact decimal strings and unknown/null are retained, never coerced into zero or inferred ROI. `OutcomeHistory`: bounded immutable version history. `OutcomeReviews` delegates the actual plan owner. |
| `outcome-reviews.tsx` | `OutcomeReviewControls` / `ReviewEditor`: optional 30/60/90-day plans, actual explicit wall time/timezone/DST choice, native current owner, exact source/message/channel/provider/privacy binding, explicit save/approve/cancel. SMTP acceptance is not receipt/read; Teams is never an owner-reminder channel. Replacement approval cannot resolve an original unknown external effect. |
| `outcome-publication.tsx` | `OutcomePublication` / `PublicationEditor`: positively authored fields, explicit current-source/publication versions, current audience and deliberate event/team-name sharing; exact approved released media ID/version, per-reader current consent. No private source/evidence/assessment/recording URL is copied. Draft save, exact publish and withdrawal remain separate commands. |
| `outcome-command.tsx` | `useOutcomeCommand`, `CommandState`, `useOutcomeUnsent`: current session/preflight, CSRF, original UUID/expected version/immutable payload, exact original receipt, deliberate same-command retry only after native receipt absence plus current source validation. Full migration must separate safe original metadata above route/tab/capability/version owners from raw private payload retained only in a currently authorised editor. Do not lift raw payload into global CommunicationSafety or generic shared history. |
| `outcome-api.ts` | Generated outcome/publication/review types and exact dispatch/read helpers. `readOutcome`, `readPublication`, `readReview`; `sendIntent`; `intentReceipt` uses `command_id`, never a generic current projection as a receipt. |

## Native implementation contract inventory

- `internal/httpapi/outcomes.go` owns the existing HTTP list/detail/history/owners/evidence, publication/projects, review-list/detail/change routes. `api/openapi.yaml` and generated Go/TypeScript remain the API-shape authority.
- `internal/outcomes/service.go`, `model.go`, `measurements.go`: current readable resource and role/team/owner capabilities, actual live/source validation, immutable outcomes/evidence, explicit review/moderation/owner changes. Cancelled-event writes are denied; archived current-owner exceptions remain native decisions.
- `internal/outcomes/read.go`: own/review/all native projection, selected-event organiser check, bounded cursor and owner/evidence/history reads. The UI retains its existing own/review surface rather than inventing additional discovery.
- `internal/outcomes/receipts.go`: original command receipts recheck current actor/resource/source authority, require the original actor and command UUID, then project that retained version. A successful unrelated command or current-version GET cannot confirm the original acknowledgement.
- `internal/outcomes/projects.go`, `publication.go`: positively selected publication DTOs, native current-audience filtering, native server search/cursor, explicit correction/withdrawal and fresh media-owner consent resolution. Revoked media is omitted without granting access through the project. No inferred awards, financial outcome or success.
- `internal/outcomes/plans.go`, `review_model.go`, `reviews.go`, `review_delivery.go`; `internal/automation/outcome_plan.go`, `internal/communications/outcome_plan.go`: exact reviewed reminder composition, current source/owner/provider/privacy proof, explicit approved timing and native worker/inbox rechecks. Saved drafts, queued effects, provider acceptance and human receipt stay distinct.
- `internal/database/queries/outcomes.sql` and existing migration `00043_outcome_follow_through.sql`: current-read and publication boundaries, distinct ordered UUID cursors and event filters. No schema changes are planned for the redesign.

## Existing verification inventory

`web/e2e/outcomes.spec.ts` covers ordinary-member proposal/read without event authority, exact receipt and safe retry, field rejection, archived owner evidence with zero versus unknown, current denial and transient read failure, held preflight navigation, lost closure acknowledgement, native released-project search/paging/revocation, inbox review destination and source-team denial. `web/e2e/outcomes-admin.spec.ts` covers positive-field publication/withdrawal, private-source exclusion and explicit timezone-aware reminder approval/cancellation. Both portal configurations apply where native routes/capabilities allow.

Native evidence resides in `internal/httpapi/outcomes_test.go`, `internal/outcomes/{service,publication,reviews,review_delivery,model,schema}_test.go`, `internal/automation/outcome_review_verification_test.go` and `internal/app/outcomes_runtime_test.go`. Supporting media, privacy, inbox and communications contracts remain their existing owners.

## First real consumer and proof

The private ideas directory now uses production CollectionHeading/CollectionRecords/CollectionPagination, native links with explicit icon/text Open actions, truthful loaded count and native status/source/owner/version columns. Its read owner is keyed by actor, draft session, selected event, mount and explicit view generation; a fresh mount/view performs its own current native GET before showing private records. Definite denial latches above child presentation and clears the owned list/team context. Private directory output is excluded from print. Proposal and nested detail/review/publication forms are deliberately still pending migration.

Current terminal local proof: ESLint and both production builds pass. Focused standard admin desktop/mobile checks pass 2/2; focused standard public desktop/mobile checks pass 2/2. These use actual production components and synthetic native API fixtures, assert native keyboard navigation, inert source prose, private print hiding and no write or event-authority escalation. They do not prove the whole outcome surface or native pagination end to end.

## Discovered native integration defect — corrected and verified below

The original `ListOutcomes` forwarded `(actor, eventID, mode, &cursor, limit)` to a service whose signature is `(actor, cursor, mode, eventID, limit)`. This swaps the cursor and selected event. `OutcomeHeadsPage` confirms separate after-ID and event-filter slots. Existing direct service tests use the correct tuple and therefore do not catch this HTTP defect. Own-mode pagination can repeat the first page; selected-event review can authorise/filter the wrong UUID.

The root has authorised only the bounded forwarding fix and actual HTTP+PostgreSQL regressions: multiple distinct own pages, current selected-event review authorisation/filter/cursor and denied wrong-event/role without private disclosure. Establish red HTTP proof before changing the handler; preserve omitted/nil semantics and generated contracts. The bounded fix and actual native red→green proof are recorded below. No other backend changes were introduced.

## Required remaining #242 scope

1. Done: bounded native forwarding correction and real HTTP/PostgreSQL race proof below.
2. Rebuild proposal, current outcome detail and all nested action/evidence/history forms with explicit prepare → review → result → next action, preserving exact receipts and current source/owner rules.
3. Lift safe original unknown/pending metadata above private child/route/version/capability lifetimes; retain immutable private retry payload only while authorised, and prove late response/denial/partial restoration without replay or cached resurrection.
4. Rebuild owner review plans/reminders and positive-field publication, current released-project directory/detail and existing archive projections. Preserve consent/current audience/owner exceptions and explicit publication/correction; no inferred impact/ROI/awards.
5. Complete actual both-portal keyboard/mobile, native field/error, account/event/owner/archive, timing/channel/consent/publication, held read/write and print proofs. Capture matching actual production before/after images. Update the independent production guide if shared behaviour is added.
6. Full appropriate lint/unit/build/browser/native checks, exact-head independent reviews and all repository CI before ready/merge.

#119 owns missing archive/export backend work; #163 owns the additional searchable cross-event showcase; #113 owns result/feedback publication. This redesign does not complete or reopen those separate product deliverables.


## Native HTTP read integration correction

The HTTP list adapter passed the event UUID into the service cursor slot and the
cursor UUID into the optional event slot. A real PostgreSQL HTTP regression
reproduced repeated own pages, forbidden authorised event review, and incorrect
missing-event validation before the fix. The adapter now passes the native
cursor and original optional event pointer in their declared positions. No API,
schema, service policy, publication or provider semantics changed.

`HACKBOX_INTEGRATION=1 go test -race -count=1 -p 2 ./internal/httpapi -run
'^TestOutcomesHTTP(PaginationAndEventReviewScope|OrdinaryMemberPrivateHistoryAndReviewedPublication)$'`
passed both tests, including own/default-mode multi-page no-repeat ordering,
current event review filtering/cursor, unrelated event/role denial without
private payload, omitted-event validation and the existing private publication
journey. Red and green logs: `/private/tmp/fleet242-http-red.log` and
`/private/tmp/fleet242-http-green.log`.


## Second compiling consumer and lifetime checkpoint

Reviewed-project discovery uses the native server search and opaque cursor,
production responsive records and explicit Open links. Source, current released
project and native owner-review routes require a fresh per-mount current GET.
Definite denial clears the owned cache and latches above private children; a
partial restoration requires an explicit current check. All three read views
are excluded from print. Transient current reads retain only the existing
same-page draft while hiding saved private detail and disabling writes.

`OutcomeSafety` owns only whitelisted original command kind, resource/review ID,
command UUID, expected version, native action and pending/unknown state. It sits
above sibling routes and private editor keys, scoped to the authenticated actor
and draft session. Current source reads cannot remove this barrier. A late
original exact response or explicit original-command receipt can settle it;
replacement writes remain blocked meanwhile. No private command body, reason,
evidence, field text, rendered projection or browser persistence is introduced.
Private immutable retry payload stays in its authorised mounted hook. Existing
CommunicationSafety and other owners' private capsules are unchanged.

Terminal proof at this checkpoint: lint and both production builds pass; new
metadata boundary/lifetime unit tests pass 2/2. Standard admin outcome journeys
pass 34 plus the corrected native searchbox/title-target journey 2/2 (the prior
full run had two obsolete ambiguous selectors, not weakened source assertions).
Standard participant outcomes pass 32/32. Both desktop/mobile configurations
cover original pending and unknown writes across source→project→directory→source
navigation, no replacement POST/private payload restoration, exact original
settlement, held current read on remount, definite denial→partial restoration,
print exclusion, existing archived owner/evidence/null-versus-zero/native
publication and explicit reminder contracts. Synthetic fixtures only.

Remaining: full nested proposal/adoption/moderation/owner/evidence/history,
publication and reminder composer presentation with explicit review continuity;
all final capability/owner/source/consent/account/error scenarios; matching
actual desktop/mobile before/after artifacts and shared guide examples where
behaviour is added; final complete checks, independent reviews and exact-head
CI. This checkpoint does not complete #242.


## Full production workflow implementation checkpoint

All present nested owners now use Fleet native field/action controls and flat
sections rather than legacy panel/card composition. Proposals, source revision,
event adoption, deliberate status/measure review, owner/due-date assignment,
follow-through and immutable evidence use a captured prepare → review → confirm
step. Separate publication save/publish/withdraw and owner-plan
save/approve/cancel review the exact retained fields/source/version. The local
review hook clones private payloads, focuses the review heading and returns
correction focus to the actual initiating control. Zero and unknown remain
distinct, all evidence bases/source references remain explicit, publication
fields are separately authored, and existing native archive/owner/reminder rules
remain authoritative.

Each nested private projection owns a mount key and child request scope beneath
the current source scope. A real pre-refactor production browser regression
reproduced cached private owner choices after moderation capability was removed
and partially restored (desktop and mobile both fail at the private text
assertion). The new projection owner requires its own current read and fences
late responses. Supporting event/media lookups now scrub their owning source on
current denial; no unscoped option enumeration is left behind.

Original recovery is displayed once above capability-bound editors. Minimal
metadata includes an opaque live-editor identity, solely to direct recovery to
the actual current private capsule or expose the read-only original-receipt
control after that editor is removed. No original absence/current GET is treated
as confirmation. A rejected retry retains the original unknown barrier; the
private immutable payload stays local while currently authorised. No provider,
backend policy, browser persistence or private global payload is introduced.

The independent guide now imports the actual production OutcomeDecision and
useOutcomeReview controls with a bounded synthetic local owner, copyable usage,
accessibility/focus and anti-pattern guidance. Evidence/proposed-versus-observed,
separate publication drafts, exact owner-plan timing/template and pending/unknown
states are demonstrated without account, API or provider access. Guide feedback
is explicitly local, never a native receipt.

Current compiling proof: latest lint, both production builds, six focused
metadata/review unit tests and the independent guide build pass. The earlier
full prepare/review administrator journey passed 36/36. Expanded current both
portal journeys (including checkpoint editor replacement, rejected retry,
current role restoration and matched screenshot captures) are running at this
checkpoint. Independent guide browser checks and complete final native/web CI,
matching before/after artifact review and paired frozen-source reviews remain
required. The full issue remains open, draft and not ready for merge.


## Per-opening authority and focus correction (after main 6e8ab511)

Private publication, evidence and history readers now mount only while their
own disclosure is open. Their query and local request lifetime end together on
close, so reopening cannot render prior private fields before its own current
GET. Publication existence is only a boolean above that boundary. Proposal
source-team options also have an opening-specific projection scope. Every child
projection cancels/removes its exact key on teardown and ignores late responses.

The actual initiating native button/form submitter remains mounted but hidden
while reviewing; correction returns to that same control. Production and guide
owners pass the actual control explicitly. This fixes the real desktop/mobile
CI focus failure; the original focus assertions remain. The copyable guide
snippet and accessibility reference teach the same ownership requirement.

Latest terminal proof: lint, both production builds and all 148 unit tests pass;
focused independent guide workflow passes 2/2; full standard administrator
outcomes pass 56/56, including private publication/evidence/history close→held
current GET→reopen absence, restored moderation owner choices, unknown recovery
after every write capability disappears, native opener correction focus and the
unchanged 20-second source-team denial assertion. The earlier owner-reader read
count incorrectly forbade an additional harmless current GET; the test retains
actual fresh held-read/private absence and no-write assertions without imposing
an unsupported exact GET count. The mobile local failure showed no second GET
and a 236-second host/trace suspension; the same original assertion passed real
CI and this current standard run. No timeout or retry threshold was raised.

Current participant run is still active at publication of this checkpoint.
Complete guide/browser/state/consent/correction evidence and actual matching
before/after review, paired independent final review and exact-head CI remain.
The issue and draft remain open.
