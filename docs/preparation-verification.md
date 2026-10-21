# Event setup and registration delivery

Approved PR sequence: branding (#43, #44); template workspace (#42); setup isolation (#46); forms (#49, #50); capacity (#51); registration journeys (#54); offers/status/withdrawal (#55, #57); applicant review (#56); teams (#59, #63); registration concurrency verification (#58).

## Branding (#43, #44)

The event-scoped branding API exposes only colours and raster images, not the
configuration snapshot's private forms, briefs or message drafts. Organisers save
under the event lock and expected revision; the change preserves other definitions
and records an audit event. Existing templates clone branding by value.

PNG/JPEG content is fully decoded after bounded dimension/type checks (512 KiB,
4096 pixels per side, one image per kind). SVG, scripts and arbitrary CSS are not
accepted. Text/background and primary/background require contrast of at least
4.5:1. Buttons derive a readable black/white text colour. Accent is decorative.

The workspace previews changes before save, protects unsaved edits, scopes theme
variables and portals to the current event, and clears its favicon on navigation.
`brandStyle` and `brandAsset` provide bounded rendering inputs for the later email
and room-display adapters; this PR does not implement those delivery features.

Verification: branding validation and real PostgreSQL isolation/revision tests;
OpenAPI HTTP request/response checks; desktop/mobile preview, save, event switching,
invalid palette and unsafe-image selection checks. Existing browser journeys are
included in the regression run.

## Template workspace and starters (#42)

The template workspace edits reusable event defaults, agenda offsets, forms,
tasks/dependencies, deliverables, role briefs, rubrics, message drafts and branding.
It previews changes against saved/published definitions. Saving creates a new
immutable draft; publication is separate and does not update existing events.
Cloning uses a chosen event-local live start and explicit DST resolution, and
requires acknowledging the lasting organiser competition restriction.

Two generic starters describe all-team judging and mentor qualification followed
by a fresh final. Both contain role briefs, owned tasks and equal-weight five-part
rubrics, and preserve the minimum-confirmed, admission, freeze, challenge and
submission order. They are reusable process definitions, not an implementation of
judging execution (#107 onward). Starter access requires event-creation authority.

Verification: both starters pass full domain validation; spring/autumn DST clones
preserve gate order. HTTP tests validate starter payloads and deny unauthorised
access. Desktop/mobile browser tests create, edit, compare, publish and clone a
template, preserving the earlier version and an existing event.

## Setup isolation (#46)

Concurrent retries of one clone create exactly one event/audit creation. Competing
branding and event-configuration saves at the same revision produce one success
and one conflict, preserve reusable definitions, and leave the source template
unchanged. Browser conflict checks retain the user's unsaved branding values.

The combined PostgreSQL race run includes the existing lifecycle suite: confirmed
attendance, explicit preparation, publication readiness, cutoff/outcome gates,
reasoned postponement and invalidation of obsolete schedule approvals. Those tests
exercise domain gate contracts; registration/team/publication execution receives
its own integrated coverage as those modules arrive. All 34 desktop/mobile browser
scenarios passed. Conflict screenshots were visually checked at both widths.

## Registration forms (#49, #50)

Migration 11 adds event-scoped form headers, immutable definition versions,
publication history and versioned answers. Cloned form definitions remain drafts
until deliberately saved/published. Form edits update reusable definitions by value
without replacing unrelated event configuration. Publication explicitly confirms
that existing responses retain their original version and new requirements apply
to new responses; correction validates against the originally answered version.

The shared `internal/forms` rules validate bounded text, choices, checkboxes and
1–10 ratings, required answers and earlier-question conditions. Hidden/unknown
answers are discarded before persistence and cannot activate hidden descendants.
The organiser editor supports sections, ordering, conditional controls and branch
previews through the same server validator. Preview answers are never persisted.

Stored answer histories follow the existing event retention policy; worker
maintenance deletes expired histories after completion/cancellation/archive.
Definition versions remain reusable configuration. No answers enter audit metadata.
Registration submission and self-service correction endpoints are delivered in the
subsequent registration/withdrawal PRs.

Verification: full Go suite with PostgreSQL and race detection; form HTTP request
and response schemas; typed-answer, hidden-answer, unreachable-branch, original
version correction, stale revision and retention checks. Frontend lint/build,
five component tests and all 36 desktop/mobile browser scenarios pass.

## Capacity, FIFO and confirmed attendance (#51)

Migration 12 installs registrations, server-assigned FIFO sequences and immutable
registration decisions. Applications lock the event with role grants and lifecycle
changes, validate the current published form, check lasting eligibility, allocate
one place and persist answers in one transaction. The stable application ID and
canonical request fingerprint make retries return the original outcome, including
after registration closes. A different request cannot reuse an active application.

Facilitator applications remain received for review and consume no competitor
places. Team preferences never reserve additional seats. New arrivals stay behind
an existing waitlist even if capacity becomes available. Explicit organiser
promotions require a recorded reason, available unreserved capacity and an open
admission window. Capacity reductions below occupied/reserved places are blocked.

Registration now supplies authoritative attendance facts. Only confirmed claims
count; reaching the floor never advances the stage. Preparation rejects new
applications while retaining existing registrations. Admission/freeze deadlines
also apply server-side. Authenticated discovery exposes an allowlist of event
information and published forms, with only the caller's registration status.

Verification: eight simultaneous applications for one place produce one confirmed
competitor and an ordered waitlist; retries, duplicates, explicit go, closure,
facilitator separation, owner ineligibility, reasoned promotions, capacity reduction
and deadline checks pass with PostgreSQL/race detection. HTTP schemas validate
registration discovery, submission and private status. Event, template and database
regression suites and Go lint pass.

### PR 6 — registration journeys (#54)

Added authenticated event discovery, selectively branded application pages, captain/join/solo preferences and separate facilitator applications. Requests retain their application ID when retried, and status comes from the committed server result. The starter form now leaves team preferences to the dedicated application controls. No private event definitions or rosters are exposed by discovery.

Validation: four desktop/mobile browser checks pass, including submission retry identity, confirmed versus waitlisted versus received states, and closed applications. Registration, HTTP authorization/OpenAPI and template PostgreSQL suites pass with the race detector; frontend lint/build pass.

### PR 7 — offers, status and withdrawals (#55, #57)

Offers reserve capacity and expire using the event timezone, configured working days and holiday exclusions, capped by admission cutoff and team freeze. Organisers explicitly approve automation; each reconciliation rechecks that approver's authority. Event locks serialize offers, acceptance, expiry and cancellation. Acceptance/decline retries return authoritative outcomes; obsolete offer notices are cancelled and notification intents are unique. Answer corrections retain their original form version. Own-status screens explain deadlines and withdrawal impact; selected staff relinquish their corresponding role, preserving lasting exclusions and last-organiser protection.

Notifications are durable outbox intents for the future communications adapter (#82); this slice does not claim email delivery. Team roster effects attach to the same withdrawal transaction in PR 9.

### PR 8 — organiser applicant review (#56)

Added a paged, organiser-only applicant directory, original response and decision history, facilitator selection/backup/decline, reasoned attendee promotion, capacity navigation and explicit FIFO automation controls. Staff selection and grants share a transaction and the existing permission checks. Fallback invitations require the applicant's preference, current competition eligibility and open new applications; retries create one durable intent and never allocate a seat.

Validation: real PostgreSQL selection/withdrawal/role rollback and fallback tests pass, with the race detector. HTTP authorization and schema checks cover private review routes. Desktop and mobile review flows exercise incompatible roles and recorded backup decisions.

### PR 9 — team formation and frozen membership (#59, #63)

Added opaque invitation codes, pending requests that reserve no seats, confirmed-only draft membership and organiser publication. Team names may repeat; identity uses UUIDs. Each team has one entry, retired when disbanded. Captains see their own requests; attendees see only their own teams; organisers can place solo competitors, rebalance, remediate captains, publish and freeze. Reasons and revision checks protect organiser moves. Target five/preferred four-to-six is advisory; publication requires at least three and a valid captain. Freeze requires every confirmed competitor and closes pending joins. Server time and the recorded freeze both prevent subsequent membership changes.

Registration withdrawal removes both membership snapshots transactionally and updates lifecycle facts. Before freeze, undersized teams require organiser rebalancing. After freeze, remaining members stay in place; an existing member can replace a departed captain. Empty teams and entries are retired. Repository transfer and reassignment after freeze are intentionally absent under ADR-0005.

Validation: real PostgreSQL tests exercise concurrent joins, missing seats, duplicate names, private rosters, captain departure, rebalancing, one entry per team, freeze and consistent withdrawal facts. Registration/event/HTTP suites pass with the race detector; Go/frontend lint and build pass. Twelve relevant desktop/mobile checks pass (the registration status assertion is scoped to its region because the team panel loads independently).

### PR 10 — registration and lifecycle concurrency (#58)

Added real PostgreSQL races for registration versus the explicit go decision, offer acceptance versus withdrawal and worker replay, organiser grants versus acceptance, and team freeze versus a membership move. Calendar coverage checks a Friday offer across a weekend and excluded Monday, then simultaneous acceptance/expiry at final admission with no later backfill. Tests prove offers do not satisfy minimum attendance, FIFO admission continues during preparation, and frozen teams may continue below three after withdrawals. Browser journeys now cover application → waitlist → offer → acceptance → response correction → withdrawal, plus facilitator fallback preferences without competitor enrollment. Removed the obsolete empty team placeholder package after the implemented team owner was verified.

Final validation:

- `HACKBOX_INTEGRATION=1 go test -race -timeout 5m ./cmd/... ./internal/...` — passed.
- `golangci-lint run ./cmd/... ./internal/...` — zero issues.
- Frontend build/lint — passed; five component tests and 54 desktop/mobile browser tests passed.
- Desktop/mobile frozen-team screenshots inspected: readable controls, wrapping navigation and no horizontal overflow. Screenshots use synthetic browser fixtures; backend transactions and authorization are exercised separately against real PostgreSQL.
- Generated contracts are checked for reproducibility after each PR commit.

External delivery remains a future communications adapter: offer and fallback notifications are durable intents, not claimed deliveries. Challenge publication, submission/judging, automatic team suggestions and mentor allocation retain their separate backlog owners.
