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
Fresh normal browser OIDC execution is still required; neither connectivity nor
component isolation alone is counted as a browser or complete lifecycle pass.

The next native execution at `efc5eb6a` reached the real synthetic provider
heading, but its implicit select label included option text, so an exact
`Synthetic actor` accessible-label locator found no field. A minimal browser
loop reproduced zero exact matches before and one after an explicit for/id
label association. Only the synthetic provider's label markup changes; normal
OIDC actor selection, authorization-code/PKCE/state/nonce/native session checks
remain untouched. No passing OIDC checkpoint is claimed until fresh execution.
