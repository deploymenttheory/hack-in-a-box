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
