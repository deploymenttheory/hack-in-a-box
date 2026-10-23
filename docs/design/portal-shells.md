# Independent Fleet portal shells

The participant and administration builds share presentation components, not
runtime availability or authority. `main.tsx` still chooses separately compiled
entry points; the admin build never loads the participant room-display entry.
Each process serves its own compiled assets and keeps its own existing session
cookie. Neither frontend forwards a request through the other deployment.

`PortalFrame` supplies a compact global header, skip link, content boundary and
footer. `PortalNavigation` runs inside the existing private QueryClient lifetime,
keyed by authenticated person and session draft scope. Current event context uses
the same event query key as the feature workspace. A failed current event read
withdraws the event name, access description and contextual destinations. Server
capability facts label access; the portal hostname never stands in for a role.

`Event.can_read_review_coverage` is an optional additive permission fact from the
shared `access.ReviewCoverageAuthority` check used by the existing coverage guard:
current event read plus an active organiser or mentor role. Absent means false.
The organiser-only `can_read_staff` fact does not gate mentor discovery. This is
permission to open the owning service, not proof of service availability or access
to every team's private records; roster/content/configuration checks remain in
that service. Navigation does not fetch or index assessment records. The new
projection changes no role, policy, database schema or grant.

The event switcher loads the existing bounded cursor API only when opened. It
rechecks cached choices and hides them during the current read or on failure;
"More events" fetches the next real cursor page. Each destination still performs
its existing current-resource authorisation when opened. Unknown/direct URLs
continue through the unchanged router and shell allowlist.

Destination search searches only the complete, small list of permitted navigation
destinations assembled from current session/event facts. It does not pretend to
search every paginated event or private entity. Query text is local to its mounted
account/event scope, clears when closed and is never persisted. Search and account
controls keep accessible names in the compact mobile header; primary navigation
keeps visible icons and labels. The active destination uses `aria-current` and a
restrained underline, with system colours in forced-colour mode.

Directories compose the real #232 collection patterns: comparable rows on desktop,
labelled records on narrow screens, explicit loading/empty/failure states and real
cursor pagination. No filter is applied to a partial event page. Account settings
use flat sections for organisation identity, private email verification and GitHub
ownership. Existing native validation, clear-on-hide, callback, acknowledgement,
CSRF, revision and receipt behavior remains in those feature services. Pending
operations disable their controls and show a visible spinner. An uncertain write
never triggers automatic replay.

The shared shell retains the existing session refresh/cancellation, separate
private cache, draft lifecycle broadcast, sign-out and late-response protections.
Sign-in continues using the same safe same-origin `loginURL` return destination.
The shell is not a new auth client or a substitute for backend access checks.

See the [route/nested-component inventory](portal-route-inventory.md) for the
remaining feature migration owners and the [verification evidence](../verification/portal-shells/README.md)
for synthetic desktop/mobile, real HTTP/session and symmetric deployment proof.
The full Today/readiness product remains with #88.
