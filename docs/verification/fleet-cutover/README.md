# Fleet cutover — in-progress delivery (#245)

## Current final-parity workstream — 25 October 2026

The earlier checkpoints below retain their original sources and limits. The
current final workstream starts from the reviewed showcase candidate63844452,
whose tree equals merged main076f6162. It completes the whole #245 cutover;
this checkpoint is Draft and does not claim release readiness.

The existing source check reproduced13 undocumented registrations on that
baseline. The maintained inventory now includes the qualification panel,
announced/prepared results, every assigned judge ballot, and versioned own-team
feedback routes. Both routers import the reviewed showcase directly; its
obsolete outcome-workspace re-export has been removed after moving both callers.
The source record now resolves94 registrations and100 reference qualities,
with no inventory errors. This establishes source ownership only.

| Complete #245 requirement | Current evidence / remaining gate |
| --- | --- |
| Every first-release route and nested feature has a migrated owner and actual API integration | Current94-route source inventory is regenerated; judge/panel/results/report/showcase owners are now merged. Reconcile each domain's native HTTP and browser proof with final source, including the new #112 mentor advisory journey after its merge. Source presence alone is not native integration. |
| Both portals/mobile/actions/icons and100-quality applicability | Existing source-attributed domain images and100-quality map remain. Complete the current reference-to-product audit and document justified variations; inherited images are not silently relabelled as new captures. |
| No unused legacy presentation/imports/competing tokens; canonical API and business rules retained | Removed the concrete showcase migration shim and both callers now reference its production owner. Full remaining import/token/caller audit and final canonical check remain required. No domain/API/authority policy changes here. |
| Published versioned guide, production components, contribution rules and runtime independence | Full independent guide build passes with both portals. The source11375165 normal-image four-case browser/realOIDC/creation/read/logout/guide/public-process-failure proof now passes with14 original captures and zero remaining resources. Final integration after #112 and current-source reconciliation remain required; the user's normal preview is preserved. |
| Exact-head CI and optionalM5 exclusion | Full web lint,237 unit tests, both portal builds and guide build pass on the working-tree checkpoint. Independent reviews and final full CI remain gates. Certificates/session booking stay optionalM5. Separate #120/#121/#122 release scopes are not replaced by this audit. |

Verification command: `node scripts/check-fleet-cutover.mjs`, followed by
`cd web && npm run lint && npm test && npm run build && npm run build:styleguide`.
The source check first failed with13 missing route registrations; it now passes.
The frontend check log is retained locally as
`/private/tmp/hackbox245-web-checks.log`. No focused current native/browser
matrix, full import audit, runtime-failure proof or final CI is claimed by this
first checkpoint.

---

This is the full cutover workstream, based on main
`2d82ac402fc68e39af0c49708fa6f363742bbd66`. It is **not Ready**, does not close
#245 and does not claim the complete first-release UI exists. The accepted source
is #227's Fleet preview and pinned 100-quality study, with the production Tailwind
tokens, primitives and task patterns. No business policy or assessment gate is
replaced by a visual migration.

## Acceptance ledger

| #245 criterion | Current evidence and remaining work |
| --- | --- |
| Every first-release route/nested owner, actual API integration, no legacy fallback | Source inventory resolves 77 build/route registrations: 40 administrator and 37 participant, covering 40 unique registered paths. The display and static guide are separate entries. Existing domain evidence is attributed below. #109, #111, #113 and #163 are still OPEN and their missing final surfaces prevent full acceptance. Native smoke/complete final route walkthrough is pending. |
| Reference evidence across portals, mobile actions, continuity and meaningful icons | Existing domain before/after artifacts and current production components are mapped below; the 100 original quality identities retain evidence/applicability links in `quality-evidence.csv`. This is an evidence index, not a claim of 100 universal pass results. Current cutover-specific finite and native proof is recorded separately. |
| No obsolete presentation/imports/competing tokens; generated API/invariants intact | Production import/caller audit is in progress. Existing flat semantic aliases with live callers cannot be removed merely because their name contains “card”. The bounded resource-lifetime correction preserves native endpoints, CSRF, If-Match, exact responses, cutoff and current authority. Generated contract parity and final broad checks remain gates. |
| Versioned production guide, contribution rules, independent availability | Existing independent production host/build and contribution rules are retained. A current isolated image and separately browsed guide must be proved; an already running guide at 8082 is not candidate-release evidence. |
| Current-head CI; optional M5 not a blocker | Draft checkpoint only; no exact-head CI success claimed. #164/#165 remain optional M5 and do not block first-release cutover. #121/#122/#120 retain their separate release scopes, rather than being replaced by this audit. |

