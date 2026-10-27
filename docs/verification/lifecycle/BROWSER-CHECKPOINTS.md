# Connected native Fleet browser work

Implementation in progress; no full browser lifecycle verdict exists yet.
The browser bridge uses a host-only proxy in the explicit verification image.
It forwards only the isolated native public/admin/provider/guide origins, keeps
actual cookies/CSRF/Origin and denies external destinations and TLS tunnels.
It is absent from normal images. Eight component isolation cases passed under
the race detector (1.676s); that is not an OIDC or complete journey pass.

`browser-bridge.mjs` first checks ordinary browser OIDC login on both portals,
actual native session identity/separate CSRF, mobile/desktop rendering, real UI
logout on the public portal with the admin session retained, and independently
published guide access. It fulfils no application route and injects no session.
Host browser secure-context treatment is restricted to synthetic HTTP origins;
production TLS remains separately verified by the normal deployment envelope.

This transport checkpoint is not a substitute for the same-event phase/action,
offline/ambiguous-write recovery and current-role matrix required by #120.

The first host execution at `7ab3a869` failed before OIDC with
`ERR_PROXY_CONNECTION_FAILED`: the proxy was healthy internally, but the
internal-only Docker network could not return published-port host traffic.
Its private original logs/database and cleanup remain retained. A single-variable
topology probe on the same image measured host readiness000 before,200 after
adding a proxy-only edge network; the real native login then returned302.

Only the localhost-bound proxy now joins both the isolated synthetic network
and its scoped edge network. Native APIs, database and provider remain on the
internal-only network, with unchanged cookies/CSRF/Origin and destination guards.
Connectivity and component isolation were not counted as browser acceptance;
the subsequent real browser execution is recorded below.

The next native execution at `efc5eb6a` reached the real synthetic provider
heading, but its implicit select label included option text, so an exact
`Synthetic actor` accessible-label locator found no field. A minimal browser
loop reproduced zero exact matches before and one after an explicit for/id
label association. Only the synthetic provider's label markup changes; normal
OIDC actor selection, authorization-code/PKCE/state/nonce/native session checks
remain untouched. That failed run did not count as a passing OIDC checkpoint.

At `226f9d84`, normal browser sign-in reached the native `Your events` screen,
but the ancillary Playwright request client's session read returned403 through
the guarded proxy. That client is not the browser's own same-origin request
context. The checkpoint now reads its normal session using browser-native fetch,
with actual cookies/metadata/Origin behavior; the proxy and application authority
rules stay unchanged. That failed run did not count as a passing two-portal checkpoint.

## Executed browser transport checkpoint

The versioned `scripts/verify-lifecycle-browser-bridge.sh` passed at clean source
`b0c1d1ab50203a0de89ad6f21e1d0bf0c78670d5`. The exact immutable image IDs and
positive bounded results are in `browser-bridge-proof.json`. It used normal
production style-guide image source `ef676fea`, with an asserted identical `web`
tree. Normal OIDC on both actual portals, same native identity, independent CSRF,
public UI logout retaining admin access, independent guide access and zero page
errors all passed. No application route was fulfilled or session injected.

Desktop admin and mobile public screenshots were visually inspected: both show
the actual Fleet Your events screen and its appropriate next action. Original
screenshots, logs and database dump remain in the private evidence directory.
The fresh fixture's containers, volumes and networks were all removed and the
source remained clean. These receipts prove the browser transport and portal
boundaries only; same-event actions, offline recovery, role checks and the full
#120 acceptance matrix remain open.

## Same-event action rehearsal

The first connected branding run (`cc1f031d`) stopped before the UI: the Unix-
socket PostgreSQL readiness check returned success during its temporary first-
start server. Migration began at 12:58:52.308Z, during that server's shutdown;
the durable TCP server became ready at 12:58:52.411Z. A disposable pinned-image
probe reproduced local readiness before TCP readiness (8 samples). The isolated
fixture now probes TCP on 127.0.0.1, matching the native processes' transport.
All failed-run project resources were removed. No browser action pass is claimed
from that startup failure; a fresh full connected execution is required.
