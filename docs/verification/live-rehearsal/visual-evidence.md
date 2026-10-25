# Native live rehearsal visual evidence

Eleven actual production screenshots share real PostgreSQL/native HTTP run
`e34b2e41-d98d-40cc-99ad-54118e5bb4fb`, clean source start/end
`a5f0278f875cf182758cd5264e3e41d7b9225eb3`. Seven Chromium cases passed together,
zero skips/flaky cases. [proof.json](proof.json) contains fixture IDs, exact
selection/status, dimensions and SHA-256 hashes. All people/data are synthetic;
no display key or session cookie is published.

## Matched interruption sequence

These 1280×720 images share the same display/event/source. The disconnected
state retains the last authorized opening; after an actual native MC advance,
reconnect displays the current building cue.

| State | Actual image |
| --- | --- |
| Connected opening | [Connected](images/native-connected-opening.png) |
| Outage: stale notice and same opening | [Disconnected](images/native-disconnected-last-authorized-opening.png) |
| Native current building after reconnect | [Reconnected](images/native-reconnected-display.png) |

## Preparation and live Today

The same isolated native phase event starts with an approved but unreleased
challenge. At preparation09:30 its Today shows no common release and no formal
submission opening. The actual native dispatcher runs at T−7 10:00; at live
opening Today shows the recorded release and formal eligibility. Each phase
follows the real next action into Deliverables. That event then reaches cutoff
monotonically; no settled outcome is erased, reopened or reused by another case.

| Phase | Actual image |
| --- | --- |
| Preparation before actual common release | [Preparation Today](images/native-isolated-today-preparation.png) |
| Live opening after native common release | [Live Today](images/native-isolated-today-live-opening.png) |

## Production journeys

| Consumer / state | Actual image |
| --- | --- |
| Independent administrator MC during display outage, key dismissed | [MC controls](images/native-mc-approved-controls.png) |
| Isolated monotonic event cutoff settled while lifecycle remains preparation | [Cutoff](images/native-cutoff-before-lifecycle-transition.png) |
| Deliberate recovery after native authorization, still unsent | [Mentor recovery](images/native-current-authorized-mentor-recovery.png) |
| Current common correction version2, no private source question | [Common answer](images/native-current-corrected-common-answer.png) |
| Mobile Today after native rule acknowledgement/personal completion, ordinary live team remains open | [Mobile Today](images/native-today-personal-and-team-readiness.png) |
| Administrator Today into its currently permitted MC console | [Today to MC](images/native-today-to-authorized-mc-console.png) |

The author inspected all eleven originals: actions/labels remain visible, mobile
has no horizontal overflow and the room contains no private source prose/keys.
Native assertions also verify keyboard opening/opener focus return, exact
acknowledgements and authorization erasure. The mentor image intentionally shows
its authorized synthetic unsent canary; the test subsequently proves role-loss
removal. These are production builds with separate native portal sessions.

The evidence commit adds only documentation/artifacts to the tested harness.
Parent's pure DST support patch changes other native tests, not these production
consumers or rehearsal runtime. Final-candidate CI reruns the default engines.