## Production caller and legacy selector audit

`node scripts/audit-presentation-callers.mjs` follows static imports, reexports,
literal dynamic imports and CSS imports from the normal portal/display entry and
both independent guide entries. At source5d8fd041, all314 source modules are
reachable; no relative import is unresolved. The only Tailwind `@theme` owner is
`design-system/tokens.css`; event palette adapters retain native branding authority.
Reachability does not prove every export/selector is useful or every route works.

The bounded [caller record](presentation-caller-audit.json) identifies retired
card-grid, preparation-board, attendance-tile, credential-table, old media/survey
and other migrated layouts with no complete class token in any reachable TS/TSX.
Their100 simple selector arms and matching responsive/print overrides are removed.
Mixed live arms, current private data-attribute print protection, shared semantic
aliases and dynamically constructed connection-state classes are preserved.
The checker refuses if a retired class gains a possible caller or another token
owner appears. Current CSS/browser/native integration and the full reference
quality audit remain separate gates; this record does not certify full245.

## Reproducible normal-image runtime and browser probe

`python3 scripts/verify-fleet-runtime.py --artifacts /PRIVATE/NEW/ARTIFACT-DIRECTORY`
builds only tracked, committed HEAD into uniquely named normal public/admin/guide
images. Install the locked frontend dependencies and Playwright Chromium first.
The four fixed loopback ports18180/18181/18182/19190 must be free; an existing
project container, volume, network or artifact directory fails preflight instead
of being overwritten. Every run gets a distinct project suffix.

`python3 -B scripts/test_verify_fleet_runtime.py` exercises the two runner
guards without starting Docker services: leftover volume/network refusal and
concurrent live-fixture edits while only the frozen archive is executed. These
are tool-boundary regressions, not application/browser verification.

The probe uses a private PostgreSQL database and a verification-only external
OIDC provider on loopback. The application images run their normal commands,
OIDC/PKCE/nonce/signature/session checks and bootstrap flow. They have no test
clock, test entrypoint, seeded session or successful mocked API response. The
Compose/provider/browser fixture files execute from the same archived source
until cleanup; the normal image revision and locked browser dependency versions
are checked against that archive. Real
browser form → review → native event creation → both portal readers is followed
by independent logout, an unauthenticated guide and scoped public-process
failure while administration/guide continue. Desktop/mobile, light/dark,
finite-animation settling, overflow, serious/critical Axe checks and synthetic
screenshots are part of that bounded probe. It does not prove every domain route
or replace the full lifecycle/security/accessibility release scopes.

Only project-prefixed ephemeral services/volumes/networks are removed in the
runner's finally block. It never restarts Docker or modifies the normal user
projects. Artifacts are private, contain no authentication trace/cookie values,
retain image IDs/source and image hashes, and report zero remaining project
resources after successful cleanup. The external provider script is not included
in the application runtime image. Its identities and credentials are synthetic.

The first runtime attempt at065e2bc3 built all three images and reached healthy
containers, but the host browser failed before login. A guide-only differential
kept the image/port fixed: DockerDesktop29.6.1 internal network produced an empty
published binding and connection refusal; an ordinary dedicated bridge produced
the intended127.0.0.1 binding and HTTP200. See
[the source-attributed differential](network-forwarding-proof.json). This fixture
therefore uses a dedicated bridge, publishes only loopback ports and leaves its
database unexposed. Both probes cleaned their scoped resources.

The next attempt atfc2963eb reached real sign-in in both portals and the
creation form, then stopped before creation at a Playwright datetime-fill
normalisation error. A bare step1 datetime-local input reproduced the same
error: requesting12:00:00 yields the browser value12:00, while supplying the
canonical12:00 succeeds. The fixture now enters that canonical minute form;
the existing application already adds seconds for its native time resolver.
No product input, timing or assertion is weakened.

