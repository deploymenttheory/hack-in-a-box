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

## Connected queued invitations and accepted access checkpoint

Frozen source `2a9fc5d11619ff74c77c842da3f07c1b1d16aaf9`, fixture image
`sha256:ff9b91e06bc54c4a980a330045e0fa1ba8ea6485c7f7927c583cecdd4557d077`,
main base `076f6162`, schema54 passed **114.71s**, runner89032. The same actual
admission/freeze/App/36-person GitHub linking workflow first creates twelve
private repositories. Own invitation URLs return403 before common publication.
After actual T−7 publication and exact acknowledgement, the organiser explicitly
reviews and approves twelve exact current plans for the frozen 32-person cohort
in each event. Participant plan mutation is denied; no linked account alone
supplies access. Current event/team/repository/fingerprint/version/approver and
all original action IDs remain bound throughout observation.

Real minute reconciliation/River workers deliver all **64 original access
effects**. The fixture's actual App REST client receives64 distinct201 pending
invitations; no invitation is treated as an accepted collaborator. Only after
every original effect is natively delivered/attempted and every original managed
grant is known pending with no current flags does each participant use their
actual prior synthetic GitHub OAuth browser cookie and exact Origin to accept
externally. The fixture changes external provider state only. Native ordinary
refresh independently reads collaborators and records **64 managed accepted
grants**, with empty diagnostics. Every current own-invitation API exposes only
that person's accepted grant. Read-only original approval receipts retain actor,
plan/version and command fingerprint; no approval is replayed or replaced.
Both retained event contexts then complete early-live denial and actual live
entry. Normal PostgreSQL records64 delivered access operations/attempts,64
attempted original dispatch effects and released leases; the original twelve
repository deliveries remain intact. External counters are804 verified AppJWTs,
280 scoped tokens,52 template reads,216 repository reads,36 PKCE exchanges/user
reads,12 creates and64 invitations/acceptances.

[journey-repository-access.json](evidence/journey-repository-access.json) retains
safe exact source/image, plan/action references, pending-versus-accepted censuses
and the complete native operation-kind counts. It explicitly does not certify
other queued work:14 submission captures remain pending before cutoff, four
team-publication jobs and twelve access-poll jobs are retained as pending scope.
Current-source notifications/polling, capture and other L01–L18 variants still
need their own actual effects. The final full rehearsal must integrate all such
owners; a passing repository subset cannot substitute for them.

The previous frozen `ebc306c6`, image
`sha256:d328d44e8919dee39b061b1c046fd8fe2f4e24a1c3e1a1000a61ec7fa3cece1f`,
failed **105.86s**, runner15035. Its driver rejected a combined plan/flags check
while actual workers ran. It had delivered64 original effects to64 managed
pending grants; no external acceptance was attempted. A subsequent read-only
walkthrough signed in through normal configured OIDC and read all twelve current
views: intact version1 plans, zero flags and64 pending grants. The original
transient response metadata was not captured. Therefore the native dispatch
barrier's intentionally ambiguous pre-provider state is a source-consistent
explanation, **not a recovered observation of the original failing flag**.
[journey-repository-access-red.json](evidence/journey-repository-access-red.json)
keeps this uncertainty and the later diagnostic separate from the failure.

The driver now observes only known original planned/dispatch-barrier states;
only the two explicit in-flight observation flags may wait while original effects
remain unfinished. Every effect must eventually be delivered/attempted and every
original grant pending/managed with zero flags before acceptance. Changed source,
plan/version, target, approver, authority or unresolved terminal evidence stops
the run. No retry/reapproval, timeout extension, product policy or clock change
was made. Safe current metadata is captured on subsequent observation failures.
The pure observation controls plus strict loopback access protocol passed at
27391 (package0.516s), and affected lint reported zero issues. Earlier real-client
access protocol RED11479 failed before scoped access endpoints existed (0.05s);
GREEN7996 covered pending invitations (0.11s), extended18615 additionally proved
anonymous/foreign-Origin rejection, authenticated external acceptance and real
REST re-observation (0.07s). Combined1521 executed all four protocol parents,
package0.588s. These component checks do not replace native execution.

Build35456/startup72467 preceded the successful run. The earlier70abba5e image
was build-only; before any deployment, the driver added accepted-own-reader and
original-receipt assertions and frozeebc306c6. Both actual RED/GREEN projects and
their profile volumes were removed, with all owned resource-class queries empty.
The short read-only diagnostic helper also terminated. No user project or live
GitHub account was touched. External acceptance is explicitly synthetic-provider
browser behavior on a private network, not a claim of GitHub's production web UI,
TLS or live sandbox. **complete_lifecycle remains false.** Early private work,
provider failure/reconciliation variants, all output/cutoff/judging/advisory,
awards/showcase/archive/outcomes and final integrated browser/CI gates remain.

## Actual remaining team-source fact audit

A fresh unchanged `2a9fc5d1`/`ff9b91e0` admission profile passed **0.46s** and
retained actual operation ID/key/version/due/creation timestamps. Its domain now
was `2026-03-14T09:00:00Z`. Both initial (`teams-published:87`) and corrected
(`teams-published:100`) facts were due at exactly that domain instant; their
physical database creation times were respectively
`2026-10-26T01:21:51.398361Z` and `01:21:51.460031Z`. Thus these facts were not
waiting for domain time to reach database October. There was no reviewed team
binding: zero binding versions, approvals and communication occurrences.

