# Registration recovery follow-up (#236, PR #255)

Merged PR #252 left recovery inside mounted editors. The actual event section
control could discard an unknown write and recreate an idle editor. This
follow-up uses the existing event-scoped, memory-only planning history in both
portals. Pending writes prevent departure. Unknown attempts survive local
replacement; current GETs cannot settle them or permit replay. Apply's explicit
captured UUID/fingerprint retry remains its separate native contract.

## Diagnosis and regression loop

On actual merged main `22a42bc`, all four focused desktop cases in
`registration-recovery.spec.ts` failed: unknown form and policy reentry lost the
original intent, a synthetic long name produced a 1697px document at a 1024px
viewport, and the 72rem canvas transition hid the focused collection control.

Ranked predictions were editor-local recovery state, an overbroad nowrap selector,
and CSS-only canvas hiding without focus synchronization. The corresponding
bounded changes made all four original cases pass (3.5s), with native request
counts and revisions retained. No debug instrumentation was added.

The final regression suite additionally holds a native form PUT to verify local
and route pending barriers; compares current GET state without replay or
publication; clears history/input after missing-resource or access denial and
account departure; and verifies keyboard focus returns to the current source
row. Participant offer recovery verifies cancel/accept of the explicit end-session
acknowledgement while preserving original POST body, CSRF, If-Match and one write.
The direct native anchor also raised its browser before-unload guard; the router
exit assertion uses the actual Main navigation link, matching its custom prompt.
An earlier broad Find events locator matched both links and was corrected.

## Local verification

- Web lint: passed on final product source.
- 120 units across 17 files: passed, including immutable original intent after
  editor replacement, denied-read history cleanup, synchronous no-replay fences,
  stale read/new review and pending planning departure.
- TypeScript and independent admin/participant production builds: passed.
- Final actual admin browser run: 32 desktop/mobile cases passed, covering forms,
  applicants and the recovery regressions.
- Final actual participant browser run: 52 desktop/mobile registration,
  offer/correction and shared design-system/primitive cases passed.
- No backend/API/schema changes or real provider effects. Existing native service
  behavior is exercised through deterministic route fixtures with original
  request-body/revision/CSRF/count assertions, not invented command receipts.

Commands run from `web` used equivalent isolated local configs on admin port4196
and participant port4296, one worker, because three implementation lanes share
this workspace. Standard CI configs remain unchanged. Logs are retained under
`/private/tmp/registration-recovery-{red,admin-frozen,public-frozen,units2,lint-final,build-final}.log`.
Exact-head CI and independent review remain required; this record does not claim
release certification or that either merge gate has completed.

## Synthetic actual-portal evidence

- [Unknown original form command after section replacement and current GET](unknown-form-history.png).
- [Long applicant name wrapping inside the actual 1024px collection](long-name-tablet.png).

Images were inspected. The form screenshot records recovery state in the existing
form layout; this follow-up changes recovery ownership and applicant wrapping/focus.
No full traces, auth headers or private real data are published.
