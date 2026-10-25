# Repository readiness administration: native ownership and verification

Issue #94. Repository administration and own-team readiness are implemented on
merged archive main `c759a42e3d8561326b976db1ecf1966c315599d6`. The draft retains
Refs #94 until final frozen reviews and all actual CI complete. Live GitHub App
sandbox verification #91 is separate and remains open; this work uses synthetic
providers only.

## Complete native surface inventory

| Surface | Native contract and constraints | Final owner |
| --- | --- | --- |
| Repository collection | GET `/events/{event_id}/repositories` returns a complete authorized array, without cursor/search/total. Staff sees current mapped teams; regular UI additionally proves published own membership and native own-invitation authority. | `repository-workspace.tsx`: named Fleet collection, readiness, diagnostics, unused state, current mapping count and responsive selected inspector. |
| Selected repository | Current event/team/organisation/name/numeric repository binding. No inferred grant, acceptance, publication or score. | Fresh private inspector, visible current-read action, confirmed native GitHub link; keyboard return to actual opener. Current denial clears selection before partial restoration. |
| Organisation/template | GET/POST `/github-app`, `GitHubAppStatus`/`GitHubAppCommand`; current StaffRead, guarded EventEdit/CSRF/IfMatch. Native rotate re-encrypts the stored key, without a new private key or GitHub key rotation. | `repository-setup.tsx`: current permission/provider diagnostics; configure/re-encrypt/disconnect review. PEM/webhook remain local and never enter review summaries, history or storage. |
| Installation | POST `/github-app/installation-authorizations` returns native actor/session/event/revision-bound single-use GitHub URL. Does not bump event revision. | Review actual saved App/organisation, validate exact GitHub App destination; acknowledge only own draft before intentional navigation. Parent/config/session loss fences held redirects. |
| Initial provisioning | POST `/repositories` queue is event-wide, has no team_id and retains successes/provenance idempotently. | `repository-provision.tsx`: explicit published-team initial batch, exact template/org/team review and acknowledgment separate from provider completion. |
| Unknown create | POST reconcile requires selected team_id; recovery is lookup-only. Missing never authorizes another create. | Deliberate selected uncertain mapping review; no fake selected queue/retry, no automatic resend. |
| Access planning/change | GET access current plan/grants/execution/authority; POST review/approve/refresh retain original IDs/version/reason/revision. Approved action is not delivered/accepted access. | `repository-access.tsx`: exact members/actions/coverage/flags/numeric repo, current observation, immutable reviewed intent, actual response matching and retained original tuple retry only. |
| Original receipt recovery | Authorized read-only historical receipt GET by plan/action, current organizer, immutable event/team/numeric repo and original actor. Absence proves nothing. | Native `receipt.go`, HTTP/OpenAPI/client and shared deterministic command fingerprint; current GET flags never settle original unknown. Writes-disabled current organizers retain read-only original recovery. |
| Own invitations | Native current person’s verified GitHub ID/link revision and own grants only. | `repository-invitations.tsx`: pending/accepted/ambiguous effects, earlier identity warning and account verification link. No fake accept/invite/transfer command. |
| Private ownership | Per-opening fresh native reads and inherited cancellation; no cache-as-authority. | `repository-read.ts`, local command/config fences; synchronous parent denial invalidates child continuations before teardown. Transient failures keep local unsent fields and pause commands. |
| Original command safety | Authenticated actor/session/event/resource safe metadata above private owners. | `repository-command.tsx` plus whitelisted opaque intentFingerprint only. No raw key/webhook/reason/body/people in shared state, logs or storage. Actual original receipt only; no revised IfMatch recovery. |
| Published guidance | Actual production Review/Plan controls, no API/auth/provider traffic in guide. | `styleguide/repositories.tsx`, usage snippet and accessibility documentation; synthetic pending/unknown/stale/correction/focus examples. |

## Domain constraints

ADR-0005 disbanded repositories remain unused/unscored. Repository controls never
change team assignment, reopen freeze, execute participant code or delete/transfer
repositories. Provider creation/acceptance, queued work, pending invitations,
accepted grants, ambiguous effects and original command acknowledgement remain
separate. Operational activity never accepts a submission or changes a score.

