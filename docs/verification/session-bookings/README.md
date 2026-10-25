# Optional session bookings — issue #166

This is the full backend implementation candidate. Independent acceptance reviews,
upstream integration and every final-head CI check remain merge requirements.
Issue #166 remains optional M5; first-release Today and ordinary agenda use do not
require bookings. Frontend workflows belong to #167.

## Current native slice

`internal/sessionbooking` attaches optional capacity, duration, booking windows,
room/meeting link and facilitator assignments to a stable native agenda item ID.
It takes the event lock before the session lock. Session places, FIFO queues and
attendance use separate tables; they never write competition admission or teams.

The owner implements configuration, permission-filtered session reads, reserve /
cancel with retained exact original acknowledgements, eligible FIFO promotion,
capacity-shortfall reporting and an assigned-facilitator roster/attendance seam.
A facilitator must have current event access and the exact current assignment;
this does not grant event-wide logistics or assessment access. Reused command IDs
with different actor, session or payload are rejected. Current state reads are
not substituted for original command acknowledgements.

## Verification

The initial actual PostgreSQL/race run passed three native cases in **6.972s**:
16 concurrent bookers / three places, eligible FIFO promotion and unchanged
competition claims; exact receipts after later cancellation; capacity reduction
without eviction; current assigned-facilitator authority and removal; current
mentor-role loss preventing waitlist promotion; event-waitlistee denial.

The checkpoint adds an empty migration rollback / re-upgrade and populated
rollback rejection with original acknowledgement preservation. The current four top-level tests (including both rollback subcases) passed with
`-race` in **11.087s**. Bounded owner lint reported zero issues. Final source
provenance and complete issue evidence will be recorded before completion. No provider is used;
PostgreSQL containers are test-owned and synthetic.

## Native/API integration checkpoint

The actual event update/transition and registration withdrawal owners now call the
session reconciliation seam inside their event-locked transaction. Semantic
session changes retain the ID and advance calendar sequence; an unrelated event
rename does not. Removing an agenda item cancels its session without exposing a
meeting link. Invalidated date/window definitions require explicit review.

Discovery and personal agenda projections include warning-only conflicts. Native
HTTP adapters and generated Go/TypeScript schemas expose current sessions,
configuration, own reserve/cancel, original acknowledgements, cancellation and
permission-filtered facilitator roster/attendance. The new SessionRevision header
allows initial revision zero; the existing event revision header remains unchanged.

The coherent integration run passed all **eight native top-level cases** (including
two rollback subcases) with the race detector in **20.701s**. Its real authenticated
HTTP case passed in **6.656s** and validates responses against OpenAPI, CSRF,
initial revision zero, original-ACK replay, private link/configuration suppression,
roster field allowlists, unrelated-reader denial and exact assignment removal.
Native registration Apply/Respond and own-session cancellation race under the
same event lock; withdrawal frees/promotes the session place without readmission.

Canonical SQLC/OpenAPI/TypeScript generation completed. Bounded lint across the
owner, HTTP, event, registration and app packages reported zero issues. The first
HTTP probe rejected initial revision zero because the new path used the stricter
event header; the separate session header corrects that integration defect. The
native reschedule fixture now preserves the full preparation week rather than
relaxing the existing event policy.

## Notification/calendar implementation

The production graph now registers a typed native session approval owner with the
existing communications, inbox, SMTP and outbox adapters. An organiser saves a
new immutable channel-contract version, reviews its exact current source/cohort
and provider revisions, then explicitly approves the fingerprint. Generic campaign
rows retain composition identity only; they acquire no generic approval.

Every occurrence binds the event/session/person, exact own booking revision,
source revision, approved configuration and personal calendar sequence. Queue
reports distinguish retained intents from unavailable recipients; they do not
claim provider acceptance or reading. Current event access/audience is required
for ordinary updates. Withdrawal invalidates the place in the same transaction,
and its active recipient may read only a fixed cancellation with no event name,
session title/time, meeting link, roster or invented browser destination. No
permission is granted to solve cancellation delivery.

Cancelled/archived parent events retain explicit suppressed/unavailable source
records. Their existing no-writes/outbox policy is unchanged: no new job exception,
implied send or automatic later replay. Prepared/pending/ambiguous SMTP effects
block notification configuration replacement until the existing provider-owner
review/disposition permits it. Original operations, attempts and snapshots remain
immutable; current state reads do not settle an original uncertain effect.

