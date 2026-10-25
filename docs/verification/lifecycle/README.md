# Complete synthetic event lifecycle (#120)

This is the implementation contract for the new integrated rehearsal. It is
**not a passing verification record**. The existing #90 live rehearsal remains
attributed to its narrower live/Today/MC scope. None of its directly seeded
registration, team, gate or session records will be used as this scenario's
lifecycle setup.

Initial source baseline: `56d00be40e098f9ab71b2387377d2ad7cd81b63c`.
Results/report UI #113, independent verification #114, showcase #163 and the
Fleet cutover #245 remain prerequisite gates, rather than inferred completion
from their draft branches or a closed supporting issue.

The confirmed category-adjudication rule is that mentors provide advisory input
only. All required judges must vote; only judge ballots count toward a strict
majority, and a split remains unresolved. The backend/UI advisory-input
capability is still a prerequisite for that integrated journey and must not be
simulated by counting mentor ballots or inventing a completed native owner.

## Infrastructure checkpoints under construction

`Dockerfile` and `compose.yaml` in this directory build an explicitly named
verification image containing the test-only process entrypoint. They do not
modify the production image targets. The network is internal, PostgreSQL has
no host binding, and provider clock control uses only the owner-private Unix
socket. The provider refuses to reuse an existing control clock, so each
independent rehearsal requires a fresh isolated project/volumes.

The verification relay is configured through the normal SMTP settings and uses
real authenticated TLS, with an ephemeral synthetic CA trusted through the
standard `SSL_CERT_FILE` mechanism. Temporary/permanent recipient rejection
and an unknown DATA acknowledgement can be selected only through the private
socket. Mailbox content (including account-confirmation links) remains in
fixture memory; release artifacts must record bounded assertions and hashes,
never raw tokens or message bodies. Its real queued native effects are pending
execution. GitHub transport redirection is likewise confined to the test
entrypoint and a fixed provider-host allowlist; it does not verify a production
GitHub account, TLS connection, installation or live sandbox.

The `bootstrap` profile runs only normal OIDC, configured admin bootstrap,
draft-event creation, private/unprivileged denials and independent portal
session/logout assertions. Its `bootstrap-only.json` receipt explicitly sets
`complete_lifecycle` to false. The `admission` profile is another partial native
HTTP checkpoint: published forms, facilitator selection, confirmed minimum,
capacity/FIFO, explicit go, withdrawal and offer acceptance during preparation.
It then publishes teams of five, depletes a published team to two through real
withdrawals, rejects its republication, explicitly unlocks/reassigns the pair,
republishes six teams of four-to-six and freezes them. A post-freeze move must
return the exact native rejection without changing the allocation projection.
These profile names cannot be used as the final release command.

The dedicated verification image **built successfully** from source
`2b203526e53ec2c88e272c9890b65f473ca1aa54`:
`hackbox-lifecycle:verification`, image ID
`sha256:cb28085ecab6a2cbd074262857daa2c48c995183e83cde510d5a113c18768907`.
This compiled both portal frontend bundles and the Linux test binary, migrate
and healthcheck commands, with Go limited to `GOMAXPROCS=2` and `-p=2`.
No runtime service, provider, PostgreSQL or browser was started by that build.
The Compose configuration passed daemon-free `config --quiet` validation.
The separate normal style-guide target and production application images were
not built or executed as part of this checkpoint.

The bounded initial Compose bootstrap subsequently **passed** in 0.04 seconds
using that exact image and Compose source `8ee123e6`. Its real PostgreSQL schema
migration and independent public/admin/provider processes became healthy;
normal OIDC callbacks, configured owner bootstrap, two private draft creations,
stranger denials and independent public-only logout passed. See the
[partial receipt](evidence/bootstrap-initial.json). The worker, admission
cohort, browsers and separate production images were not run. SMTP and GitHub
delivery were not exercised. The project had no host ports, each fixture
process was capped at 0.5 CPU/256 MiB, PostgreSQL at 0.75 CPU/384 MiB, and Go at
two processors. Only this project's services, volumes and network were removed
after exporting its safe receipt.

