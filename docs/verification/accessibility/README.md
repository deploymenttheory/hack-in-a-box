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