The corrected normal-image journey **passed all four cases** at11375165,
including actual native200 creation, both surface readers, separate logout,
unauthenticated versioned guide, and admin/guide with public stopped. All14
PNG hashes and scoped cleanup are verified in the
[bounded normal-runtime evidence](normal-runtime-11375165/README.md).
This is not full domain cutover acceptance; final #112 integration/current
source, remaining inventory/caller/quality audit, reviews and fullCI remain gates. The earlier
237-test/source-inventory results above do not establish its success. Preserve
each failed attempt separately, diagnose its actual symptom, then record the
exact executed source and measured result before changing this status.

## Source inventory versus actual integration

Run `node scripts/check-fleet-cutover.mjs`. It parses registered router members,
resolves their actual owner imports, checks the maintained route inventory and
the 100 distinct reference IDs. `route-inventory.json` records the inspected
source hashes and registration list. **The script does not call APIs or prove
authorization, browser operation or backend integration.** It also corrects the
old inventory's misleading display parameter: `/display/$eventId` carries no
pairing secret in the URL; the key travels only in the bearer header.

| Domain / nested ownership | Existing bounded evidence to reconcile with the candidate |
| --- | --- |
| Independent shells, identity/account, event directory and direct routes | `../portal-shells/README.md`; `../../design/portal-shells.md` |
| Event/template/setup, staff, scheduling, inventory, venue/attendance/logistics | `../event-planning/README.md` |
| Admission/offers/application recovery | `../registration-recovery/README.md` |
| Team/mentor allocation, membership/freeze, captain and own team access | `../fleet-team-coverage/README.md` |
| Released resources, acknowledgements, controlled access, personal preparation and deliverables | `../../design/patterns.md`; `../../design/today-workflows.md`; current deliverable lifetime proof below |
| Communications, providers, approved campaigns, inbox/history/unknown effects | `../fleet-communications/contract-map.md` |
| Mentor private work, help, clarification, current scorecards/huddles | `../fleet-mentor-workspaces/README.md` |
| Surveys and protected results | `../../design/survey-workflows.md` |
| Ideas/outcomes, follow-up and current released project projection | `../fleet-outcomes/contract-map.md` |
| Media/consent, protected readers, review and release | `../fleet-media/README.md` |
| MC/pairing/rehearsal, independent room canvas, native interruptions | `../fleet-mc-display/README.md`; `../live-rehearsal/README.md` |
| Today role/task/readiness journeys | `../../design/today-workflows.md`; `../live-rehearsal/README.md` |
| Repository administration and own invitations | `../fleet-repositories/contract-map.md`; #97 delivered the actual assigned-build and submitted-build consumers in merged #278; its native evidence retains its original source attribution |

Some of these records describe their original frozen checkpoint, not this
candidate. Their dates, source hashes, synthetic transports and native/provider
limits must stay attributed. Existing `/projects` and current moderation helpers
do not substitute for unfinished #163 or #111/#113 product scope. Native backend
availability is not a demonstrated frontend route.

## Reproduced team-evidence lifetime failures

The production `TeamDeliverables`/`DeliverableEditor` path was an unscoped
remaining consumer of `useResourceOperation`; the earlier Today correction had
scoped only personal preparation and selected resource-reader paths.

The new production-component/query-cache regressions reproduced three failures
before correction (`/private/tmp/hackbox245-deliverables-red.log`):

- A native teams403 removed the view but retained a private saved evidence URL in
  the query cache. A later partial general access read could not be authority to
  retain or restore that private projection.
- Membership loss while a draft save was pending discarded the only original
  operation record. Both a later direct acknowledgement and a lost response
  therefore lacked safe authenticated tracking outside the disposed owner.

The correction uses current event → current teams → current submission → private
draft lifetimes, fresh per-mount private query keys and explicit current-access
retry. Parent denial invalidates held descendants and removes owned submission
caches. Membership changes affect the selected owner's authority; unrelated team
changes do not intentionally discard its unsent work. Transient read errors pause
actions while preserving the mounted unsent fields.

Before sending, fixed-label intent ID/revision/team reference metadata is retained
in authenticated memory. No evidence, URL, label, reason or team name enters it.
A matching original native response may settle that metadata after private owner
teardown; its private payload cannot write caches or refill fields. A lost response
stays unknown, and a current GET cannot settle it or replay the write. Completion
review retains its independent native source/revision guards. Private confirmation
content is excluded from print.