Authority is always native current event/resource policy. Frontend event flags
only control discoverability; current reads and guarded native writes decide
access. Every private owner uses fresh per-opening current GET, cancellation and
late-response fences. Current denial removes private detail before restoration;
restoring a parent alone cannot reuse an old access projection. No report/export
capability is invented. No private keys or installation tokens are previewed,
reported, cached globally or persisted in the browser.

## Final verification gate

Owner acceptance is complete at runtime source
`bea455107aab4b90f915b349cdc9105942c3e817`; evidence and tests are included in the
final candidate. Historical checkpoint sections below remain attributed to their
executed source. The remaining merge gates are frozen independent Standards/Spec
reviews and all actual final-head CI; live provider sandbox #91 stays separate.

Native PostgreSQL/race contracts remain in `internal/repositories/service_test.go`,
`internal/repositoryaccess/{service,receipt}_test.go` and
`internal/httpapi/repositoryaccess_test.go`. They preserve external-effect
boundaries, current authority and exact immutable historical receipts.

## Executed first checkpoint proof

- Lint PASS: `/private/tmp/hackbox94-first-lint.log`.
- Both production portal builds PASS: `/private/tmp/hackbox94-first-build.log`.
  Initial shared older node_modules lacked merged QR dependencies; isolated locked
  `npm ci` resolved that dependency mismatch without source/package/lock changes.
- Actual synthetic native collection→inspector→keyboard return: desktop/mobile
  **2/2 admin + 2/2 regular PASS**, logs `/private/tmp/hackbox94-first-{admin,public}.log`.
  Exact team/numeric repository, staff gaps, disbanded state, no regular-portal
  staff access read, no domain/provider writes and no horizontal overflow checked.

This is first-slice proof only. No command/guide/full-acceptance proof is claimed.

## Expanded source checkpoint — not completed UI verification

The draft now contains App setup/rotation/disconnect/installation review,
event-wide published-team queue and selected-team lookup-only reconciliation,
exact access review/approval and distinct provider/grant status, own invitations,
private drafts and safe actor/session/resource metadata above private owners.
Regular UI projects the current own team and native own-invitation authority;
organiser flags do not expose administration. Pure production review controls
are imported by the synthetic independent guide. Expanded UI/guide source still
needs fresh compile/lint/unit/browser proof; first-slice proof does not cover it.

A bounded authorised native addition closes the original access receipt gap:
GET `/repositories/{team_id}/access/receipts/{plan_id}?action=review|approve`.
It requires current organiser StaffRead and original creator/approver, binds the
immutable numeric repository and returns the requested historical definition or
approval. It never calls GitHub or writes audit/jobs/revisions. A missing receipt
never proves no effect. Original write bodies and IfMatch are unchanged. Updating
IfMatch could derive a different original review, so it is never an exact retry.
No migration. App/provisioning unknown outcomes retain safe metadata and are not
blindly resent; their current state reads are not original receipts.

Command body identity is SHA-256 of a versioned UTF-8 byte-length-framed tuple:
action, original actor/event/team, numeric repository, plan ID, native review
command version zero or approval plan version, and Unicode-trimmed reason.
Shared synthetic golden vectors cover Unicode, literal markup and delimiters.
Optional `intentFingerprint` metadata accepts only 64 lowercase hex characters.
No raw reason, people, keys or command body enters shared memory/storage. Browser
capture occurs before POST and exact receipt recovery recomputes the body digest.

Executed native proof: `HACKBOX_INTEGRATION=1 GOMAXPROCS=2 GOFLAGS=-p=1 go test
-race -p 1 -parallel 1 ./internal/repositoryaccess ./internal/httpapi -run
'TestOriginalRepositoryReceiptCurrentAuthorityAndNoEffectsPostgres|TestRepositoryCommandFingerprintBrowserParity|TestRepositoryAccessHTTPPrivacyCurrentAuthorityAndExactReview'
-count=1` PASS. Real PostgreSQL service/history/privacy/no-effect and Go shared
fingerprint parity pass (9.447s package); actual HTTP historical receipt,
committed/lost reply → changed revision → original tuple conflict → exact GET,
role/member/other-actor privacy and zero event/audit/jobs/provider effects pass
(7.044s package). Log `/private/tmp/hackbox94-native-receipt-proof.log`.
Go/TypeScript contract generation PASS: `/private/tmp/hackbox94-generate.log`.
No SQL definitions changed. Browser-side vector/unit proof remains pending.

