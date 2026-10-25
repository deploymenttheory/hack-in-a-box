# Today source ownership and verification boundary

Issue #88 / LIV-04. Base `bc350d8c3145e8cb1787a840568a8f7b42cf349f`.
This is the full currently implementable Today surface, still awaiting browser,
HTTP/PG parity, visual and final review/CI verification. Current judge assigned preparation and released-document acknowledgements use
the existing owners and must be verified as that role’s available assigned work.
Final scoring assignments remain owned by #108; an explicit unavailable state is
not proof that judges have no assignments and does not claim a scoring journey.

## Read projection and owning facts

`GET /events/{event_id}/today` supplies current native Event.Read, only the current
actor's active grants/competition state, and the published room current/next
program. Event.Read's organiser-only raw configuration schedule remains private.
The display owner shares its exact content release/current/next/override/timer
projection with Today. The read includes no drafts, controller capability,
pairings, display bearer or heartbeat, and does not initialise presentation state.
Archived/cancelled operational display state is not revived by retained event
reader access. The owning clock remains server-confirmed.

Today owns no write, notification record, parallel task list or completion store.
The frontend projects existing API observations:

| Context | Existing owning facts and real destinations |
| --- | --- |
| Personal | Own registrations/offers; current team/freeze; assigned preparation; paged required released-document acknowledgements/rules/support; published-team submission metadata; credential metadata and own repository invitations; help and official clarifications. |
| Mentor | Published own coverage/window/slot; authorised paged review metadata; own role tasks; help and released guidance. |
| Organiser | Native lifecycle blockers, preparation exceptions, private staff schedule review, content release readiness and authorised paged review exceptions. |
| Judge | Current grant, own preparation/released guidance; final assignment/score domain unavailable pending #108. No fabricated zero/none assignments. |
| MC/logistics/photographer/privileged access | Current grant and authorised assigned preparation/guidance; presentation/logistics destinations only under their existing capability. |

A context chooser does not confer a role. Native APIs remain the permission
boundary. The initial native admission/offer workspace continues to own users
without Event.Read; an unaccepted offer never gains Today/event-reader access.
Team-resource reads wait for a published own membership, and repository invitation
reads wait for actual common release as their native owner requires. Old GitHub
identity/link-revision grants cannot be reported as current accepted access.

Team freeze, actual challenge release, per-team draft/formal submission, assigned
mentor review and the common cutoff are rendered separately. Preparation checks
and optional guidance add no new competition lifecycle gate. Frozen smaller teams
may continue; no post-freeze reassignment is offered. Disbanded repositories remain
unused/unscored. Current observations never settle another owner's unknown write.

## Private lifetime and navigation

Actor/draft/event/current-role/stage boundaries own local read snapshots. There is
no raw QueryClient/browser storage state. Current authority changes synchronously
fence old child requests; denial clears private data and stays latched until an
explicit current read. Held old reads cannot restore a revoked projection.
Temporary failure retains the last observation and pauses actions. Polling never
restarts a still-pending read; explicit reads may replace it. Session end fences
all descendants. Native router navigation preserves existing unsent/pending/
unknown guards; local destinations invoke the existing parent guard/owner.

Private Today projections are excluded from print. It never reveals credentials,
fetches raw team drafts, edits tasks, accepts offers, submits evidence, completes
reviews, settles unknown provider outcomes or calls a real provider.

The independent style guide imports actual production `TodayView` and
`TodayLifecycle` with a backend-free synthetic read owner. Its snippets and
accessibility guidance demonstrate role context, explicit owner navigation,
keyboard return, temporary failure/denial and a changed server observation that
cannot publish a challenge or open a submission. The guide imports no auth/API
adapter or recovery store.

## Checkpoint evidence (not final acceptance)

- Additive contract generation and native package compilation passed.
- Real PostgreSQL current roles/revocation/outsider/disabled reader, unapproved
  room draft/private staff schedule exclusion, actual display projection parity,
  no display heartbeat/state initialisation and unknown-event semantics passed.
  The initial assertion incorrectly searched `draft` anywhere, including the
  legitimate event stage; exact forbidden JSON property checks corrected that
  fixture assertion. The later archived-program branch and HTTP contract fixture
  still require their bounded real-PG run.
- The complete frontend/production-guide source passes TypeScript and focused
  lint. Initial root-shared dependencies lacked QR packages; checks use the
  complete locked dependency tree read-only, with private build-info output.
- Browser role/readiness/clock/privacy/navigation journeys, guide verification,
  matched before/after screenshots, full builds/checks and independent review/
  exact-head CI remain pending. There is no all-roles/whole-issue completion claim.

### Independent administration destinations

`HACKBOX_ADMIN_ORIGIN` is optional outside Compose. It is validated as an origin with no credentials, path, query or fragment and must use HTTPS in production. It does not replace `HACKBOX_WORKSPACE_ORIGIN` or confer a role. Compose supplies the existing independent admin origin to the public runtime as well. The authenticated Today read exposes only this trusted origin, so a participant-portal staff action can link to `/events/{event_id}?section={mounted_section}` in the separate administration portal without guessing a host or port. Each destination validates the requested section against its own current capability registry; forged/unavailable sections show the ordinary overview. Existing session/departure barriers remain active. Without configured origin, an unavailable admin action is labelled explicitly rather than silently opening the wrong workspace.

