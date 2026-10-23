# DS-02 primitive verification

The before captures use the actual DS-01 frozen foundation build (`29f6114`,
subsequently squash-merged as `f063235`). After captures use the DS-02 production
build. Both capture the same synthetic registration, server rejection and
viewports through the real route; no demonstration route ships in either bundle.
No participant data or provider calls are involved.

| Portal | Desktop | Mobile |
|---|---|---|
| Admin before | [Before](admin-desktop-before.png) | [Before](admin-mobile-before.png) |
| Admin after | [After](admin-desktop-after.png) | [After](admin-mobile-after.png) |
| Participant before | [Before](participant-desktop-before.png) | [Before](participant-mobile-before.png) |
| Participant after | [After](participant-desktop-after.png) | [After](participant-mobile-after.png) |

Visual inspection against the accepted Fleet reference confirms flat surfaces,
compact 4px controls, restrained teal actions and navy text, shared named icons,
and visible field error/focus treatment without changing control geometry. Both
mobile portals retain readable text and avoid horizontal overflow. These are
bounded consumer migrations, not acceptance of the remaining legacy layouts;
shells and domain composition remain assigned to their later backlog issues.

The controlled Teams confirmation has additional [desktop](confirmation-desktop.png)
and [mobile](confirmation-mobile.png) captures. Its browser proof checks focus
containment, Escape/cancel, explicit focus return, theme portal ancestry, computed
contrast, coarse-pointer target size, pending dismissal protection and exact intent.

## Automated proof

- ESLint: zero warnings; 77 unit/component tests passed; TypeScript and both Vite
  production builds passed. No dependency changes.
- Admin production browsers: 64 cases across desktop and mobile. Includes account
  and event changes, registration forms, server errors, recovery and Teams writes.
- Participant production browsers: 46 cases across desktop and mobile; a further
  16 room-display cases cover cue retention, revoked/unsafe destinations, QR,
  readable 720p/1080p composition and native/threshold palettes.
- New browser cases prove native Enter submits once; server validation preserves
  answers, focus and additive help/error descriptions; pending buttons with and
  without icons preserve label and dimensions; uncertain requests never replay.
- Component cases cover initial-open portals, menu/tabs keys, dialog focus return,
  labelled tooltip/popover, local toast scope reset, finite/indeterminate progress,
  native refs/ARIA, disabled fieldsets and nonfocusable invalid wrappers.

The first new iconless browser fixture returned a single form where the actual
list endpoint requires `{items:[form]}`; correcting the fixture made both viewport
cases pass. Existing field-error assertions were updated from repeated alerts to
specific focused controls and their accessible descriptions. Admin-only form and
Teams screens are tested only in the admin bundle. No product route was exposed
to make the participant test suite pass.

These focused tests do not close the independent full accessibility audit #122.

## Forced-colour menu review regression

The actual account menu initially failed on both viewports: ArrowDown focused and
highlighted Account settings, but computed `outline-style` was `none`. Forced
colours removes its normal inset shadow, making the focused row indistinguishable.
The scoped media rule now gives highlighted/focus-visible menu items a 2px inset
system `Highlight` outline. Normal palette rules are unchanged.

After the fix, all 20 design-system cases passed in each production bundle (40
cases total), including the new desktop/mobile keyboard navigation and Escape
focus-return proofs. Both production builds and ESLint passed again. Captures:

| Portal | Desktop | Mobile |
|---|---|---|
| Admin | [Forced colours](admin-desktop-forced-colour-menu.png) | [Forced colours](admin-mobile-forced-colour-menu.png) |
| Participant | [Forced colours](participant-desktop-forced-colour-menu.png) | [Forced colours](participant-mobile-forced-colour-menu.png) |
