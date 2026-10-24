# Consent-aware media UI — issue #243 / UI-10

Initial base: `093791cb7e2511bedf3c74516b43ef167a73c84b`.
This is an early compiling draft checkpoint, not full UI-10 acceptance or a
security/accessibility release certification. It follows the accepted Fleet
reference, visual contract, production primitives/patterns and Tailwind v4 tokens.
No backend/schema/policy/provider changes are included.

## Complete nested ownership and native inventory

| Surface / nested owner | Actual current contract | Intended composition and remaining work |
| --- | --- | --- |
| `MediaWorkspacePage` / workspace collection | Actor/draft/event/media route scope; GET media UUID `after` cursor, limit30, native revision/can_collect/can_review; no global search, total or sort. Backend projection gives organiser all, photographer own private plus permitted released records, confirmed readers current released/valid media. | Flat heading and media-first gallery are the first checkpoint. Complete parent projection/denial/partial-restoration fences, deliberate task navigation and cursor integration remain. |
| Deep-link detail / `MediaContent` | GET exact current media ID, not historical version authority. Native `can_edit`, `can_review`, `can_publish`, `can_withdraw`, `can_read_original` are independent. | Named current-resource inspector, distinct reading versus correction/review/release tasks; preserve real URL and current native access on every new mount. |
| `MediaCollector` / file selection and preparation | PNG/JPEG up to8MiB browser bound; native decoder further validates dimensions/pixel budget/content; atomic native file upload+protected association+first media version. POST upload has captured UUID, exact metadata/reason/filename/media type/base64 bytes, event revision, CSRF and Idempotency-Key. | Focused selection → validation → exact private submission review → deliberate upload/result. Capture before file awaits, fence held file/session/preflight work; no persistent bytes or unsafe duplicate identities. |
| `useMediaWrite` | PUT save/review/publish/withdraw carries immutable command_id, expected item version and event revision/CSRF. Identical command fingerprint/native key retry returns acknowledged_version for that accepted intent, while every other response field describes only the CURRENT safe projection. Current GET alone is not a receipt. | Explicit exact review and native acknowledged result. Distinguish preflight not-sent, definite rejection, timeout/unknown and confirmed response; record minimal unresolved metadata before HTTP, keep full captured intent only in its still-authorised private owner. Retain safe deliberate identical native retry, never automatic resend. |
| `MediaEditor` correction | Save replaces metadata, returns draft and clears all old review/consent/release bindings. Type cannot change after creation. Native UTF-8 byte lengths, source versions and reason remain authoritative. | Separate focused correction/editor with dirty/review/local/route cancellation and actual responsive opener focus; preserve input during transient/stale source comparison. |
| Organiser review/approval | Native review requires identification+content attestation, explicit no-identifiable-people only when no tags, and current affirmative eligible consent for every identified person. It records state `reviewed`, not `released`. There is no separate backend `approve` action. | Identification/content review → exact approval confirmation uses this existing reviewed-state command. Do not invent an additional native approval state or imply gallery publication. |
| Explicit release / withdrawal | Native publish requires reviewed state, continuing current organiser reviewer and exact consent bindings; challenge-related content additionally requires actual common challenge publication. Native withdraw is a separate owned command. Archive ordinary writes remain closed. | Separate reviewed gallery-release and withdrawal/correction actions with exact source, current effect and original intent acknowledgement. |
| `MetadataFields` / consent directory | Consent guidance is a separate UUID-cursor endpoint, names/choice/eligibility only for current organiser/photographer. Unknown/no never grants publication. Current captured-at field is optional; recording metadata requires HTTPS/no credentials and is never fetched/embedded. | Shared labelled fields, minimal consent selection, explicit local-time interpretation, field errors and correction without hidden private-profile data. No client-side global search over a truncated directory. |
| `PhotoConsent` / `OwnPhotoConsentPage` | Independent self-only GET/PUT, expected operational-profile revision, choice yes/no/unknown and actual can_opt_in/can_revoke. No command/key receipt API. Archived current eligible/profile owners can revoke no/unknown even when ordinary writes/new opt-in close. Choice endpoint edits no other profile fields. | Separate explicit self-choice review, pending/definite/unknown/current-state distinctions; current GET cannot prove an uncertain original write or unlock an automatic replay. Remain reachable after gallery denial. |
| Native consent incarnation | Review stores internal profile-incarnation+photo-specific revision tokens, not browser-owned consent proofs. Unrelated profile corrections do not break approval. Revoke/re-consent or delete/recreate never revives old approval. Retention is checked synchronously. | Present current native indicators and permission loss; do not expose tokens, fabricate lasting approvals, infer rights from initial membership or change backend consent policy. |
| `ProtectedPhoto` preview/original | Native original and generated PNG thumbnail endpoints reauthorize current item/audience/consent on every read; generic file and content-attachment downloads deny protected media even to organisers. Blob URLs are transient; current implementation clears on blur/visibility/beforeprint/expiry. | Actual protected thumbnail/detail/download with parent/child/session/version tickets, immediate object-URL revoke and held-response fences; every print path hides image/private controls. Never use file IDs or direct stored URLs as grants. |
| External recording link | Backend returns recording_url only under its current safe projection. Native metadata validation requires HTTPS/no credentials. This application does not host, fetch, embed or recall external recordings. | Safe named external anchor with current metadata lifetime; no autoplay, unreviewed link, stale URL continuation or claim to recall an external copy. |
| Decision/history | Internal immutable media_versions and command receipts exist. OpenAPI/generated frontend and existing UI expose current list/detail, uploads/commands/bytes/consent, but NO media-history route. Event-wide audit exists under separate native AuditRead authority, with action/target/actor/revision/reason-code metadata only. | Actual API gap reported to parent before any backend expansion. Do not invent browser history or use current state as immutable version history. A bounded native audit-metadata composition may be considered separately; no new endpoint/policy is silently introduced. |

