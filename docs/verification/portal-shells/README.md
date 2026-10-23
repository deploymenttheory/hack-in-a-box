# Independent Fleet shell verification

The synthetic before build is the final DS-02 production source (`37d17a1`),
byte-identical under `web/src` to actual main `1e3b9d1`. After captures use this
issue's actual production builds. The same real directory/account routes and
same deterministic API fixtures are used in each portal and viewport. Screens
contain synthetic people only; no provider effects or authentication bypass.

| Surface | Before desktop / mobile | After desktop / mobile |
|---|---|---|
| Admin directory | [Desktop](admin-desktop-directory-before.png) / [Mobile](admin-mobile-directory-before.png) | [Desktop](admin-desktop-directory-after.png) / [Mobile](admin-mobile-directory-after.png) |
| Participant directory | [Desktop](participant-desktop-directory-before.png) / [Mobile](participant-mobile-directory-before.png) | [Desktop](participant-desktop-directory-after.png) / [Mobile](participant-mobile-directory-after.png) |
| Admin identity | [Desktop](admin-desktop-account-before.png) / [Mobile](admin-mobile-account-before.png) | [Desktop](admin-desktop-account-after.png) / [Mobile](admin-mobile-account-after.png) |
| Participant identity | [Desktop](participant-desktop-account-before.png) / [Mobile](participant-mobile-account-before.png) | [Desktop](participant-desktop-account-after.png) / [Mobile](participant-mobile-account-after.png) |

## Scope and visual review

Compared with the accepted Fleet workspace references: one compact global bar,
named icons with labels for primary navigation, restrained active underline,
current event/access context, flat comparable event rows, comfortable narrow
record lists, named back navigation and separated account sections replace the
old header/hero/card composition. Mobile secondary search/account tools retain
accessible names while the primary destinations retain visible labels. Fields,
focus and current server error/receipt guidance remain visible.

The [complete route and nested-feature owner inventory](../../design/portal-route-inventory.md)
assigns existing tabs, readers and overlays to their migration tickets. This PR
does not claim completion of those feature presentations or the full #122 audit.
The complete Today/readiness product remains #88.

## Verification

- ESLint clean; 105 unit/component tests; TypeScript and both production builds.
- Focused production browser runs: 66 admin and 60 participant cases, covering
  desktop/mobile:
  permitted navigation/search/no-match, event switching, current access removal,
  direct routes/login return, expiry, loading/empty/retry, native account fields,
  email uncertainty/private clearing, GitHub acknowledgement/revision/callback,
  exact logout CSRF, transport recovery without replay and delayed old-session401.
- Real PostgreSQL/race HTTP and identity tests pass: independent admin MC session
  and role revocation, substituted cookie rejection, email account current session
  and CSRF, GitHub callback/ownership, private response caching, distinct deployment
  cookies, safe return destination and isolated private draft scopes. Full OIDC
  state/PKCE/nonce/audience/revocation uses the existing deterministic local IdP.
- `scripts/smoke-compose.sh` uses a unique synthetic project/images/volumes. It
  proves admin health/readiness/assets/deep link/protected-session boundary with
  participant stopped, then the reverse. Restart persistence and non-root runtime
  also pass; the script removes only its own project.

Earlier browser selectors expecting a card's event-name heading now target the
actual record link, preserving the delayed401/new-session assertion. The new
revocation test dispatches `visibilitychange` on `window`, where the installed
TanStack Query SDK listens. The participant directory navigation assertion is
scoped to Main navigation because the screen also offers the same destination.
No authority, timing or receipt assertion was removed to accommodate the UI.

A new coarse-pointer geometry assertion caught a 42px-wide search control when its
mobile label became visually hidden. The shell now applies the semantic control
minimum to its width as well as its height; the 44px proof is retained in both
portal browser runs. Labels remain accessible and normal desktop labels visible.
