# Shared Fleet design system

The accepted [Fleet visual contract](design/visual-contract.md) replaces the earlier
forest/cream presentation-preserving direction from PR224. Use that contract and the
[versioned reference](design/fleet-reference/README.md) for current tokens, type,
spacing, modes, density, branding, focus and motion.

Both independent portal builds consume Tailwind v4 CSS-first semantic tokens.
Production primitives are rebuilt in #231, task/collection patterns in #232, and
independent portal shells in #234. The [interactive production-backed style guide
(#233)](https://github.com/deploymenttheory/hack-in-a-box/issues/233) will publish
examples, states, code, accessibility and usage rules without requiring the app API.

Existing feature layouts are temporary migration consumers. New work must follow
the Fleet contract; #245 owns removal of old presentation after route parity.
Current API, domain validation, session/privacy and exact command-receipt semantics
remain authoritative throughout. Each semantic UI change updates its production
component, guide example and guidance together.