An obsolete interim compiler reported nullable-plan accesses now fixed in source;
no current-source compile is claimed. Its preceding lint passed only at that
intermediate executed source. The compiler naturally ended with that failure
before a proposed stop; no running test was abandoned. All proof handles terminal.
Remaining: fresh serial compile/lint/units, complete command/secret/unknown/current
role/partial restoration/held-read/late-response browser journeys in both portals,
independent guide proof and matched actual visuals, full exact-head checks and
paired review. No readiness, merge or issue closure is claimed.

## Current boundary/guide checkpoint on merged MC base

Rebased onto `18b23f4e3201f9ab0d10ef3875628585f0ffd74f`, preserving MC production
styles, guide examples and tests. Actual draft CI exposed only Go formatter
issues and a shared fingerprint-vector file outside Docker's web COPY context;
those are corrected without native behavior changes. Golden vectors now live in
`web/src/repository-access-fingerprint-vectors.json`, shared with the native test.

The repository root now supplies synchronous parent authority to every nested
private read/write/install continuation. Enclosing event-cache denial is also
fenced before React teardown. Transient reads with retained data keep private
editors mounted and pause changes. Current native team names identify mappings;
raw team/numeric repository references remain in the inspector and exact review.
Disbanded mapped repositories retain read-only original receipt recovery, with
all new approval/retry/assignment controls unavailable.

Executed: both production builds pass after the parent-boundary changes and
rebase. Actual original receipt/browser recovery **6/6 pass** on desktop/mobile,
including lost committed reply and later revision, wrong/absent receipt refusing
to settle, immutable original retry, capability loss, receipt denial and held
fresh restoration (`/private/tmp/hackbox94-receipt-recovery-green.log`).
The actual parent-event QueryCache denial unit checks a child ticket before React
unmount while safe original metadata survives. Removing only parent scope
inheritance reproduced its precise red; source was restored and **6/6 focused
units pass** (`/private/tmp/hackbox94-parent-mutant-red.log`, mutant diff and
`/private/tmp/hackbox94-boundary-units.log`).

Actual guide CI ambiguity was reproduced **2/2 red** locally: workflow and snippet
copy status both matched an unnamed status selector. Naming the actual workflow
status keeps the snippet status separate; the original corrective focus and
unknown/current-read assertions now **2/2 pass** with no timeout/assertion
weakening (`/private/tmp/hackbox94-guide-focused-{red,green}.log`). Full guide build
passes. An initial local green attempt used the wrong working directory for the
standalone host; corrected web-directory execution produced the passing result.

Expanded full native browser/guide/privacy/freeze/visual proof and final checks
are still being completed. No readiness or issue closure is claimed.

## Verification checkpoint before archive review scheduling

The native App rotate adapter now follows the existing at-rest re-encryption
contract: no replacement private key is sent and installation is retained.
Actual stale source disables confirmation while leaving correction available;
source/permission/provider guidance remains distinct. Current readonly authority
also fences in-flight writes independently of retained read authority. Exact saved
App identity/revision is captured for installation; a known configuration change
fences old navigation without losing local drafts. Acknowledging an installation
settles only its own synchronous leave bit, preserving warnings for another owner.
The successful/other-owner cases use a synthetic intercepted GitHub destination;
no real provider request occurs.

Fresh full administrator/common browser campaign: **31 pass, 3 fail, 6 regular-only
skips** (`/private/tmp/hackbox94-complete-admin.log`). The three failures identified
mobile collection-toolbar use while the inspector is selected, and a duplicate
beforeunload prompt caused by an acknowledged request's stale own pending bit.
The visible inspector read and synchronous owner-only settlement fix those actual
paths. Focused desktop/mobile parent denial, disbanded historical recovery,
readonly authority, successful handoff, changed configuration and other-owner draft
protection: **12/12 PASS** (`/private/tmp/hackbox94-handoff-mobile-green.log`).
Fresh portal builds pass (`/private/tmp/hackbox94-handoff-build.log`).

Regular parent-denial focused proof: **1 desktop pass, 1 mobile fail**
(`/private/tmp/hackbox94-regular-denial-green.log`). The held fresh authority and
old-invitation absence assertions pass. Mobile then automatically reopens the
previously selected inspector after fresh restoration, so the collection's
Inspect action remains hidden. Selection is now explicitly cleared on current
parent denial; this last source correction is preserved but NOT yet rerun or
compiled. No hidden-control force click or weaker privacy assertion was used.

