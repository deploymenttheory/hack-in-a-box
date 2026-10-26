# Release capacity profile (#123)

This deliverable is **in progress**. No burst, saturation or 30-minute capacity
run has been executed. The measurement components below are implementation,
not a passing load report. The complete event lifecycle (#120), final integrated
source, native workload adapter and dedicated baseline execution remain gates.

## Fixed acceptance envelope

The release profile is five concurrent events, each with capacity 200, with at
least 1,000 distinct authenticated people making successful native requests in
**every minute** of the 1,800-second sustained stage. Staff/mentor sessions and
bearer-authorised displays are additional explicitly counted traffic, never a
replacement for attendee activity. A thousand stored people, initial sign-ins
or duplicated jars for one person do not satisfy the target.

The baseline uses Docker Compose, external verified TLS termination and durable
PostgreSQL/file volumes. API/admin/worker execute the normal production image
with the default wall clock. Deterministic synthetic OIDC, SMTP and permitted
provider fixtures remain separately identified external boundaries. The driver
records placement, OS/CPU/RAM, Docker CPU/RAM, exact image IDs, source/tree,
schema, per-service caps, data counts, seed, mix, duration and saturation curve.

The following thresholds are fixed in code, not adjustable report settings:

| Observation | p95 requirement |
| --- | --- |
| Routine complete HTTP read | ≤500 ms |
| Routine complete HTTP write | ≤1 s |
| Accepted current cue to matching authorised SSE receipt | ≤2 s |
| Healthy-provider due work to actual guarded attempt start | ≤30 s |

Unexpected HTTP errors must remain **below 1%**. Every status and outcome remains
counted. Deliberate authority denials, capacity rejections and claim conflicts
require their domain oracle; 5xx and transport failures cannot be classified as
expected. No 409/429 blanket ignore, success-only latency distribution, average
substitute, timer increase or silent target revision is permitted.

Routine HTTP timings include the complete response body and TLS/network/server
time. They exclude provider-facing operations by operation ownership, never by
subtracting an estimated provider delay. Provider timings, due starts, attempts,
delivery receipts and ambiguous outcomes require separate observations. Provider
acceptance does not establish user delivery. An original lost response remains
uncertain until its actual native receipt/reconciliation supplies evidence;
current state is not that original acknowledgement.

## Source structure and first checkpoint

```text
normal OIDC + application callback → private cookie jar per native person
  → real monotonic sustained scheduler → native operation adapter (in progress)
  → bounded per-operation distributions + per-minute activity
  → original accepted-effect ledger
independent native PostgreSQL/read-only oracles + real SSE/provider receipts
  → reconciliation, privacy/capacity/deduplication checks
scoped Docker resources + queue time series
  → quantitative report → unchanged release gates
```

`internal/loadprofile` currently supplies:

- Normal HTTPS OIDC login/callback/native-session client, independent cookie
  jars, native CSRF/revision requests and no automatic writes/retries. No public
  test authentication, application route or handler override is added.
- Real monotonic stage scheduling. Release configuration rejects fewer than
  1,000 distinct people or less than 1,800 seconds. Smoke is explicitly separate.
  Requests already in progress drain within native client deadlines; their
  uncertain outcomes cannot disappear at the stage boundary.
- Bounded distributions with conservative millisecond p50/p95/p99 and visible
  slow-tail overflow. Failed/expected requests retain their measured durations.
- Per-minute authenticated cohort activity and immutable accepted-effect
  reconciliation, exposing missing, changed and duplicated effects.
- Scoped read-only Docker resource collection and an aggregate report gate
  requiring all named domain oracles, samples and baseline metadata.

Component tests use synthetic inputs solely to verify these components. They
are not native API, provider, authentication or load evidence. No report is
written from unit-test fixtures. Run the lightweight checks with:

```sh
GOMAXPROCS=2 go test -p 1 ./internal/loadprofile -count=1
```

## Complete workload and oracle implementation still required

| Runtime workload | Required independent assertions/evidence |
| --- | --- |
| Five-event concurrent burst admission, including over-capacity applicants | Native registration/form/FIFO/version facts, exactly 200 confirmed per event, no excess seat or duplicate person/event registration, accepted original responses reconciled |
| 1,000 continuously active attendee sessions plus current staff | Normal OIDC/session issuance, distinct native people, every actor/minute activity, operation mix and achieved request rates, auth/session boundaries preserved |
| Concurrent mentor observations/corrections | Current published coverage, expected native revisions, private author/source separation, every accepted version retained in native history |
| Actual SSE displays while approved cue changes are accepted | Current authorised streams, accepted revision/time correlated to matching received revision, reconnect/revocation and private-canary checks; HTTP enqueue is not receipt |
| Scheduled healthy-provider deliveries under actual worker pressure | Native River/guarded operation attempts, due→attempt-start distribution, approved source/current audience, stable operation keys, provider outcomes and retained uncertainty |
| Concurrent help claims | One successful owner per claim version, expected losing conflicts, no duplicate accepted transition, retained native audit |
| Permission-filtered clarification and cross-event showcase searches | Current source/audience before page limits; canaries absent from results/metadata/bodies, guessed target and revoked-reader probes remain denied |
| Due approved follow-up reminders | Native approved owner-plan scheduling and actual worker dispatch, current permission/retention, native attempt/result/operation and provider deduplication census |

Preparation must reuse the coherent native lifecycle (#120), parameterising its
cohort and event count. Its historical domain clock is useful for authorised
setup phases only; it cannot measure sustained wall time or replace normal
production worker timing. Any setup clock/fixture process must be stopped before
the normal-image runtime stage, and the transition/data/source must be recorded.
No direct insertion of finished competition facts, sessions or delivered jobs
may be represented as native workload execution.

The complete runner must preserve separate setup, warm-up, burst, saturation and
sustained receipts. Initial saturation points will use documented fixed cohort
and decreasing think time, recording achieved traffic and resource/queue
pressure. The chosen baseline must then execute the full real 30-minute stage
in an exclusive heavy-work window. A diagnostic stop or failed threshold remains
failed/non-release evidence; repair the cause or obtain an explicit owner target
revision before release sign-off.

The public report must omit cookies, CSRF/bearer tokens, provider codes,
credentials and private domain prose. Raw authenticated traces/private
reconciliation rows stay private; publish bounded counts, hashes, distributions,
source/configuration provenance, commands, failures and limitations instead.

## Measured worker-cadence correction (component evidence only)

A real normal-wall-clock worker probe exposed a release-target defect before
full profile execution. At `857ece74`, 31 newly approved durable intents created
**after** the actual worker startup barrier reached guarded attempt start with
p95 **55.701732s**, exceeding the fixed 30s target. The separate post-start native
`Enqueue`/`InsertTx` positive control succeeded. The old minute reconciliation
cadence delayed intent-only producers; this was measured from native
`operations.due_at` to `delivery_attempts.started_at`, not provider completion.

The minimal correction at local `e38a642d` gives durable intent reconciliation
its own 10s ticker. Existing startup reconciliation, 5s publication maintenance,
60s provider/retention maintenance, current policy, approval/cancellation,
leases, recovery, retry/snooze and River deduplication remain in force. The same
31-sample probe passed **15.714s**, p95 **5.710714s**, maximum **5.711054s**.
All eight affected native jobs owners then passed with `-race` in **42.433s**;
the repeated due-start probe measured p95 **5.572473s**. Scoped jobs/load-profile
lint passed 0 issues.

The [bounded receipt](evidence/worker-cadence.json) records exact sources, all
native parent results and private-log hashes. Commands:

```sh
# Exact RED at 857ece74; same command GREEN at e38a642d.
GOMAXPROCS=2 HACKBOX_INTEGRATION=1 go test -p 1 ./internal/jobs \
  -run '^TestApprovedIntentCreatedAfterWorkerStartupStartsWithinThirtySeconds$' \
  -count=1 -parallel=1 -timeout=2m -v
# Complete affected native owner regression at e38a642d.
GOMAXPROCS=2 HACKBOX_INTEGRATION=1 go test -p 1 -race ./internal/jobs \
  -count=1 -parallel=1 -timeout=5m -v
```

This uses real PostgreSQL, the normal `jobs.Service.Run`, actual River and
native guarded attempt timestamps. Only the external handler is an immediate
healthy deterministic fixture. Event/intent setup uses the native database
query shape used by repository producers; it is not a full owner-API journey,
production-image baseline or real SMTP recipient observation. No test manually
calls reconciliation, execution or delivery, advances time or treats a later
current read as an original command acknowledgement. The release-load result
remains **not executed**: this receipt sets `full_load=false` and
`release_passed=false`. The full baseline still needs measured burst/saturation,
all workload/oracles and actual 1,800s/1,000-person sustained execution.
