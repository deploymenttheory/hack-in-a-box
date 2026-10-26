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

The partial receipts below retain their original source/schema provenance.
Main subsequently gained the reports/scoring owners and schema 54; final #120
acceptance requires integration with the final merged owners, a fresh image
build and complete execution on that source. These verification images are
separate from the normal latest application deployment.

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

The `publication` profile subsequently **passed** in 3.28 seconds after rebasing
onto main `2cfbce40`/schema 54. Source `07c207a4` built verification image
`sha256:864e6ebd23f05108a67d40e1643274c6ddd93d6da53a2e53a96f464368ce742f`.
It repeated genuine admission/allocation through HTTP, configured the explicit
retained-response policy through the native HTTP owner, approved the exact
rubric/challenge, denied early route/search/rubric access, and observed the real
worker's publication maintenance at the independent DST/T−7 instant. Publication
was March 23 at 09:00 UTC for March 30 at 09:00 London time: 167 hours, with a
2.746-second observed wall delay after domain-clock advance. Attendee rubric
bytes matched and native version-bound acknowledgements persisted. The
[partial receipt](evidence/publication-only.json) remains
`complete_lifecycle=false`; browser and provider-delivery proof is excluded.
Its project was removed and label-filtered cleanup checks were empty. Historical
pre-rebase proof sources remain under the local evidence branch
`evidence/lifecycle120-before-schema54-rebase`; their earlier receipts are not
relabelled as results on this newer source.

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


## Normal production-image calendar checkpoint

`TestLifecycleNormalImageCalendarAPI` passed in 0.51s (runner session 71084)
against the actual normal `/app/api` commands, real PostgreSQL schema 54 and
normally configured native OIDC. The independent London example has live start
2027-03-29T08:00Z (09:00 BST), publication 2027-03-22T09:00Z (09:00 GMT),
and an elapsed interval of 167 hours. Native preview, create and public read
agreed. The normal image contains `/usr/share/zoneinfo/Europe/London`; no timezone
packaging defect was observed.

Exact normal public image: `sha256:6dac9e0b773d4d3f922ac0b3732de09258bb4cc58d31247d901a897b08fdc265`.
Exact normal admin image: `sha256:3283e8b8139ef72ceb030bb7b74e17aedb37b50db7fa82a87a161da37c99b63d`.
These are the separately built normal images from main `2cfbce4046625e719bfec3a442641a70fda54cdc`.
Runner source `77849f83`, image `sha256:b4b4a0196f48681e8ac19dae5f2a8b1a9fce0c0fd63b15c1459162e1799680d8`;
provider source `6e374ae6`, image `sha256:802f0dd429a69543526ef5cf95d30f2aec260b7eb9f35d6318f6c510c07dfbb8`.
The provider/runtime/auth/transport source bytes were unchanged between these
two source commits. Build session 45554 completed the tagged runner image;
its original terminal handle was unavailable after context restoration, so
this checkpoint relies on the inspected immutable image and executed test,
not an invented build log.

`compose.normal-probe.yaml` pins the normal API and migration images separately
from the synthetic provider/runner. Container inspection confirmed both APIs
ran `/app/api`, had no `HACKBOX_LIFECYCLE_*` environment names, no control
mount, and no host port bindings. Only the test provider mounted `/control`.
Each API/provider/runner was capped at 0.5 CPU and 256 MiB, PostgreSQL at
0.75 CPU and 384 MiB, with GOMAXPROCS=2 and private networking. The safe receipt
is [normal-image-calendar.json](evidence/normal-image-calendar.json).

Project `hackbox-lifecycle120-normal-calendar-20261025` was removed with its
private database, files, control and artifacts volumes and network. Other
projects were preserved. This is production-image calendar/startup/portal
configuration evidence only: full recovery, lifecycle and final merged-source
acceptance remain pending, and `complete_lifecycle` remains false.


## Queued SMTP acceptance checkpoint