`teams.published` is deliberately a native automation source fact, not a direct
jobs handler. The production resolver checks its approved/current source and
uses latest `due_at,id` ordering. The frozen fixture timestamp tied both separate
commands, and the actual UUID ordering selected the physically earlier initial
publication. This is not claimed as a production latest-source defect. The
fixture must advance monotonically between separate real publication commands,
then review its notification binding before the new source, rather than alter
production ordering or retrospectively approve an old source. The source-only
one-second action tick is not yet a native passing result.

[team-source-fact-audit.json](evidence/team-source-fact-audit.json) preserves the
actual tied-source result and explicit limitations. The unique audit project and
all profile volumes were removed; all final container/volume/network label checks
were empty. Actual latest-source notification/obsolete zero-effect evidence and
queued access-poll reconciliation remain pending at this audit checkpoint.

## Connected current-team notice and live access-poll checkpoint

The next unchanged normal-owner journey passed **225.23s** at frozen source
`5c5833e34a8cb9f8f4a389208cb8dc51f4fbbf33`, fixture image
`sha256:f0bcbd14229f085df7e390006c35527512249a7dedacd89d7cc3741190876bfe`
(main076f/schema54). Build86157 and startup89213 completed before actual runner78431.
The [safe receipt](evidence/journey-team-notice-poll.json) preserves the original
coherent two-event admission/freeze/App/GitHub/repository/access/publication/live
journey and adds native notifications and actual queued live-phase polling.
This remains the explicit test-entrypoint image, not the production image.

The fixture advances its existing private domain clock by one second between
separate actual publication/review commands. The original tied-source audit above
remains unchanged. In the first event, old publication, binding approval and
corrected publication are respectively March14 at09:00:00,09:00:01,09:00:02Z;
the second event uses09:00:06,09:00:07,09:00:08Z. Their reviewed notice due times
are09:00:06Z and09:00:12Z. No production latest-source ordering, worker cadence,
lease/auth/transport/River clock, deadline or policy changed.

Each event's real campaign preview/approval binds exactly32 current confirmed
people; the automation binding is approved before the corrected source exists.
Real queued dispatch produces exactly one reviewed-content native inbox record
per current person and none for its four withdrawn people. The retained old
source has `missed/source_precedes_approval`, no dispatch operation and zero
effects. The corrected source has the current queued occurrence and32 locally
persisted records; the two actual dispatch operations and attempts are delivered.
Both underlying `teams.published` source facts remain pending intentionally:
these are resolver inputs without direct job handlers, not unexecuted notices.
The source IDs, revisions, due times, occurrence references and channel reports
are retained separately from wall-clock creation/attempt timestamps.

After both native live transitions, the driver issues only current access GETs;
it sends no refresh/reapproval/reconciliation command. Normal minute maintenance
creates12 live poll intents and normal reconciliation/River workers deliver all12
with one delivered native attempt each. Every original approved plan remains
version1 with its exact identity/fingerprint and accepted managed grant census.
The12 observations move from actual publication March23 at09:00Z to actual live
March30 at08:00Z. The receipt retains each poll's due/current source, wall attempt
and lease references, current native observation, original plan and64 accepted
grants. All operation leases are released. The12 repository provisions and64
original access effects remain delivered.

Only the unique `hackbox-lifecycle120-team-poll-20261026` deployment and its profile
volumes/network were removed. All final container/volume/network label queries
were empty; the normal user preview and other projects were untouched. The
historical pending-notice/poll counts above describe their earlier runs, not this
later result. Precutoff captures remain due later; output/cutoff, support, all
judging/advisory/awards/showcase/archive/outcomes, final integrated owners/browser
journeys and CI remain required. **complete_lifecycle remains false, and no
L01–L18 row is declared fully complete.**

## Connected private preparation drafts and actual live submissions

The same coherent two-event journey now passed **248.16s** at frozen
`611deb9a51bf7b5b0cd14f8dfc5f6df16b89a392`, fixture image
`sha256:627d3e184e7838dbed4f4d0f7946ecf7c44ed927a8a6a6360107e35dfec0ef2c`
(main076f/schema54). Build54407/startup63091 preceded actual native runner68834.
The [bounded receipt](evidence/journey-submissions-live.json) records exact native
submission/draft identities and times without their private prose or evidence
bodies. This is the verification image, not production-image/browser acceptance.

Before common release, actual snapshot GET/PUT configures each event's required
repository-or-URL deliverable while retaining its other native definitions.
After common publication and64 genuinely accepted managed grants, current members
of all12 frozen teams save actual private drafts at March23 at09:00Z. Every
member's early formal PUT fails409 with `live_start/not_open`; every organiser's
early formal review POST fails409 with `live_start/review_not_open`. Exact
competition and private-draft projections remain unchanged. Each organiser and
an actual member of another team receives403 for that private draft.

After both native live transitions and real queued current-access polling, four
teams per event explicitly submit their retained evidence through native PUT.
Normal PostgreSQL contains eight immutable formal revisions at March30 at08:00Z:
six managed-repository references and two non-repository demonstration URLs.
None is late. Four other drafts remain private-only with no formal revision;
they are inputs to future absent/not-submitted cases, not inferred submissions.
No repository URL or draft is treated as a cutoff SHA or reviewed final output.
All twelve repository provisions/polls and64 access effects remain delivered;
all operation leases are released. Fourteen capture intents remain pending
before cutoff. This run does not claim capture/freeze/judging completion.

