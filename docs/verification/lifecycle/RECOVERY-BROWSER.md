# Same-event live browser recovery

In progress; no recovery pass is claimed yet. The driver uses normal competitor
and mentor OIDC on the same native event, an accepted200 response interrupted
at Chromium Fetch response stage, actual browser offline/online transitions,
explicit original-request recovery and actual mentor claim/resolve controls.
The native runner independently requires exact requester/team/input, one request,
revision3/resolution, correct assignee and other-team denial. The final native
receipt refuses a missing recovery proof when this mode is required.

The first clean source eca15134 run reached the live barrier but failed before
submitting the help request: the driver looked for an alertdialog while the
mentoring UI uses an inline named review section. Original logs/database and
scoped cleanup were retained. It is not a network-loss or recovery pass. Only
the synthetic driver's locators now match those actual inline sections; no
application response, session or UI state is fabricated.

The response interception follows the Chromium Fetch response-stage API:
https://chromedevtools.github.io/devtools-protocol/tot/Fetch/

Use HACKBOX_LIFECYCLE_BROWSER_PHASES=live-recovery with the versioned connected
browser runner. Full role/route/archive browser acceptance remains separate.

The smaller transport probe at84f948d0 found the missing transition's root
cause: isSecureContext=false and crypto.randomUUID undefined on the synthetic
HTTP names. structuredClone was available. The earlier restricted-origin flag
had not established browser trust; earlier branding/OIDC proofs did not exercise
UUID creation, so their bounded actions remain distinct from recovery evidence.

The fixture now uses four exact localhost origins, relying on the browser's
standard local-origin treatment rather than flags or a UUID shim. The guarded
proxy and test-only Go transport map those logical origins to the same isolated
services, preserve native Host/Origin/cookies/CSRF, and still deny all other
destinations. Native OIDC callbacks use those actual configured origins. No
normal application or user deployment changes. The capability checkpoint must
pass on both portals before the full recovery is attempted again.

[Secure Contexts: trustworthy origins](https://www.w3.org/TR/secure-contexts/#is-origin-trustworthy)
