# Accessibility and supported-browser verification (#122)

**Status: incomplete.** This directory keeps the full release audit visible while
verification is implemented. Green automated checks do not establish WCAG
conformance or complete the required browser/device and screen-reader work.

The scope is both independent Fleet portals, the independent style guide, all
first-release critical and extended journeys, keyboard-only operation, branding,
small screens, touch, zoom and assistive technology. Optional M5 booking and
certificates remain separately owned. Final cutover #245, judging #114 and
showcase #163 must supply their completed production journeys before closure.

## Evidence owners

- `journey-review.json`: all six issue criteria and full processes, including
  missing final judging/practice/showcase paths. A partial check never passes a row.
- `route-review.json`: currently registered route inventory. Include nested
  feature owners, modals, field/server errors, loading/empty/no-match, expiry,
  changed revision, unknown effect, current denial and restoration before sign-off.
- `wcag22-aa-review.json`: all 55 current A/AA criteria from the pinned W3C
  recommendation, initially **not evaluated**, including applicability reasons.
- `browser-review.json`: actual stable desktop/mobile targets, initially
  **not executed**. Supplementary engine/emulation evidence stays distinct.
- [Manual procedure](manual-review.md): execution steps and record requirements.
- `defects.md`: actual findings, reproduction, correction and retest evidence.

These are review records, not generated pass certificates. Update evidence with
actual source, environment, observed result and defect disposition. An inapplicable
criterion needs a documented reason; unavailable implementation is incomplete.

## Executable checks

Build locked dependencies and production assets using normal repository tooling:

```sh
npm --prefix web ci
npm --prefix web run build
npm --prefix web run build:styleguide
```

Native PostgreSQL, authenticated router and production portals:

```sh
HACKBOX_INTEGRATION=1 HACKBOX_ACCESSIBILITY=1 GOMAXPROCS=2 GOFLAGS=-p=1 \
  go test -v -race -p 1 -parallel 1 ./internal/app \
  -run '^TestAccessibilityBrowsers$' -count=1 -timeout=10m
```

Independent real Go guide host:

```sh
cd web
node node_modules/@playwright/test/cli.js test \
  --config playwright.accessibility-guide.config.ts
```

The native owner reuses the merged live-rehearsal fixture and private Unix IPC.
No public fixture routes, real providers or user datasets are used. Test-owned
containers/listeners are cleaned up; existing services are untouched. The guide
host serves the actual independent production build.

Keyboard helpers reach controls with Tab and activate with Enter. They never
force focus or click a target to bypass navigation defects. Scans cover the
current visible state, including error/review/dialog states; no rules are disabled.
Axe A/AA tags are a useful automated subset, not the full evaluation boundary.
Reports retain incomplete rules for human review and omit raw private DOM content.

Current configurations run **Playwright Chromium engine**. They do not establish
current retail Chrome/Edge/Firefox/Safari, actual mobile, real screen-reader or
browser zoom acceptance. An emulated viewport is not an actual device. Record
those executions separately rather than renaming an engine project.

Results live in `web/test-results-accessibility-native` and
`web/test-results-accessibility-guide`. Native provenance records start/end commit,
worktree cleanliness, fixture identity, configuration and actual selection.
A dirty diagnostic run is not a clean final-source proof. Screenshots must retain
the source and environment actually executed.

## Clean checkpoint proof

At clean committed source `beec062ad33fcee9ee1265c84f432c3612b28593`, native
keyboard/state checks passed **3/3, 6.6s**, real-PG/race owner **17.384s**.
The independently hosted guide also passed its two cases. Start/end source and
cleanliness are recorded in [checkpoint-proof.json](checkpoint-proof.json).
Browser scenarios and production code are unchanged in the subsequent evidence
commit; its matrix correction excludes obsolete 4.1.1 and includes current 4.1.2.
The initial extraction crossed a criterion boundary; the corrected inventory
matches the pinned recommendation. No standard criterion is marked passed.

## Extended native checkpoint

Clean source `7289af26d5991d86c93859cd2ada64a0d9616737` passed **5/5 native
keyboard journeys, 20.7s**, real-PG/race package **37.888s** (test owner 36.36s). The extension adds
Today → private help → official answer collection/reader/return and actual native
objective/success-measure save/version/return. The saved unknown baseline remains
null; no achieved outcome is fabricated. Fourteen visible-state scan points are
exercised across these five journeys. [Extended proof](extended-native-proof.json)
records exact source and clean start/end. The separate guide proof remains at its
original `beec062` source; it is not relabelled as a new execution.

