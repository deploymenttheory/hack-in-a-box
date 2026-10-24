# Fleet MC controls and independent room display

UI-11 / issue #244 replaces the MC's stacked control panels with a flat workspace
for live control, rehearsal, urgent announcements and display management. Every
write captures its action, reason, source revision and affected cues or display
for review before an explicit confirmation. The room canvas remains a separate,
distance-readable public entry.

The baseline is main `bc350d8c3145e8cb1787a840568a8f7b42cf349f`. Full workflow
source `9fc084e5bff3e61b3acd6f02d4334c59275789d0` has the CI results below.
`e1f3981` then replaces the handwritten MC header with the actual production
`CollectionHeading`. Final source `c49abe44a98bcd5c06e5cae4fdc73cddad36c5fb` also
tracks independently edited scalar drafts and wraps legal unbroken next-cue
titles; each bounded correction has separate local verification below.
Final candidate reviews and every exact-head CI check remain required before
readiness or merge. This record does not certify the broader release audit #245
or the integrated live rehearsal #90.

## Production ownership and composition

| Owner | Responsibility |
| --- | --- |
| `mc-console.tsx` | Independent admin MC task, current/next/countdown, named rehearsal selection, live controls, urgent override and explicit return, nested unsent inputs and actual opener focus. |
| `cue-rehearsal.tsx`, `cue-preview.tsx`, `presentation-model.ts` | Shared native fields, UTF-8/HTTPS/countdown bounds, local reorder/remove/edit, read-only cue preview and saved-rehearsal isolation. |
| `presentation-review.tsx` | Pure production review of the captured native intent. Activation from a saved rehearsal explicitly reviews all cues replacing the approved program. Save alone leaves the room unchanged. |
| `presentation-reader.tsx` | Fresh per-mount native reads, polling and source revision, transient read pause, synchronous private child-lifetime expiry and no retained query-cache resurrection after denial. |
| `presentation-operation.tsx` | Current session/console preflight, native CSRF/If-Match, one explicit POST, direct-response handling and safe unknown-command metadata above route/editor lifetimes. |
| `presentation-pairings.tsx` | Current complete pairing projection, contact/expiry/revocation labels, reviewed label/lifetime, once-only local key. Contact never claims rendering or connectivity. |
| `room-display.tsx`, `room-canvas.tsx`, `room-stream.ts` | Dedicated public entry, credential-only transport, allowlisted current/next projection, timer/QR/destination/branding, bounded frames, heartbeat/reconnect and current authorised asset lifetimes. |
| `styleguide/presentation.tsx` | Independent synthetic owners importing the actual `CuePreview`, `PresentationReview` and `RoomCanvas`, with snippets, keyboard/focus, current-source/unknown behavior, intended semantics and antipatterns. No application API, account, credential or private storage. |

The native presentation service, HTTP/generated API contracts, roles, database
schema and providers are unchanged. Existing locked current-role,
writes-allowed and revision checks remain authoritative. Participant/admin
builds, origins, cookies and API processes remain independent.

## Decision and private-state boundaries

Selecting, editing and saving a rehearsal do not publish it. A reviewed activation
can atomically publish the entire saved rehearsal and start its selected cue;
approved-cue jumps operate on the current approved program. Back, advance,
finish, hold, resume and countdown adjustment remain explicit. Reaching zero
never advances a cue. Urgent overrides retain the planned cue/countdown and
require an explicit return to schedule.

Each native write checks the current signed-in session and reads the console
immediately before sending the captured revision. Stale sources pause writes
without overwriting unsent work. Temporary read failures retain nested inputs
with controls paused. Current denial clears private drafts, selected/reviewed
snapshots, display keys and held continuations; a later partial restoration does
not restore the old owner or its data. Correction/cancellation preserve the
mounted editor and return focus to the actual initiating control. Deliberate
route discard can be cancelled; pending commands cannot be replaced.

The presentation endpoints have no original-command UUID/status lookup or
idempotent replay. Minimal action/revision metadata starts before POST in the
existing authenticated safety capsule and survives native SPA departure. No cue
text, reason, display label or key enters that capsule. A direct matching
response can settle its safe metadata even after the private owner ends; the
private response remains fenced. A lost/408 response remains unknown. Reading
current room state cannot establish that earlier outcome. A deliberate fresh
starting point permits a different reviewed action while old safe metadata
continues to say the earlier outcome is unconfirmed. Nothing is automatically
resent and no exact-retry control is invented.

A successful pairing key is shown once in local memory. Print, page hiding,
page departure, task change, current access loss and session end clear it. A
disclosure epoch also prevents an already-held successful response from
repopulating the key after those boundaries, while still recording its direct
acknowledgement safely. A lost pairing response cannot recover its key from the
current list; the operator deliberately reviews a revocation or a new pairing.
Private MC content and the display/key-entry surfaces are excluded from print.

