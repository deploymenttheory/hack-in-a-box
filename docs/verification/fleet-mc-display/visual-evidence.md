# Matched MC and room-display evidence

All people, room text, clocks, account responses and display keys are synthetic.
The baseline is main `bc350d8c3145e8cb1787a840568a8f7b42cf349f`. Both stages
use the same capture fixture and built production entry points. No domain POST
is permitted during capture; no real provider or credential is used.

| Surface | Width | Before | After |
| --- | --- | --- | --- |
| Administrator MC | Desktop | [Before](before-mc-desktop.png) | [After](after-mc-desktop.png) |
| Administrator MC | Mobile | [Before](before-mc-mobile.png) | [After](after-mc-mobile.png) |
| Independent room | Desktop | [Before](before-room-desktop.png) | [After](after-room-desktop.png) |
| Independent room | Mobile | [Before](before-room-mobile.png) | [After](after-room-mobile.png) |

The MC captures prepare the same urgent title/text/countdown/reason against the
same published planned cue. The new flow deliberately proceeds to its immutable
review without sending: target, preserved planned countdown, return cue and
confirm/correct/cancel actions replace the old simultaneous panel stack.
Desktop uses a 1280×900 CSS viewport; mobile uses the same iPhone13 Chromium
390×664 CSS viewport at DPR3. Screenshots include the full document, so heights
differ with the composed task. The room desktop viewport is 1280×720.

The room captures use the same held timer, current/next cue, brand and canonical
HTTPS destination. The complete synthetic stream ends, so both capture the
disconnected projection; the new display explicitly labels it last-known.
The new canvas has labelled icon actions and an explicit scan/open call to
action, with distance-readable text and matching QR/link destinations. Room
assets and account APIs are absent from this synthetic projection.

Root visually inspected all four pairs. This visual evidence checks hierarchy,
readability and task composition; it does not establish backend authority,
original receipts or full lifecycle correctness. Those checks remain in the
owned native tests and exact-head repository CI documented in [README](README.md).
The image manifest records exact source revisions and SHA256 hashes.

The subsequent `c49abe4` delta adds scalar-only draft departure guards and wraps
a maximum-length unbroken next-cue title. Its existing desktop/mobile keyboard
and maximum-frame cases provide separate behavioral/bounds proof. The images
remain attributed to their executed source; these fixtures already have a dirty
reason and a short next title, so they do not illustrate those boundary cases.

Reproduce with `web/playwright.mc-visual.config.ts` and
`web/e2e/mc-display-visual.capture.ts`. Build the named baseline and candidate
first, then run the before/after projects with one worker. The config documents
the isolated baseline/candidate preview paths and ports. No synthetic capture
route or fixture enters either production build.