Four production-component cases passed after correction, including a native
submission-denial/fresh-retry case. The two existing parent-operation lifetime
cases also passed. These tests use the actual components/cache with a synthetic
native-shaped transport; they do not replace the pending PostgreSQL/HTTP/browser
integration and complete Docker/schema proof.

## Coherent draft checkpoint

- Current TypeScript and focused ESLint pass; both production portal builds pass.
- Seven affected owner/safety cases pass (3.11s): four team-deliverable cases,
  two existing original-operation lifetime cases and the authenticated safety
  observer case.
- Existing deliverable and participant-deliverable journeys pass **84 cases**
  across administrator/regular portals and desktop/mobile viewport descriptors.
  The same initial finite run had eight new harness failures, not a clean
  92-case result. Pending-modal metadata required `includeHidden` for its
  read-only assertion, and membership loss required the actual 15-second native
  polling read rather than a window-focus event. The corrected regression also
  asserts hidden private editor removal, waits for that real request and does not
  relax confirmation, receipt, cache or timing checks.
- A clean subsequent selection of the **eight new lifetime browser cases passed
  in 28.7s**, one worker and zero retries. Original matching ACKs settle safe
  metadata after teardown; lost responses remain unknown across section changes.
  No old evidence returns and there is exactly one native-shaped PUT.
- The first sandbox listener attempt failed before executing cases (EPERM);
  authorized local test-owned listeners used ports4185/4285. No user containers
  or listeners were stopped. Log files under `/private/tmp/hackbox245-` retain the
  execution checkpoints; they are not claimed as exact-head repository CI.

`checkpoint-proof.json` hashes the exact changed runtime files and attributes
these selections. This draft remains the complete #245 workstream with the
remaining acceptance ledger intact, rather than closing cutover after a narrow
mocked-screen check.

## Remaining full-deliverable work

- Reconcile the full current nested-owner inventory, the remaining unscoped
  resource/upload consumers and removal of obsolete presentation only after
  proving callers moved. Finish guide usage/example coverage for any changed
  interaction semantics.
- Complete the actual #109/#111/#113/#163 routes through their owning tickets;
  do not fabricate placeholder routes or mark source inventories as integration.
- Capture candidate/current-source browser/native proof, complete the 100-quality
  applicability cross-domain comparison and preserve honest before/after sources.
- Build/run isolated current app images with matching backend/schema and browse
  a separately built static guide. Use test-owned project names, images, ports and
  volumes only; existing app containers and the guide at8082 are off limits.
- Full existing web units/lint/builds/guide, generated parity, relevant native
  integration, independent reviews and every exact-head repository CI check remain
  required before Ready/merge/closure.

## Existing unit-fixture integration correction

The first published checkpoint 204ec348 failed full web-quality CI in two existing team-deliverables unit cases. They mounted the editor without its new authenticated CommunicationSafety owner, then simulated authority loss by updating an obsolete exact cache key. The new private owner intentionally adds a mount lifetime to its query keys. Root reproduced both failures locally, including the missing-provider error.

Only the existing test fixture was corrected: use the actual authenticated safety provider, return changed permission facts from the mocked native read, invalidate the active submission query by its stable prefix, and inspect all descendant draft cache entries. The held-response case still requires the actual read signal to abort before its deferred response arrives. Existing DOM clearing, disabled formal submission and no-private-canary cache assertions remain intact. No production owner, permission, cache policy or assertion was weakened.

The two original cases plus seven adjacent owner/safety cases passed together: four files, nine tests, no unhandled errors, 8.41 seconds. Focused ESLint and diff checks passed. This local proof ran on the working-tree fixture correction before its commit; fresh final-head CI remains required. This fixes the checkpoint's unit-fixture integration and does not establish full #245 acceptance, native Docker proof or missing-route completion.

## Participant CI discovery correction

At checkpoint `1c44ac4`, the ordinary participant configuration omitted
`team-deliverable-lifetimes.spec.ts`: its focused discovery returned zero cases.
Adding the file to the explicit allowlist discovers all four existing cases.
The unchanged acknowledged/lost-response journeys passed on desktop and mobile
through the actual participant configuration: four tests, 4.5 seconds, one worker.
Focused configuration ESLint passed. This is local working-tree evidence before
commit; fresh exact-head CI and the remaining full cutover criteria still apply.

