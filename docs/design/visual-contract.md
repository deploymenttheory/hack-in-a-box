# Fleet visual contract

This is the production foundation for the accepted [Fleet reference](fleet-reference/README.md)
and [backlog #227](https://github.com/deploymenttheory/hack-in-a-box/issues/227).
The interactive guide is tracked in [#233](https://github.com/deploymenttheory/hack-in-a-box/issues/233).

## Token ownership

`web/src/design-system/tokens.css` defines brand inputs, semantic roles, component
metrics and Tailwind v4 mappings. Consume semantic utilities or variables; do not
copy hex values into pages. `branding.tsx` is the bounded adapter for persisted event
palettes. The event's configured primary/background/foreground are still respected;
invalid palettes fall back to the new Fleet defaults. Each branded scope supplies a
complete palette, including focus, links and status colours, so a system-dark parent
cannot contaminate a custom-light event. Portals render inside their owning Theme.
No branding is written to `body` or to a different event.

| Purpose | Foundation |
|---|---|
| Default ink / canvas / action / link | Navy `#192147`, white, teal `#007e67`, purple `#4745b4` |
| Typography | Self-hosted Inter upright variable, system fallback; regular 400 / semibold 600 |
| Caption / body / section / page | 12 / 14 / 16 / 24px at the default 16px root; rem units respect zoom |
| Spacing | 4 / 8 / 12 / 16 / 24 / 32px |
| Controls | 4px radius; 36px minimum, 44px on coarse pointers; explicit larger variants remain available |
| Collection density | 16px comfortable / 12px compact row padding |
| Canvas | Flat surfaces, fine separators; no card shadow or rounded dashboard panels |
| Focus | 2px semantic ring with 3px offset; forced colours use system Highlight |
| Motion | 100ms fast / 160ms normal, ease-out; reduced motion removes animation/transitions |
| Icons | 16px semantic size, 1.75 stroke reference; consistent Lucide primitives arrive in #231 |

`Theme` accepts `mode="system" | "light" | "dark"` and
`density="comfortable" | "compact"`; system/comfortable are defaults. Explicit
light takes precedence over a dark system preference, explicit dark works under a
light system, and event inline palette inputs remain authoritative in every mode.
Theme persistence/controls belong to the shell and guide tickets, not this token API.

Text and actions require 4.5:1 contrast; interactive field boundaries/focus need
3:1 against their adjacent surface. Decorative separators need not impersonate
interactive boundaries. Status is independent from the action colour and always
requires visible text/icon semantics. Event branding may collapse a status hue to
readable foreground when no approved status colour is readable; labels carry meaning.
Accent remains decorative and is not used for essential text or focus.

## Migration boundary

#230 changes the visual foundation, not every page's composition. Current feature
layouts remain temporarily during the sequenced migration. #231 replaces primitives,
#232 owns collections/review/task patterns, #234 rebuilds independent shells, and
the domain tickets rebuild their screens. #245 audits route/panel parity and removes
legacy presentation. Keeping an old screen at that final gate is not acceptable.

Existing semantic names such as `--card` and `--radius-card` are transitional
consumer aliases, now flat and unrounded. They are not an instruction to retain
cards. No API, domain timing, session/CSRF, permissions, storage or PostgreSQL schema
changes are part of this foundation. Public/admin deployments and cookies stay separate.

For any addition or semantic change, update the shared token/component, the relevant
guide example and its usage/accessibility guidance in the same PR. Use synthetic
fixtures and verify real computed colours in browser tests; JS theme assertions
alone cannot prove CSS cascade, portal inheritance, focus or responsive behaviour.