`TestLifecycleComposeSMTP` passed in 6.78s (runner 6641) from frozen source
`0a21f4a6`, image `sha256:3fd3cd2b1d615d071f1cf0cc8ab25695618f4e6380bd19bf1caf29305a5ec3ef`,
based on main `076f6162558df2ac3d7fb1ff7c23b3d35dc21784`, PostgreSQL schema 54.
The real native OIDC claim owner supplied the verified account contact. Native
HTTP saved the message/audience/campaign, rendered an exact organiser-only
review, and queued the reviewed intent. The actual River worker used native
SMTP with certificate verification, TLS and AUTH against the isolated relay.
The native bounded receipt was `accepted/relay_accepted`; the private relay
contained exactly one matching synthetic message. A read-only PostgreSQL query
confirmed one `communication_test_smtp` operation, one delivered attempt,
accepted snapshot and released lease. Acceptance does not establish mailbox
delivery or reading. The safe receipt exports only IDs, digest and bounded
state, never message contents, mailbox, token or credentials.

The first attempt, source `c414aeb0`, image `sha256:e7f71c6ee7796d0c7725dc7e53e4fb6d5d953d6fdb9c872132e42f38075d67d0`,
failed in 0.05s before queueing at the first communication-source HTTP save.
The driver incorrectly serialized an absent optional object as null; the native
wire contract correctly rejected it with 400. The corrected driver omits absent
objects. No production validation was weakened. Build 52850 and corrected build
65060 both terminated successfully; corrected fresh startup 55736 passed.
Both private projects and all their labelled volumes/networks were removed,
including the profile-only artifacts volume. Other user projects were preserved.

This proves accepted organiser-only transport, not the campaign current-audience,
failure/unknown reconciliation or full lifecycle requirements. `complete_lifecycle`
remains false. A later formatting-only commit resolves the CI golines finding;
the historical execution remains attributed to the frozen source above.


## Queued campaign unknown-outcome checkpoint

`TestLifecycleComposeSMTP` passed its extended accepted + scheduled-campaign
scenario in 45.61s (runner 71050), frozen source
`8e13c403499b1d04f63facd4a55e6bd8ed0dccca`, immutable image
`sha256:43792266b01df15091ff2a0344e3e49f7314f02a3028be90950d47b10fa22aee`,
main base `076f6162`, schema 54. This is separate historical evidence from the
accepted-only 6.78s checkpoint above.

Normal OIDC/native HTTP created the event, exact current-organiser audience,
message/campaign preview and approval, and scheduled automation binding and
approval. Only the synthetic relay fault was configured through its private
Unix socket: consume DATA then close without the SMTP acceptance response.
Advancing the domain eligibility clock to the approved due instant let actual
native maintenance and River workers dispatch the campaign and attempt SMTP.
No handler, queue retry or provider-owner success was invoked directly.

Native delivery history retained one campaign recipient and one ambiguous
attempt. The organiser deliberately recorded `resolve_no_resend`; repeating
that exact original request ID/payload/revision returned 200 with the same one
decision. Reusing the ID with a changed payload returned 409. The original
operation, attempt and uncertain receipt were preserved. After 32 seconds of
wall time, beyond the initial native retry/snooze interval, no second campaign
send/attempt appeared. The earlier independent organiser self-test remained a
separate history item; it was not counted as campaign evidence.

A read-only PostgreSQL query confirmed one delivered self-test attempt, one
delivered `communication.dispatch` attempt and one ambiguous
`communication_campaign_smtp` attempt with `cancelled=true`; every lease was
released. The relay contained two total messages: the accepted self-test and
the one uncertain campaign DATA capture. Safe evidence is
[smtp-unknown.json](evidence/smtp-unknown.json); no raw payload, mailbox,
credential or provider token is exported.

The initial same-source build 38097 failed with a shared Docker control-stream
EOF after its Go stage completed (148.7s); no image/native result was claimed.
After root restored Docker, tag inspection confirmed no image existed. Retry
73599 exported the pinned image successfully from the same source/cache;
fresh private startup 46209 and native run 71050 then passed. Cleanup 83451
removed project `hackbox-lifecycle120-unknown-20261026`, all its private volumes
and network; label queries confirmed no leftovers. Normal user projects were
preserved.

This proves the unknown/no-resend branch for a real scheduled organiser-only
campaign. Definite temporary/permanent failures, deliberate safe retry/resend,
changed current audiences/sources, other providers and the coherent full-event
matrix remain required. Neither L11 nor the full lifecycle is marked complete.


## Definite SMTP rejection and reviewed retry checkpoint

