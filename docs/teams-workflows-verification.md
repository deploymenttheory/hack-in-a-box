# Teams Workflows verification (#80)

The adapter owns encrypted per-event connection configuration, immutable channel delivery intents/attempts/receipts and the `teams_workflow` job handler. Both API and worker construct the same provider graph. Campaign execution uses #78's current approved public broadcast resolver; it never accepts arbitrary message text, a recipient preview body or a caller-supplied destination. The delivery retains the approved audience preview ID/fingerprint and immutable campaign version, rather than claiming individual channel recipients received a message.

## Boundaries and evidence

| Acceptance | Implemented boundary | Executed evidence |
| --- | --- | --- |
| Authorised public broadcasts only | Exact campaign version and opaque connection revision; current organiser, current confirmed audience, public purpose/token/source resolution and embargo checks inside the event transaction. Rechecked after durable attempt creation, with locks retained through the bounded request. | `TestOnlyCurrentApprovedPublicCampaignCanReachChannel`, `TestQueuedBroadcastRechecksCurrentRecipientsAndCancellation`, existing #78 public resolver tests. Withdrawals, cancellation and a newly personal template prevent any mock HTTP call; stale queue versions fail. |
| Endpoint confidentiality | AES-GCM external keyring with distinct Teams domain/event/connection/version binding. Explicit metadata-only DTO, typed safe audit actions, private provider errors. Write-only password input uses component state and direct request only, outside query/mutation caches. | `TestConnectionEncryptionAuthorityAndExplicitChannelTest`, `TestQueuedTestRechecksRoleConnectionKeyAndEvent`, `TestTeamsWorkflowHTTPSecretAndCurrentAuthorityBoundary`; four component tests inspect caches/storage and session-expiry clearing. |
| SSRF and redirects | Current documented Power Platform HTTPS workflow shape only. Validate all DNS answers, pin public IP, verify TLS hostname/trust, disable proxies and redirects. Do not forward authorization or cookies. | Transport table, real local TLS/pinned dial tests, mixed private/public DNS, forged TLS identity/trust, redirect and no-replay tests. All destinations are synthetic; no Microsoft call occurs. |
| Rate/failure/uncertainty | One-second per-connection spacing, bounded Retry-After, at most five actual provider attempts. 2xx is provider acceptance only. Uncertain writes/5xx/408 and missing crash receipts cannot replay. | `TestRateLimitHasBoundedRetryAndAmbiguousSendNeverReplays`, `TestProviderAttemptsAreBoundedAndCrashRecoveryNeverReplays`, `TestRiverWorkersResumeAfter429AndSharedConnectionWindow`. Safe receipt history is visible in setup. |
| Ownership and test UX | Owner/co-owner labels and continuity guidance; explicit confirmed fixed channel TEST, distinct from unavailable private Teams self-test. Configuration changes invalidate approval. | Six desktop/mobile browser cases plus four component cases. Browser replacement preserves composer edits and requires current fact review. [Setup guide](teams-workflows-setup.md). |
| Migration preservation | Schema 27→28, guarded rollback refuses to discard even a revoked connection or immutable delivery history. | `TestTeamsSchemaUpgradeAndGuardedRollback`: actual Goose DownTo succeeds only for empty new tables; seeded state survives rejected rollback. |

The root's transport review identified missing IPv6 exclusions. Added failing cases for `5f00::1`, `fec0::1` and the current IANA dummy prefix `100:0:0:1::1`; all fail against the old policy and pass after the exclusions, while ordinary public IPv6 remains accepted. Primary registry links and the endpoint policy's supported public-cloud scope are in the setup guide.

The actual River worker test reproduced a completed River job with a still-pending Teams operation before the shared jobs fix: `River completed a temporarily ineligible Teams operation` (4.77s). With the shared pending-operation snooze fix, the same worker test passes (35.77s), delivering the initial rejected attempt, its accepted retry, and the second intent queued during the shared rate window. Direct manual calls alone would miss this integration failure. The shared worker fix is already merged on main; this PR retains no duplicate jobs implementation.

## Checks run

- Actual-main schema-27 rebase: focused Teams PostgreSQL/race/transport suite passes (53.861s), including ordered 27→28 upgrade/guarded rollback and actual River client execution; current-authority HTTP regression passes (3.748s). Earlier full Teams suite passed (42.416s).
- Additional current-source, retry-cap and crash-recovery PostgreSQL/race cases: pass (13.379s).
- Current-organiser/CSRF/If-Match/idempotence/secret-canary HTTP test: pass (1.999s).
- Schema upgrade and actual guarded rollback PostgreSQL/race: pass (3.242s).
- Desktop/mobile setup browser scenarios: 6 pass (2.5s), isolated port 4380; combined existing composer, SMTP receipt and setup regression after the actual schema-27 rebase: 22 pass (9.0s). Scoped component cases: 4 pass.
- Scoped Go lint: zero issues. Web lint and public/admin production builds pass.

These are local results, not claims about a future exact-head GitHub CI run. The branch is rebased onto actual merged main `6d7ae5f` (#194/schema27), including #74/schema25, #78/schema26 and the merged worker lock-order/snooze fixes. Only Teams commits remain in its diff. Both generated Go and TypeScript contracts reproduce without a diff; the shared runtime preserves SMTP campaign/self-test handlers and canonical participant workspace URLs. Final merge still requires green exact-head GitHub checks.

## External verification still required

**No approved live test-channel endpoint was supplied, and no real Teams message was sent.** The deterministic local TLS fixture covers accepted responses, revoked URL responses, unsafe destinations, 429 and uncertainty, but cannot prove a real tenant's workflow posts its card to the intended channel. Before declaring the issue's live-channel verification complete, a current organiser must configure a privately supplied approved endpoint, confirm the visible shared-channel TEST, and have its owner/co-owner inspect both workflow run history and the actual card. Record provider acceptance separately from that observed channel result. This prerequisite is also recorded in the setup guide.
