# Consent-aware media UI — issue #243 / UI-10

The media routes now use the accepted Fleet composition, production controls and
Tailwind v4 tokens throughout collection, correction, organiser approval, explicit
release, withdrawal, own consent, gallery/detail and history. This follow-up is
based on `6e8ab511f00f3c3e8b63c91200a52637bee65119` (merged #263). Backend write,
consent, ownership, provider and byte policy remain unchanged. #263 separately
added the authorised metadata-only history read support. This evidence does not
claim #245's broader release/security/accessibility audit.

## Current nested ownership and native contracts

| Surface | Production owner and current behaviour |
| --- | --- |
| Collection and detail | `MediaWorkspacePage` uses native list UUID cursors and exact current detail GET. Loaded counts describe the loaded page, with no invented total, global search or sort. Native projection supplies current audience and separate edit/review/publish/withdraw/history/byte flags. Internal event/detail links use native SPA navigation. |
| Private lifetime | `MediaLifetime` observes fresh projection flags synchronously, fences old continuations and replaces private descendants when disclosure is lost. Collection downgrade drops previous private record snapshots; definite denial clears owned detail/guidance/history caches. Only a fresh authorised read can restore current content; a restored parent list cannot resurrect old detail or editor fields. |
| Collection | `MediaCollector` opens a flat task, validates native field bounds and PNG/JPEG metadata, captures metadata/reason/File/source/IDs before file awaits, then shows `MediaIntentReview` before sending. The backend still validates actual file bytes, dimensions, pixel budget, quotas and atomic protected association. No bytes or drafts enter browser persistence. |
| Metadata and consent guidance | `MediaMetadataFields` uses production labelled fields and actual paged names/choice/eligibility guidance only. UTF-8 byte limits, HTTPS-without-credentials recording destinations and explicit UTC captured time are checked without inventing private profile data. Recording content is never fetched or embedded. |
| Command/unknown owner | `useMediaOperation` keeps exact body/key only inside its current private owner. The existing actor/draft root `CommunicationSafety` stores allowlisted intent/action/resource/source metadata before POST, under an exact media namespace. Resource UUID and positive safe expected version are runtime validated. Pending/unknown survives route, version and capability teardown; raw title/caption/reason/tags/URL/file never enters this capsule. |
| Original acknowledgement | Native upload idempotency and save/review/publish/withdraw command IDs remain authoritative. `acknowledged_version` identifies the original accepted intent independently of current projection `version`. A matching late original ACK may settle safe metadata while its private response remains fenced. Lost responses/408 remain unknown. GET/history cannot settle them. Deliberate identical retry retains the original body/key and current owned/write authority, even after successful original publication makes current `can_publish` false. No automatic resend occurs. |
| Correction and approval | `MediaEditor` captures exact source/action/reason and immutable current metadata for review. Save returns a draft and clears old consent/review/release bindings. Native `review` is organiser approval (`reviewed`), with identification/content and no-identifiable-people attestations; it never implies release. Dirty fields/review remain across transient reads and explicit stale comparison; local/route discard is deliberate and actual initiating controls regain focus. |
| Explicit release and withdrawal | Gallery publication is a separate reviewed `publish` command. Current organiser/reviewer, consent incarnation and actual common challenge release remain native gates. Withdrawal is a separate current-owned command. Archives do not reopen ordinary writes. |
| Own photo consent | `PhotoConsent` and `PhotoConsentReview` capture a self-only revision and proposed yes/no/unknown before PUT. Archived eligible/profile owners retain native no/unknown revocation. The endpoint has no idempotent retry or original command-status receipt: a current GET never settles a lost PUT. Deliberate acknowledgement of a different new base keeps the earlier unresolved metadata. A direct successful PUT is distinct from its current projection body. This route remains reachable after gallery denial. |
| Consent incarnation | Internal profile-incarnation/photo-specific tokens remain backend-only. Unrelated profile edits do not break approval; revoke/re-consent or delete/recreate cannot revive older approval. The UI consumes current flags and never manufactures token/provenance or lasting consent. Existing PostgreSQL/native tests own these rules. |
| Protected photo/recording | `ProtectedPhoto` inherits the current record scope and adds a per-mount fence. Every thumbnail/original GET uses current native authorization. Late bytes cannot create URLs after scope loss. Hide/blur/visibility/print/session end/teardown revoke URLs immediately; private controls and images are hidden in print. Generic file URLs remain unavailable. External recordings use only the current projected HTTPS anchor and cannot be recalled by this app. |
| Decision history | `MediaDecisionHistory` inherits the current record scope and mounts a fresh native reader on every open. `MediaHistoryDisclosure`/`MediaHistoryContent` show only version/action/recorded time, native descending cursor pages and failure/denial suppression. Current owned history permission is separate from edit access, including read-only archives. No historical document, reason, actor, token, file ID or old bytes are exposed, and history never confers approval/access or an original receipt. |

## Interactive published guidance

The independent production style guide imports the actual pure
`MediaIntentReview`, `PhotoConsentReview`, `MediaHistoryDisclosure` and
`MediaHistoryContent`. Synthetic local owners demonstrate exact source review,
correction/focus return, separate approval/release/withdrawal actions, unknown
versus current read, fresh history loading/paging/failure/retry/denial and print.
Copyable snippets and intended-use, accessibility and anti-pattern notes ship
with the examples and in `docs/design/styleguide.md`. The guide's existing
API/cookie/storage guards apply; callbacks fabricate no native receipt or byte grant.

## Meaningful verification

The full-scope proof below was recorded at source `3672bf8`; the bounded
Standards correction and its affected proof at `70ecb6d` are recorded separately
below. All data/providers are synthetic and deterministic. No real upload, people,
provider credential or browser private persistence is used.

| Check | Completed local evidence |
| --- | --- |
| TypeScript / production builds | Both portal builds and independent guide build pass. |
| Source/spec lint | Full repository ESLint passes (`npm run lint`). |
| Native existing media/history | 26/26 participant + 26/26 administrator across desktop/mobile; protected bytes, archive revocation, denial, deferred file/session/preflight no-POST, exact lost-upload retry, transient recovery, current history and print assertions retained. |
| Added lifecycle composition | 18/18 participant + 18/18 administrator: keyboard local/route discard and actual opener focus; real SPA unknown persistence with same-document marker; held ACK versus lost response across capability teardown/routes; non-idempotent consent unknown; held thumbnail no URL; withheld recording URL at fresh200 while can_edit remains true; parent denial→partial restoration before/after stale time; unsafe-file and UTF-8 preflight. |
| Scoped units | All **153/153** tests across 25 files pass with `--maxWorkers=2`, including **13/13** across media operation, lifetime and shared safety: stale/reentrant/408 handling, held preflight, original ACK versus current version, identical native publish retry, late ACK versus loss, privacy/UUID/integer allowlist, lost disclosure and partial restoration without stale private snapshots. |
| Independent guide | 4/4 desktop/mobile history/lifecycle journeys with no API/cookie/storage access. Exact-source review, corrective focus, unknown not settled by read, consent, fresh history paging/failure/denial/reopen/print. |
| Native history read support (#263) | Real PostgreSQL/race media and authenticated HTTP tests pass, including current role/owner/wrong-event/contended-lock checks, archive/consent revocation, pagination/strict metadata allowlist/no effects; generated-contract drift and native lint pass. Native implementation is unchanged in this follow-up. |

Relevant browser files are `media-workspace.spec.ts`, `media-history.spec.ts` and
`media-lifecycle.spec.ts`; all are registered for both portal CI builds. Local
port-only configs and node_modules symlink remain untracked. Full exact-head CI
and independent two-axis reviews remain merge gates; this README's local counts
are not a substitute for those gates. Source `3672bf8` passes the final combined
**44/44 participant + 44/44 administrator** native media/history/lifecycle gate,
**4/4** independent guide journeys, full repository lint, TypeScript and both
portal/guide builds. The final regression keeps pending distinct from unknown
after route reentry; the existing held-ACK/loss SPA cases prove replacement stays
blocked until an actual original acknowledgement. Product copy names each action
and explains access/consent checks without exposing internal mechanisms.

## Matched visual evidence

Actual baseline `6e8ab511` (tree `a85ecd13`, identical to reviewed #263 source)
versus production source `70ecb6d` (valid-review images remain pixel-identical to
the approved source `3672bf8`). Each pair uses the same synthetic organiser,
event, one private draft photograph, caption, reason and checked identification /
no-person / content attestations. The baseline shows the prior inline approval
control; the new UI shows the explicit exact-source review before confirmation.
No upload or command is performed. These are full-page captures from real portal
builds at matching browser viewports, without compositing or image editing.

| Portal / viewport | Before | After |
| --- | --- | --- |
| Administrator · 1280×900 | [Before](before-administrator-desktop.png) | [After](after-administrator-desktop.png) |
| Administrator · 390×844 | [Before](before-administrator-mobile.png) | [After](after-administrator-mobile.png) |
| Participant · 1280×900 | [Before](before-participant-desktop.png) | [After](after-participant-desktop.png) |
| Participant · 390×844 | [Before](before-participant-mobile.png) | [After](after-participant-mobile.png) |

The initial 8 capture cases pass; after the bounded copy correction, all 4 AFTER
captures were regenerated and pass against source `3672bf8`.
Administrator-desktop and participant-mobile pairs were
visually inspected for removed legacy form boxes, clear distinct approval actions,
readable exact-source preview and responsive controls without clipping. Artifact
source, viewport and SHA-256 values are in [visual-evidence.json](visual-evidence.json).

At the pre-correction source checkpoint `3672bf8`, the full 25-file unit suite passes
**153/153** (not only the scoped 13), repository lint/builds pass, and the combined
native suite passes **44/44 in each portal**. The independent guide passes **4/4**.
Root independently inspected all eight original paired screenshots and all four
refreshed AFTER images: action-specific approval heading, plain access/current-
consent explanation, flat dividers, labelled icons, responsive wrapping and separate
approval/release are verified. This visual inspection is separate from the pending
independent Standards/Spec reviews and exact-head CI merge gates.

## Standards P2 correction — field validation and focus

Source `70ecb6d` resolves the reviewed field-validation finding without changing
native APIs, authority, scope fences, consent policy or write recovery. The pure
validator returns field-keyed errors. Submission and metadata correction pass them
to production `Field.error`; the owner hook calls `focusFirstInvalid` after invalid
errors render. Error editing does not steal focus, and local errors remain separate
from transport feedback and inside the current private editor. The production guide
uses the same helper and publishes its usage/accessibility guidance and snippet.

The existing unsafe-file/UTF-8 lifecycle case now asserts focus, `aria-invalid` and
associated descriptions for invalid file/title/reason/HTTPS destination and for the
metadata-correction title/reason path; both no-write assertions remain. No redundant
case matrix was added. Affected proof is terminal: **6/6 participant + 6/6
administrator**, including the existing keyboard correction/discard and valid
organiser-approval/print journeys; **2/2** independent guide journeys with API,
cookie and storage guards; TypeScript, full repository ESLint, both portal builds
and the independent guide build pass. The earlier full 153-unit/44-per-portal proof
is historical; exact-head repository CI and the independent delta review remain
required before readiness.

All four AFTER images were recaptured from the correction source and their SHA-256
values are identical to the approved valid-review images. No visual layout changed;
the manifest records both source versions and this recheck. Invalid-state behaviour
is proven by the actual field/focus/accessibility assertions above.