This still does not complete registration/team editing, full triage contention,
final judge/practice/showcase, manual standards assessment or actual stable
browser/device/screen-reader/touch/zoom acceptance. All corresponding rows remain
pending, and full #122 stays Draft/open.

## Isolated admission and draft-team extension

The admission fixture creates a fresh PostgreSQL pool and API graph with its own
fixed pre-admission domain instant. Each admission browser page starts its own
virtual JavaScript Date at that same fixture instant, so client deadline displays
do not depend on the host year; authentication and transport retain real time. Native event creation, form publication and
registration transition establish the event. Team actors are confirmed through
native applications; the browser applicant starts unregistered. The existing live
and cutoff fixtures are never rewound or reopened. No competition publication,
freeze, assessment or provider send is added.

The 390 CSS-pixel Chromium registration journey reaches a real server field
rejection, checks accessible error/focus, corrects and reviews the form, verifies
one accepted native application identity/version and follows the confirmed result
to the team workspace. This is narrow desktop-engine evidence, not an actual phone.

The team journey creates/corrects a draft, explicitly joins through another
confirmed actor, approves as the actual captain and verifies draft membership
without publication/freeze. A third actor's native join returns 200 at the server
but its browser response is deliberately lost. Current roster reads retain the
unknown barrier and original metadata; exactly one command is sent, with no replay
or inferred original acknowledgement. Current non-captains cannot approve.

Initial diagnostic selection: the team case passed in 23.4s; registration failed
because an Enter after native select type-ahead had already submitted to review.
The corrected keyboard sequence continues by Tab to the explicit review control.
Registration-only retest passed 1/1 (8.7s), test owner 67.52s/package 69.585s.
These are separate dirty diagnostic runs, not a combined clean seven-case pass.
The clean combined configuration subsequently passed **7/7, 70.8s** at source
`9285577d3a19efa883711c37128f6c69714242ce`; native race test owner **111.97s**,
package **113.860s**. [Combined source/environment/clock/scan proof](admission-and-live-native-proof.json)
records seven passed cases, 23 unsuppressed state scans, matching clean start/end
source, unset selection filters and the admission clock unchanged at its own
pre-gate instant. Browser/axe versions, viewports and incomplete rules are retained.
These timings describe the executed verification host, not product latency targets.
The guide's unchanged two-case proof remains at its previously attributed source.

Criterion 1 and full #122 remain incomplete: final judge/practice/showcase,
additional critical states and manual/current-stable-browser/device/assistive
technology review are still required. No broad row is marked passed.

## Current diagnostic checkpoint

Executed with dirty worktree diagnostics, not yet a final-source certification:

- Native real-PG/race owner **30.828s**, three keyboard journeys **3/3, 11.8s**,
  with eight visible-state scan points. Earlier native run exposed the genuine
  admin label defect; later MC reason omission was a harness correction.
- Independent guide **2/2, 5.8s**, five scan states. Dark native-select type-ahead
  replaced the initial ineffective Home/ArrowDown harness sequence.
- Existing account home-link/privacy case: administrator **2/2, 9.1s** and
  regular **2/2, 7.6s**, desktop/narrow Chromium. These are mocked account API
  regressions, separately attributed from the real native proof above.
- TypeScript, focused lint, production portal builds and independent guide build
  passed. An unsupported `tsc -b --tsBuildInfoFile` invocation was rejected;
  the corrected `tsc --noEmit --incremental --tsBuildInfoFile` check passed.

The pending matrices remain pending: these finite state checks do not establish
all journeys, standards, stable browsers, real devices or assistive technology.

## Acceptance gate

Before #122 can close, complete every applicable standard row and every journey
on the final first-release screens, execute the required manual and stable-browser/
mobile matrix, fix and retest critical defects, obtain independent review and pass
all final-head repository checks. Keep evidence limitations explicit.

Primary methodology: [W3C evaluation](https://www.w3.org/WAI/test-evaluate/)
requires human assessment alongside tools; [Playwright accessibility guidance](https://playwright.dev/docs/accessibility-testing)
describes stateful scanner use. [WCAG 2.2 recommendation](https://www.w3.org/TR/2024/REC-WCAG22-20241212/)
is the full standard for this audit.