## First meaningful consumer checkpoint

`media-gallery.tsx` is the actual current-authorised gallery presentation used by
`MediaWorkspace`: flat semantic records, native kind/state/consent/version labels,
current-resource deep links and cursor-scoped loaded counts. It keeps photos as
media-first records rather than forced table rows. Protected previews/downloads
still use the real native owner, and private record print exclusion is preserved.
The legacy upload/editor/consent adapters remain deliberately identifiable work
for the next checkpoint; this PR does not claim those workflows are rebuilt yet.

Both independent portal production builds and focused source/test ESLint pass.
The existing native protected-gallery denial and independent self-consent-after-
gallery-denial cases pass at desktop/mobile (4/4) with their authorization and
protected-byte assertions preserved. The old private-record CSS selector is changed
to its semantic `data-media-record` boundary; its print/privacy assertion remains.
Temporary local port-only browser config and node_modules symlink are untracked.

## Full remaining scope and required proof

Complete every owner above: upload and exact review/approval/release; correction,
withdrawal and real authorised history; own archived self-choice; focused gallery
and direct detail/protected bytes/print; native current projection loss, parent
collection denial then partial restoration, current source/version changes, held
file/URL/session/preflight/POST/read continuations, unknown minimal metadata and
unrelated-owner history preservation. No component key substitutes for clearing
ancestor caches, selected state, raw private snapshots or held continuations.

Meaningful native browser cases must retain existing uploader idempotency/current
version/consent/privacy expectations, exercise unsafe file/field errors, wrong
reader/current role, revoke/re-consent, no old approval revival, stale/current read
failure, definitive denial versus408/5xx, no silent resend, archived self revocation,
keyboard/narrow layouts and actual gallery/task opener focus in both portals. Use
synthetic people and deterministic in-memory fixtures only. Add actual matching
before/after desktop/mobile images and production guide examples/guidance where
shared media task behaviour is introduced. Full lint/unit/both builds, appropriate
dependent consumer checks, independent Standards/Spec reviews and full exact-head
CI remain required before ready/merge. #245 broader parity/release audit remains
separate; this first checkpoint is not a claim that broad gates passed.
