# Fleet cutover — in-progress delivery (#245)

This is the full cutover workstream, based on main
`2d82ac402fc68e39af0c49708fa6f363742bbd66`. It is **not Ready**, does not close
#245 and does not claim the complete first-release UI exists. The accepted source
is #227's Fleet preview and pinned 100-quality study, with the production Tailwind
tokens, primitives and task patterns. No business policy or assessment gate is
replaced by a visual migration.

## Acceptance ledger

| #245 criterion | Current evidence and remaining work |
| --- | --- |
| Every first-release route/nested owner, actual API integration, no legacy fallback | Source inventory resolves 73 build/route registrations: 38 administrator and 35 participant, covering 38 unique registered paths. The display and static guide are separate entries. Existing domain evidence is attributed below. #97, #111, #113 and #163 are still OPEN and their missing final surfaces prevent full acceptance. Native smoke/complete final route walkthrough is pending. |
| Reference evidence across portals, mobile actions, continuity and meaningful icons | Existing domain before/after artifacts and current production components are mapped below; the 100 original quality identities retain evidence/applicability links in `quality-evidence.csv`. This is an evidence index, not a claim of 100 universal pass results. Current cutover-specific finite and native proof is recorded separately. |
| No obsolete presentation/imports/competing tokens; generated API/invariants intact | Production import/caller audit is in progress. Existing flat semantic aliases with live callers cannot be removed merely because their name contains “card”. The bounded resource-lifetime correction preserves native endpoints, CSRF, If-Match, exact responses, cutoff and current authority. Generated contract parity and final broad checks remain gates. |
| Versioned production guide, contribution rules, independent availability | Existing independent production host/build and contribution rules are retained. A current isolated image and separately browsed guide must be proved; an already running guide at 8082 is not candidate-release evidence. |
| Current-head CI; optional M5 not a blocker | Draft checkpoint only; no exact-head CI success claimed. #165/#167 remain optional M5 and do not block first-release cutover. #121/#122/#120 retain their separate release scopes, rather than being replaced by this audit. |

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
| Repository administration and own invitations | `../fleet-repositories/contract-map.md`; #97's frozen-submission consumer is still missing |

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
- Complete the actual #97/#111/#113/#163 routes through their owning tickets;
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
