# Communications UI contract map — issue #239

This is an implementation checkpoint, not a completion report. The first consumer is the inbox collection in both portals. The message detail is still the existing reader; composer, provider controls, automation and delivery views remain to be rebuilt.

## Native boundaries

| Owner | Native contract and implementation constraint |
| --- | --- |
| `inbox.tsx` | Personal recipient-scoped `/inbox`, opaque cursor, native filtered `unread` aggregate; detail and read commands recheck current source access. Read flag is an absolute boolean setter, not an external-send command. Bounded event choices must not imply a complete event catalogue. ResourcePage remains owned by resource composition and is unchanged. |
| `campaign-composer.tsx`, `campaign-model.ts` | Campaign save/preview/approve/cancel/test-self validate event revision and campaign version. Self-test has no client command ID; a lost response cannot be replayed as an idempotent request. |
| `campaign-source-editors.tsx`, `campaign-settings.tsx`, `campaign-preview.tsx` | Retain exact template/audience versions, channel settings and schedule time zone. Show reviewed content and actual audience before approval or TEST. |
| `smtp-test-delivery.tsx` | Poll native test status. Queued, provider accepted, delivered and ambiguous outcomes remain distinct. No real-provider tests in this UI change. |
| `teams-workflow-setup.tsx` | Test request UUID supports exact replay for the same event, actor and connection-test kind. Keep the original UUID after an unknown outcome; never replace it to retry. Webhooks remain local secret input, cleared on private-scope loss. |
| `delivery-history.tsx`, `delivery-context.ts`, `delivery-summary.tsx` | Native campaign/channel filters and independent history cursors. Private current capability controls visibility; current projections are not original receipts. |
| `delivery-review.tsx` | Native recovery replay checks request UUID, operation, actor, revision, action and reason. Preserve immutable pending intent; explicit duplicate-risk acknowledgement is separate from a safe exact retry. |
| `survey-invitations.tsx` | Actual automation consumer lives here: separate binding and occurrence cursors, native event revision, exact survey version. Coordinate the survey owner; authoring and response readers belong to the assessment lane. Current effect matching must never fabricate a command receipt. |

## Remaining proof and work

- Recompose message detail and every owned nested communication editor/control using production Fleet primitives and patterns.
- Fence reads, commands and late responses by current actor/session/event/private capability; clear denied raw snapshots above keyed presentation frames.
- Separate unsaved, pending and unknown state; preserve focus through mobile views and actual initiating controls.
- Prove exact template/audience/config fingerprints, provider TEST review, native recovery, scope denial, role/account/config changes and reconnect behaviour with deterministic providers and synthetic people only.
- Keep SMTP secrets, Teams URLs and private audiences out of shared history, query metadata, guides, screenshots and storage.
- Add synthetic before/after evidence and independent final reviews, then require final-head CI. Issues #80/#82 retain their real-provider and lifecycle acceptance gates.