The extended `TestLifecycleComposeSMTP` passed in 95.04s (runner 83930), frozen
source `ce8cb0c17e90bc9e09ed342e55d9f8f62e1c3607`, image
`sha256:343dbaa0fe4402018e758fb380e2d071879997453f5d5a748308161dfa2a86e5`,
main base `076f6162`, schema 54. It repeated accepted/unknown handling and added
two separately approved current-organiser campaigns and native scheduled
bindings. Both were executed through actual River dispatch and SMTP workers.

The relay returned 550 at RCPT for one campaign: native history retained a
permanent rejection and one attempt; an ordinary retry command returned 409.
For the other it returned 451 at RCPT: native history retained the original
retryable attempt, and no DATA capture occurred. After explicitly clearing the
synthetic fault, the organiser's reasoned native retry command queued the real
worker again. The same delivery then retained its original rejected attempt
and one accepted second attempt, with one retry review. A 32-second wall-time
observation confirmed the old queued work did not duplicate the accepted effect.
Only three relay DATA messages existed overall: initial accepted self-test,
uncertain original campaign and the accepted safe retry. Each operation's lease
was released. These are SMTP acceptance/rejection facts, not human delivery.

Safe receipts [smtp-permanent.json](evidence/smtp-permanent.json) and
[smtp-temporary.json](evidence/smtp-temporary.json) include bounded read-only
native operation/attempt/review metadata. No message body, mailbox, credential
or token is exported.

The preceding source `093ef7d6` / image
`sha256:524e0f88893b36d6789e59fde0652d2a6c0fdc2903ce23a48940da018404ceb1`
failed an identity assertion in 69.31s (runner 42495), after native safe retry
had succeeded. A read-only native query retained the original retryable attempt,
accepted second attempt and one retry review. The driver had shallow-copied its
expected detail (`final := original`), then decoded responses into slices shared
with the original expectation. A standalone encoding/json reproduction confirmed
the original expectation was overwritten through the shared slice. The fix
allocates an independent response value; the identity/attempt assertions remain.
This driver failure is not reclassified as a passing run or production defect.
Corrected build 14010 and fresh startup 90110 passed before runner 83930.

Both failed and corrected private projects were removed with all their labelled
containers, volumes and networks; no user project was changed. Remaining work
focuses on the connected business lifecycle and deterministic repository/provider
flow, current audience/source changes and all final matrix/browser acceptance.
Full L11 and `complete_lifecycle` remain unfinished/false.


## Connected two-event start-to-live checkpoint

`TestLifecycleComposeJourney` passed in 1.45s (runner 28744), frozen source
`467e0b40`, image `sha256:73c325d5353b7b5e7547c6c256eeb1d4156005c5289ba591e8400cd7ef88e5f5`,
main base `076f6162`, schema 54. Unlike independent phase checkpoints, one
owner/control/timeline retains both actual events and their cohorts, native
objectives, frozen teams, common challenges and rubric references throughout.
The same 36 real OIDC participant identities applied to both events. Each
actual native admission/FIFO/go/withdrawal/reassignment workflow produced six
frozen teams and 32 confirmed competitors; the second event's preparation did
not mutate the first event. Distinct native brands and planned objective owners
were saved, with estimated baselines explicitly remaining planning inputs.

The events used `all_teams` and `mentor_qualification` rubric modes. Both
unpublished themes/rubrics were inaccessible. The native worker released both
common challenges at the approved calendar instant; independent attendees in
each event received identical exact public rubric bytes without private judge
guidance and acknowledged exact released versions. Public branding stayed bound
to its own event. Private undisclosed planning remained forbidden. Early live
transitions were rejected for the actual scheduled-start gate without changing
revision/state; advancing to live then allowed both actual native transitions.
The safe [journey-start-live.json](evidence/journey-start-live.json) retains IDs,
references, censuses and hashes. Its `complete_lifecycle` is false.

The first source `3ddcb0cc`, image
`sha256:fa1ddcf74f18519f63d94fb7bb87217a248a805223e4cb1bbc83c31b74ed7f8a`,
failed in 3.27s (runner 85980) because the driver incorrectly expected 200 with
an empty private planning index. The documented native attendee contract is
403, which the actual API returned. The correction asserts 403 and absence of
private planning prose in the response, with no production policy change.
Corrected build 9055 and fresh startup 8272 passed before runner 28744. Both
private projects and all labelled volumes/networks were removed. Existing user
projects were preserved.