The first frozen9faf/image5ccc run failed **102.39s** because the driver sent POST
to the formal endpoint, whose canonical OpenAPI and existing native HTTP contract
require PUT. It had saved one real private draft and zero formal revisions.
[journey-submissions-method-red.json](evidence/journey-submissions-method-red.json)
preserves that source/result and bounded native state. The correction changes
only the early-denial and live-formal method sites; all exact rejection,
version/body/CSRF, identity, privacy, time and invariance assertions remain.
Compilation passed0.538s; the initial new-driver app lint reported0 issues before
this two-token method correction. No production code, clock, timeout or policy
changed.

Both unique RED and GREEN projects and profile volumes/networks were removed;
all final label queries were empty. The normal preview and other user projects
were untouched. The remaining post-freeze depletion/unused repository, offer and
postponement/exception variants, provider work/cutoff reconciliation, live support,
independent judging/advisory/awards/showcase/archive/outcomes and final integrated
browser/CI matrix remain required. **complete_lifecycle remains false; no
L01–L18 row is declared fully complete.**

## First connected preparation-work/cutoff run — still RED

Frozen58fcc1fe/image349d1a9d (main076f/schema54) compiled and passed six actual
strict-loopback GitHub protocol parents (package0.882s), with app lint0 issues.
The provider now requires the real OAuth browser cookie, exact Origin and actual
accepted write collaborator for external synthetic work. Pending/read-only,
anonymous, foreign-Origin and other-repository writes fail; native App REST reads
require repository-scoped metadata/contents permission and matching numeric
identity. A later current ref never becomes a historical lookup. The added hashes
are opaque synthetic work receipts, not real Git objects or live GitHub effects.
The initial missing scoped branch contract failed (0.02s/package0.324s) before
implementation; its corrected missing-branch proof passed package0.491s. An
extended unit initially omitted the native request provenance marker and failed;
correcting that request preserved native validation and passed package0.521s.

Actual connected runner39200 then failed **236.74s** on its first organiser
reconciliation POST (400, expected200). Before that failure, the same actors and
normal APIs completed12 authenticated preparation-work transactions/native branch
configurations,12 private drafts, strict early formal/review denials,8 actual live
submissions, current queued access polling and real queued cutoff capture.
Read-only SQL independently records14 delivered capture operations/14 delivered
attempts plus one retryable attempt,14 immutable sources (8 frozen,4 not-submitted,
2 withdrawn), six current-ref observations plus one provider-unavailable
observation,12 configuration acknowledgements, zero reconciliation decisions and
zero retained leases. Both events remain live: cutoff does not require advancing
to assessment. The late replacement PUT denial/nonmutation check also completed.

[journey-cutoff-command-red.json](evidence/journey-cutoff-command-red.json)
retains exact source/image, actual native references/timestamps and limitations.
The original400 body and terminated driver's exact external-work receipts were
not exported and are not reconstructed. The likely driver issue is its domain
Command JSONnull slices against nonnullable API arrays; contract reproduction and
correction remain pending. No production bug or completed reconciliation is
claimed. The unique project/profile volumes/network were removed and all final
resource label queries were empty. The aggregate journey budget is now8m solely
to add two90s cutoff waits to the already measured248s setup; individual existing
guards, native windows, auth/lease/transport clocks and retry policy are unchanged.
All remaining full L01–L18 requirements and integrated schema55 owners/browser/CI
remain gates; **complete_lifecycle remains false.**

The first-reconciliation400 diagnosis was then reproduced against the actual
native OpenAPI schema: the extracted unchanged wire-body factory failed at
`/observation_ids` because JSONnull is not nullable (80759,0.04s/package0.492s).
The correction initializes the two optional evidence-reference arrays as empty
arrays; it retains the exact source/native/submission revision, selected earlier
work SHA, original request identity and reasoned external-provenance fields.
Native production validation is unchanged. A regression checks the actual factory
against the independent native schema, rather than duplicating its implementation.

After the now-merged #292 owner,291 rebased additively onto mainbba43a6/schema55
at2dc366a2. All57 lifecycle patches were identical and its source/evidence bytes
were unchanged; six loopback protocol parents compiled/passed package0.998s.
A local evidence branch preserves the prior schema54 source graph. These are
rebase/component results, not a new schema55 native execution. The corrected
schema test plus six provider protocol parents passed19441 (package0.845s).
A bounded `work-progress.json` now retains synthetic preparation/later-work
receipt metadata before subsequent phases can fail, without bodies or sessions;
the historical58 failure's missing work-receipt IDs remain missing. Corrected
full native execution, all remaining lifecycle variants and final integrated
advisory/browser/CI gates remain required.

## Connected retained-work/cutoff/reconciliation — schema55 native PASS

Actual runner57406 passed **237.22s** at frozen
`0aad4fb959a6eaa9e0473f00f304d6399b0dfe32`, fixture image
`sha256:eb71d2bcd3d5954aed7d0b4fa00204390af6eaf11d53f22cda1180da3aae2362`,
mainbba43a6/schema55 (build39407/startup92311). The
[source-pinned receipt](evidence/journey-cutoff-reconciled.json) contains actual
work/source/decision references, bounded native SQL census and wall attempts.
It includes the same retained two-event normal OIDC/admission/team/common/API
journey, not separately seeded live or submission facts. Its test entrypoint and
provider network remain distinct from normal production-image/TLS/browser proof.

