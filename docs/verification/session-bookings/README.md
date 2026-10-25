# Optional session bookings — issue #166

This is a compiling backend implementation candidate; final acceptance and merge
still require the remaining verification and independent reviews below.
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

## Remaining completion gates

Finish the meaningful bounded role-loss/promotion, window-edge, calendar-edit and
retained-approved-delivery rollback checks; verify affected existing message-owner
compatibility and final source/contract parity; publish the final acceptance map;
obtain independent Standards/Spec reviews and every exact-head CI check. Frontend
keyboard/mobile workflows and interactive guide composition remain owned by #167.
No feature-completion or merge claim is made here yet.

Migration 49 is reserved for this owner. Recovery migration 48 will be inherited
from its independently reviewed PR #269 before final integration; no recovery
runtime or migration implementation is duplicated here. Root-reviewed pure
migration-fixture isolation is a separate support commit and will be deduplicated
when its upstream recovery merge becomes the base.
