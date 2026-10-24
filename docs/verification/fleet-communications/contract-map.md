# Communications UI contract map — issue #239

This is a draft implementation checkpoint, not a completion report. Source now covers inbox collection/reader, the campaign collection and focused editing steps, provider TEST review, automation binding/occurrence records, delivery evidence and recovery, and an independently published interactive example. Final browser/visual/privacy proof and independent reviews remain outstanding. Administrator campaign tools retain their existing portal boundary; personal inbox is available in both portals.

## Native boundaries

| Owner | Native contract and implementation constraint |
| --- | --- |
| `inbox.tsx` | Personal recipient-scoped `/inbox`, opaque cursor, native filtered `unread` aggregate; detail and read commands recheck current source access. Read flag is an absolute boolean setter, not an external-send command. Bounded event choices must not imply a complete event catalogue. ResourcePage remains owned by resource composition and is unchanged. |
| `campaign-composer.tsx`, `campaign-model.ts` | Campaign save/preview/approve/cancel/test-self validate event revision and campaign version. Self-test has no client command ID; a lost response cannot be replayed as an idempotent request. Unknown shared history contains only a local diagnostic identity/action/revision, never message or reason payloads. New/Review opens a controlled Message → Audience → Schedule → Review & TEST → Channels workflow; stage changes preserve permitted edits. |
| `campaign-source-editors.tsx`, `campaign-settings.tsx`, `campaign-preview.tsx` | Retain exact template/audience versions, channel settings and schedule time zone. Show reviewed content and actual audience before approval or TEST. |
| `smtp-test-delivery.tsx` | Poll native test status. Queued, provider accepted, delivered and ambiguous outcomes remain distinct. No real-provider tests in this UI change. |
| `teams-workflow-setup.tsx` | Test request UUID supports exact replay for the same event, actor and connection-test kind. Keep the original UUID after an unknown outcome; never replace it to retry. Webhooks remain local secret input, cleared on private-scope loss. |
| `delivery-history.tsx`, `delivery-context.ts`, `delivery-summary.tsx` | Native campaign/channel filters and independent history cursors. Private current capability controls visibility; current projections are not original receipts. |
| `delivery-review.tsx` | Native recovery replay checks request UUID, operation, actor, revision, action and reason. Preserve immutable pending intent; explicit duplicate-risk acknowledgement is separate from a safe exact retry. |
| `survey-invitations.tsx` | Actual automation consumer lives here: separate binding and occurrence cursors, native event revision, exact survey version. Coordinate the survey owner; authoring and response readers belong to the assessment lane. Current binding/occurrence facts retain unknown intent and never settle the original acknowledgement. A deliberately selected new base retains the earlier metadata capsule. Export/props remain stable for the survey refactor. |

## Remaining proof and work

- Finish final usability, current capability, stale/late-response, keyboard/mobile and field checks across every owned nested consumer.
- Fence reads, commands and late responses by current actor/session/event/private capability; clear denied raw snapshots above keyed presentation frames.
- Separate unsaved, pending and unknown state; preserve focus through mobile views and actual initiating controls.
- Prove exact template/audience/config fingerprints, provider TEST review, native recovery, scope denial, role/account/config changes and reconnect behaviour with deterministic providers and synthetic people only.
- Keep SMTP secrets, Teams URLs and private audiences out of shared history, query metadata, guides, screenshots and storage.
- Add synthetic before/after evidence and independent final reviews, then require final-head CI. Issues #80/#82 retain their real-provider and lifecycle acceptance gates.

## Supporting native TEST review seam

`GET .../campaigns/{campaign_id}/self-test-preview?channel=smtp|in_app` requires current write authority and runs the existing organiser-only renderer. It saves no preview, intent, job, audit record, event revision or calendar sequence. The response includes exact subject/core text, full adapter plain-text additions and calendar attachment facts, with no mailbox/address.

The SMTP adapter shares a pure calendar projection and body formatter with actual delivery. `calendarFor` alone retains effect-time sequence. The review hash binds actor, saved campaign/source versions, current rendered output, provider revision, source facts, full presentation and the privately resolved mailbox. Enqueue validates the exact single rendered snapshot it persists. Account/contact locks retain mailbox identity through immutable SMTP preparation. Worker resolution rechecks the optional retained review binding before effect.

Newly reviewed requests namespace the UUID-bearing durable core fingerprint with their review binding, so an older core-only worker rejects them. Omitted-binding legacy requests remain unchanged. This is a private optional snapshot/command extension with no migration, new provider traffic or scheduling policy.

## Validation checkpoint

- Native selected PostgreSQL/race tests pass for communications, SMTP and HTTP: deterministic own-recipient preview, no preview writes, exact full prepared plaintext/calendar equality, source/mailbox/date/origin/branding/role/account changes before first effect, mixed-worker rejection and legacy campaign contracts.
- Changed-package Go lint passes. Frontend lint and both portal builds pass.
- Before the focused step navigation change, 54 administrator consumer checks and 32 regular-portal applicable checks passed. The focused workflow run completed 27 passes and one setup-destination viewport failure. After correcting post-commit focus/scroll scheduling, that unchanged assertion passed in both desktop and mobile targeted checks (2 passes). A complete rerun at the final head remains required.
- Full final four-project coverage, survey automation integration, published-guide browser run, synthetic before/after artifacts, independent reviews and final-head CI are still required. No live provider verification is claimed; #80/#82 remain separate.