After actual common release/64 accepted native-managed grants, each current team
performs an authenticated synthetic external work transaction; native branch
configuration records main before cutoff. All twelve genuine private drafts and
eight live formal entries remain distinct (six repository/two non-repository).
The real queue's selected cutoff read receives one deterministic503; native
history retains `provider_unavailable` and one retryable attempt. Only after that
observation does the domain clock advance past cutoff and the same accepted
collaborator create deliberately later external work. Natural queued retry reads
that later current head; it never silently selects it as historical evidence.
A member's late formal replacement PUT fails409/submission_cutoff/locked and the
exact native frozen projection remains unchanged.

All14 capture operations deliver with14 delivered attempts plus one retryable
attempt,15 distinct wall-clock lease tokens and zero retained leases. Native
immutable sources cover8 frozen,4 private-draft-only not-submitted and2 retired
withdrawn entries. Each event includes a non-repository frozen source whose
repository capture is correctly not-applicable. Six App-scoped current-ref
observations plus the unavailable observation remain separate from decisions.
There are exactly12 branch-configuration acknowledgements and6 explicit native
organiser reconciliations (18 commands). Each selects the genuinely retained
preparation-work receipt through the current source/native/submission revisions;
original actor-bound acknowledgements are recovered with GET, without replaying
the write. The failed/retried target's selected earlierSHA (`6b4da101…`) differs
from its observed laterSHA (`807c32dc…`). Both events remain **live**, proving
cutoff applies before assessment-stage advancement. Actual external counters are
13 work transactions,7 scoped branch reads/1 failure,12 repository creates,
36 PKCE/user reads and64 invitations/acceptances. These are opaque synthetic work
hashes, not real Git objects or an automatic historical guarantee.

The canonical-null-array RED80759 and corrected wire factory are preserved above;
production validation remains unchanged. Schema55 integration includes the now
merged advisory owner but does not yet execute its full lifecycle advisory path.
The combined source also contains the minimal shared explicit OIDC-people
constructor for #123: copied client/people inputs, exact unchanged52 default
actors and separate36-account GitHub boundary, no new count environment setting,
runner, domain/auth clock or authentication shortcut. A real alias-mutation RED
failed before defensive copying (76950,package0.439s); final11 constructor/schema/
provider parents passed65602 (package1.223s), with affected lint0. An initial
unknown-actor control omitted required OpenID scope and hit the existing request
guard; correcting only the test request preserved that guard. Explicit cohorts
cannot acquire default/other-cohort people or duplicate/mismatched identities.
These constructor/component checks do not claim1000-session/load acceptance.

Final/work-progress/SQL receipts were exported before scoped cleanup. All owned
containers, volumes and networks were removed and label queries were empty.
The historical58 schema54 RED remains attributed to its original image/source;
its missing work-receipt IDs are not retroactively reconstructed. Current-source
post-freeze depletion/unused existing repos, offer cutoff, eligibility,
missed/postponed release/attendance exception, full staff/practice/support and
judging/advisory/awards/showcase/archive/owned outcomes plus all final native
browser/normal-image/recovery/CI gates remain. **complete_lifecycle remains false;
no L01–L18 row is declared fully complete.**


## Connected selected staff and isolated judge practice — schema55 native PASS

Frozen `2aab98ea625965f31f16cfc357f97dbd98c8f891` passed runner16254 in
**246.20s**, built by58691 as fixture image
`sha256:d0a5c6e4b5400db3307d1caec401690a94383f0d049c3357464e583f1cbfea72`
on main03bfae7/schema55. The [actual receipt](evidence/journey-judge-practice.json)
retains this run's own event, staff, round, practice, source and queue references;
the earlier cutoff237.22s receipt remains attributed to0aad.

Mentors and three judges apply through actual normal OIDC and native staff-form
review. Their selection does not increase the competitor census. Before live,
each event plans a native final run and assigns all six frozen teams to the three
selected judges. All six judge/event contexts submit the dedicated fictional
practice card against their event's approved public rubric. The fixture asserts
that private judge guidance is absent, actual competition teams are distinct,
original actor-bound practice acknowledgements are retained and neither practice
nor denied real scoring starts the actual final run. Assigned real-entry reads
and real-card writes return403 before live; all six practice reads return403
after the actual scheduled live transition. Both templates then complete the
existing formal-submission, actual queued cutoff/retry and explicit retained-work
reconciliation checks in the same connected contexts.

Safe final/progress/native metadata was exported before scoped cleanup; all
three resource-label classes are empty. This is the isolated test entrypoint,
not normal production-image/browser proof. Actual final judge cards, mentor
qualification/checkpoints/advisory input, judge-only strict-majority decisions,
remaining lifecycle variants, awards/showcase/archive/owned outcomes and final
CI/native-browser/normal-image recovery gates are still required.
**complete_lifecycle=false; no L01–L18 row is declared fully complete.**


## Native mentor coverage/access-source RED — preserved guard

