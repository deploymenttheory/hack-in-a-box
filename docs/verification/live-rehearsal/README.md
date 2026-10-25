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
| MC pairs display, native SSE, backend outage, last authorized cue, latest reconnect, no duplicate command, revocation | `web/live-rehearsal/display.spec.ts`, Chromium + Firefox | Chromium native production journey passed; Firefox local launch fails before page creation |
| Native cue/timer controls, true browser offline, browser-clock drift and display projection/cache privacy | Integrated browser owner | Chromium passed with actual native private observation and unreleased cue canaries |
| Mentor local draft recovery after current authorization; role/session/account/event loss | Integrated browser owner | Offline deliberate recovery and current mentor-role-loss purge passed; account/event journeys remain |
| Preparation, T−7 release week, fixed assignments, live opening and final cutoff with lifecycle lag | Controlled native service clocks + actual team dashboard | Chromium passed; frozen writes rejected by actual native POST, cutoff settles before stage advances |
| Today readiness, rules, contacts, help links and corrected clarifications across current account/event/role | Final native #88 implementation | Dependency pending; no substitute route |
| Production independent guide and matched screenshots | Existing actual production guide consumers | Remaining |

The constructor extraction preserves the existing `RunAPI` ownership of schema
checks, storage/authentication creation and shutdown. Future entrypoint recovery
gates remain before provider/runtime construction. The browser owner cannot
bypass production request authorization or synthesize successful API responses.

This checkpoint does not complete #90. In particular, final Today acceptance
and merging depend on the completed #88 implementation being merged first.

Initial full TypeScript, focused rehearsal ESLint and both production portal builds passed before the first browser run. Browser outcomes are not yet claimed.

## Executed second checkpoint

The current real-PostgreSQL/race browser owner passed all three Chromium
journeys in 16.9s (Go owner 22.891s): clock/production team dashboard, independent
MC/display interruption, and mentor offline/current-authority recovery.
Current full TypeScript and focused harness ESLint also passed. Both production
portal builds passed before these test-only additions; no runtime web source
has changed.

Fixtures create an approved rubric and challenge through native services,
explicit review/approval and actual common dispatch. T−7 uses
`schedule.ChallengePublication`: the spring-DST scenario releases at
22 March 2027 10:00 UTC for a 29 March 09:00 UTC live start, preserving local
calendar time. It does not replace the calendar rule with 168 elapsed hours.
The native `can_create` flag is actor eligibility; the separate frozen gate is
proved by an actual reviewed/CSRF-bound command rejection, not by reinterpreting
that flag as write permission.

Local Firefox 1555 installed successfully but exits before page creation with
`Could not find profile folder`, including a direct binary probe against a
confirmed existing mode-0700 directory and canonical temporary paths. No Firefox
pass is claimed. The committed Linux workflow runs both engines against the
same real native harness without skips, retries or mocked APIs; its actual
result remains required evidence. Default execution always includes both engines.
`HACKBOX_REHEARSAL_PROJECT=chromium` is an explicit local diagnostic selection.

The independent guide remains covered by its existing production-component
suite; final matching source/evidence and Today/account/event/clarification
acceptance are still outstanding. This is a draft checkpoint, not completion.

The next current-source Chromium run passed four journeys in 17.7s (Go owner
24.255s), adding actual other-event denial, deliberate original-event recovery,
UI sign-out/native logout, and a different account entering the same browser
storage. Each context now receives a fresh native PostgreSQL-backed session over
private IPC; the manifest contains no cookie values. Current full TypeScript and
focused rehearsal ESLint passed before this run. Final Today journeys remain
blocked on the actual #88 merge.

Current support-fixture checkpoint is intentionally not green: the first real
native common-answer publish reached the actual inbox owner resolver and failed
with `AuthorizeNotificationInTransaction: resource not found`. The direct HTTP
service clocks were controlled, but runtime owner callbacks retained value/pointer
copies constructed with real time. The harness must inject the same native clock
before that graph is wired; replacing the resolver or dropping notifications
would weaken the integration and is not an acceptable correction. The new
support browser journey is authored but has not run past this fixture failure.

The captured-runtime clock regression is now proved red→green at the native
seam: `TestLiveRehearsalSupportClockPropagation` passed real PostgreSQL/race
3.22s after constructing all shared clock-aware runtime instances with the same
optional domain clock before callback/value wiring. Native inbox List/Open
resolved the exact common-answer UUID and mentor-only source access remained
denied. Omitted production clocks stay nil and use their existing real-time
fallbacks. The original failed source is f5d04e4; its Linux failure matches the
same resolver chain. The full support browser reproof and new Today fixtures
remain pending; no old-head CI rerun is used as evidence.