The bounded native admission/allocation checkpoint subsequently **passed** in
0.39 seconds from source `c403ddc56a81e9553ceb03474e12bd7a55ca1086`, image ID
`sha256:363166f25378dcbb727ed181120804206526f24c53b76a223fcac4f3127d42e1`.
The [partial receipt](evidence/admission-teams.json) records 36 actual normal-OIDC
attendee applications: a native minimum rejection at 29, no automatic advance
at 30, explicit go at 35, the first FIFO waitlist position, an outstanding offer
excluded from the count at 34, and acceptance of that exact offer back to 35.
Native team publication, withdrawals leaving a pair, rejected republication,
explicit unlock/reassignment, reviewed republication and freeze left 32 people
in six teams of five/five/five/five/six/six. Exact team/member UUIDs and the
native rejection/invariance assertions are retained. This used a fresh isolated
project with the same caps; no worker, browser or provider delivery ran. The
project's containers, volumes and network were removed and label checks were
empty after cleanup. This remains partial evidence, not L01–L18 completion.

Focused production clock/retention regressions also proved genuine retained
queue deferral, natural resume, wall-time lease/attempt ownership, the unchanged
default exclusive expiry and explicitly configured native 30/60/90 effects.
Their [exact run and failure provenance](evidence/clock-runs.md) distinguishes
component fixtures from the required full HTTP lifecycle and production images.

The next `publication` profile is implemented and compiles, but is **not yet
executed**. It repeats genuine admission/allocation through HTTP, configures the
explicit retained-response policy through the native HTTP owner, approves the
exact rubric/challenge, denies early route/search/rubric access, and requires
the actual worker's publication maintenance at the independent DST/T−7 instant.
It also checks identical attendee rubric bytes and native version-bound
acknowledgements. Its partial receipt remains `complete_lifecycle=false`; a
fresh verification-image rebuild and native run are still required.

The most recent lightweight compile check compiled both checkpoint drivers and passed the
independent DST oracle (`go test ./internal/app -run
'^TestLifecycleCalendarFixtureActuallyCrossesDST$' -count=1`, 0.464 seconds).
No browser, PostgreSQL, native OIDC or queued effect is implied by that result.

## Execution boundary

The full scenario runs in an isolated Docker Compose project with real
PostgreSQL, separate public/admin API processes, the native worker, a synthetic
OIDC/provider fixture and browser/API runner. Its fixture entrypoint uses the
same native configuration, identity, router, domain composition and maintenance
callbacks as the application. The test-only entrypoint is explicitly different
from the production command: production-image startup/isolation smoke remains
a separate required proof and is not inferred from the fixture binary.

All people enter through the normal OIDC authorization-code flow. The fixture
signs RS256 tokens and validates client/redirect binding, PKCE and one-use codes;
the application still validates state, browser binding, nonce, issuer/audience,
creates the real identity, applies the configured bootstrap subject and writes
its own session. Scenario code never inserts people, sessions, registrations,
teams, gate facts, submission receipts, scores or signed decisions into SQL.
Read-only SQL census/hash assertions are allowed, and their queries are part of
the execution record.

Only native domain time is controlled. Authentication expiry, HTTP timeouts,
provider deadlines, River execution and operation leases retain wall time.
Clock/provider controls belong to the fixture binary and an owner-private Unix
socket/internal Compose network; no test route or clock option is added to the
production HTTP/configuration surface. The domain timeline must be monotonic.

The worker's real queued execution is required. Direct handler invocation and a
fulfilled application response cannot stand in for a queued native effect.
Provider faults may be deterministic; the native source, operation, approval,
attempt, receipt and current-audience checks remain real.

## Data and timeline

- Two differently branded events, one all-team judging and one mentor
  qualification/fresh final, share synthetic identities while retaining separate
  grants, team membership, sources, settings, content and results.
- The admission cohort exercises a configurable minimum of 30, capacity and FIFO
  overflow. Staffing and outstanding offers do not count as confirmed attendees.