The next frozen driver `d61a4bc1e3faac3a0b8f717264ba95b12c8fa975`
(build81739, image`sha256:643a73d2d63243624e28f3dee58b94ea88d0a02aca2050295ae2017980491db4`,
main03bfae7/schema55) failed runner21370 at **112.28s**. Its
[original safe failure capture](evidence/journey-mentor-coverage-access-red.json)
retains the actual old approval and current native flags: two
`verified_account_required` plus`plan_stale`. Publishing actual primary/second
mentor coverage after competitor-only access approval legitimately expands the
desired source. Both selected mentors lack verified GitHub accounts. The original
plan ID/version/fingerprint and all its delivered grants remain intact; the
strict current-source access-poll guard correctly refuses to call it current.

Actual native setup has six isolated practice cards, twelve published coverage
slots per event, and zero live mentor cards. This proves a missing scenario
setup step, not a production defect. The next driver must link explicit selected
staff identities through real PKCE, prepare coverage before access review, and
approve/accept the expanded current grant set. It must preserve the canonical
default52 OIDC people/36 participant GitHub accounts and the strict stale-plan
guard. No reapproval/write retry or source substitution happened in this RED.
Safe metadata was exported; scoped containers, networks and volumes are empty.
Full lifecycle acceptance remains false.


## Explicit staff identity/live-source checkpoint — browser-owner RED

Frozen6dd804b7 (image`sha256:b772ec21220f292db4f5917207c896266dccc2ead58fd8b2f5ff8e7976f5c8b1`,
main03/schema55) failed runner80013 at **163.10s**. The
[actual native metadata](evidence/journey-mentor-browser-owner-red.json) records
38 verified native GitHub identities, all88 real delivered access operations,
64 accepted competitor grants,12 accepted first-event mentor read grants and
12 pending second-event mentor read grants. Six fictional practice cards and
zero live mentor cards are retained. Initial T7 plans remain competitor-only;
current live coverage requires distinct explicit approvals for24 read grants.

The second event's separate OIDC browser successfully reads only its native own
invitation but has no authenticated external GitHub browser cookie; the external
acceptance correctly returns403. The originally linked first-event browser
represents the same verified native person. The correction must select that
retained external browser only after exact native-person equality, retaining
the second event's own native API reader. It must not clone cookies, bypass
OAuth/PKCE, replay approvals or relax source/authentication guards. Ten exact
provider/default/explicit-identity parents passed1.113s and lint was0 before
this native run, including unknown-actor zero-code/session and accepted
read-only mentor external-work denial. These component results never substitute
for the failed native journey. All owned project resources were cleaned;
complete_lifecycle remains false.


## Connected current-window mentor access and live checkpoint cards — native PASS

Actual runner47326 passed **295.62s** on frozen
`5e7abc62e6fb4191cf7bd0b6e6a8d4052f54cd73`, fixture
`sha256:812dc43a7410fef08cf35e567de1f5681528809547de88d2a81bf43dac5cde39`,
main03bfae7/schema55 (build27531/start52669). The
[source-pinned receipt](evidence/journey-live-mentor-checkpoints.json) includes
actual two-event source/plan/card references, real operation and attempt metadata,
and the same connected cutoff census. Earlier d61a/6dd failures remain separate
and were not reclassified as successful executions.

Canonical default52 OIDC people and36 participant GitHub actors remain unchanged.
The isolated provider explicitly copies those36 plus selected mentor-a/b with
distinct numeric IDs and logins. Ten real component parents passed1.071s and
lint0 at the corrected source: input mutation/identity collision/unknown-actor
zero-code/session controls, PKCE and native scoped REST, including accepted
read-only mentor access that still cannot create external work. Actual native
execution links38 people through PKCE; it never seeds accounts or grant facts.

Both actual mentors publish independent primary/second coverage for six teams
per event, with native availability/capacity and a distinct checkpoint configured
before live. Coverage is valid only inside its published live window: initial
T7 access remains64 competitor grants. At live, twelve distinct version2 plans
explicitly review and approve only24 additional read-only mentor invitations.
Original competitor approval receipts stay historical. All88 external invitations
are separately accepted and verified by the native provider refresh and current
own invitation readers. The retained external GitHub browser is selected only
after exact native-person equality; the other event's native reader is retained.

Explicit refresh is not queued-poll evidence. The domain advances one actual live
minute after acceptance. With no further refresh, twelve real queued polls
observe08:01 after earlier08:00 observations. Native metadata proves88 access
and12 poll operations delivered,100 distinct real wall-clock attempt leases and
zero retained leases. Both contexts then submit twelve independent private live
checkpoint cards (24 total), with actual author/team/card/run references and nil
final-submission revision. The second author never inherits another author's
card. Early checkpoint writes remain403; the six fictional judge practices close
at live and never supply final judge scores. Eight formal entries and the
existing14 actual queued cutoff captures/one retry/explicit reconciliation also
pass in these same contexts. All project resources were exported and cleaned.

This remains test-entrypoint/native-HTTP proof, distinct from production-image,
TLS and browser proof. Actual final mentor cards/qualification, fresh final judge
cards/advisory-only mentor input/judge-only decisions, remaining variants and
later lifecycle/final validation remain required. **complete_lifecycle=false;
no L01–L18 row is declared fully complete.**


## Fresh frozen-output mentor cards and recorded signed review — native PASS

