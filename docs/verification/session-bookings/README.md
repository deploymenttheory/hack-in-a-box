# Optional session bookings — issue #166

This is an early native implementation checkpoint, not completed acceptance.
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

## Full remaining scope

- Explicit versioned approved notification configuration through current SMTP,
  inbox and outbox owners, with current eligibility and original ambiguous-effect
  handling. A withdrawn person's cancellation must use a minimal own-booking
  projection without meeting links, rosters or renewed event access.
- Session-specific stable calendar identity, monotone sequence and cancellation
  rendering, preserving existing event calendars and original attempt snapshots.
- Parent-event no-writes remains authoritative: cancelled/archived parents retain
  explicit suppressed/unavailable notification reasons; no job-policy exception,
  implied send or later automatic replay.
- Complete notification/calendar/role-loss concurrency and native HTTP evidence,
  rollback guards for retained delivery intents, final two-axis review and every
  exact-head CI check. No completion or merge claim yet.

Migration 49 is reserved for this owner. Recovery migration 48 will be inherited
from its independently reviewed PR #269 before final integration; no recovery
runtime or migration implementation is duplicated here.
