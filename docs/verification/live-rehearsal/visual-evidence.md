# Native live rehearsal visual evidence

Nine actual production screenshots share real PostgreSQL/native HTTP run
`cce767e9-0577-4fe4-b92a-35eae5f5fdea`, clean source start/end
`1ee06906fcbe6a28b8e29be1ce36d200ca87fcd2`. Seven Chromium cases passed together,
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

## Production journeys

| Consumer / state | Actual image |
| --- | --- |
| Independent administrator MC during display outage, key dismissed | [MC controls](images/native-mc-approved-controls.png) |
| Cutoff settled while lifecycle remains preparation | [Cutoff](images/native-cutoff-before-lifecycle-transition.png) |
| Deliberate recovery after native authorization, still unsent | [Mentor recovery](images/native-current-authorized-mentor-recovery.png) |
| Current common correction version2, no private source question | [Common answer](images/native-current-corrected-common-answer.png) |
| Mobile Today after native rule acknowledgement/personal completion, separate team state | [Mobile Today](images/native-today-personal-and-team-readiness.png) |
| Administrator Today into its currently permitted MC console | [Today to MC](images/native-today-to-authorized-mc-console.png) |

The author inspected all nine originals: actions/labels remain visible, mobile
has no horizontal overflow and the room contains no private source prose/keys.
Native assertions also verify keyboard opening/opener focus return, exact
acknowledgements and authorization erasure. The mentor image intentionally shows
its authorized synthetic unsent canary; the test subsequently proves role-loss
removal. These are production builds with separate native portal sessions.

The evidence commit adds only documentation/artifacts to the tested harness.
Parent's pure DST support patch changes other native tests, not these production
consumers or rehearsal runtime. Final-candidate CI reruns the default engines.
