# Release capacity profile (#123)

This deliverable is **in progress**. No burst, saturation or 30-minute capacity
run has been executed. The measurement components below are implementation,
not a passing load report. The complete event lifecycle (#120), final integrated
source, native workload adapter and dedicated baseline execution remain gates.

## Fixed acceptance envelope

The release profile is five concurrent events with up to 200 people each and
at least 1,000 distinct authenticated people making successful native requests
in **every minute** of the 1,800-second sustained stage. The approved cohort
keeps roles inside that count: 180 confirmed competitors plus 20 current staff
per event, with the actual competitor capacity and distinct staff/people census
reported separately. It never describes 200 competitors plus 20 staff as a
200-person event. Any extra control actors/sessions and display capabilities
are explicitly counted and cannot substitute for the continuously active cohort. A thousand stored people, initial sign-ins
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
| Five-event concurrent burst admission, including over-capacity applicants | Native registration/form/FIFO/version facts, configured competitor capacity and the distinct 200-person competitor/staff cohort per event, no excess seat or duplicate person/event registration, accepted original responses reconciled |
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

The complete runner must preserve separate setup, warm-up, burst, sustained and
saturation receipts. The declared baseline executes the real 30-minute stage
in an exclusive heavy-work window, then freezes its native oracles before the
separate saturation appendix. Pressure points use the full cohort and decreasing
think time, recording achieved traffic and resource/queue pressure. An uncertain
write at overload cannot be silently replayed or retargeted into a later baseline.
All accepted writes, including saturation writes, still require independent
reconciliation; saturation failures and limits remain explicit. A diagnostic stop or failed threshold remains
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

## Native adapter checkpoint (execution still pending)

The current source adds hard-coded native HTTP adapters for actual registration
(including accepted waitlist records), private mentor evidence, help-claim
contention and current session/registration/help/clarification/showcase reads.
Read projections use independent fixture publication/permission allowlists,
strict DTO decoding, nonempty known-match checks and private canaries. Baseline
traffic has no blanket expected-denial mode. Only the contention owners' exact
`revision_conflict` response is classified as expected; registration form drift,
idempotency conflicts, 429 and transport/server failures remain unexpected.

Every sent write intent is retained as identity/version/fingerprint, including
403/409/429/transport-unknown responses. Independently read immutable effects
can establish persistence without rewriting an unknown original acknowledgement
or enabling another write for that actor. A native owner may commit and then
lose current read authority before replying; a 4xx alone cannot prove rollback.
The ordinary accepted-write ledger and complete attempt ledger are both required.
No replay body/private prose enters either public report.

Read-only PostgreSQL adapters observe the actual admission/staff census, queued
operations by kind, guarded attempt start timestamps, immutable registration/
mentor/help effects and cue audit anchors. Raw all-operation due lag remains
visible: intended automation source facts such as `teams.published` can remain
pending without a delivery handler. Runnable healthy due-start measurements must
use explicitly reconciled native dispatch operation IDs, not that raw gauge.

The SSE client uses a verified HTTPS capability header without a person cookie
or URL token. It checks native event/revision and positive projection shape.
The ledger ignores heartbeats/current snapshots without an acknowledged native
publication, preserves receipts arriving before the publisher response, and keeps
missing expected display/revision pairs in the observations and oracle. Native
audit/database timestamps and bounded driver round trips conservatively account
for clock offset. These components have unit/race checks; no SSE/native SQL/API
profile proof is claimed until the actual composed runtime executes them.

Measurements now retain operation, phase and origin separately, with every raw
status/outcome. Burst and sustained baseline limits are evaluated independently;
setup, warmup, saturation and explicit probe distributions remain visible. A fast
portal cannot hide a slow one by averaging. The monotonic phase boundary excludes
request drain time, and interrupted/smoke runs cannot certify the sustained stage.

The five simultaneous event stages are registration, preparation, two live
events and one archived follow-up event. Already active other-event cohorts can
cross-apply to the full registration event to create real waitlist contention
without inventing another thousand people. The full role-specific workload
driver integration, native cue/runtime acceptance, negative current-access
cases, due follow-up/provider receipt oracles, normal-image TLS Compose bridge
and full burst/saturation/1,800-second run remain unfinished. The prerequisite
#120 provider's strict OIDC constructor will be reused with an explicit 1,000-person
map; its historical clock/control runner will not be inherited by the wall-clock
load runtime. No public test-authentication route or seeded application session
is introduced.


The next source checkpoint adds native help creation and resolution alongside
claims: every runtime request is measured and reconciled by its exact retained
command/action/transition. Role-specific journeys alternate current reads with
mentor corrections and deliberate help work; an unresolved original write blocks
all later writes for its actor while ordinary authorised reads may continue.

Cohort setup selects five distinct native events, 180 competitors plus 20 staff
each, and authenticates declared people once through the ordinary OIDC client.
It retains attempted/failed/distinct-person counts; duplicate native identities
cannot satisfy the census. This is setup only, never evidence of sustained
activity. The public receipt omits subjects, person IDs and credentials.

SSE receipts additionally require the exact acknowledged current cue projection,
not merely a matching sequence number. A wrong cue at the right revision remains
an unexpected receipt. Synthetic HTTPS and pure component controls verify these
measurement boundaries; real application/OIDC/SQL/SSE acceptance and the complete
1800-second profile are still unexecuted.


Due-start accounting selects exact expected native owner operation identities,
event/kind/key and original due timestamps, rather than all approved pending
rows. It retains not-yet-due and unstarted work, duplicate attempts and invalid
start order; these cannot pass the independent complete-work oracle. A missing
start contributes its actual overdue duration and an unexpected observation,
not a zero or an omitted sample. Database time supplies the final boundary.
The admission census also records total attendee applications and waitlisted
counts separately from confirmed competitors/current staff, so the cross-event
burst cannot conceal extra waitlist records inside the 200-attendee description.
The scoped SQL-component parent seeds query inputs solely to verify native SQL
compatibility. At clean `3eb99300` it passed with `-race`: parent2.08s, package3.443s,
no skip/failure and scoped resources empty. Its [source-attributed receipt](evidence/native-query-component.json)
keeps this separate from an owner/worker/load journey.


The cue journey now reads current controller authority, sends one native
presentation override against that exact revision, verifies the original cue
response and anchors its public projection to the immutable native audit. A
synthetic decision marker binds the cue hash in the native decision history;
it is not an invented application idempotency key. Independent reconciliation
includes every committed cue candidate, including a lost original response, so
an untracked committed source cannot disappear from the SSE oracle. The real
[native cue-owner component](evidence/native-cue-owner.json) passed at clean
`5a10ee5a` with race detection: parent1.62s/package2.853s, no skip/failure and
scoped resources empty. This proves owner/transaction/projection compatibility,
not HTTP/OIDC/SSE transport or latency/load acceptance.

Two causal driver corrections protect measurement integrity. A real local HTTPS
control reproduced net/http transparently replaying a consumed POST after its
response was lost (two wire writes and apparent success). Clearing write GetBody
preserves the required native Idempotency-Key while leaving the original
response uncertain, with no consumed-body replay. A separate scheduler control
reproduced accepted-write loss when a later independent oracle failed; exact
original ACKs are now retained even when that later check fails. Both controls
passed in the component/race suite; neither is application or load acceptance.


The source now includes a genuine concurrent registration-burst primitive: 200
existing cross-event actors (50 from each other home event) apply against the
frozen published form of the full registration event. One barrier releases
them together; original ACKs, raw failures, action-start spread and achieved
in-flight concurrency remain recorded. Accepted waitlist records are writes,
not capacity errors. The full runner must keep all 1,000 actors making routine
background reads during this burst; no extra fixture identities are introduced.

The saturation primitive uses real monotonic windows, point-local native
subrequest distributions, actual scoped resource/queue sampling and strictly
decreasing think intervals. Release windows require all 1,000 native people
and at least one real minute each. The first measured routine p95/error-rate
boundary stops the curve and remains visible. Smoke cannot stand in for that
curve or the separate 1,800-second sustained baseline. These primitives still
need the coherent native fixture, complete CLI/oracle integration and actual
baseline execution; no capacity evidence is inferred from their configuration.


## Shared external issuer bridge

The load branch carries the canonical `lifecycle120_oidc_test.go` byte for byte
from `0aad4fb9` (SHA256 `552b7f1307b19e6b80b25847ac483740a0c00f0f7468715cf42b33db942a64c4`).
Both bounded reviews approved that seam before integration. Its default52
subjects remain unchanged; the explicit constructor takes only the declared
cohort. The original people-test file also calls an independent GitHub helper,
so it is not copied. Dedicated123-prefixed protocol controls keep the issuer
independent of the GitHub provider and historical lifecycle/domain runner.

The strict bounded manifest declares all known synthetic subjects and exact
HTTPS callback clients; it has no native session seed or clock option. The
external-only fixture process exposes the actual issuer handler behind the
profile TLS proxy. It is compiled into a private test-provider image, not the
normal API/admin/worker images. Its default CI invocation is skipped; no process
execution is claimed from compilation. Bootstrap is one declared staff member
inside the1,000-person cohort, not an additional dormant count proxy.

At clean `3ef1889b`, all four dedicated provider protocol parents passed with
race detection (package1.873s). They check known-set isolation and default52,
ambiguous identity refusal, real RSA signatures, PKCE/client/redirect/nonce,
single use and wall-relative expiry. The [bounded component receipt](evidence/oidc-provider-component.json)
explicitly records zero native application/session/load evidence. Full provider
process/TLS/native-auth and actual profile execution remain required.



## Native driver source checkpoint

`go run ./cmd/loadprofile` consumes owner-only prepared-fixture, exact-loopback
TCP target and scoped-container JSON files. It authenticates each declared
person through normal HTTPS OIDC/state/PKCE/callback/session paths, releases the
registration barrier alongside routine reads from the whole cohort, runs the
real monotonic sustained stage, freezes baseline observations and then records
the pressure curve. Every sent write remains in the same reconciliation ledger
across phases, including unknown original responses and saturation effects.
The read-only PostgreSQL observer is explicitly loopback-bound and is separate
from native API commands. Normal application routes perform all writes.

The transport trusts only the fixture CA, preserves external URL/Host/SNI,
rejects undeclared destinations and uses no ambient proxy or DNS fallback. Its
bounded pool supports the full cohort; the limit must remain in baseline
metadata. A real HTTPS control passed correct-CA/hostname and rejected wrong
CA/hostname, plaintext, undeclared targets, alternate Host and caller-map
retargeting. This is transport evidence, not application authentication proof.

The CLI refuses release duration overrides, reserves a new mode-0600 receipt
before traffic, never overwrites historical output and keeps provider bodies,
capabilities, cookies and database credentials off stdout/stderr. The prepared
manifest is private acknowledged native setup output; validation does not turn
its claims into authority or an oracle. The runner still requires the complete
native preparation/export, Compose integration and independent current-
permission, contention, provider receipt and follow-up oracles. Its workload
receipt explicitly cannot return `release_passed: true` at this checkpoint.
No native driver process or full load profile has run yet.

## Private factory and baseline composition

`go run ./cmd/loadfixture -new-directory /private/tmp/hackbox-load123-new`
creates a new owner-only directory containing the canonical five-event request
plan, explicit external issuer manifest, setup clock and fresh fixture CA/server
certificates. It creates no native application people, sessions or finished
facts. The CA signing key is discarded; the server certificate covers only
`public.load.test`, `admin.load.test` and `issuer.load.test` for 48 real hours.
Existing input directories and historical receipts are never overwritten.

The native admission checkpoint is separately executable with `-admit-plan`,
`-ca`, `-targets` and `-output`. It requires the isolated TLS setup API and the
`HACKBOX_LOAD_OBSERVER_DSN` dedicated read-only observer. It authenticates all
1,000 people, submits 180 actual competitor applications per event and selects
19 additional staff through native application/review. The declared native
owner is the twentieth staff member. For the four other events it transfers
ownership and removes the bootstrap's event organiser grant, preventing a
hidden 201st person. The final read-only census independently requires 180+20,
confirmed capacity/claims and no waitlist, duplicate or excluded competitor.
This admission phase is source only so far; no successful native execution is
claimed from compilation or issuer protocol tests.

The private `compose.yaml` uses normal source-pinned API/admin/worker images,
independent TLS front doors on 8443/8444/8445 and durable PostgreSQL/files volumes.
The separate `setup.compose.yaml` selects only the private preparation API
entrypoints. Normal application processes receive a CA-only volume; a scoped
one-shot initializer copies only declared synthetic inputs into UID 10001
private fixture volumes for the nonroot issuer/proxies. An explicit observer
bootstrap grants its dedicated role SELECT after normal migrations. Its
loopback port is benchmark-only; normal production Compose is unchanged.
Resource limits, private upstream HTTP and synthetic fixture provider costs
must remain explicit in the actual baseline report. Compose syntax validation
uses dummy values only and is not runtime/TLS/native proof.

The setup API may read a private historical domain clock, with no HTTP clock
route or native session/identity seeding. Missing or nonprivate clock control
fails closed. Setup will stop before measured production images/default-wall
workers and fresh 1,000-person authentication. The reviewed shared worker composition and separately reviewed optional due-
eligibility clock are integrated. Production omits that clock; River scheduling,
attempts, transport and leases stay on wall time. Actual native setup
publication/capture is still required; direct maintenance calls are not proof. Positive
archived entries use actual URL formal submissions and native capture/freeze;
URLs never invent repository SHAs. Their native execution/export, project
publication and follow-up preparation remain pending.

A second real HTTPS control found automatic replay of a bodyless command on a
reused connection despite retaining its native Idempotency-Key: two effects and
apparent success. The verified driver now uses a fresh connection for each
unsafe request, preserving header/body identity and the unknown original
response. The causal control passes with one effect. TLS connection cost stays
inside measured write latency, including at the separate front-door proxy; it
is not removed as external-provider time. No domain/provider write is retried
by the driver to improve the measurements.

## TLS streaming proxy component

The [private fixture proxy](tlsproxy/README.md) now has real loopback TLS/SSE and
native-header/acknowledgement controls. Five component parents pass with race
checking (1.395s package). A controlled shared-connection mutation reproduced a
lost bodyless keyed write returning200 after two effects; the restored transport
returns502 after one effect. Native intent headers remain intact and the driver
must retain uncertainty. The allowlisted image builds as UID10001 with a full
source label; independent Standards and Spec reviews found no issues. The
[component receipt](evidence/tls-proxy-component.json) keeps source/log/image
hashes and scope. This is neither the normal app deployment nor a capacity result;
CA mounts, actual issuer/SSE integration and the complete profile remain gates.



## Native clock and workload handover checkpoints

The [source-attributed worker clock receipt](evidence/worker-domain-clock.json)
records a real PostgreSQL/River regression at clean `6c0d551c`: nil/default and
historical-domain subcases passed, parent 38.30s/package-race39.520s, no skips or
failures and exact scoped resources empty. The historical case first retained
zero application attempts/leases while not due, then resumed through real
River scheduling; attempt timestamps and the two-minute lease stayed on wall
time. The entire existing worker Run tail, including 10-second reconciliation,
is byte-identical. The synthetic handler and this component do not prove real
application delivery, retained occurrence queries, setup publication/capture or
full load. The earlier skipped invocation is explicitly non-evidence.

Preparation now requires a fresh native installation and reads global counts,
so a scoped five-event census cannot hide an old sixth event. Each known setup
jar uses one normal CSRF-protected logout; original lost responses remain
unresolved. A final native count must show zero unexpired setup sessions before
fresh runtime authentication. This prevents a hidden second dormant cohort.
The extended SQL census component still needs its actual PostgreSQL rerun;
source/component controls alone are not native session/logout evidence.

All cohort people must complete a successful native background read before the
registration barrier opens. These same read streams continue during the burst
and drain already-started requests when scheduling stops. Neither login counts
nor the first few staggered readers stand in for all 1,000 active background
people. The component's deliberately held reader requires zero registration
wire commands before full readiness; it is coordination evidence only.

The native display server deliberately ends a connection at five minutes. The
driver now records each connection and reconnects only that known clean finite
rotation after the same one-second delay as the production display. Every new
GET checks the same current scoped bearer capability; no cookies, account grant
or stale-frame replay are introduced. Early EOF, revocation, dependency and
invalid/private frame errors stay visible failures. Actual five-minute native
rotation must still be observed in the full sustained run.

After writers drain, final already-published SSE receipts may arrive within the
existing native-client I/O deadline. An event-driven drain retains actual
publish/receive timestamps; the 2-second p95 target is unchanged. Late samples
stay late, partial/missing receipts stay in the denominator, and the actual
1,800-second activity window is not padded by drain time. Synthetic late/partial
controls and normalized rotation-reader controls are not native load evidence.


### Private setup phase control

The private fixture image includes `loadfixture -advance-setup-directory /fixture
-expected-time <RFC3339Nano> -next-time <RFC3339Nano>`. Run this one-shot command
as UID 10001 with **only this run’s** `fixture_private` volume mounted writable,
no network and a read-only root filesystem. The setup API and worker retain
their read-only mount of that volume. Normal production processes have no clock
flag or writable clock volume.

The command requires an existing private directory and private `domain-time`,
an exact expected current instant, and a strictly later next instant. A private
exclusive controller lock rejects competing commands; an atomic same-directory
replacement prevents partial timestamps. A stale command cannot rewind a later
phase. Advance only after the preceding native owner actions and worker receipts
have been retained. Advancing this private file is a **setup request**, never
proof of challenge publication, submission capture, follow-up delivery or any
measured elapsed runtime. Observe those facts through their actual native owners.
Stop the private setup processes before launching the default-wall production
images and authenticating the measured cohort.


The positive project preparation recipe uses a real team member’s native
`team_outcome` proposal, organiser assignment and acceptance, and a separate
publication save/publish review. Its restricted source carries private synthetic
canaries; only deliberately authored organisation-visible problem, solution and
URL fields are published. The current project read is checked independently of
the original publication acknowledgement. This does not create awards, finalist
status, a frozen submission, or proof that the whole factory ran. Actual native
execution of this recipe and the cross-event current-permission probes remain
required before release acceptance.

`admission.compose.yaml` is an additional **setup-checkpoint-only** override:
public/admin each 0.75 CPU and 512MiB, PostgreSQL 0.5 CPU and 512MiB,
issuer 0.25 CPU and 128MiB, and three proxies each 0.125 CPU and 64MiB.
The running admission services total at most 2.625 CPU and 1,856MiB;
the host driver uses GOMAXPROCS=2. Migration and observer initialisation run
before the APIs, at 0.5 CPU/256MiB and 0.25 CPU/128MiB respectively. The
setup worker is not started for this admission-only checkpoint. Keep these
caps and all observed errors in its receipt; this checkpoint cannot substitute
for the independently measured full-profile baseline.

The [native 1,000-person admission receipt](evidence/native-admission-1000.json)
records actual verified external TLS, normal OIDC/state/PKCE/native sessions,
five native events and 180 confirmed competitors plus 20 staff in each event.
The SELECT-only installation observer found exactly 1,000 distinct active people,
five events and zero unexpired sessions after 1,000 original logout acknowledgements.
All preparation calls and raw statuses are retained; no application identity,
registration or session was SQL-seeded. The private API and normal migration
images have separate explicit source attribution.

This establishes the real setup census only (`full_load=false`). Its historical
setup clock, smaller admission caps, and 7.479-second sign-in phase cannot stand
in for a normal production default-wall runtime or the required real 1,800-second
active-cohort profile. The project’s containers and network were removed; four
owned durable volumes intentionally retain the admitted dataset for the next
native factory phases. Teams, challenge release, positive URL archive, runtime
oracles, actual sustained targets and saturation remain open.


The next private factory phase uses the same `-admit-plan`, CA, exact TCP targets
and new output path, with `-sources-from-admission` pointing to the retained
original admission JSON. It hashes that original receipt, rejects foreign or
changed event/subject/person/stage inputs before native source writes, and
requires the current installation to retain exactly five events, 1,000 active
people and no old sessions. All actors authenticate again through normal OIDC.
Native current owner/event guards remain authoritative.

That phase keeps the registration event open. For the other four events it
configures required URL evidence, approves the common rubric/challenge, obtains
the native go decision, allocates 36 real five-person teams, freezes the two
intended live events and archive source, and publishes complete current mentor
coverage. The intended preparation event’s teams remain unfrozen. The early
organiser freeze is an explicit native command, not a fabricated cutoff fact.
This phase still does not publish a challenge or capture a submission: those
require the actual shared worker and the later monotonic historical setup
phases, followed by the normal default-wall runtime handover.


The [native source-preparation receipt](evidence/native-source-preparation.json)
records the next actual checkpoint at driver `c9f8ac74`, linked to the original
admission receipt by its full SHA256. After fresh normal 1,000-person sign-in,
the four selected events passed their native go gate, obtained 36 five-person
teams each, approved the common rubric/challenge and published 72 current mentor
assignments with no gaps. The intended preparation event remains unfrozen;
three later release sources are frozen. The registration event is unchanged.
All 2,831 preparation calls completed (1,831 HTTP200, 1,000 logout204), and the
final global census again found five events, 1,000 active people and zero
unexpired sessions. No challenge was released and no worker ran in this phase.

Compose may start its completed initializer again when bringing up another
service. Initialisation therefore creates `domain-time` only when absent;
restarting preparation cannot rewind an explicitly advanced private phase.
The sole clock controller still requires the exact expected previous instant.
Actual worker release/capture and the production-clock handover remain separate
required checkpoints. Source-preparation containers/network are removed; the
four owned durable volumes retain the acknowledged dataset.

### Actual shared-worker historical setup phase

The private host CLI accepts `-release-from-sources <original-source-receipt>`
with the existing canonical plan/CA/TCP targets, plus the exact `-setup-project`,
immutable `-setup-image sha256:…` and `-setup-source <40-hex>` of the already-built
private fixture. It verifies both the existing volume's Compose labels and the
image's source label. Clock controls run as UID10001 with no network, no caps,
a read-only root, 0.125 CPU/64MiB and only that owned private volume writable.
Every controller has a retained deterministic name/project label; uncertain
original control responses abort the phase without a blind retry.

Start the canonical private setup worker with the setup override/profile first.
Its extra 0.25 CPU/128MiB brings services to 2.875 CPU/1,984MiB; the one-shot
controller makes the transient peak exactly 3 CPU/2GiB. The CLI observes River's
actual unexpired leader lease before advancing. It never calls maintenance,
reconciliation or a delivery handler directly.

The phase advances monotonically through the archive's exact T−7 release,
live start and cutoff, then the two live events' T−7 release/live start. It
requires real native publication, 34 original formal URL acknowledgements,
36 immutable cutoff sources (34 frozen and two explicitly not submitted), and
36 genuine guarded capture starts/completions. URL-only capture must remain
not applicable to repositories, with no invented repository/commit evidence.
A native database wall boundary distinguishes actual attempt starts from the
historical due/source timestamps. Final archive/live states and separately
reviewed positive projects are read through their native owners. Original
admission/source hashes and every original request/revision identifier remain
in the private receipt; source reads never become original write acknowledgements.

New plans derive schedules at PostgreSQL's microsecond timestamp precision,
without relaxing any release/cutoff comparison. The whole phase remains setup,
not due-start/SSE or capacity acceptance. Stop private worker/APIs before normal
production images/default-wall operation and fresh runtime authentication.

A first actual worker run at `bd0058c7` stopped because the observer used a
normal content read as an organiser for a confirmed-competitor audience. The
native worker had already published correctly at T−7; a SELECT-only, READ ONLY
query confirmed the exact source/version/timestamp. This is retained as a failed
reader checkpoint, not successful completion or a product publication failure.
The corrected observer polls the explicit organiser `?preview=true` view, then
requires a normal confirmed member to read the same released version/rubric.

`-known-release-prefix` supports only that one positively identified read-only
prefix: exactly the acknowledged first archive clock control, the original
source/admission hashes, five successful event reads, the one normal-content404,
and 1,000 acknowledged logouts with zero remaining sessions. Unknown control or
request outcomes, domain writes, later phases, changed sources and unrecognised
reads are rejected. The acknowledged clock advance is retained without replay;
current native owner/configuration/version/publication guards still run, and
the next advance uses the existing private clock's exact compare-and-replace.
No ordinary source read is interpreted as an original domain-write receipt.

The [actual worker release/capture receipt](evidence/native-worker-release-capture.json)
records the successful continuation at `d1efa1a2` on main `5f78d51f`, retaining
the explicitly historical private API/worker image at `735b44f8`. Native state
reached registration, preparation, live, live and archived across the five events.
Three challenges released at their exact T−7 instants; a confirmed member read
each common source. The archive retained 34 original formal URL submissions and
two explicit not-submitted entries, 36 native capture operations with exactly one
guarded actual-wall attempt each, and no invented repository/commit evidence.
Seven separately reviewed positive projects span the archive and both live events.
All 1,000 fresh actors logged out with original acknowledgements; the independent
installation census again found five events, 1,000 active people and zero sessions.

Both earlier failed harness checkpoints remain separately source-attributed:
the staff/audience read error, and an over-strict single-revision assumption.
The native common publication transaction bumps the event twice (rubric record
and challenge publication), so continuation accepts exactly that known delta
while rejecting unrelated revisions/configuration/authority changes. The first
acknowledged clock operation was retained without replay. Setup timestamps and
historical due dates are not normal-wall job-start latency evidence.

During subsequent Docker storage recovery, the project PostgreSQL container
was no longer running when the private logical backup was attempted. Its
zero-byte failed output is not backup/restore proof; the stop cause is not
inferred from that response.
The four owned durable volumes and successful host receipts remain preserved.
No full workload, normal production handover, baseline target or release result
is inferred. A fresh native preparation run is required if this fixture's live
window has expired by the time the real 1,800-second baseline is ready.
