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