A separate pure fixture support commit `74978274e9d7be4098dc191819af6e45c2f25b10`
pins scorecard migration time to an ordinary fixed instant. Actual PostgreSQL/race
at the London fallback instant reproduced the original ambiguity red; the same
unchanged migration/guarded-rollback assertions pass at the fixed ordinary date
(`/private/tmp/hackbox94-scorecard-{fold-red,fixed-green}.log`). Production DST
validation is unchanged. This support is shared with the other active lanes.

Remaining before full #94 acceptance/freeze: verify the final selection-reset
correction; run final full repository browser/units/lint/guide evidence, final
native receipt assertions (including readonly access/digest), actual before/after
visuals and documentation; obtain exact-head checks and paired independent review.
All owned processes at this scheduling checkpoint are terminal. No claim of
readiness, completed issue acceptance or merge is made.

## Final owner acceptance

- Runtime source `871fed83aeffa3a226cd227bf1191db411377405`: full lint and both
  portal production builds PASS (`/private/tmp/hackbox94-final-{lint,build}.log`).
  Exact regular denial/held-read/partial-restoration proof **2/2 PASS**
  (`/private/tmp/hackbox94-final-restoration.log`). Complete scoped administrator
  journeys **38 PASS / 6 regular-only skips**, regular journeys **10/10 PASS**
  (`/private/tmp/hackbox94-final-{admin,public}.log`). No assertions, native
  original tuples, unknown barriers or provider/acceptance distinctions weakened.
- Runtime source `bea455107aab4b90f915b349cdc9105942c3e817`: bounded review labels
  are copied from current authorized event/team/repository context into the
  private original review snapshot before asynchronous hashing. Readable labels
  lead, immutable IDs/revision remain supporting evidence. Native body/hash and
  global safe metadata are unchanged. Targeted lint and both builds PASS
  (`/private/tmp/hackbox94-label-{lint,build}.log`). Final added proof lint/types
  PASS; **10/10** desktop/mobile focused journeys PASS, including original labels
  surviving current rename, exact approval/correction focus, unknown original
  receipts, capability loss and held outdated installation suppression
  (`/private/tmp/hackbox94-label-{proof-lint,proof-types,browser}.log`).
- Actual published `75137fe` native Go CI completed SUCCESS, run40141029157,
  job121012621092. This includes the real PostgreSQL/race and HTTP immutable
  original-receipt/current-actor/current-role/no-effects proofs. Native code has
  not changed afterward. Actual `871fed8` quality CI (full lint + units), full
  independent styleguide, image, Go lint and patterns completed SUCCESS.
  These executed checkpoints do not replace required final-head CI.
- The unrelated existing mobile campaign CI failure at `75137fe` was inspected
  from actual artifact12578374453, run40141029178. Save intentionally opens
  Review & TEST; adding a manual test navigation would mask that contract.
  The unchanged exact case passed **1/1 + 10/10** locally, logs
  `/private/tmp/hackbox94-campaign-{exact-red,repeat}.log` (the first log's filename
  does not imply failure). No campaign code, assertion, timeout or retry changed.
  Any recurrence must be diagnosed from its new actual artifact.
- Sixteen real synthetic before/after PNGs were captured and inspected, with
  zero domain POST and no real providers. The baseline had no repository page;
  its adjacent task context is explicitly labeled. Source identity, methods,
  terminal capture results and artifact hashes are in
  [visual-evidence.md](visual-evidence.md). The independently published guide
  imports actual production review/plan controls with usage/accessibility notes.

No repository action changes allocation, reopens freeze, accepts an entry or
scores a repository. Queued, accepted, pending, delivered and ambiguous effects
remain distinct from original command acknowledgement. No App key, webhook,
raw private reason, people projection or exact command body enters shared safe
history or browser persistence. Live App installation/provisioning #91 was not
performed or claimed complete.

## Final rebase preservation

Rebased onto actual main `c759a42e3d8561326b976db1ecf1966c315599d6`.
The sole conflict was the generated embedded OpenAPI byte blob. Native Go and
browser contracts were regenerated from merged YAML, preserving archive APIs
and repository original-receipt APIs. The equivalent scorecard migration-clock
fixture commit was dropped automatically because it is already on main.
Runtime `web/src` diff against tested pre-rebase `d2152ab` is empty except erased
generated API types; no screenshot or runtime UI changes were introduced by the
rebase. Final Go/TypeScript contract regeneration parity is recorded in
`/private/tmp/hackbox94-rebase-parity.log`. All required frozen reviews and actual
final-head CI remain merge gates.