## Rebase after session UI merge

Rebased onto main `b2f7f5a0e128cf6843dbc0250fd29098afb8c00b`.
Configuration conflicts were resolved additively: session and cutover artifact
ignores, all TypeScript entry points, and both participant test registrations.
The shared Today safety observer retains preparation, reading, session-booking
and team-deliverable command histories together. Neither namespace can remove
the other's pending/unknown navigation barrier.

The existing safety regression now emits all four real owned command families,
checks both session/evidence histories before and after lost responses, and keeps
unrelated actor/session/event namespaces untouched. It and six adjacent
deliverable cases passed: three files, seven tests, 1.45 seconds. Focused ESLint
and diff checks passed. Participant discovery includes48 session cases and4
deliverable-lifetime cases (52 total); discovery is not execution. Actual TypeScript build-mode type checking passed
as well. These are local working-tree rebase-integration results,
not complete245 acceptance or final CI.

## Rebase after submitted-build UI merge

Rebased the four cutover commits onto main
`c723e693c21529c8411cae98863abd6e29d37648` after PR #278 merged.
Conflict resolution preserves the submitted-build action link, all accessibility,
booking and submitted-build test registrations, and the cutover private-owner
changes. The Today observer retains all five owned command families:
preparation, reading acknowledgements, bookings, code cutoff and team evidence.
The existing regression now checks code command history before and after an
unknown response alongside the other families, without changing unrelated
actor, session or event history.

Local verification on the rebased working tree: three affected unit files,
seven tests passed in 1.47 seconds; TypeScript build-mode type checking and
focused ESLint passed. These results establish this integration check only.
Fresh published-head CI and the remaining full cutover acceptance are still
required. Issue #97 is now delivered by #278; #109, #111, #113 and #163 remain
separate integration dependencies.

## Protected upload lifetime audit

The remaining protected-upload callers are content attachments, organiser task
evidence and team deliverable evidence. They share the actual `ProtectedUpload`
transport. Its private lifetime already fenced late attachment callbacks, but no
safe original-attempt record existed outside that lifetime: three new focused
cases failed on unchanged production code in 1.05 seconds. This is a separate
missing-history defect, not evidence that late bytes were rendered.

The shared transport now records only a fixed operation label, opaque original
request/event/team IDs and expected revision in authenticated memory. File bytes,
names, MIME metadata and reasons remain in the private owner. Its validated
original response settles the record before checking whether the private owner
still exists. A lost or unrelated response stays unknown; no automatic replay or
GET acknowledgement is introduced. Deliberate recovery while the original owner
exists repeats the same native key, bytes, metadata, team, reason and revision.
The new actor/session observer in both shells preserves history and departure
barriers across form, event and capability changes; expiry clears it.

Five focused upload cases plus seven adjacent owner/safety cases passed in
1.29 seconds. Both production builds, TypeScript and focused ESLint passed.
The actual built admin and regular portals then passed sixteen browser cases
on desktop/mobile in 11.4 seconds, zero retries: the eight existing draft
lifetime cases plus eight held-upload membership-loss cases. These check
matching response versus unknown outcome, no private filename revival, exactly
one upload and no submission write. Browser transports are synthetic; this
does not claim live provider verification or full #245 runtime acceptance.

The current source audit also found both #97 build paths absent from the migration
inventory. Their actual registered owners are now recorded and the source-only
coverage check passes for 77 registrations, 40 paths and all 100 reference IDs.
This updates source coverage, not native/browser proof for every route.

Final local checkpoint verification also passed all 224 existing/current web unit
tests in 8.69 seconds, full ESLint, the independently built guide and its two
protected-upload browser journeys in 9.1 seconds. The guide ran from this
worktree's static build through a separate test-owned Go host on port 4179; it
did not use the existing guide service. That focused guide run proves the
existing upload control interaction only, not every guide pattern or full
independent production availability. The temporary local test configuration was
removed after its process exited. Source hashes and command attribution are in
`upload-lifetime-proof.json`. All results precede the checkpoint commit; fresh
exact-head CI is still required.
