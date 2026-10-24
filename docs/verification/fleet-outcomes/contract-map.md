# Ideas and outcome follow-through — #242 / UI-09

This is the first compiling draft checkpoint from main `093791cb7e2511bedf3c74516b43ef167a73c84b`. Only the private ideas directory has been recomposed. Full nested scope below remains owned by #242; this is not ready for merge or a completion claim.

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

## Discovered native integration defect — authorised supporting fix pending

`ListOutcomes` currently forwards `(actor, eventID, mode, &cursor, limit)` to a service whose signature is `(actor, cursor, mode, eventID, limit)`. This swaps the cursor and selected event. `OutcomeHeadsPage` confirms separate after-ID and event-filter slots. Existing direct service tests use the correct tuple and therefore do not catch this HTTP defect. Own-mode pagination can repeat the first page; selected-event review can authorise/filter the wrong UUID.

The root has authorised only the bounded forwarding fix and actual HTTP+PostgreSQL regressions: multiple distinct own pages, current selected-event review authorisation/filter/cursor and denied wrong-event/role without private disclosure. Establish red HTTP proof before changing the handler; preserve omitted/nil semantics and generated contracts. No backend edits or native passing results are claimed at this checkpoint.

## Required remaining #242 scope

1. Implement and prove the bounded native forwarding correction above.
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