- Published allocations target five, prefer four to six, and include the actual
  before-freeze under-three withdrawal/reassignment path and after-freeze
  smaller-team continuation/withdrawal path.
- The calendar fixture crosses a Europe/London DST boundary. T−7 means the same
  local time seven calendar days earlier, with a recorded publication instant
  inside the agreed 60-second scheduler window. A fixed 168-hour subtraction is
  not the oracle.
- A missed/blocked common publication requires native postponement before a
  later publication. A separate approved post-publication postponement and
  attendance exception retain their native reason, source and audience records.
- Both events reach cutoff before stage advancement. The final census accounts
  for every team, including genuine unresolved repository capture followed by
  reviewed reconciliation, explicit missing/withdrawn entries and unused
  disbanded repositories.
- Owned 30/60/90-day outcome work retains estimated/self-reported/demonstrated
  distinctions and cannot modify the frozen competition. The execution record
  must disclose its domain/wall-clock anchors and queue timing limits; dates or
  approval windows may not be distorted solely to make a fixture pass.

## Complete acceptance-to-action matrix

Every row is **pending integrated execution** until a source-pinned receipt
records its actions, assertions and result. Unit-test existence and historical
component passes are supporting evidence only.

| ID | Native actions and browser journey | Required assertions / evidence |
|---|---|---|
| L01 Planning and identity | Normal OIDC sign-in on independent portals; create both events, objective/measure setup, branding and facilitator grants | Exact person/session identities, owner restriction acknowledgement, different brands/configuration, no anonymous or cross-surface cookie authority, no accidental second-event mutations |
| L02 Registration and go | Publish actual forms, register below/at minimum, fill capacity and waitlist, explicitly advance preparation | Below-minimum transition blocked without mutation; reaching the threshold does not auto-advance; offers/staff excluded; new registrations closed after go |
| L03 Surviving offers | Withdraw a confirmed attendee, maintain FIFO offer, accept/reject with current versions through preparation | Original order and offer identity, unaccepted offer not counted, eligible offer survives until admission cutoff; post-cutoff acceptance/backfill rejected |
| L04 Eligibility | Grant/revoke former-organiser and early-access roles with restriction acknowledgement; attempt competing registration | Lasting ineligibility; active/waitlisted registration blocks incompatible grant; no silent registration cancellation or cross-event restriction |
| L05 Allocation and before-freeze remediation | Suggest/review/publish target-five teams; actual team withdrawals create under-three case; explicit unlock/rebalance/republication | Remaining two are reassigned before freeze; expected four-to-six preference and approved exceptions; draft changes do not silently alter published membership; real disband lifecycle |
| L06 Team freeze | Native freeze/reviewed publication before common challenge; attempt queued stale move/publication and late offer | Frozen identities and one entry per team; no post-freeze competitor movement, no post-cutoff backfill; unaffected event unchanged |
| L07 Common challenge and rubric | Configure/approve exact rubric, review/approve challenge, blocked or missed publication, native postponement and correct later common publication | Seven local calendar days/DST oracle; current minimum/frozen assignments required; same exact challenge/rubric for all confirmed attendees; early-theme URL/file/search/cache denial; lateness beyond tolerance not silently accepted |
| L08 Preparation and acknowledgements | Read published challenge, private resources and versioned rules, acknowledge exact current document, save early work | Building/private drafts allowed; formal submission, mentor review and scores rejected before live; credentials absent from logs/room/export/notification; replaced acknowledgement does not satisfy current version |
| L09 Approved exceptions | Change live/cutoff dates after publication through recorded organiser exception; withdraw below minimum and explicitly approve live attendance exception | Public challenge retained; reason/approval/revised dates and communication audience recorded; missing exception blocks live; invested work and unrelated event preserved |
| L10 Live support and interruption | Mentor checkpoint/observation, team help with simultaneous claims, private question→common clarification, corrected answer, private resource; interrupt/reconnect Wi-Fi | One claim wins, losing claim has no side effect; private origin/raw notes withheld; corrected notification uses actual current published revision; pending/unknown write retains original identity and reconnect does not replay it |
| L11 Provider failure and recovery | Approve message, let real worker attempt deterministic provider failure/unknown outcome, review/reconcile using original identity | Durable operation/attempt/native provider policy, exact audience, no blind uncertain resend, explicit safe retry or unresolved state; real queue/lease effect, not synthetic application success |
| L12 Repository and cutoff | Native repository setup/access; submit repo and non-repo entries; fail one cutoff capture, attempt late replacement, reconcile exact cutoff evidence | Frozen SHA/branch/revision as actually evidenced, late head never substituted; disbanded repository remains unused/unscored; cutoff applies even before stage advancement; census covers every team with frozen/explicit absent/withdrawn outcome |
| L13 Independent judging | Synthetic practice before live, actual live checkpoints, cutoff final mentor cards where applicable, signed qualification, fresh required judge cards and recorded decisions | Both templates, no practice/checkpoint accumulation or mentor carry; missing cards block; current assignments/conflicts and frozen source enforced; category adjudication requires every judge's ballot and a strict judge-only majority, mentor input is advisory, unresolved splits block |
| L14 Awards and feedback | Deliberate private result preparation/confirmation, separate finalist/award announcements, organiser report preview/release/amendment, native feedback survey | Embargo across both portals, room, inbox and exports; explicit own-team score selection; amended history/audience/author retained; no raw assessment or unpublished rank leakage; current role/member loss removes access |
| L15 Reviewed nonwinner showcase | Explicitly select/review/publish a non-winning project's approved fields/media; search/filter/page across both events; revoke consent/audience or withdraw/correct | Publication independent of submission/awards/archive; no private tally/award inference; approved resource permissions; guessed route/thumbnail/cache denial, correct current status and authorised published awards only |
| L16 Archive and owned outcomes | Archive competition, execute only explicitly approved retained follow-up, record owned 30/60/90-day estimates versus evidenced outcomes | Frozen submission/team/rubric/score/result hash/version unchanged; rejected competition mutations; auditable current authorised outcome changes; unapproved operational work cannot continue; relevant native archive/export contains intended evidence without private leaks |
| L17 Complete Fleet journeys | Drive current final screens for each major prepare→review→result→next action, both independent portals and separately published guide | Actual production primitives, keyboard/narrow flow, real native responses, current denial/private recovery, screenshot/provenance index; no prototype-only substitution |
| L18 Reproducibility and roles | One documented clean Compose command, deterministic providers/clock, synthetic actor roster, exact source/images/schema and per-case ledger | Organiser/coordinator/attendee/captain/mentor/judge/MC/logistics/member/stranger and revoked-role matrices throughout; no production accounts/effects; no omitted case counted green; optional M5 certificates/bookings explicitly outside M4 scope |

## Initial implementation evidence

The synthetic OIDC provider and native authenticated HTTP client compile at the
initial checkpoint (`go test ./internal/app -run '^$'`, 0.541s). This starts no
database/provider/browser and proves neither OIDC execution nor any lifecycle
criterion. At that initial checkpoint, native/Compose/browser execution had not
yet occurred; later partial evidence is recorded above.

Current execution gates include final showcase/Fleet ownership, execution
against the final merged results/report/scoring owners, the missing native
mentor advisory-input capability and exact-source final CI. Approved live GitHub/Teams observations are
separate unavailable external acceptances; they do not excuse omission of the
deterministic provider-fault paths in this rehearsal.

Second local checkpoint: the shared private worker factory and isolated
Compose process entrypoint compile, and both independent DST-fixture cases
passed in 0.598s. At that checkpoint no Docker, PostgreSQL, worker, OIDC or browser
execution was claimed; Compose packaging and the bounded eligibility seam were
still pending. Subsequent real queued-worker RED/GREEN, explicit policy and
native partial Compose evidence are linked above. The complete integrated
scenario, independent browser journeys, normal production-image proof and final
evidence remain unfinished. Leases, transport/auth expiry and River scheduling
retain wall time.
