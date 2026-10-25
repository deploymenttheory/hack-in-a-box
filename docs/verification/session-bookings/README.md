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

## Full remaining scope

- Atomic native registration-withdrawal and event-edit/cancellation reconciliation;
  actual concurrent role/withdrawal/promotion races and unchanged frozen-team facts.
- Warning-only overlap projections, discovery and personal agenda; current audience,
  visibility and archive/retention boundaries on all projections.
- Explicit versioned approved notification configuration through current delivery
  owners, with fresh eligibility and original ambiguous-effect handling.
- Session-specific stable calendar identity, monotone sequence and cancellation
  rendering, preserving existing event calendar behavior and retained attempts.
- Versioned OpenAPI / HTTP adapters and generated Go/TypeScript contracts; native
  authorization, no-store, receipt and field allowlist tests for #167 consumers.
- Meaningful DST/window/date-edit, HTTP, migration, concurrency and delivery proof,
  final two-axis review and exact-head CI. No completion or merge claim yet.

Migration 49 is reserved for this owner. Recovery migration 48 will be inherited
from its independently reviewed PR #269 before final integration; no recovery
runtime or migration implementation is duplicated here.