This is the connected foundation for remaining repository/access, live support,
cutoff census, independent judging, embargoed awards/feedback, reviewed nonwinner
showcase, immutable archive, retained outcomes and real browser journeys. Those
phases and the remaining negative/exception/provider variants still gate every
final L01–L18 acceptance claim.

## Connected native GitHub App attachment checkpoint

The same two-event journey passed in 1.85s (runner 11852), frozen source
`0c72445bb3bbc89e43d5443f0576b49d7fa6e3c9`, fixture image
`sha256:c8a633cc11d1f7de54469c3d40818e9d05e726ff04faeabb6849b5e828ac685a`,
main base `076f6162`, schema 54. Normal configured credential encryption backed
both native event App owners. Each organiser obtained a browser-session-bound
installation state, followed the strict synthetic external installation redirect,
and completed the real native callback (303). Replaying that callback through
the authenticated native route returned 400 with no-store. Native diagnostics
used the actual GitHub REST client: the external fixture verified twelve signed
App JWTs, minted four scoped installation tokens and served four template reads.
Read-only database assertions matched both event IDs to App 42, installation 7
and organisation 99. The journey then completed its existing native publication,
acknowledgement and scheduled live transitions in both contexts.

The safe [journey-github-app.json](evidence/journey-github-app.json) records exact
source/image provenance, native bindings and counters without keys or tokens.
This is an explicitly test-only provider transport and fixture process; it does
not demonstrate real GitHub effects, production TLS, repository provisioning,
participant GitHub OAuth or the remaining lifecycle. `complete_lifecycle` stays
false.

The initial `48845b14` image
`sha256:45e66bca857ba2e4d4be319aa028550bfdebff659f200d64bc9215cf130cabcd`
failed in 0.43s after the native callback had already returned 303: the driver's
API-only helper rejected its non-API callback replay locally. No native replay
result was claimed for that run. An unsuccessful source replacement was followed
by cached build 60308 of the same old bytes; that build is not corrected-source
evidence. The correction uses an explicit same-origin authenticated callback
request while retaining the generic API-route guard. Corrected build 26623 and
fresh startup 26849 preceded passing runner 11852. Both private projects were
removed; cleanup 82553 verified empty owned container, volume and network labels.
Normal user projects were untouched.

## Connected real queued repository checkpoint

The same two retained events now provision their twelve actual frozen teams'
private repositories **before common T−7 theme release**, then continue through
actual common publication, exact acknowledgement and scheduled live entry.
Normal authenticated APIs deny an attendee's queue command and accept the current
organiser's command. The normal worker's minute reconciliation inserts real River
jobs; no driver invokes a handler/maintenance callback or writes queue timestamps.

Frozen source `2a711829bfb667378a9cc28e1dd99fcc5c853f28`, fixture image
`sha256:ce5234c44fd42ffac28abc02a868ca8e3e15b49bd9228d3ccb8c8fdc2ba54d26`,
main base `076f6162`, schema 54 passed in **53.29s**, runner 59117. The strict
external fixture verified 156 signed App JWTs, minted 64 scoped tokens, served 52
template reads and created exactly twelve repositories. Native PostgreSQL held
twelve ready mappings with distinct GitHub IDs, twelve delivered operations,
twelve completed River jobs and twelve delivered attempts; every operation lease
was released. Exact current event/team-derived names, private/template/provenance
identity and native creation policy are retained, not seeded into native state.
[journey-repositories.json](evidence/journey-repositories.json) records the safe
source-pinned receipt. The fixture transport is not actual GitHub/TLS acceptance.

The preceding same-order source `27b332b1`, image
`sha256:97d3a21066e6803465267b7f100fc7ed94ab5d055971a66a1c7c35afa5f2bd7e`,
failed in **90.78s**, runner 14262. Normal reconciliation created twelve actual
scheduled River jobs, but repository intent due times used database wall time
(October 26) while domain eligibility remained historical (March). All twelve
operations/mappings stayed pending with zero attempts. That actual RED is
preserved in [journey-repositories-clock-red.json](evidence/journey-repositories-clock-red.json).

