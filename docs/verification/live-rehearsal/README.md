# Native live rehearsal (#90)

This is an integrated verification owner, not another API-mocked browser suite.
It uses an isolated real PostgreSQL database, the API dependency constructor
shared with `RunAPI`, native session/CSRF/current-role policy, actual routers and
the separately built administrator and regular portal assets.

## Run

From `web`, install the locked dependencies and Chromium/Firefox using the
project's normal tooling, then run `npm run build`. From the repository root:

```sh
HACKBOX_INTEGRATION=1 GOMAXPROCS=2 GOFLAGS=-p=1 \
  go test -race -p 1 -parallel 1 ./internal/app \
  -run '^TestLiveRehearsalNativeAuthority$' -count=1

HACKBOX_INTEGRATION=1 HACKBOX_LIVE_REHEARSAL=1 GOMAXPROCS=2 GOFLAGS=-p=1 \
  go test -race -p 1 -parallel 1 ./internal/app \
  -run '^TestLiveRehearsalBrowsers$' -count=1 -timeout=10m
```

The Go owner creates and removes only its own database/container, temporary
files, browser subprocess and listeners. The private control channel is a
mode-0700 temporary directory containing a Unix socket; neither production
portal registers a clock, fixture or outage control route. Session expiry and
transport timers retain real time. Only domain-service clocks are controlled.

Available proxy requests reach the actual native router unchanged. Outages
close existing proxy connections and return 503 for new API requests. Browser
offline interruptions are separate from this backend availability control.
No external provider worker is started and no real identity or user data is used.

## Acceptance and current checkpoint

| Journey | Owner / proof | Current checkpoint |
| --- | --- | --- |
| Native MC/current actor authority, separate portal cookies, no-store, controlled server clock, construction has no provider jobs | `TestLiveRehearsalNativeAuthority` | Real PostgreSQL/race passed, 10.025s, initial constructor source |
| MC pairs display, native SSE, backend outage, last authorized cue, latest reconnect, no duplicate command, revocation | `web/live-rehearsal/display.spec.ts`, Chromium + Firefox | Authored; first integrated run in progress |
| Native cue/timer controls, true browser offline, browser-clock drift and display projection/cache privacy | Integrated browser owner | Remaining |
| Mentor local draft recovery after current authorization; role/session/account/event loss | Integrated browser owner | Remaining |
| Preparation, T−7 release week, fixed assignments, live opening and final cutoff with lifecycle lag | Controlled native service clocks | Remaining |
| Today readiness, rules, contacts, help links and corrected clarifications across current account/event/role | Final native #88 implementation | Dependency pending; no substitute route |
| Production independent guide and matched screenshots | Existing actual production guide consumers | Remaining |

The constructor extraction preserves the existing `RunAPI` ownership of schema
checks, storage/authentication creation and shutdown. Future entrypoint recovery
gates remain before provider/runtime construction. The browser owner cannot
bypass production request authorization or synthesize successful API responses.

This checkpoint does not complete #90. In particular, final Today acceptance
and merging depend on the completed #88 implementation being merged first.

Initial full TypeScript, focused rehearsal ESLint and both production portal builds passed before the first browser run. Browser outcomes are not yet claimed.