Personal calendar UID includes **event, session and person**. Native schema IDs are
event-scoped, so omitting the event caused a real collision regression. Calendar
sequence is retained per booking: a real cancellation advances it, unrelated later
agenda edits cannot change or resend a withdrawn person's cancellation, and a
genuine eligible rebooking advances it again. Minimal cancellations follow the
[RFC 5546 cancellation shape](https://www.rfc-editor.org/rfc/rfc5546#section-3.2.5).
The actual existing MIME adapter preserves event calendars and emits session
`METHOD:CANCEL` without private cancellation payload. Already accepted email
cannot be revoked; the implementation makes no such claim.

Native APIs expose configuration/preview/explicit approval, bounded organiser-only
source/queue history and a minimal exact-own cancellation read. The latter neither
implies an original command acknowledgement nor restores event/detail access.
Reservations capture `session_revision` as well as the own-booking If-Match revision;
stale dates/windows/configuration return 409 before a new reservation is written.
Original exact command replays still return their retained acknowledgement.

## Additional executed focused proof

- Production inbox/SMTP/outbox graph with a deterministic recording transport:
  **four real-PG/race cases, 28.938s PASS**. Queued ordinary updates are suppressed
  after withdrawal; only the minimal higher-sequence cancellation is sent; repeats
  and reconcile cannot replay an ambiguous original; parent no-writes is preserved.
- Genuine repeated-cancellation lifetime failure **RED 3.201s** and cross-event UID
  collision **RED 0.238s**, then **five app cases 28.924s PASS / calendar 1.257s PASS**
  on their bounded owner correction. Cancellation identities survive later edits.
- Genuine readmission/rebooking, existing provider `resolve_no_resend` recovery and
  unchanged original snapshot: app **16.173s PASS**. A receipt-store SQL failure
  cannot become authoritative absence: owner **10.557s PASS**.
- Metadata-only platform administration cannot gain an all-audiences session:
  **RED 4.311s → GREEN 5.975s**, including permission-filtered discovery pagination.
- A captured reservation targeting a changed date was **RED 3.134s**. Native source
  revision protection and the real withdrawal race then passed in **7.935s**;
  authenticated configuration/approval/status/cancellation HTTP passed **10.348s**.
- Expanded owner/communication/inbox/SMTP/mail/app lint was zero before the final
  version-precondition addition. Canonical full generation has completed repeatedly
  as contracts changed; final frozen-head parity/checks remain required.

All datasets and transports are synthetic. These focused proofs are attributed to
implementation checkpoints, not represented as a complete final-head CI campaign.

## Final local candidate proof

The complete native owner suite passed **13 top-level tests** (including two
migration rollback subcases) with PostgreSQL and the race detector in **31.336s**.
This includes 16 concurrent reservations, current-role transaction interleaving,
exact window edges, native DST gaps/folds, date revision conflicts, scoped
facilitator capacity/attendance, current-reader filtering and receipt-store outage.

The affected production graph selection also passed with `-race`: seven app
cases **14.411s**, two authenticated HTTP cases **5.887s**, two existing
communications cases **4.791s**, current-recipient inbox compatibility **3.008s**,
and three calendar/MIME cases **1.223s**. The full-owner run is separate from
this selection; repeated cases are not represented as additional coverage.
Commands and terminal logs are retained under `/private/tmp/hackbox166-` on the
verification host, with `GOMAXPROCS=2 GOFLAGS=-p=1 HACKBOX_INTEGRATION=1`.

`sh scripts/check-generated.sh` completed and explicit generated-source diff
checks were clean. Installed `golangci-lint run` across all changed owners and
adapters completed with **zero issues**. An initial `go tool golangci-lint`
invocation did not run because that tool alias is not installed; the successful
check used the installed repository linter binary.
The only new final-checkpoint changes are meaningful synthetic tests and this
evidence; production source remains the published native/API candidate.

## Remaining completion gates

Obtain independent integration Standards/Spec reviews and pass every exact-head
CI check. Recovery and notification-clock upstream merges are now inherited. Frontend keyboard/mobile
workflows and interactive guide composition remain owned by #167. No merge or
broad release-certification claim is made here.

Migration 49 follows recovery migration 48, inherited from merged PR #269. The
recovery startup gate remains before storage cleanup, runtime/provider graph and
workers. No recovery runtime or migration implementation is duplicated here;
equivalent schema47/DST support commits were dropped during the rebase.

## Acceptance evidence map

| Issue criterion | Native implementation and meaningful proof |
| --- | --- |
| No oversubscription; exact retry; eligible FIFO | Event-first/session lock, independent FIFO queue and immutable command ledger. `TestConcurrentSessionPlacesHaveIndependentFIFOAndOriginalReceipts` races 16 reservations for three places; the native role-loss transaction test holds the same event lock until permission loss commits before promotion. |
| Separate competition admission and teams | Only session-owned places/attendance mutate. `TestNativeWithdrawalAndSessionCancellationSerializeWithoutAdmissionBypass` races actual registration withdrawal and own cancellation; event claims and team membership remain under their existing owners. Event waitlistees cannot book or be promoted. |
| Current eligibility and approved change delivery | Current active identity, agenda audience, event claim, exact facilitator and source revisions are rechecked at native reads/mutations and effect dispatch. Runtime/SMTP tests prove withdrawal suppression, minimal own cancellation, parent-readonly suppression, immutable unknown attempts and explicit existing no-resend review. |
| Native timezone, windows/conflicts, stable calendars | Native DST gap/fold choices, exact open/close edges, warning-only conflicts and captured session revision. Calendar tests retain event/session/person UID, advance personal sequence on real edits/cancellation/rebooking and preserve prior attempt snapshots. |
| Current private meeting/roster scope; no early output scoring | Authenticated OpenAPI-validated HTTP proof checks attendee/private configuration, unrelated readers, current facilitator removal and field allowlists. The session contract exposes no competition submission or scoring command and grants no assessment authority. |
| Capacity reduction with explicit remediation | Existing bookings are retained; organiser projection reports shortfall. `TestCapacityReductionAndCurrentAssignedFacilitatorScope` verifies no eviction plus current exact attendance assignment. |

The additional real-PG rollback proof rejects dropping schema 49 with approved
SMTP deliveries, retains the exact original snapshot and schema version, then
executes the surviving approved update and later cancellation through the native
outbox. This is an immutable-history preservation check, not an assertion that a
current read can settle an uncertain original command.

## Archive-suppression review correction

Independent review of candidate `418ecc6` found that a future session's archive
suppression collided with its earlier approved occurrence: unchanged source
revisions shared a uniqueness key that omitted the source disposition. The real
native registration → preparation → live → assessment → completed → archived
regression reproduced **zero explicit suppression records (RED 3.854s)**.

The pending migration's occurrence identity now includes source state and reason;
the exact occurrence binding includes them too. Archive adds a separate immutable
`parent_event_readonly` record without changing the original approved occurrence,
SMTP snapshot, booking revision or personal calendar sequence. Reconciliation
repeats deduplicate that disposition. Suppressed rows never queue adapters, and
parent writes/jobs policy stays unchanged. The regression checks the actual
organiser history projection, zero new jobs or sends, and the original attempt.

The corrected regression and four related native SMTP/parent-lifecycle cases
passed with PostgreSQL and `-race` in **13.513s** (the archive case itself 1.91s).
Empty and retained-receipt rollback cases passed in **5.139s**. Canonical
`check-generated.sh` completed cleanly, changed-owner lint reported **zero issues**,
and `git diff --check` passed. All are bounded delta proof; prior full-owner proof
remains attributed to candidate `418ecc6`, not rerun or relabelled.

## Recovery-main integration

Rebased the reviewed archive correction onto merged recovery main `c0018bc3`.
The only source conflict was `database.SchemaVersion`: retained recovery 48 plus
session 49. Range-diff preserved the five session commits except base-context
changes; equivalent test-support commits were already upstream. Canonical generated
parity was clean. Native session rollback and archive/unknown-SMTP cases passed on
the rebased source.

The bounded recovery startup test exposed a test-only latest-schema assumption:
empty session49 legitimately rolls down before populated recovery48 rejects
rollback, so checking global49 afterwards was invalid (**RED 2.796s**). The test
now isolates its owned48 rollback phase, explicitly asserts retained48 and
re-upgrades the current schema before continuing. Production hold and rollback
guards are unchanged.

The actual recovery hold/startup/review test passed after that test-only correction
(**GREEN 4.277s**). Rebased session migration proof passed **6.080s** and the two
archive/unknown-SMTP cases passed **6.108s**. This is bounded integration evidence;
prior full-owner and API proof remains attributed to its original source head.


## Shared API composition integration

Rebased onto merged live-rehearsal main `c7971052a2c2d22eece4487c07b4926bff42791a`.
The extracted `newAPIDependencies` now receives the shared runtime session owner
and its event/registration reconciliation callbacks. The session clock is set in
`newCommunicationRuntime` before the API value copy and callback capture.
Production still omits the optional clock. `app.go`, including API/worker recovery
startup gates before storage/provider work, is unchanged from this base.

A new actual PostgreSQL test exercises the composed session reader at its open
and exact closing instants, then a native event update through its captured
reconciliation callback. The bounded real-PG/race app selection, including this
case, existing native authority/shared-clock cases and session lifecycle/SMTP
journeys, passed in **40.908s**. Session rollback passed **6.018s** and the recovery
hold/startup/rollback case passed **3.278s**. These are integration selections,
not a rerun of the complete prior owner or browser campaign.

Canonical `check-generated.sh` completed cleanly. Focused lint across app,
session booking and recovery owners reports **zero issues** after the required
composition formatting correction. Independent integration review and fresh
exact-head CI remain required; no merge is claimed by this rebase.
