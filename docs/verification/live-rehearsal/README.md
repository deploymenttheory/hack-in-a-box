# Native live rehearsal (#90)

The rehearsal runs the production administrator and regular portals against
isolated real PostgreSQL, native HTTP/SSE, native sessions/current permissions,
and the service graph shared with `RunAPI`. Today #88 is merged in the current
base `c0c8b21`; no placeholder or mocked Today route is used.

## Acceptance matrix

| Issue #90 criterion | Actual owners and assertions | Current evidence |
| --- | --- | --- |
| Disconnected displays retain only their last authorized cue and report stale status | Production MC pairing, display SSE, forwarding outage, visible last-known opening, native advance and reconnect to the building cue | Current `15e9f6c` integrated display case passed |
| Display responses/caches exclude mentor observations, secrets and embargoed output | Actual native private observation, private question/help and unreleased cue fixtures; complete snapshot key allowlist; bearer-only display GETs; account-controller denial; no-store; URL/storage/cache scans | Current `15e9f6c` integrated display case passed |
| Late reconnect and clock drift converge without duplicate cues/actions | True browser offline, incorrect client wall clock, controlled native time, current SSE snapshot, native presentation revision delta, held countdown and revocation | Current `15e9f6c` integrated display case passed |
| Preparation/build week, live submission/review opening, fixed roster and final cutoff with lifecycle lag | Native calendar-derived T−7 release; actual team dashboard and frozen command rejection; mentor `can_record` versus operational status; cutoff settles while event remains preparation | Original submission/freeze/cutoff journey passed. Corrected canonical `mentor-observations` exact clock/review case passed (1/1, 1.5s). A fresh combined current-source pass is still required |
| Today/readiness/rules/contacts/help/corrected clarifications across roles/events/accounts | Mobile keyboard rules opening and exact native acknowledgement/focus return; actual personal task PUT versus unchanged formal submission; admin Today→MC; Today→help→common answer→native correction; native mentor-role loss; other-event denial; actual logout and different account | These six browser cases passed together at `15e9f6c`; no complete seven-case combined pass is claimed yet |
| Completed Fleet screens/guide and action-to-next-action continuity | Both built production portals, native MC/display/mentor/competitor contexts; production reader/navigation/confirmation controls; unchanged independent guide consumers and their normal suite | Native consumer evidence above. Current `15e9f6c` full guide CI is pending; no duplicate local guide run is substituted |

## Proof and remaining work

- Current combined local run at `15e9f6c`: six of seven Chromium browser cases
  passed; the added clock review GET used `/observations` instead of the actual
  generated/native `/mentor-observations` route and correctly failed its strict
  200 assertion. The correction preserves every native status/window/freeze
  assertion and its exact tight case passed (1/1, 1.5s). Native authority passed 6.18s and shared-clock owning-resource
  regression passed 4.71s. This is not a completed combined browser pass.
- Linux native rehearsal at `f57fa17` passed with both configured engines:
  [run 40151899729](https://github.com/deploymenttheory/hack-in-a-box/actions/runs/40151899729).
  This proves actual Firefox execution, not a claimed local Firefox run.
  The newer `15e9f6c` Linux workflow is active and remains independently required.
- Current full TypeScript and focused rehearsal ESLint passed. Both production
  portal builds passed after rebasing onto merged Today. Native runtime clock
  defaults and the ordinary guide remain subject to final-head CI.

Finite remaining work: finish the exact clock/review case and a fresh combined
current-source run; inspect and publish its matching synthetic images and
non-secret provenance; complete the current-head Linux/ordinary/guide checks;
obtain independent Standards and Spec reviews. The PR remains Draft / Refs #90.

## Architecture and safety boundaries

```text
RunAPI → newAPIDependencies → newCommunicationRuntime
Go test owner → same constructors + one pre-wiring domain clock
  real PostgreSQL + native routers + PostgreSQL-backed sessions
  production admin / regular assets
  MC + independent display + mentor + competitor browser contexts
```

`RunAPI` retains infrastructure/startup/shutdown ownership. Its omitted optional
clock keeps every service's existing real-time fallback. The test clock is
initialized before shared service instances are copied or captured by callbacks.
Identity expiry and transport timers retain real time.

The test owner controls time/fixture transitions through a Unix socket inside a
mode-0700 temporary directory. Neither production HTTP listener registers a
clock, outage, session-issuance or fixture control route. Synthetic sessions are
issued through native PostgreSQL-backed SCS over that private channel; the
manifest contains no cookies. Available proxy requests and SSE frames reach the
native router unchanged. An outage closes proxy connections and returns 503 for
new API requests. Browser offline mode is a separate interruption.

Fixtures use an actually approved rubric, reviewed/approved challenge and native
common dispatch. `schedule.ChallengePublication` derives T−7 across spring DST:
22 March 2027 10:00 UTC for a 29 March 09:00 UTC live start, preserving local
calendar time. The small fixture explicitly configures its minimum attendance;
production defaults and lifecycle rules are unchanged. Native `can_create` is
actor eligibility; the distinct frozen gate is proved by actual CSRF/If-Match
command refusal. Private drafts never count as formal output.

No provider worker starts and no real identity/provider/user data is used. Only
test-owned database containers, listeners and temporary files are cleaned up.
The native fixture sidecar and Playwright JSON report identify the actual run,
source and selected engines; screenshots exclude display keys. Final artifacts
must be attributed to the source and selection actually executed.

## Run

Install the locked dependencies and pinned Chromium/Firefox with the normal
project tooling, then build both production portals with `npm --prefix web run
build`. From the repository root:

```sh
HACKBOX_INTEGRATION=1 GOMAXPROCS=2 GOFLAGS=-p=1 \
  go test -race -p 1 -parallel 1 ./internal/app \
  -run '^TestLiveRehearsal(NativeAuthority|SupportClockPropagation)$' -count=1

HACKBOX_INTEGRATION=1 HACKBOX_LIVE_REHEARSAL=1 GOMAXPROCS=2 GOFLAGS=-p=1 \
  go test -v -race -p 1 -parallel 1 ./internal/app \
  -run '^TestLiveRehearsal(NativeAuthority|Browsers|SupportClockPropagation)$' \
  -count=1 -timeout=10m
```

Default execution includes Chromium and Firefox. `HACKBOX_REHEARSAL_PROJECT`
and `HACKBOX_REHEARSAL_GREP` are explicit local diagnostic selections and must
not be represented as a full default run. Linux CI runs the default selection
without skips/retries and uploads `web/test-results-live` as actual proof.
Local macOS Firefox exits before page creation on this host; actual Linux
Firefox results are the authoritative engine proof.

## Historical regression evidence

1. **Captured service clocks:** native common-answer publication failed through
   `ActAnswer → Inbox → AuthorizeNotificationInTransaction: resource not found`.
   Direct handler copies used 2027 while captured runtime owners still used real
   2026. Initializing the shared clock before wiring fixed the actual resolver;
   the isolated real-PG/race regression passed 3.22s and current combined native
   regression passed 4.71s. Mentor-only common-source denial remains exact.
2. **Acknowledgement completion:** the mobile harness selected Today after PUT
   headers arrived, before the accepted save callback completed its current Event
   read. The existing planning gate retained Deliverables. Waiting for the
   owner's visible `Preparation progress saved.` result fixed the exact case
   (1/1, 2.0s), followed by a pass in the current combined run. No production
   navigation change, blind retry or timeout relaxation was made.
