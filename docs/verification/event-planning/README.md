# Event planning — frozen review candidate

This evidence covers #235 against actual merged main `1c94473`. The PR remains
draft for independent re-review of the corrections recorded below. Backend services, generated API and schema remain
unchanged; no provider operation or PostgreSQL transactional change was made.

## Completed checks before review corrections

| Check | Result |
|---|---|
| Web lint / TypeScript | PASS |
| Full unit suite | 112 PASS |
| Participant + admin production builds | PASS |
| Complete admin browser suite | 654 PASS, 4.9m |
| Complete participant browser suite | 430 PASS, 3.7m |
| Final targeted planning / survey / MC regression set | 96 PASS, 25.9s |
| Actual independent Go style-guide host | 28 PASS, 29.8s; private-store/API guards active |
| Clean frozen-parent baseline capture | 2 PASS, desktop/mobile |

The complete portal runs use their ordinary project names and all ordinary suite
cases on isolated ports 4395/4396, with a bounded three-worker local budget. No
timeout or domain assertion was relaxed. The style-guide proof uses isolated
port 4398 and one worker. Portal assets were frozen throughout their full runs;
only the separate guide bundle was rebuilt for the coordinated reset-key fix.
Exact-head repository CI and independent review remain required.

## Acceptance coverage

| Contract | Meaningful browser evidence |
|---|---|
| Setup → validation → review → saved revision → readiness | Native required fields, retained mounted sections, capacity/minimum/calendar payload, preview, captured expected revision and separate confirmation; pending native preview pauses editing and subsequent input invalidates old review. |
| Confirmed-attendance gate and lasting exclusions | Native shortfall rejects advancement; post-publication attendance exception needs deliberate checkbox/reason. Distinct confirmed/waitlisted grant fixtures return the actual `withdrawal_required` code; retained final-owner protection and role/revocation assertions remain. |
| Published versions and native approval | Template create/edit/diff/save/publish/clone; independent copied objectives; exact rules save/review/approve/release retains old released version until new publication; native schedule approval retains event-local time/state. |
| Private authority and exact unknown state | Direct current-source reads, captured If-Match, stale retention/explicit new-base choice, unknown write without replay or latest-state receipt inference, transient mounted-draft pause, definite denial, late rules responses after account/event departure, native objective exact-version recovery. |
| Planning and operations | Owner/deadline/dependency/checklist/reason/waiver, run sheet, operational progress/history, DST ambiguity choice, attendance correction request identity/history, catering distinct from admissions. |
| Field-specific privacy / exports | Shared check-in excludes restricted logistics; bounded logistics page filters and audited complete-page export; revoked scope removes private details; organiser/participant snapshot links retain native audience; inventory values remain masked/write-only, never revealed/exported. |
| Fleet controls and responsive ownership | Grouped desktop nav and native labelled narrow selector from the same capabilities; plain named local records with labelled action buttons; local return focus and breakpoint identity; current native review-coverage flag, dark/custom threshold contrast, keyboard, overflow and private browser-storage assertions. |

[Every owned nested panel and its separate domain boundaries](../../design/event-planning-inventory.md)
are enumerated. Registration, allocation, general released readers, campaigns,
assessment, surveys and other migrations retain their own owners.

## Retained failures and corrections

- Initial composition/type/lint errors and jsdom's absent native `matchMedia`
  were corrected before the first compile-green draft.
- Initial visual inspection caught stylesheet layer ordering/default-nav
  inheritance. A later inspection found the mobile rail buried the editor and
  local step controls inherited the old nav order. Corrected native narrow
  selection and explicit local ordering are exercised by the final suites.
- The first affected run passed 98/114. A real staff-error banner was hidden in
  an inactive section; feedback now sits outside that switcher. Obsolete
  structural/step selectors were adapted without removing domain assertions.
- Intermediate affected runs passed 130/130 and 154/154 admin; participant
  9/10 exposed a remaining visible-button assertion which now checks the actual
  narrow option capability too. Fresh participant checks then passed 10/10.
- First complete suites passed 647/652 admin and 425/430 participant. An overly
  broad test adapter had targeted survey-local Overview controls as event
  navigation; those calls are restored. The MC renderer's no-injected-select
  assertion is scoped to that renderer, retaining native shell navigation.
  The untouched Help peer-keyboard timeout passed both isolated cases without
  product or timeout changes, then passed in the complete corrected suite.
- The coordinated #236 local-record guide example originally shared a sibling
  React reset key. The actual distinct `local:` identity correction passes all
  28 guide journeys, including repeated reset, local actions and return focus.
- Before images were recaptured from an untouched archive of frozen `6a49648`;
  the initial intermediate capture affected by CSS layer ordering was replaced.

## Review corrections and fresh verification

The reviewed `69f7170` candidate exposed four real gaps. These are corrected in
this follow-up; the draft still requires independent re-review and exact-head CI.

