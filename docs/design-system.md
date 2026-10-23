# Shared portal design system

Both portal builds import the same Tailwind 4 CSS-first system through
`web/src/styles.css`. Forest green, warm cream and copper preserve the existing
brand. No UI package, authentication flow or feature state machine is replaced.

## Inventory and rollout

The initial inventory spans 99 feature TSX files: 447 shared Buttons, six native
buttons, 178 inputs, 98 selects, 47 textareas, 75 fieldsets, seven tables and nine
navigation regions. Feature families include event setup/templates/branding,
registration/applicants/teams/allocation, preparation/logistics/attendance,
resources/content/deliverables/credentials, mentor/help/scorecards/reviews,
communications/inbox/delivery, objectives/outcomes/media and survey results.

| Layer | Owner | Existing surface adoption |
| --- | --- | --- |
| Brand and semantic tokens | `design-system/tokens.css` | Every `.theme`, including nested event scopes and their portal hosts; room controls use the native room palette. |
| Native foundations | `design-system/foundations.css` | Every native button, input, select, textarea, checkbox/radio, file control, table, disclosure and focus target. Labels, IDs and native behavior remain intact. |
| Shared components | `components/ui` | Base UI Button with CVA variants/sizes/states; native field primitives in question/definition/event/objective editors; Card in account; Alert in shared failures; badge/card/notice aliases support existing markup. |
| Feature layouts | `design-system/workspaces.css` | All existing workspace layouts consume shared surfaces, typography, radius and status roles in the component layer. Responsive layouts and print exclusions remain feature-owned. |

## Token and state contract

Brand inputs (`--primary`, `--background`, `--foreground`, decorative `--accent`)
feed semantic surface, text, action, input, border, focus and status roles. Tailwind
`@theme inline` exposes those roles as utilities at the consuming element, so
nested event scopes and Base UI portals inherit their own current values.
Component spacing, control height, radii, motion and shadows also have shared
names. Utilities remain the final intentional override layer.

The system preference supplies dark mode for the platform theme. An event's
validated saved palette remains authoritative in its own scope, including native
control color-scheme. Muted text and status foregrounds retain contrast for custom
palettes; decorative accent is never used as a focus or text contrast guarantee.
Menus use the existing Theme portal container. No theme state is written to
browser storage. The explicit `.dark` variant is also supported for embedding and
verification.

Controls have a consistent 44px default target, multiline labels, visible
keyboard focus, distinct read-only/disabled/error states and semantic native
checkboxes/selects. Existing compact layout exceptions remain explicit. Reduced
motion removes transitions, and forced-colour focus uses the system Highlight.
Tables retain captions/headers and existing mobile overflow or stacked layouts.
Private print exclusions and QR code quiet zones remain unchanged.
