# Same-event live browser recovery

The connected run passed at clean integration source
`3671d66f47eab4fd229039f867582baaf414fe0e`: native journey **282.15s**,
browser exit0, both events archived and exact retention checks passed.
`connected-recovery-proof.json` records immutable source/image identities and
bounded native receipts. Production Go/UI/runtime inputs match separate
PR #332 head `456f308a`; this draft contains only its dependent test fixtures.

On the first event, a competitor signed in through normal OIDC, reviewed and
submitted a private help request. Chromium intercepted the genuine accepted200
response, failed its delivery and put that browser offline. The UI displayed an
unconfirmed outcome. After reconnecting, the original identity was recovered by
GET; exactly one PUT occurred. A separately signed-in mentor used the actual
claim/resolve review controls. No application API response or session was
fabricated. The driver observed zero page errors.

Go independently verified exact requester/team/input, revision3/resolution,
mentor-a assignment and timestamps, other-team denial, and exactly the original
native help plus this recovered request in the owner queue. Both same-event
branding save/reload phases, queued permanent message failure, judging, feedback,
archive and configured120-day expiry also completed. All scoped containers,
volumes and networks were removed. Original private journal, dump, screenshots
and logs are retained outside this repository.

This is a bounded recovery verdict. `complete_lifecycle` remains false: full
Fleet action-to-next-action journeys, role matrix, remaining provider/live
sandbox prerequisites and final audit reconciliation remain open.

## Failed probes and corrected fixture assumptions

Earlier runs failed before an accepted help response: an alertdialog locator did
not match the actual inline review section; then the synthetic HTTP origins
lacked secure-context UUID support. The small capability probe at84f948d0
recorded isSecureContext=false and crypto.randomUUID undefined. These are failed
attempts, not recovery passes. Earlier branding proofs exercised different,
explicitly bounded actions.

The fixture now uses four exact localhost origins without unsafe-origin flags
or UUID shims. The guarded proxy preserves native Host/Origin/cookies/CSRF and
rejects other destinations. The smaller probe at7a60bd07 passed ordinary OIDC,
independent sessions/logout, guide and actual secure-context/UUID capabilities
on both portals. The first connected attempt with those names failed immediately
at native sign-in because Docker DNS lacked aliases. Exact isolated aliases at
3671d66f fixed that transport boundary; the complete connected run above passed.
Normal production images have no fixture transport, provider or clock override.

Use HACKBOX_LIFECYCLE_BROWSER_PHASES=live-recovery with the versioned connected
browser runner. Host and Docker free space are checked before builds and runs.

The response interception follows the [Chromium Fetch response-stage API](https://chromedevtools.github.io/devtools-protocol/tot/Fetch/).
Local browser trust follows [Secure Contexts: trustworthy origins](https://www.w3.org/TR/secure-contexts/#is-origin-trustworthy).
