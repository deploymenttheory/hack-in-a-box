# Communications UI contract map — issue #239

This is a draft implementation checkpoint, not a completion report. Source now covers inbox collection/reader, the campaign collection and focused editing steps, provider TEST review, automation binding/occurrence records, delivery evidence and recovery, and an independently published interactive example. Final browser/visual/privacy proof and independent reviews remain outstanding. Administrator campaign tools retain their existing portal boundary; personal inbox is available in both portals.

## Native boundaries

| Owner | Native contract and implementation constraint |
| --- | --- |
| `inbox.tsx` | Personal recipient-scoped `/inbox`, opaque cursor, native filtered `unread` aggregate; detail and read commands recheck current source access. Read flag is an absolute boolean setter, not an external-send command. Bounded event choices must not imply a complete event catalogue. The inbox ResourcePage owns only its scoped parent event-access latch; the shared ResourceReader/export remains owned by #254. |
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

## Current validation checkpoint

- Native selected PostgreSQL/race tests pass for communications, SMTP and HTTP: deterministic own-recipient preview, no preview writes, exact full prepared plaintext/calendar equality, source/mailbox/date/origin/branding/role/account changes before first effect, mixed-worker rejection and legacy campaign contracts.
- Changed-package Go lint passes. Frontend lint, both production portal builds and all **131 unit tests** pass.
- All **102 applicable communications browser checks** pass across administrator and regular desktop/mobile portals, including source editing, audience/schedule/TEST review, Teams setup/recovery, automation, delivery/recovery and inbox. Focused replacement-TEST and responsive portal-policy checks separately pass **6/6**.
- The independently hosted interactive guide has **32 passing checks** with private storage, cookies and application API access blocked. Final-head proof/reviews/CI remain required. No live provider verification is claimed; #80/#82 remain separate.
- Actual production-source before/after evidence is linked in [visual evidence](visual-evidence.md), with matched synthetic campaign and inbox-detail states and desktop/mobile viewports.

## Parent authority dependency

The inbox resource route owns a keyed actor/session/event/resource event-context denial latch, private read fencing and exact owned-key cleanup. Event-only access restoration cannot automatically reopen the reader; a deliberate Retry must recheck current event access. The shared ResourceReader implementation/export remains owned by #254. The unchanged strict direct-route regression holds source reads across denial and Retry, then requires the newly mounted reader's own successful current GET before showing content or ACK. At e159 baseline it fails at the new-GET requirement. It is explicitly excluded from the 102-pass applicable consumer checkpoint and must pass unchanged after rebasing the actual #254 merge.

### Known queued Email TESTs: separate provider-outcome barrier

The event-lifetime `CommunicationTests` owner sits above editable-capability and campaign editor keys; survey composer wrappers inherit it where present. It retains only native TEST ID, campaign ID/version, opaque reviewed provider revision and exact native submission state. It never holds addresses, rendered messages, credentials, reasons or command payloads. Pending, retryable and ambiguous SMTP TESTs block replacement SMTP TESTs across campaign/section/setup changes. Only the native delivery-status GET for that exact acknowledged TEST settles this barrier; a campaign GET, a changed template/provider configuration, and unknown-command history cannot settle it. The status panel offers an explicit read of the original TEST. Current denied status propagates to the workspace private-access latch. Session-ending events clear memory. In-app TEST handling remains the existing transactional inbox path, without fabricated SMTP receipt semantics.

Proof at this source checkpoint: current dual-portal production builds and ESLint pass. Focused browser checks pass **6/6**: desktop/mobile original-TEST pending→retryable→ambiguous barrier survives real event-section unmount and provider revision change, remains separate from unknown-command history, then permits a fresh review only after reading native acceptance for the original ID; both portals' desktop/mobile navigation obeys the actual responsive capability registry. The broader applicable browser run subsequently passed 102/102; final-head native/guide checks, independent reviews and CI remain outstanding. The unchanged strict resource remount regression still needs the actual #254 shared-reader merge.

Schedule field validation also mirrors the existing native integer bounds (days −366…366, minutes −1440…1440) and requires fixed local time before Save. Errors use shared Field semantics and preserve input; the native service still owns schedule/DST validity. The focused desktop/mobile empty/out-of-range/fractional/fixed-time check passes **2/2**, with zero commands until a valid deliberate save.

## Full CI consumer correction

The `bf93262` full admin CI run had 740 passes and 12 failures. Two were the documented shared-reader dependency; the other ten were actual standard-config test assumptions omitted by the earlier focused inventory. These are now reproduced and corrected without removing native safety assertions:

- Clarification, equal-UUID feedback/follow-up and survey invitation readers use the actual explicit `Open <label>` inbox action. Canonical URL, distinct native request, released-content and keyboard/privacy checks remain intact.
- Portal policy is explicit project metadata in standard administrator/public and mixed four-project configs, rather than a guess based on custom project names. The public standard config now runs the same portal-boundary scenario.
- Unknown Teams transport asserts the specific retained uncertainty barrier alongside the separate transport error, checks replacement remains disabled after reconnect/current read, and preserves the single-request/no-replay assertion.
- The public-only help inbox consumer had the same obsolete action selector plus a session fixture missing the required native `draft_scope`. It now uses a typed native Session fixture. Current private-read fences are unchanged.

Red→green proof: the exact five standard administrator scenario families failed **10/10** before correction and pass **10/10** afterward. The public-only help scenario separately failed **2/2** before correction; the corresponding native destination/portal standard public run now passes **10/10**. ESLint passes. The expanded four-project communications inventory includes these destination and primitive consumers; its full run is still in progress at this publication checkpoint. The unchanged strict resource-remount regression still depends on the actual #254 merge; none of this claims complete CI success.
