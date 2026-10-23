# DS-03 pattern verification

The shared patterns were checked against actual merged primitive main `1e3b9d1`.
The implementation is additive: domain screen migrations, APIs, CSRF/revision and
receipt ownership remain with their existing owners. These results describe the
shared production components and their isolated synthetic owner, not a completed
migration or certification of every product page.

## Fresh local results

- Web lint: PASS.
- Full web unit suite: 103 PASS, including 26 focused pattern/owner-boundary cases.
- Both participant and admin production builds: PASS.
- Focused production-built harness: 52 PASS in 23.4s across admin/participant
  desktop/mobile scenarios.
- Final representative screenshot capture: 4 PASS using complete synthetic
  example views so neighbouring content and controls are not clipped.

The browser harness is built separately and is absent from both portal routes
and build outputs. The main CI browser job runs its focused configuration after
the existing portal suites. No backend/transactional changes or real API/provider
operations are part of this PR. Native API requests are asserted absent in the
synthetic end-to-end journeys. Exact-head repository CI and independent review
remain required before merge; no local full existing portal-browser result is
claimed here.

## Acceptance map

| Contract | Evidence |
|---|---|
| Clear primary action and named row navigation | Controlled heading, quiet toolbar/queue actions, native anchors; desktop table and narrow labelled records. |
| Honest capabilities and counts | Cursor source supports server search only at the type boundary; no implicit search/sort/export/bulk controls. Loaded counts remain distinct from supplied authoritative totals. Search finds a permitted record beyond the initial page. |
| Useful selection and return context | Eligible page IDs, mixed checkbox state, query/cursor/selection reset on changed scope, preserved search/back focus; selected requests are reviewed individually with separate receipts. |
| Narrow detail and review continuity | Inspector becomes the selected canvas; field errors focus their input, correction retains notes, cancellation discards only unsent input, receipt offers the next selected review or return to collection. |
| Unknown outcome integrity | Exact command/scope/entity/expected revision match; latest resource version remains unconfirmed. Read-only status lookup does not resend, and uncertain intent cannot be discarded by a row action. |
| Current authority and privacy | Transient failure preserves mounted drafts while pausing writes. Definite denial removes both panes and old supplied data. Late responses after revocation, event or account replacement cannot refill private views. |
| Stale/offline/failure handling | Offline preflight sends no command, definite rejection retains correction input, and stale versions require an explicit current-source read and a fresh deliberate review. |
| Theme/accessibility | Actual computed text/action contrast in dark/custom near-threshold palettes; scoped portals, native keyboard focus, breakpoint focus retention, forced colours, no narrow overflow and zero reduced-motion transitions. |
| Guide reuse | `PatternExamples withinTheme` consumes the caller's actual Theme/portal scope without nested light defaults. The guide can reuse the pure synthetic scenario helpers without importing the test harness. |

## Failures retained and corrected

The first admin-desktop run passed 9/10 cases. Its remaining reduced-motion
assertion compared equivalent computed CSS strings `0s` and `0ms`; it now checks
that the actual control transition durations are zero.

The first full matrix passed 38/40 and exposed a real mobile feedback defect:
a selected detail paused writes and retained its draft, but the current-authority
failure message was inside the collection pane hidden at narrow widths. The
workspace now presents one visible shared state banner, avoiding duplicate desktop
alerts. A definite-denial guard also removes both panes. Unit coverage and the
complete corrected 44-case browser matrix pass. No authority or domain rule was
relaxed to make the assertions pass. Final visual inspection also corrected the
new queue badge to keep its icon with its label using bounded flex wrapping; full
lint, both builds and the final 44-case matrix passed with that CSS correction.

Independent review then reproduced two additional defects: an unchanged deliberate
search (including an initial empty search or clearing unsubmitted text) entered
loading without changing any read-effect dependency, and narrow records omitted
owner-supported sorting. The synthetic owner now increments its read generation
on every deliberate search/clear. Narrow records expose labelled controls only
for visible declared sortable columns, including active direction and disabled
state, while the owner still supplies rows and performs sorting. The test-only
complete-collection owner exercises this capability without adding sorting to the
cursor example or changing the production controlled API.

The pre-fix browser run reproduced all four stuck-search cases and both missing
mobile sort controls. Its two desktop sort failures were an incorrect fixture
expectation for alphabetical contact order, corrected against the actual names.
The first corrected matrix passed 51/52; the remaining existing palette check
sampled a running CSS background transition. It now awaits the actual transition
completion before measuring the unchanged 4.5:1 steady-palette requirement. The
final complete matrix and all 103 unit cases pass. A local typecheck also caught
an unsupported Testing Library `exact` test option, removed before final builds.

## Synthetic visual comparison

The **before** images retain the former event-grid/card composition with the same
synthetic rows. They are explicitly a test-only composition baseline, not a claim
that a later domain screen has been rebuilt here. **After/review** images use the
actual production patterns. Compared with the accepted [Fleet reference](../../design/fleet-reference/README.md):
flat canvas, restrained typography, scoped toolbar, fine separators, named entity
links, quiet secondary actions and action → review → result continuity are retained.
Cursor examples intentionally show loaded counts rather than inventing prototype
totals. Mobile uses records and a full detail canvas.

| Scenario | Before | Collection after | Review after |
|---|---|---|---|
| Admin desktop | [Image](admin-desktop-before.png) | [Image](admin-desktop-after.png) | [Image](admin-desktop-review.png) |
| Admin mobile | [Image](admin-mobile-before.png) | [Image](admin-mobile-after.png) | [Image](admin-mobile-review.png) |
| Participant desktop | [Image](participant-desktop-before.png) | [Image](participant-desktop-after.png) | [Image](participant-desktop-review.png) |
| Participant mobile | [Image](participant-mobile-before.png) | [Image](participant-mobile-after.png) | [Image](participant-mobile-review.png) |

All people, records, decisions, counters and transports in these images are
synthetic. Critical receipts and failures stay inline; no completion is inferred
from a toast, button click or local queue state.