- Native preview and API failure blocker links now map `configuration.*` and
  calendar paths to the actual mounted field, reveal its section and focus it.
  Desktop/mobile keyboard journeys cover both actual response shapes.
- Resource time resolution reports pending state to the parent before awaiting.
  Back, new selection and section/route departure cannot dispose the active
  command. Private tickets fence every continuation after time resolution and
  the credential POST; account departure cannot send a delayed old POST or
  refill the next editor. Both held-resolution and held-POST cases are covered.
- Clean content approve/release operations now count as pending/unknown changes
  for local Close, parent section changes, route blocking and unload warnings.
  An unknown response cannot be escaped by closing and reopening a clean editor.
- Event setup, branding and content retain immutable uncertain command capsules
  in event-session memory, independently of the editable current base. Explicit
  current-base selection permits a new deliberate command; it never labels the
  original attempt resolved. The old capsule survives a later successful command
  and section replacement. Current authority loss clears it; leaving the event
  session warns that its memory-only history will be discarded. No command
  payload is placed in browser storage or a query/mutation cache.

Native contract inspection supports a deliberately new configuration command:
[`event.Service.Update`](../../../internal/event/service.go) locks the event row,
checks current edit authority and compares the supplied revision before applying
configuration (`317–341`). [`templates.Service.SaveBrand`](../../../internal/templates/branding.go)
does the same (`177–200`), saves validated branding and bumps the event revision.
This protects the new command against another intervening writer. A fresh GET
is still only current-state evidence, never the original write's receipt. No
backend policy or transaction was changed.

Fresh verification of the corrections:

| Check | Result |
|---|---|
| Full units / lint / TypeScript / both production builds | 114 PASS / PASS / PASS / PASS |
| Expanded affected admin desktop/mobile matrix | 190 PASS, 1.1m |
| Final core review regressions after authority-reset safeguard | 66 PASS, 18.3s |
| Final participant desktop/mobile matrix | 10 PASS, 7.5s |

The 190-case matrix preceded the final provider reset on loss of edit authority;
the final 66-case matrix exercises that reset and every reported defect against
the final production assets. The earlier 654/430 complete suites are retained
above as historical evidence, not claimed as rerun on this correction. First
corrected core run passed 64/64; two additional held-POST departure cases brought
the final core matrix to 66. A sandboxed preview initially failed to bind its
port; the authorised isolated run succeeded without product/timeout changes.

The event-session command summary was visually inspected in both actual portal
viewports: [desktop](admin-desktop-unconfirmed-history.png),
[mobile](admin-mobile-unconfirmed-history.png). Its original outcome stays
explicitly unconfirmed alongside the separately successful later command.

The actual #236 ApplicantReview dirty callback and #237 TeamWorkspace
canEdit/dirty callbacks must remain wired into EventContent in whichever merge
order lands; these corrections do not fabricate future owner props.

## Native calendar group follow-up

Independent re-review found that `event.Validate` (`internal/event/model.go`,
142–143) emits the group field `calendar`, with code `invalid` and message
`invalid business calendar or relative offset`, when `Calendar.Validate` rejects
an excluded date. The first correction covered nested calendar field paths but
missed this actual native group shape. A fresh mobile regression reproduced the
still-hidden field on `424e5a2` before the fix.

The group now targets Excluded dates in Business calendar; prefixed
`configuration.calendar` and existing specific nested mappings remain supported.
The regression submits `2027-02-30` in the actual calendar payload, returns the
native preview blocker, activates its link by keyboard from Event details and
checks that the retained excluded date becomes visible, focused and correctable.
Both group spellings run on desktop and mobile. Fresh final checks: **70 core
browser cases PASS (17.1s)**, **114 units**, lint, TypeScript and both production
builds PASS. No other production change, rebase or backend change is included.
Logs: `/private/tmp/hackbox-planning-calendar-{red,browser,lint,unit,build}.log`;
the red log retains the expected failed reproduction. Independent spec re-review
and complete exact-head CI are still required before ready.

## Representative synthetic visual comparison

The before images use the same named synthetic event/values in the untouched
frozen-parent production build. After images use the real rebuilt production
routes, not a replacement component harness. They were visually inspected
against the accepted flat Fleet canvas, typography, controls and responsive
patterns. All people and records are synthetic.

| Context | Before overview | After overview | Before setup | After setup |
|---|---|---|---|---|
| Admin desktop | [Before](admin-desktop-before-overview.png) | [After](admin-desktop-after-overview.png) | [Before](admin-desktop-before-setup.png) | [After](admin-desktop-after-setup.png) |
| Admin mobile | [Before](admin-mobile-before-overview.png) | [After](admin-mobile-after-overview.png) | [Before](admin-mobile-before-setup.png) | [After](admin-mobile-after-setup.png) |

Independent participant chrome: [desktop](participant-desktop-after-overview.png),
[mobile](participant-mobile-after-overview.png). Separate portal runtimes and
native authorised resource readers remain intact.