## Final review correction: native write authority

The final Standards review identified two authority defects in `51a6bef`:
Teams `can_manage` grants StaffRead even when native Event `can_edit` is false,
and a successful current parent Event response losing writes did not fence
held command continuations inside that same cache notification.

The bounded correction requires native EventEdit as well as current Teams
capability before exposing any write controls. The latest matching cached native
Event flag is checked after collection awaits and again at rendering; an older
held collection closure cannot renew writes after a known current write loss.
A successful matching parent Event response with `can_edit=false` immediately
invalidates the distinct write scope only. Authorized readonly history and
original-receipt scopes remain valid, and global original-command metadata is
not removed. Error/denial fencing and native command bodies remain unchanged.

Executed tight proof, without timeout/retry weakening:

- Before correction: `npx vitest run src/repository-read.test.tsx --maxWorkers=1`
  gives **1 PASS / 1 FAIL** in 1.37s. The successful-parent case fails exactly
  `command.current(): expected false, received true` inside the cache notification,
  while the child is still mounted. Existing 403 behavior passes.
  Log `/private/tmp/hackbox94-write-loss-red.log`.
- Before correction: `npx vitest run src/repository-authority.test.tsx --maxWorkers=1`
  gives **2 FAIL** in 1.72s. Both actual Workspace cases incorrectly retain the
  App private-key textarea: initial readonly Event with Teams StaffRead, and a
  held older collection resolving after known current parent write loss.
  Log `/private/tmp/hackbox94-readonly-promotion-red.log`.
- After correction: the exact two files give **4/4 PASS**, with additional
  assertions that the readonly child ticket stays current and original pending
  metadata/fingerprint stays present. Final log
  `/private/tmp/hackbox94-write-authority-final-units.log` (3.51s).
- Targeted lint is recorded in `/private/tmp/hackbox94-write-authority-lint.log`.
  Updated real-browser fixtures now retain native Teams StaffRead on readonly
  EventEdit loss, exercise actual successful parent refresh and readonly
  historical-original recovery, and include initial readonly setup/control
  assertions. These updated browser cases and full types/builds are deliberately
  **not claimed as executed locally**; mandatory final-head CI supplies that
  proof, followed by fresh independent delta Standards/Spec review.

Permitted-state appearance is unchanged, so existing actual visual evidence
remains attributed to its captured source. No backend, provider policy, broad
permission store or shared safe-metadata behavior changed in this correction.

## Delta review correction: authorized write restoration

Both delta reviewers found that the previous `wasWritable` Boolean could remain
true when successful parent false→true responses coalesced before a readonly
React commit. A subsequent fresh collection then left the write scope inactive,
although controls were enabled.

A new actual Workspace/App-command test reproduced this: two existing Workspace
cases passed but the restored command silently produced no POST. The stronger
version holds an original command through loss/restoration and fresh collection;
its old private acknowledgment remains suppressed, while a new deliberate command
must POST after the original acknowledgment settles. Before correction it failed
exactly `expected POST 2 times, got 1`. Logs
`/private/tmp/hackbox94-restoration-{red,held-red}.log`.

The correction removes duplicate Boolean bookkeeping. Only the existing fresh,
authorized collection path renews an ended write scope; cached permission alone
does not renew it. Renewal changes the epoch, so original captured tickets stay
invalid. Existing readonly scopes, safe metadata and immutable commands are
unchanged.

- Exact authority suite **5/5 PASS** (3.41s), including all four prior regressions
  and the held-original/restored-new-command case:
  `/private/tmp/hackbox94-restoration-green.log`.
- Targeted ESLint **PASS**: `/private/tmp/hackbox94-restoration-lint.log`.
- One full current-source TypeScript check, `npx tsc -b`, **PASS**:
  `/private/tmp/hackbox94-restoration-types.log`.
- Actual prior `b5210df` CI compiler failures were unsupported `exact` options on
  Testing Library role queries, not browser execution failures. Removed those
  options; string accessible-name matching remains exact by default. Behavioral
  assertions were retained. Full browser/build proof still belongs to mandatory
  final-head CI; it was not rerun locally or claimed successful here.
