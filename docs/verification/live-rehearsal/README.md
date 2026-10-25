# Native live rehearsal (#90)

The rehearsal runs the production administrator and regular portals against
isolated real PostgreSQL, native HTTP/SSE, native sessions/current permissions,
and the service graph shared with `RunAPI`. Today #88 is merged in the current
base `c0c8b21`; no placeholder or mocked Today route is used.

## Acceptance matrix

All seven native Chromium journeys passed together at clean source
`1ee06906fcbe6a28b8e29be1ce36d200ca87fcd2`. The PR remains Draft pending final
exact-head CI and independent reviews.

| Issue #90 criterion | Actual verification | Result |
| --- | --- | --- |
| Disconnected displays retain the last authorized cue and report stale status | Native MC pairing/SSE, forwarding outage, retained opening, native advance, latest building cue on reconnect | Combined native Chromium pass; matched three-state images |
| No observations, secrets or embargoed output in display responses/caches | Actual private observation/question/help/unreleased cue fixtures; complete snapshot allowlist; bearer-only GETs; controller denial; no-store; URL/storage/cache scans | Combined native Chromium pass |
| Reconnect/clock drift converge without duplicate actions | True browser offline, incorrect client clock, native snapshot/DB revision delta, held countdown and revocation | Combined native Chromium pass |
| Preparation/build week/live review/submission/freeze/cutoff with lifecycle lag | Native calendar T−7, actual dashboard and frozen POST refusal, mentor review versus operational status, cutoff settlement before lifecycle advances | Combined native Chromium pass across all four phases |
| Today/readiness/rules/contacts/help/corrections across roles/events/accounts | Mobile keyboard/version acknowledgement/focus return; real personal PUT versus unchanged formal output; admin Today→MC; Today→help→native common correction/account-event denial; mentor recovery/role loss; actual logout and another account | Combined native Chromium pass on both production portals |
| Completed Fleet consumers/guide and action continuity | Production reader/navigation/confirmation consumers and unchanged independently published guide | Native pass plus [full guide CI at15e9f6c](https://github.com/deploymenttheory/hack-in-a-box/actions/runs/40152932109/job/121048067372); final-head guide CI remains required |

## Executed proof and merge gates

- Clean `1ee0690`: **7/7 Chromium, 0 skipped, 0 flaky**,20.5s. Real-PG/race
  browser owner25.28s, owning-resource clock regression2.47s and native
  authority3.11s; package32.438s. Native provenance confirms identical start/end
  SHA and `source_worktree_clean=true`.
- [Proof JSON](proof.json) records exact source/run/selection, actual statuses,
  dimensions and image hashes. [Visual evidence](visual-evidence.md) maps all
  nine individually inspected screenshots to the same native run.
- Actual default Linux rehearsal at `f57fa17` passed Chromium and Firefox:
  [run40151899729](https://github.com/deploymenttheory/hack-in-a-box/actions/runs/40151899729).
  This is attributed older-source engine evidence. The final candidate's fresh
  default seven-Chromium-plus-Firefox run must pass independently.
- Full TypeScript/focused rehearsal ESLint and both post-Today production builds
  passed. Ordinary/guide checks run normally again at the final candidate.
- Parent-provided pure test support `f92443e` preserves exact UTC campaign/common
  publication fixture instants through the autumn fold. It changes no production,
  shared helper or rehearsal/browser runtime. Parent supplied its native proof;
  no unexecuted child verification is claimed.

Remaining gates: final exact-head Linux/ordinary/guide CI and independent
Standards/Spec reviews. Root owns merging and #90 acceptance closure. No feature
or dependency work is left pending; merged Today #88 is exercised here.

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
