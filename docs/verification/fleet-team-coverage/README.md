# Fleet team formation and mentor coverage verification

Issue [#237](https://github.com/deploymenttheory/hack-in-a-box/issues/237), PR
[#253](https://github.com/deploymenttheory/hack-in-a-box/pull/253). Implementation
started from main `1c94473edb6e91adc42409bfe50317380e96194f`; the shared optional
local-record action seam is included separately until the parallel lanes merge.

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
unknown, and retains a hard write barrier. An authoritative GET can update current
state but cannot prove the original command succeeded. Unknown intent permits
read-only navigation and refresh, never automatic resend or a fabricated receipt.
`TeamCommand.request_id` still identifies a pending join request, not idempotency.

## Visual evidence

All screenshots use synthetic people and deterministic route fixtures. Baselines
were captured before domain composition changed with the same portal shells and
viewport projects. This is local evidence, not a claim of release certification.

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
- `npm test -- --maxWorkers 2`: 112 tests across 15 files passed.
- `npm run build`: TypeScript and independent participant/admin builds passed.
- `npm run build:styleguide`: independent static guide build passed.
- Focused admin Playwright: teams, mentors and allocation-lifecycle at desktop/mobile;
  44 journeys passed. Includes exact affected-roster review, cancellation without a
  write, locked draft movement, server proposals, publication/freeze blockers,
  post-freeze withdrawal, capacity/conflict checks, availability/DST preservation,
  stale revision, active access revocation, late event response, cutoff crossing,
  interrupted publication and read-only unknown reconciliation without replay.
- Focused participant Playwright: participant-teams and public-portal; 10 journeys
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
preview ports 4197/4297 with one worker to avoid parallel-lane port/process conflicts.
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