Frozen `ae532cefe94c2be5b57e9767809aa97f22aa6af6` passed runner24488 in
**284.70s**, built89686/start71047 as fixture
`sha256:f6ad29d8486d28e8413fdc317089229da19edda6da918f54b7b75f1b3057a3a0`,
main03bfae7/schema55. Its [bounded receipt](evidence/journey-final-mentor-review.json)
publishes validated synthetic UUIDs, counts, SHA256 hashes and current references.
Full local runtime exports remain private; their hashes preserve attribution.
No private scores, notes, sessions, provider configuration or keys are published.

Before live, only mentor-qualification plans a distinct mentor-final run and
checkpoint. After actual queued cutoff capture and explicit repository review,
its four genuine frozen entries receive fresh independent primary/second cards:
eight submitted version1 cards against exact current formal revisions. Their
IDs and run IDs differ from all24 private live checkpoint cards, with fresh final
scores and no other-author disclosure. All-teams correctly has no mentor-final
requirement. Ten component parents compiled/passed0.969s and lint0 before native
execution; these component results do not substitute for the actual phase proof.

Each final entry records configure, complete-second-review, open and sign-off
through its actual intended panel. Sixteen immutable huddle versions and four
persisted signoffs retain exact two-card references and submission revisions.
The response context bindings are validated SHA256 values. The export query's
flat SQL proof.binding projection was NULL because stored signoff Context is
nested; persisted signoff/context references are reported separately, without
reconstructing missing SQL values. There are32 actual private cards (24live and
eightfinal). The same connected admission/common/provider/access/poll/draft/live
submission/cutoff phases pass. All owned project resources were removed.

These signoffs prepare genuine qualification sources. They do not select
finalists, supply independent judge cards, resolve judge ties or publish results.
Remaining lifecycle variants and later phases/final validation stay required.
The isolated entrypoint remains distinct from production-image/browser proof.
**complete_lifecycle=false; no L01–L18 row is fully complete.**

## Judge-only majorities and independent award confirmation — native PASS

Frozen `dbe2304a4b95a54a7a5f47695473241d6480eaff` passed both connected event
modes in runner7896, **383.22s**, build64424/start49090, fixture
`sha256:3b80dc986faaddc84ad86e1674b483775bc0817737ec79a86b406adfb5186983`,
main03bfae7/schema55. The [bounded receipt](evidence/journey-result-decisions.json)
contains27 exact original result acknowledgements and two advisory acknowledgements,
cross-checked against actual native PostgreSQL metadata. Private runtime exports,
cards, advisory prose and provider/session configuration remain local with hashes.
Three actual driver/schema/reply-loss controls passed0.642s; twelve independent
native decision-owner parents passed0.324s; configured lint reported zero issues.

Prizes are declared before the common rubric approval. All-team final judging
has six actual tied decisions: one final and five categories. Two assigned
mentors independently contribute to a current category target. Their advice
does not change scoring, source bindings, result versions or judge vote counts.
One actual accepted HTTP200 advisory response is consumed by a test-only
transport; the caller receives an error and recovers its exact original intent
through GET. The transport observes one POST, and native storage retains one
version/command for that contribution. This is controlled reply loss, not a
physical Wi-Fi interruption test. A valid mentor ballot request returns403.

Qualification mode has one final-judge tie. Its category winners remain those
of the actual signed native qualification panel, including a possible
non-finalist. Both mentor advisory pages correctly contain zero category targets;
the driver never fabricates category ties, advice or extra judge votes there.
A valid mentor final-ballot request returns403 without changing the signed
selection or native votes. Category award selection reads that signed source.

In both modes every current judge must vote. An isolated missing third ballot
blocks confirmation with409. A complete three-way split stays unresolved and
also blocks confirmation. An explicit replacement by that third judge produces
a strict two-of-three majority. The replaced ballot's exact original
acknowledgement remains recoverable. All-team storage has18 current votes plus
one replacement command; qualification has three current votes plus one
replacement command. Two prizes per event are explicitly confirmed with
synthetic100.00/50.00GBP metadata; no payment occurs. Native storage retains
27 immutable result versions/commands, two advisory versions/commands and two
judge approvals. Actual recipient reads and SQL both show zero announcements:
confirmation never substitutes for deliberate publication.

Three earlier native failures remain attributed in the receipt: organiser
receipts carry their run in the prepared source rather than ACK.run_id;
ballot evidence requires explicit nullable deliverable_id; qualification
categories belong to the signed panel rather than new final-judge ties.
The first all-team decision proof already passed in the third failed run;
only the final383.22s execution proves both modes together. These corrections
change test drivers only. All owned containers, volumes and networks were removed.

Deliberate finalist/award announcements, own-team feedback, remaining lifecycle
variants/help/clarification/media/surveys, reviewed non-winner showcase, archive
and owned outcomes, final integrated browsers/normal-image recovery and CI
remain required. **complete_lifecycle=false; no L01–L18 row is fully complete.**


## Native recorded qualification and finalist selection — PASS

Frozen `64ba7abfbcf397c040f6ea88a0aa72aebcb5e63a` passed runner77867 in
**306.56s**, build23391/start90611, fixture
`sha256:a7324da43f518231cf34613ed96cf5d340008bb6a328554e7ce1859510436cc2`,
main03bfae7/schema55. Its [validated bounded receipt](evidence/journey-qualification-finalists.json)
contains current IDs, hashes, counts and original command references; complete
raw exports stay local with SHA256 attribution. Ten components compiled/passed
1.214s and lint0 before actual native execution.