Current checkpoint verification: native Today current-role/revocation/archive and existing published-display release/privacy/timer parity passed against disposable PostgreSQL. The actual HTTP current-role/no-store/unknown-event/received-unconfirmed denial proof passed; its initial fixture incorrectly used `offered` as a competition claim, which the native schema forbids. The corrected fixture uses `received`; no access assertion was relaxed. Optional administration-origin config and authenticated generated HTTP response proof passed. New read-lifetime and trusted-navigation units passed (4); focused lint and TypeScript passed. Newly authored native browser journeys remain pending execution, as do full guide, production builds and matched visual evidence. These checkpoint results do not claim full issue acceptance or final CI success.

### Assigned role preparation and original write outcomes

The personal preparation owner reads its own current actor-role/participation context and retains only normalized role metadata. Assignment uses the current native owner ID and owner role facts: organiser visibility over every task does not make every task personally assigned. Compatible organiser/judge actors can open their assigned judge task; unrelated manager-visible tasks remain in administration. Unchanged role polling preserves unsent work. A changed assignment/role boundary fences old private reads and writes before replacing the editor.

For this bounded owner, `useResourceOperation` accepts an optional `parentScope`, `safeCommands` from the existing authenticated `CommunicationSafety`, and a UUID `resourceID`. Default callers are unchanged. Pending metadata contains only operation/action, original revision, task UUID and local intent UUID; raw definition, reason, evidence and response stay local. The observer above capability-keyed planning owners selects only the exact actor/draft/event preparation namespace. Pending requests retain their departure barrier. Unknown outcomes require an explicit departure choice and are never replayed automatically. The native adapter validates the original direct response before settling safe metadata, even after private teardown; current GETs never settle it. Personal preparation review dialogs explicitly exclude portalled private content from print. Other default resource-operation capability-boundary behavior remains a separate #245 audit concern, not proof supplied by this owner.

### Guidance continuity checkpoint

Today composes the existing Fleet `EventGuidance` owner for rules, safe contacts, exact-version acknowledgements and its established focus/denial/partial-restoration behavior. The queue omits duplicate guidance IDs; other required released documents retain their owning reader links and opaque pagination. Today's inherited guidance/read scope has a fresh mounted query, fences late effects, and excludes its portalled reading review from print. This parent-enabled reader uses authenticated safe metadata under an exact actor/draft/event/resource UUID namespace and the fixed label `Resource acknowledgement`; native returned ID/version/acknowledgement validation precedes original-ACK metadata settlement. Non-parent reader behavior remains separately scoped under #245.

The published50dc regular-portal failure was diagnosed from its preserved actual CI DOM/trace: `/today` returned the legacy direct Event fixture, not TodayPage. With only the typed fixture corrected, the unchanged current-version journey still failed with a visible Today queue but absent owning-reader controls. Composing the actual owner turned both unchanged corrected-version and safe contact journeys green (2/2). Seventeen relevant units passed, including original ACK validation, private teardown, current-GET-not-receipt, default-reader regression and safe namespace/barriers. Parent current-authority integration, remaining browser inventories, full guide and matched visual proof are still pending; this checkpoint is not final acceptance. The separate scorecard migration fixture support commit pins a deterministic ordinary date outside London's DST fold; production timezone rules and rollback assertions are unchanged.

### Parent event authority checkpoint

The existing canonical event GET is observed synchronously, including the portal header's shared query observer. Today signals a mismatch for a fresh parent GET; it never overwrites that cache with a held Today event. Equal entity revisions do not imply equal actor capabilities. Changed native parent capability/stage observations fence old child tickets before React teardown, then renew the committed private generation. Definitive401/403/404 keeps a denial latch and removes only this actor/event's cached payload; only a successful actual event read restores it. Unchanged authority and metadata-only revision refreshes preserve unsent work. Three actual-hook tests cover equal-revision held reads, denial/cache isolation/recovery and genuine unsent input.

Today and its personal preparation/guidance descendants inherit that read lifetime. Personal task filtering also requires the actor's current required role, even when an organiser can see an earlier task whose owner ID still points to them. Administration visibility is not personal assignment; personal totals count only actual current assignments. The exact organiser+judge missing-role case and native release→draft→live→cutoff phases pass at both widths in both portals (12 cases). Parent/held-write/reading-ACK cases pass12 per portal; guide context/focus/clock/denial/print and no-private-store/API proof pass2. The initial held-parent browser trigger addressed Document rather than React Query's actual Window visibility listener; the trace showed no later parent GET. Correcting that trigger preserved and passed the full authority assertions.

Pending-modal history is inspected as intentionally inert background metadata, then the tests require actual mounted private-editor/dialog removal and visible history after teardown. The independent guide journey navigates to the real Task patterns preview iframe. These corrections preserve the original pending/unknown/revision/no-replay/private-data assertions. Full final owner/shared-consumer inventories and matched synthetic visuals remain pending before review/merge.
