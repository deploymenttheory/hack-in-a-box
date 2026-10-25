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

## Review correction checkpoint

The expanded synthetic administrator suite passed **28/28 in 22.8s**. It now
includes pre-start/read-only roster visibility, roster-only denial scrubbing a
held private attendance review while the general session remains readable,
meeting-link withdrawal, reviewed fresh-base recovery after a lost-before-commit
request, and mismatched notification configuration/approval 200 responses.

The meeting-link test genuinely failed before correction: an old cancellation
review retained one private meeting anchor after current link withdrawal
(**RED 6.1s**). Reviews now capture an explicit minimal facts projection without
meeting URL or organiser definition. Accepted current private link changes renew
the owning scope before held child continuations can publish stale private data.
Safe exact direct booking/attendance acknowledgements remain above private
remounts; current reads still cannot synthesize those acknowledgements.

A native read-projection gap is filled by `can_read_attendance = st.attendance`,
the exact existing roster authority. `can_record_attendance` retains its existing
write/window conditions. No policy, permission or migration changes. Native
pre-start/read-only/removed-assignment proof plus the existing assigned-facilitator
case passed with real PostgreSQL and `-race` **5.510s**. The authenticated,
OpenAPI-validated native HTTP field-allowlist/current-scope test passed **6.249s**,
including explicit read/write projection checks. Canonical generation completed.
An initial fixture attempted to assign a person without event access and was
correctly rejected; the corrected synthetic facilitator holds existing access.

The actual static guide host exposed an inaccessible taller confirmation footer:
its fixed 56rem nested viewport exceeded the outer screen. **Both desktop/mobile
journeys initially timed out.** Bounded viewport height alone made the mobile
journey pass **0.94s**, while desktop still failed because the iframe was only
partially visible when the overlay locked ancestor scrolling (outer 720px; iframe
612px at y372; footer inner y518). The guide-only focus/pointer geometry message
then made the same real pointer/keyboard/print journey pass on desktop **1.1s**
and mobile **0.92s** (**2 passed, 2.8s**). The bridge carries only one fixed kind,
checks exact shape, same origin and the specific iframe source, and scrolls only
the public preview into view. No portal bridge or domain command is introduced.
Additional exact-shape, standalone and deliberate outer-focus checks are pending.

The additive native browser harness compiles and reuses the reviewed production
runtime/router/static surface and private Unix-socket orchestration. Its complete
concurrency/promotion/attendance/withdrawal journey is implemented but not yet
executed at this checkpoint. Final production guide export, broad checks, native
browser proof, screenshots/provenance, reviews and exact-head CI remain open.


The first actual native-browser run executed on clean source `f8eff812` with its
rebuilt production assets. It failed **before the booking journey**, on both
viewport fixtures: the bootstrap administrator's fixture person snapshot had an
older `session_revision`, so native authentication correctly rejected the test
cookie and displayed Sign in. No successful native booking proof is attributed to
that run. Both owned PostgreSQL containers were terminated by their test cleanup.
The private test session issuer now reloads the current person through the native
repository before issuing a cookie. Production authentication is unchanged.
Action waits are bounded to fail a fixture setup error promptly; actual booking,
capacity, current-authority and independent-role assertions are unchanged.

An ancestry-only rebase onto merged backend main `2d82ac40` changed the frontend
head to `3534ce6f` with the identical tree `7a1bdd61372bdad13164b9414ecad4795498aa5a`.
The corrected native fixture requires a fresh clean-head run; prior proof is not
relabelled as that future result.


The corrected-auth native run on clean `22fefac7` reached the actual domain
journey. **Mobile passed** the full organiser configuration/notification approval,
two simultaneous native bookers, exact separate waitlist outcome, cancellation
and promotion, facilitator correction/assignment loss, native withdrawal and
unchanged competition/team-state assertions (**11.0s browser; 33.19s Go subcase**).
**Desktop failed** because the notification-channel review remained disabled after
its preview read (**17.5s browser; 32.73s Go subcase**); aggregate **66.811s FAIL**.
This is not an aggregate native pass. Per-viewport artifact directories now retain
both cases independently, and fixed response-revision/UI-enabled metadata will
help diagnose the observed desktop race without logging private bodies or auth.

Independent review also found a fresh200 roster omission case: the native roster
can remove a withdrawn/ineligible subject while the facilitator retains general
session and roster permissions. Its held attendance review retained the old name
(**RED 6.6s**). Accepted current sorted subject IDs now define the private roster
lifetime; removal synchronously renews the scope and remounts the private owner.
The held-review and held-unknown-write cases passed desktop/mobile **4/4 in14.6s**.
Safe original request metadata survives and exact lookup can settle the original
attendance acknowledgement without restoring the removed subject's name.

The native test cleanup no longer throws from a finally block: it collects all
context cleanup results, preserves the original journey error, then reports a
cleanup-only failure after the finally block. Affected lint explicitly including
`web/session-native` passed after the CI `no-unsafe-finally` finding. The duplicated
guide synthetic supply callback now reuses its existing controlled handler.