The real owner configures the selected two-mentor/organiser panel. Four moderated
qualification cards bind actual frozen repository decisions, signed independent
review contexts and current formal revisions. Equal synthetic final scores lead
to a native proposal with unresolved seats. An actual missing-ballot sign-off
returns409 and leaves round version/signoff unchanged. Both current mentors then
record each required ballot; only a complete unique-leader result permits the
organiser's explicit noncasting resolution. Sixteen ballots and eight decisions
produce an actual signed selection of three finalists and one distinct reserve.
These are mentor-qualification ballots before independent judging, not final
judge votes or imported judge scores.

All35 successful single-shot commands have exact actor/version/fingerprint
acknowledgements recovered through GET, without POST replay. Native SQL retains
35 immutable round versions,35 command acknowledgements and124 source references;
the latest four source references point to actual signed frozen reviews. The
same connected earlier lifecycle phases pass, and all project resources were
cleaned. Fresh final judge cards, advisory-only input, all-required-judge strict
majority and the remaining lifecycle variants/later phases/final validation stay
required. **complete_lifecycle=false; no L01–L18 row is fully complete.**


## Fresh independent judge cards and guarded deliberation — native PASS

Frozen `bfb58c59556cac0b95b60e6ab1297d3f53937c60` passed runner48916 in
**285.71s**, build12132/start41435, fixture
`sha256:a4a73a6850765fb2c39819d654af3e1973b66557c882bc5e86f1671eda6398f5`,
main03bfae7/schema55. The [validated bounded receipt](evidence/journey-fresh-judges.json)
contains21 actual card acknowledgements and current frozen references matched
against native SQL, plus two actual opening acknowledgements. Full runtime
exports/private scores/guidance remain local with hashes. Ten component parents
compiled/passed1.208s and lint0 before actual native execution.

The planned judge-final runs remain unstarted after fictional practice and
mentor qualification. Four all-team frozen entries and three genuine signed
finalists each receive three new version1 cards. The reserve returns403; a
current mentor's real judge-card read returns403 in both events. Judges receive
the intended private guidance and their own exact formal/repository source;
independent peer cards remain hidden. Fresh judge values differ from the
fictional practice maximum, live checkpoint minimum and final mentor maximum.
Original actor-bound card acknowledgements are recovered with GET, not POST
replay. No mentor total or previous card populates an independent judge card.

With two judges complete, actual open-deliberation commands return409 and leave
round revision/open state unchanged. Only after the third required judge submits
for every eligible entry does the organiser open deliberation; each intended
judge can then read exactly three current submitted cards per entry. Native SQL
contains21 judge cards and41 distinct command records:21 submissions, two
openings, six separate practice acknowledgements and12 planning assignments.
Both native rounds are open. Earlier connected lifecycle/qualification phases
pass in the same execution, and all owned resources were cleaned.

Actual judge-only strict-majority tie votes and advisory-only mentor input,
results/awards/feedback/showcase/archive/owned outcomes, remaining lifecycle
variants and final native-browser/normal-image/recovery/CI gates remain required.
Synthetic repository hashes do not claim real GitHub history.
**complete_lifecycle=false; no L01–L18 row is fully complete.**


## Continuation: deliberate finalist and award releases — native PASS

On main6764/schema55, frozen `f2526ef365b56cf202f01e17d111c693a2312588`
passed runner21409 in **404.60s**, build7061/start38960, immutable fixture
`sha256:5d222948c448970d28ceafec4e8d423e34ee4ee37c601e5679e65b6f2955ba63`.
The [inspected bounded receipt](evidence/journey-result-releases.json) cross-checks
all31 original result acknowledgements against native PostgreSQL, and publishes
only the four new preparation/release acknowledgements with safe source and
publication identities. Raw exports/cards/advisory prose/auth/provider settings
stay local with hashes. Five finite controls passed0.427s without skips/failures;
affected-package lint is clean. All owned containers/volumes/networks were removed.

The qualified event privately prepares the exact signed selection, then explicitly
publishes three finalists and one reserve before fresh independent judge briefs
or cards. Existing native guards verify that the judge run remains unstarted until
an intended judge actually opens a brief. Both events repeat the fresh judge,
advisory-only, all-required-judge majority and separate confirmation checks. Earlier
finalist publication remains visible during confirmation, with no premature awards.
The organiser then deliberately publishes two confirmed awards per event. Native
storage has31 immutable result versions/commands, two advisory versions/commands,
two judge approvals and exactly three publications: one finalist and two award
announcements. Actual current recipients receive the exact safe snapshots, and
private preparation remains403. A fresh duplicate source-publication intent
returns409 without changing native event/result revisions or announcements. Original
acknowledgements are recovered by GET, not command replay.

This fixes a real boundary exposed by the first chronological release run:
`a98924ed` passed finalist publication but failed316.71s at the following unstarted
judge guard. The separate native component at `f5194948` proved organiser results
preview alone changed started_at while judge cards remained zero in both modes
(RED4.04s/package4.360s). Completion now only revalidates retained card contexts;
its outer read/write guard, organiser requirement and current judge/assignment,
rubric/frozen/finalist/repository/card validity gates remain. Intended judge Brief
still starts assessment. Frozen8a94 native GREEN4.28s/package4.781s and five
existing affected native owner parents22.207s/zero skips/failures preceded the
full corrected404.60s HTTP run. Independent Standards and Spec reviews each
reported zero findings for the exact bounded owner correction and regression;
this is not a full continuation review. The rebased equivalent fix is f87e364c.

