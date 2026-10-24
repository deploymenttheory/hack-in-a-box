# Fleet team formation and mentor coverage verification

Issue [#237](https://github.com/deploymenttheory/hack-in-a-box/issues/237), PR
[#253](https://github.com/deploymenttheory/hack-in-a-box/pull/253). Implementation
started from main `1c94473edb6e91adc42409bfe50317380e96194f`; the branch was then rebased onto merged planning main
`ad634ded92f1b4984ed96b8dd00aec889e89810e` and then registration main
`22a42bc5839bb84a408ebcc41ab24735e27175df`, dropping its duplicate shared
local-record seam and preserving the actual merged parent composition.

## Scope and boundaries

`teams.tsx` owns captain creation, code-based join requests, draft/published member
detail and the confirmed placement pool. `allocation-review.tsx` owns declarations,
server proposals, lock/capacity controls and explicit publication/freeze previews.
`mentors.tsx`, `mentor-editors.tsx` and `mentor-roster.tsx` own capacity, availability,
conflicts, draft slots, publication comparison and permitted published detail.
Parent event composition is #235; registration is #236; provider access is #94/#97;
private mentor observations, help and assessment stay #240.

Both APIs return complete authorised projections, without cursor paging or exact
command receipts. Counts/search refer to those projections. Team `can_manage`
is a read projection and does not grant a write: the admin parent also supplies
current `event.can_edit`. Existing generated types, native CSRF/If-Match, backend
allocation, window resolution, conflicts and freeze/publication gates are retained.
No backend, schema, real provider or authentication policy changed.

The bounded `coverage-command.tsx` adapter fences ended sessions and late results,
blocks duplicate starts synchronously, treats interrupted/408/5xx responses as
unknown, and retains a hard write barrier. The merged planning event-session capsule retains uncertain commands across tab
replacement; pending work prevents departure. An authoritative GET can update current
state but cannot prove the original command succeeded. Unknown intent permits
read-only navigation and refresh, never automatic resend or a fabricated receipt.
`TeamCommand.request_id` still identifies a pending join request, not idempotency.

## Visual evidence

All screenshots use synthetic people and deterministic route fixtures. Baselines
were captured before domain composition changed with the same portal shells and
viewport projects. The final after evidence includes the independently merged #235
event chrome; domain composition ownership remains #237. This is local evidence, not a claim of release certification.

| Surface | Desktop before / after | Mobile before / after |
|---|---|---|
| Allocation publication | [Before](before/teams-desktop.png) · [After](after/teams-desktop.png) | [Before](before/teams-mobile.png) · [After](after/teams-mobile.png) |
| Mentor publication | [Before](before/mentors-desktop.png) · [After](after/mentors-desktop.png) | [Before](before/mentors-mobile.png) · [After](after/mentors-mobile.png) |
| Frozen team outcome | [After](after/frozen-teams-desktop.png) | [After](after/frozen-teams-mobile.png) |

Compared with the accepted [Fleet reference](../../design/fleet-reference/README.md):
flat canvas and semantic tokens (Q01–10/91–93), scoped collections and native actions
(Q13/17/19–29), comparable rows and complete-projection search (Q31–35/41–43/50),
labelled native fields with separate errors (Q51–56/61–70), full-width multi-step
reviews and concise record inspectors (Q75–80), text/icon statuses and progressive
detail (Q81–90), and scope/permission/keyboard/mobile behaviour (Q94–100).

Visual inspection rejected a cramped initial side-pane allocation review. Allocation,
placement, editing, conflict and publication tasks now use the full canvas. Concise
team/mentor detail remains beside its roster at wide widths, becoming the selected
canvas on phones. Mentor publication compares each slot's published/proposed mentor
and exact windows in one record rather than repeated before/after card grids.
No API-less export, bulk apply, total or global search was invented.

## Local execution

- Baseline: 4 desktop/mobile team/mentor publication journeys passed.
- `npm run lint`: passed.
- `npm test -- --maxWorkers 2`: 123 tests across 18 files passed.
- `npm run build`: TypeScript and independent participant/admin builds passed.
- `npm run build:styleguide`: independent static guide build passed.
- Focused admin Playwright: teams, mentors and allocation-lifecycle at desktop/mobile;
  62 journeys passed. Includes exact affected-roster review, cancellation without a
  write, locked draft movement, server proposals, publication/freeze blockers,
  post-freeze withdrawal, capacity/conflict checks, availability/DST preservation,
  stale revision, active access revocation, late event response, cutoff crossing,
  interrupted publication and read-only unknown reconciliation without replay.
- Focused participant Playwright: participant-teams and public-portal; 14 journeys
  passed, preserving confirmed/waitlisted joins, solo placement, no automatic team
  assignment, no private placement pool and read-only published coverage.
- Independent guide Playwright: 30 journeys passed with cookie/private-storage access
  made fatal and no API requests. The added production roster example exercises
  keyboard focus, current-state read and the retained unknown outcome. Duplicate
  sibling keys in the shared local-record seam were corrected using the same `local:`
  prefix as the parallel lanes.
- `HACKBOX_INTEGRATION=1 go test -p 1 ./internal/teams ./internal/mentors ./internal/registration`:
  all three packages passed against real isolated PostgreSQL containers, including
  publication timing, freeze races, withdrawals, current visibility and mentor access.

Local browser commands used untracked equivalent configs on isolated admin/public
preview ports 4197/4297 and independent guide port 4187 with one worker to avoid parallel-lane port/process conflicts.
The committed standard configs and exact-head repository CI remain the merge gate.
Independent frozen-head standards/spec review and CI are still required before ready
or merge; these results do not claim either gate has completed.

## Interactive contribution guidance

The independently published guide includes **Roster planning and uncertain responses**
under Task patterns, importing production `CoverageDetail` and `CoverageCommand`.
Use full task width for roster comparisons, keep exact reviewed scope/revision/reason
visible, restore source focus, and distinguish a current projection from a durable
receipt. Synthetic example state never enters portal consumers. For future changes,
update the owning composition, its guide example, usage documentation and meaningful
privacy/timing/recovery checks together.


## Independent review fixes, 24 October 2026

The reviewed head `92b09c83a66e50de2dc33bf2d823f296e0798354` is preserved
as `backup/fleet-team-coverage-92b09c8`. Targeted actual-browser regressions first
failed on both retained privileged mentor snapshots after a restricted projection,
the second published slot's return focus, and silent nested-capacity loss. Two
participant regressions also failed on record focus and unsent join navigation.

The corrected controls are keyed to current owning-domain management capability,
so fresh restricted coverage immediately unmounts editor/review snapshots without
waiting for an event-query update. Editor time-resolution tickets end on replacement.
A live projection generation also fences old parent command closures and late
privileged POST responses; an interrupted outcome remains unknown rather than
refilling the permitted roster or manufacturing a receipt. The helper's fifth unit
regression holds a response over a fresh restricted read and proves both fences.

Capacity drafts are lifted through review/correction and survive another successful
control change until explicitly saved or discarded. Allocation declarations and
mentor editor inputs report actual dirty state. Nested mentor editors stay mounted
hidden/inert during review, preserving typed availability/capacity on correction.
Local and route discard cancellation retain exact input; accepting discard is the
only unsaved exit that clears it. Pending and unknown outcomes retain their separate
barriers; an ordinary draft-discard decision cannot unlock replay.

Source focus uses the actual initiating control's stable record/action key or unique
native action name, scoped to its workspace. Responsive counterparts are resolved
when returning. Identically named detail/header actions have explicit distinct keys;
ambiguous or removed actions fall back to the named workspace, never an arbitrary
first button. The independently built guide imports this same focus export.

`coverage-review-regressions.spec.ts` now exercises profile/publication downgrades,
held time resolution, nested capacity and declaration cancel/accept, availability
correction/discard, successful capacity focus, exact second-slot return across
viewport changes, and the identically named slot detail/header actions. Participant
journeys cover actual record/review keyboard return and join-draft route cancel/accept.
Existing team/mentor policy assertions remain and now explicitly accept deliberate
discard when their authored journey abandons a typed placement/editor draft.

Final re-review and exact-head full repository CI remain outstanding gates.
