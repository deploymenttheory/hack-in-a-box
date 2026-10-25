# Committed coverage-review focus restoration

This is the bounded critical-defect work associated with #122. It does not close
the full accessibility, supported-browser or manual screen-reader audit.

## Observed failure and causal boundary

The existing desktop case
`coverage-review-regressions.spec.ts:136` failed in
[PR269's administrator CI job](https://github.com/deploymenttheory/hack-in-a-box/actions/runs/40154169530/job/121051838297)
at `8bf5c132af5f4e76dd4e6a11a37796af767a9edf`. After a successful capacity
save, the current **Edit event mentor capacity** control existed but was inactive
at the unchanged five-second focus assertion. A previous occurrence is retained
in [issue122](https://github.com/deploymenttheory/hack-in-a-box/issues/122#issuecomment-6390842773).
The actual trace paints the pending review across several frames before returning
to the collection; it contains no telemetry proving the precise focus ordering.

One unchanged local case passed. Previous successful stress/refresh probes did
not establish a fix, and were not repeated as a substitute for a causal seam.
The new regression holds React's queued root task across animation frames after
the synthetic API acknowledgement, then releases the closing commit. On the old
production runtime, its restoration frame finds no source, focuses the current
workspace, and never restores the source after it mounts. The final unchanged
source-focus assertion fails. Foreground state was true.

This is explicit delayed-commit fault injection. It models a deferred React
commit; it does **not** claim Chromium ordinarily executes animation frames
before its native microtask checkpoint, or prove the exact historical CI schedule.
It demonstrates the product's unsupported assumption that scheduling one frame
from an asynchronous state update establishes the closing view has committed.

An initial layout-effect implementation exposed six existing cancellation failures:
the returned inspector's passive heading autofocus overwrote early restoration.
A unit using the actual production `CoverageDetail` reproduced this competition:
the returned heading retained focus instead of the requested source. These REDs
were retained and resolved, rather than accepting capacity-only success.

## Final ownership and ordering

`useCoverageFocus` queues the existing stable control identity in its current
mounted owner and requests a React commit. A passive effect after that committed
render schedules the **one existing** restoration animation frame, after inspector
autofocus effects have flushed. At execution it resolves the unique visible,
enabled source in the current root, or falls back to that root.

The existing deliberate-focus-movement guard remains. Teardown clears pending
intent, marks the owner unmounted and cancels a scheduled frame; a retained old
closure cannot focus a replacement workspace. There is no added timeout, repeated
focus retry or manual focus at the end of the browser assertion. API authority,
native timing, original command receipt and uncertain-write behavior are unchanged.

## Executed source and verification

The final UI source was based on main
`c0c8b21fb1bc42a7a61c73a4efa2135805f63c09`.
`web/src/coverage-navigation.ts` SHA-256:
`2c3a026ac4b330e8a419092399e95f87210bb3c57c5f1e806dca516e61436bfa`. The executed administrator bundle is
`app-C5fUk-Ti.js`. This attributes the final 44-case pass to the post-passive
implementation, rather than the earlier capacity-only layout candidate.

- Both production portal builds pass on the final source; no dependency or
  lockfile change was required. An initial incomplete dependency symlink lacked
  the existing QR packages; reusing the complete locked tree resolved setup.
- **18 focus and existing coverage-command units pass**, including newly mounted
  sources, disabled/ambiguous fallback, actual heading competition, deliberate
  movement before/after commit, retained unmounted closures and frame cancellation.
- **44 finite production-browser cases pass**, one worker and no retries, across
  desktop/narrow administration and participant portals. They include the exact
  original and delayed-commit capacity cases, all six observed cancellation REDs,
  viewport/source identity, team allocation, media correction, huddle return,
  scorecard saving, mentor current-role/window loss and held original receipts.
  These are synthetic API fixtures driving actual production UI, not a claim of
  new PostgreSQL/provider or complete supported-browser verification.
- Focused lint passes. The independent guide build and **eight guide cases pass**:
  roster, mentoring queue, media lifecycle and MC production review at both widths.
  Their keyboard focus and current-read/original-receipt assertions remain enabled.

Reproduce the core units and existing administrator regression inventory:

```sh
cd web
npx vitest run src/coverage-navigation.test.tsx src/coverage-command.test.tsx
npm run build
npx playwright test e2e/coverage-review-regressions.spec.ts
```

The executed broader browser inventory used the same production bundles, one
worker, desktop/iPhone viewport descriptors and explicit administrator/regular
metadata. Its finite selection additionally covered keyboard correction in media,
huddle dirty/cancellation return, scorecard keyboard flow, and the mentor
observation/scorecard privacy and live-window cases listed above. Viewport
descriptors are not actual mobile Safari/Chrome device evidence.

All final-head repository CI checks and independent reviews remain required.
The remaining #122 route, WCAG, real stable-browser, device, keyboard and manual
screen-reader requirements retain their existing scope and dependencies.