## Independent room projection

The display imports the pure palette rather than the private branding editor. It
loads no MC/account session or private projection. Stream and asset reads send
only the display bearer credential, omit cookies and caches, and retain the
native current-authority checks. Runtime parsing keeps only allowlisted complete
snapshots, including equal-sequence withdrawals. Frames remain bounded at
128 KiB; heartbeat/reconnect/backoff never issue room commands.

Current and next remain distinct. Timers use the server clock/offset, stay fixed
when held and stop at zero. Disconnection retains only validated data, labels it
last-known and shows its last server update. Revocation removes the projection.
Current asset manifest changes suppress old images synchronously, cancel held
reads and revoke temporary URLs; a held obsolete response cannot create a new
URL after the accepted withdrawal. The visible canonical HTTPS destination and
real QR encode the same URL. Existing light/dark contrast, QR decoding,
clock-skew, maximum-frame and 720p/1080p checks remain intact.

## Verification and provenance

| Source / process | Actual result |
| --- | --- |
| Exact `9fc084e` CI web-quality | ESLint and all **168 tests in 30 files passed** (19.14s). |
| Exact `9fc084e` independent guide CI | All **52 journeys passed**, including the production MC/display examples, with API/cookie/storage guards. |
| Exact `9fc084e` complete admin browser CI | **1022/1022 passed**. Default inventory includes `mc-console.spec.ts`; the report does not provide a separate exact MC subtotal. |
| Exact `9fc084e` complete public browser CI | **674/674 passed**. Explicit inventory includes `room-display.spec.ts`. |
| `9fc084e` local native artifacts | TypeScript and both public/admin Vite builds completed successfully in `/private/tmp/hackbox265-final-build.log`. Afterward, the queued duplicate guide stage was intentionally consolidated against completed exact-source CI; its overall shell ended143, not a claimed whole-pipeline pass. |
| Header delta `e1f3981` | Targeted ESLint, direct admin Vite build and **16/16** bounded desktop/mobile native journeys passed; `/private/tmp/hackbox265-heading-{lint,build}.log` and `/private/tmp/hackbox265-focused-final-mc.log`. |
| Final scalar-draft correction | Existing keyboard/route-discard case first reproduced actual owner loss, then passed **2/2** desktop/mobile after the three omitted scalar inputs joined the dirty guard. Targeted lint/direct admin build pass; `/private/tmp/hackbox265-scalar-{red,green,lint,build}.log`. |
| Final next-title correction | Existing maximum Go-escaped case retains its original transport assertions and adds a legal 200-byte unbroken next title. Actual horizontal overflow first failed, then passed **2/2** desktop/mobile with one scoped footer wrap rule. Targeted lint/direct public build pass; `/private/tmp/hackbox265-max-next-{red,green}.log` and `/private/tmp/hackbox265-next-wrap-{lint,build}.log`. |
| Held asset regression | Desktop/mobile **2/2 passed** after observing the accepted complete withdrawal before releasing obsolete bytes. The no-URL assertion is retained. |

The 16 bounded native journeys cover invalid-field association/focus, keyboard
review/correction and route-discard cancellation, transient-read preservation,
current-projection/held-preflight fencing, unknown pairing/key non-recovery,
print/session scrubbing and held pairing acknowledgements after print, hiding
and page departure. Existing full-suite assertions also retain saved-vs-live
isolation, exact CSRF/revision bodies, competing consoles, planned urgent return,
approved jumps/finish, no private canary/account requests, independently decoded
QR, current assets, server-clock skew, bounded maximum frames and readable room
metrics. No assertion or timeout was weakened.

A separate local full-unit attempt retained **162 passes / 6 failures** in five
untouched files (five 5s timeouts and an asynchronous evidence-field lookup).
Host load140–149 was measured during that attempt. Completed exact-same-source
CI passed all168 independently; the local failures are not relabelled as
success or assigned a proven cause. No duplicate local full-unit run or timeout
increase was used to hide them.

Matched actual-build before/after screenshots and per-image source hashes are
recorded in [visual evidence](visual-evidence.md) and the
[per-image manifest](visual-manifest.json). They use the same
synthetic MC urgent-announcement source and room current/next projection at
matched desktop/mobile widths, with no write/provider effect and no key shown.
The final MC pair corrects the observed header hierarchy by using the shared
page-heading component. Later scalar-draft tracking and maximum-content wrapping
corrections do not replace the executed image provenance: room after images are
from `9fc084e`, MC after images from `e1f3981`, exactly as the manifest states.
No recapture or pixel-equality result is invented for those later corrections. Room typography remains a distance canvas, not an admin
table. Final exact-head CI and both independent review axes apply to the final
source/evidence commit, not retroactively to these earlier runs.
