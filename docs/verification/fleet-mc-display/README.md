# UI-11 / issue 244 implementation record

Draft implementation from main `ea88901f7204a0a3a441ef03610e97e6b5297740`.
This first checkpoint is read-only production presentation and a flat rehearsal
editor. It is not completion of issue 244 or verification of its final criteria.

## Finite native ownership inventory

| Surface | Native contract and owned nested work |
| --- | --- |
| `mc-console.tsx` | Independent admin-only event MC task; fresh current `can_present`/`can_control`; poll current `PresentationPage`; local cue selection, stale revision and current/next/timer; explicit reviewed control/save/activation/override/return. |
| `cue-rehearsal.tsx`, `cue-preview.tsx`, `presentation-model.ts` | Local draft/reorder/remove/title/text/HTTPS/countdown/resource-reference preview; save leaves live room unchanged; activation uses saved draft. Preferred selection and local edits must survive transient reads, not permission loss. |
| `presentation-pairings.tsx` | Current complete pairing list, authorised-contact semantics, label/lifetime config, explicit review/pair/revoke, once-only secret in local memory. |
| `room-display.tsx`, `room-canvas.tsx` | Dedicated public entry, credential-only session, approved current/next/urgent/timer/QR/CTA, light/dark branding and authorised assets; no private account projection. Pure canvas shared with guide. |
| `room-stream.ts` | Runtime allowlisted complete snapshots, equal-sequence replacement, 128 KiB frame ceiling, current-server clock and offset, heartbeat, reconnect/backoff, current revocation. |
| Native presentation / HTTP / generated API | Existing locked current-role/writes-allowed/If-Match rules stay authoritative. `Change`, `Pair`, `Revoke` have no original-command UUID replay/status receipt. Direct response and later current read differ. No backend/schema/provider change planned. |
| Recovery / private lifecycle | Minimal pending/unknown action/revision metadata above route/permission/version lifetime; no raw cues/reason/labels/secret. Current GET cannot establish unknown original effect. Late direct acknowledgement may settle safe metadata before private ticket fence; loss remains unknown. Fresh per-mount reader/preflight; definite denial scrubs private drafts, selections, secrets and old query projections, including partial restoration and held continuations. |
| Guide / proof | Actual pure production presentation with synthetic owners, snippet, intended semantics, keyboard/focus and antipatterns; existing MC/display native browser/QR/clock/privacy coverage, focused unknown/held/role/route cases; actual matching MC and room before/after screenshots. |

## First checkpoint

- Extracted backend-free `CuePreview` and `RoomCanvas`, used by the real native adapters and the independent guide.
- Flat, shared Field/Input/Textarea rehearsal composition with labelled Lucide actions; public room transport/allowlist/QR/clock/asset implementation retained.
- Display loss explicitly labels retained room content as last-known; visible QR call to action.
- Pure display palette imports now bypass the private branding editor module.

## Remaining before readiness

1. Full greenfield MC task composition and exact captured review of every existing save/control/override/return/pair/revoke action.
2. Fresh private reader/authority lifetime, post-await fences, stale/transient distinctions, nested drafts and actual opener focus.
3. Root metadata-only pending/unknown barriers, non-replay reconciliation and once-only pairing secret cleanup.
4. Final interactive review/unknown guide owners and meaningful native browser/keyboard/held/private-source proof; preserve original QR/clock/PG contracts.
5. Matched real-build MC and display visual evidence, final bounded checks and independent two-axis review/exact-head CI.

First-checkpoint lint, TypeScript/both portal builds, independent guide build and
room-stream unit tests (5/5) passed. Existing native MC browser journeys passed
12/12 and independent room display journeys passed 16/16 (desktop/mobile, one
worker), retaining QR decode, clock skew, equal-sequence withdrawal, no account
requests/private canaries and 720p/1080p assertions. The new independent guide
journey passed 2/2 with API/cookie/storage guards. Its initial status locator
also selected the snippet-copy status; scoping to the actual display corrected
that test without changing product code or weakening assertions.

Logs: `/private/tmp/hackbox244-initial-mc.log`,
`/private/tmp/hackbox244-initial-room.log` and
`/private/tmp/hackbox244-initial-guide-corrected.log`. The first-checkpoint
source and proof do not establish the unfinished acceptance criteria above. Broad
whole-route/security/accessibility release audits remain issue 245.
