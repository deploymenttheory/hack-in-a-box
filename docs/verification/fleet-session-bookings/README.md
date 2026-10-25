# Optional session frontend — implementation checkpoint

Issue #167 remains a complete in-progress deliverable. This branch is stacked on
backend #166/PR #271. It is not release certification or an acceptance closure.
The backend owns current audience/eligibility, FIFO capacity, exact original
acknowledgements, immutable calendar sequence and notification suppression.

## Owned journey

Both independent portals expose Sessions without coupling Today to booking.
The shared production Fleet collection/detail/review presentation supports
session discovery and personal agenda, reservation/cancellation, current capacity,
warning-only conflicts and date/definition changes. Meeting links arrive only
through the current native projection. The scoped facilitator view records and
corrects attendance without requesting logistics or assessment data.

Organiser setup selects an existing native agenda ID, edits the versioned session
definition and assigned facilitators, reviews a capacity reduction without
removing places, and cancels a session. Native notification configuration,
preview, explicit fingerprint approval and bounded source/queue history are
separate from booking acknowledgement, provider acceptance and delivery.

Every booking captures session revision, own booking If-Match and original
request UUID. An unknown outcome has no automatic replay. Recovery validates the
immutable receipt's request/session/person/action/state/resulting revision; a
current GET cannot settle it. Only fixed-label metadata and IDs/revisions live
above private owners in signed-in memory. Private projections/drafts remain local,
never browser storage or history. Existing unrelated command barriers survive.
Current event/session/actor scope fences read and write continuations.

## Executed checkpoint proof

- Before: reviewed backend source `3f2d7eca` contains the native API, but neither
  portal's event-section registry includes Sessions and no frontend owner calls
  the session booking endpoints. This is an absent UI baseline, not a claimed
  legacy visual comparison.
- Initial production type check and both production portal builds passed after
  integrating the generated contracts. Affected frontend lint passed. The shared
  read/Today/communication owner selection passed **7 tests**.
- First finite administrator browser run: **8 passed / 6 failed**. Four failures
  were duplicate collection/detail selectors; two exposed that generic failure
  presentation hid the deliberate receipt-mismatch explanation. Selectors now
  target the actual detail, and the domain error displays only its fixed safe
  message. No transport or receipt policy was relaxed.
- Root's early review identified a genuine recovery bookkeeping issue. A new
  production browser test lets the native-shaped facilitator capability change
  during receipt recovery's fresh list read. The read authority renews, invalidates
  the old payload ticket and left `reading=true`: **RED 1 failed in 5.8s**, with
  Personal agenda still disabled after the valid original acknowledgement.
  Latest-attempt cleanup now clears only non-private busy bookkeeping; every
  payload/receipt continuation keeps its authority ticket check. Mutable
  `can_reserve` is not used as privacy identity. Current capability still gates
  confirmation and native authority validates the actual command.

All fixtures use synthetic people, rooms and `.example.test` destinations.
Read-only locked dependencies are shared from an existing complete installation;
type-build information lives under `/private/tmp`, outside shared dependencies.
Local Chromium tests use one worker and test-owned listeners. An initial sandbox
listener attempt failed with EPERM before execution; the authorized local test
listener run succeeded. No user listener or container is stopped.

The expanded administrator desktop/mobile finite suite then passed **16/16 in
13.1s**, including the recovery authority-renewal regression and continued
agenda/discovery navigation. Final checkpoint types and affected lint passed.
These mocked-browser cases prove frontend state/transport behavior; they do not
replace the remaining native PostgreSQL concurrency/authority verification.

## Remaining full-deliverable gates

Complete the expanded administrator and participant desktop/mobile run; complete
native HTTP/PostgreSQL/browser concurrency/authority integration using the parent
rehearsal harness; finish guide callback controls and verify the actual production
static host has no API/auth dependency; capture honest before/after desktop/mobile
visuals with provenance; run final types, lint, both builds, guide checks and
relevant owner tests. Independent Standards/Spec review and every exact-head CI
check remain required. The draft must not be marked Ready, merged, or close #167
until the root coordinator has completed those gates.