The correction supplies the repository owner with the existing optional trusted
domain clock and captures one instant per new queue batch. With no injected
clock, due time remains **exactly the original native repository CreatedAt**;
audit/created timestamps, auth, transport, River and leases retain wall time.
Repeated queue commands preserve the original operation/due/provenance. The
PostgreSQL default/explicit-clock/two-team/requeue invariant test passed **1.90s**
(package 3.225s), with normal fixture termination; affected lint reported zero
issues. Both independent Standards and Spec reviews of the three-file production
seam reported zero findings before publication. This fixes test composition
consistency; it is not a claim that normal production wall-time behavior was
broken. Build 86874 and startup 60303 preceded the successful native run. Both
RED and GREEN projects were removed, including profile volumes, and every owned
resource-class label query was empty. No user deployment was changed.

The strict real REST-client component also retains a genuine initial creation
RED (0.11s) before fixture protocol implementation, then explicit inspection and
creation/lookup/duplicate tests passed (0.06s/0.02s; package 0.490s). Those loopback
checks require no PostgreSQL and do not substitute for the queued native result.

This advances portions of L08/L11/L12 only. Verified participant GitHub linking,
actual access grants/provider invitation acceptance, early private work, repo and
non-repo submissions, cutoff fault/reconciliation, frozen census, remaining
negative/provider/eligibility variants, live support, judging/advisory, results,
showcase, archive/outcomes and final browser matrix remain pending. All L01–L18
rows still require their full integrated actions/assertions; **complete_lifecycle
remains false**. New access setup belongs before pre-week work, rather than being
reconstructed after it.

## Connected native participant GitHub ownership checkpoint

Frozen source `5a17fb70420a735edc40d1d4fa3a4cff586c7f43`, fixture image
`sha256:5b56a6390deabd663d408809049ffa14509d5c83ed686b0ee03529cb2a7061b5`,
main base `076f6162`, schema 54 passed in **51.45s**, runner 35490. Normal
GitHub OAuth client/token-key settings enable the actual native account owner.
All 36 genuine OIDC participants initiate their own current-version, acknowledged
link command, receive native S256/browser-bound state and follow a strict
synthetic external authorization/code/token/user protocol. An authenticated
second browser for the same person cannot consume the first participant's state;
the original browser still succeeds, and replay returns the native failed-link
destination. Both retained event sessions observe the same numeric account,
handle, verified timestamp and exactly advanced link revision. Linking remains
person identity, not an event or repository access grant.

The external fixture verifies exactly 36 PKCE exchanges and serves exactly 36
actual authenticated user reads; wrong-verifier and code replay controls are
covered separately with real OAuth/REST clients. Native PostgreSQL contains 36
distinct verified numeric accounts and 36 protected token records. The native
owner performs normal context-bound token encryption; no token, cookie or private
response body is exported. Linking precedes repository provisioning/common theme
release/pre-week work in the coherent journey. The same run then produces twelve
ready repository mappings/twelve delivered operations/twelve delivered attempts,
with released leases, and both contexts reach native live entry.
[journey-github-identity.json](evidence/journey-github-identity.json) retains safe
counts, IDs, source/image and native-state assertions.

Build 73241 and startup 66755 preceded the run. The fresh private project and all
labelled containers/volumes/networks were removed, with every final resource-class
query empty. Normal user projects were unchanged. No production code changed in
this checkpoint. The external provider and host mapping are explicitly test-only;
this is not live GitHub acceptance or production GitHub TLS verification.

Supporting real-client OAuth protocol RED71152 returned external404 before the
fixture endpoint existed (0.06s). An intermediate incomplete fixture-field patch
failed compilation; it supplied no executed proof. Corrected protocol GREEN67339
passed0.02s; combined run45838 executed all three inspection/repository/OAuth
parents (0.10s/0.04s/0.07s; package0.671s). Affected app lint reported zero issues.
These component results are narrower than the measured native Compose run.

L08/L12 now have this additional identity proof, but actual reviewed repository
access/invitations and their external acceptance are still pending; repository
creation or a linked account alone grants no access. Live support, submissions,
cutoff faults/frozen census, judging/advisory, awards, showcase, retained outcomes,
all remaining variants and final integrated browser/source/CI gates remain.
**complete_lifecycle remains false; no L01–L18 row is declared fully complete.**