PR291 merged the earlier incomplete checkpoint; [CONTINUATION.md](CONTINUATION.md)
audits every original criterion, and reopened issue120 tracks the remaining full
scope in Draft296. This proof covers publication and recipient reads, not approved
queued result-notification delivery. Own-team feedback, surveys/media/support,
remaining admission/eligibility/freeze/publication exceptions, reviewed non-winning
showcase, archive/owned outcomes, final Fleet browsers/guide, production-image
recovery and final reviews/CI remain required. **complete_lifecycle=false; no
original acceptance checkbox or L01–L18 row is declared fully complete.**


## Continuation: private own-team feedback and amendments — native PASS

Frozen `02fddbc7c81a9a121ca1b08cbb1b592f6432a194`, main6764/schema55,
passed first native runner64283 in **510.35s**, build92620/start12899, fixture
`sha256:e41db19e914d65e3f7116cb7d25ca994b7f08c79eef33430246ef5af05aa3e1b`.
The [inspected bounded receipt](evidence/journey-team-feedback.json) cross-checks
all12 feedback acknowledgements and versions against native PostgreSQL source,
author and audience-count metadata. Body hashes were checked through actual
normal-auth HTTP reads and retained history; SQL exports no prose or numeric
scores. Raw exports remain local. Six finite wire/HTTP controls passed0.619s
without skips/failures and configured lint is clean. No production code changed.

The same two connected event contexts reach publication and then author four
actual reports: one scored team per event, an all-team not-submitted entry and
the qualification event's signed frozen reserve without fresh judge scores.
Wrong report-source bindings return409 without mutation. Valid requests to include
unavailable scores also return409; unscored teams receive authored prose without
fabricated zero scores. A private draft has no recipient release. Members cannot
read private preparation, while other-team, withdrawn, mentor and judge readers
receive403 from the own-team released route. Intended current members receive
only deliberately released own-team prose and explicitly selected current
confirmed own-team aggregate scores.

Each scored report first releases version2. Its new private version3 amendment
leaves the member's complete old version2 projection unchanged. A separate
version4 release exposes the amended prose while intentionally withholding
scores that remain available to the organiser. Earlier version2 body/history and
its exact original acknowledgement remain intact. The two unscored reports release
version2 without scores. Native storage contains12 report versions/commands for
four targets. All12 original acknowledgements are recovered with GET. Result
round/source/announcements compare unchanged before and after feedback; SQL still
has31 result versions and three deliberate publications. All owned containers,
volumes and networks were removed.

Only the harness aggregate grows8→10minutes for roughly80 added native HTTP
calls beyond the404.60s release baseline; actual runtime510.35s exceeds the old
aggregate. Individual HTTP/job waits, leases, domain deadlines and windows are
unchanged. This is four report targets, not authored reports for every team,
survey completion or queued notification delivery. Live help/clarification,
browser offline recovery, remaining lifecycle variants, consented media/native
surveys, non-winning showcase, archive/owned outcomes and final full Fleet,
production-image/recovery/review/CI acceptance remain required.
**complete_lifecycle=false; no original acceptance checkbox or L01–L18 row is
fully complete.**

## Connected native live-help claim race

Frozen source `72379c1163740bc9a788944b251c9e0e21e2fb40` on merged
main `fdb5282a9e3b5042a19a70fe85f21b3f1ee7b607`/schema55 passed
runner20523 in **479.73s**, build85654/start50009, immutable verification image
`sha256:ebd202cf99e59a311700f8699ca60522237c62b0490cabdce9ae4f82bf1e7993`.
This is the first native execution of the help extension. Both retained events
again traverse the real registration, admission, team/common freeze, GitHub
setup/access, practice, live evidence/cutoff, independent judging, results
publication and own-team feedback paths. It is not a production-image proof.

Before live, a current team member's output-help create returns the exact native
`event_blocked`/`help_output_before_live`409 and subsequent read404. During live,
the member creates a fresh waiting request; another team cannot read it. Both
currently assigned mentors first read claim authority, then issue distinct
single-shot commands concurrently against revision1. Exactly one returns200
and one409. The winner owns revision2 and resolves the request at revision3.

Read-only native metadata corroborates two resolved requests, six retained
commands and six ordered transitions. Each history is exactly create by the
actual requester, then claim and resolve by the winning mentor. Neither losing
claim has a retained command; neither rejected prelive ID has a request. The
[bounded receipt](evidence/journey-live-help.json) contains only synthetic IDs,
counts, enum states, original intent/revision metadata and private artifact
hashes. Help prose and all private runtime exports remain local. The isolated
project's containers, volumes and network were removed and verified absent.

The actual factory passed its canonical API schema control after rebase
(package0.507s); configured lint was clean on the same help source before the
patch-equivalent rebase. No production source changed for this checkpoint.
Reviewed common clarification/correction remains next. Its native publication
commits private inbox resource references in the owning transaction; this must
not be described as queued River delivery. Other help authority/escalation
variants, actual browser disconnection recovery and the remaining full
acceptance matrix stay open. No original #120 checkbox or L01–L18 row is
declared fully complete.
