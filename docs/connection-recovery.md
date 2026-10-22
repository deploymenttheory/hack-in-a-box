# Connection status and private mentor text recovery (#89)

This core supplies ordinary connection status, existing bounded room-stream
recovery and the private text mechanism consumed by #100. It deliberately keeps
#89 open until the real mentor workspace verifies interrupted note saves,
refresh/recovery, team switching, session expiry and conflicting server edits.
There is no scratchpad product route, simulated saved observation or offline
assessment/scoring queue. #100 owns the actual note form and server mutation.

## Private local copy boundary

`MentorDraftStore` projects only category, evidence and outcome text. It does not
serialize a form, credentials, scores, CSRF/session/display tokens, attachments or
private API responses. Copies live only in tab sessionStorage, bounded to16
records and the existing category/evidence/outcome byte limits. Storage failure
returns unavailable; it never claims the text is safe after refresh.

Every copy binds person, random non-authorizing session scope, event, team, note,
base event revision and note version. The session scope is separate metadata,
created at sign-in and never accepted as an authentication proof. Existing
sessions receive it lazily. Expiry is30 minutes from initial creation, never
extended by typing or background server refreshes. Reads, writes, focus,
visibility and periodic lifecycle maintenance purge expired content. Suspended
browser timers cannot authorise an expired read. An open editor must explicitly
discard/start a new copy after expiry; repeated writes cannot silently renew it.

Logout, expiry and session/account change invalidate persisted copies and in-flight
recovery. Metadata-only cross-tab signals clear peer tabs and require a current
session fetch. They contain neither identities nor draft/resource/token data.
The storage fallback signal is immediately removed. Safe event-configuration
drafts retain their separate policy. Late401 responses from an older session
cannot clear a newer authenticated session's drafts.

## Actual mentor form integration contract (#100)

1. Save each unsent text change through the allowlisted store and display its
   local/unavailable/expired result accurately. A local copy is never a server
   save; a dropped write response leaves server outcome unconfirmed.
2. Mount `MentorDraftRecovery` for the exact target. It reveals no private text
   automatically. Explicit checking/restoration invokes the caller's fresh
   current resource GET and validates target, write authority and live-review
   eligibility before providing the local text to the editor.
3. Supply the mandatory `onInvalidate` callback to clear any already-restored
   editor text, pending actions and conflict choices. Session-scoped private
   caches unmount immediately on remote invalidation. Never keep a recovered
   editor outside that boundary.
4. Immediately before a deliberate server save call `prepareSubmission` with a
   new authoritative GET. Changed revision/note version requires explicit user
   review. A further concurrent change invalidates that choice. Use the returned
   revision with the existing server If-Match/auth policy; never replay offline.
5. Remove a copy only after a confirmed server result or explicit discard. For
   an ambiguous save, fetch actual note identity/version before deciding whether
   the server accepted it; never turn local preparation into a delivery receipt.

The callback must use the actual #99 owning endpoint and current permissions,
not a cached query, transport-online flag, arbitrary recipient or forged target.
This core's tests prove the controller contract; they do not claim the still
unimplemented #100 form already supplies that adapter.

## Connection and room display behavior

The ordinary shell reports network loss, unavailable server and reconnecting
states. Reconnect refetches current GET queries, without replaying writes.
A transport response is not authority to submit a draft or judge offline.
The room display retains its already reviewed bounded fetch/SSE behavior:
8-second stale timeout,1–15-second backoff, complete fresh snapshots even when
cue revisions repeat, memory/header-only capability and terminal revoke cleanup.
No service worker or persisted private response cache is introduced.

## Current evidence and remaining aggregate verification

Real PostgreSQL/race sign-in and authenticated HTTP checks pass3.283/3.248s,
including stable metadata distinct from CSRF/session credentials and no-store.
Controller/primitive tests cover refresh, absolute expiry, unavailable storage,
current/revoked/live eligibility, fresh revision review, cancellation, typing
races and mandatory clearing of already-restored editor text. Real shell/browser
checks use synthetic local-copy fixtures, not a fake mentor form. Twenty current
participant desktop/mobile cases pass10.3s across shell/session isolation and
unchanged room SSE recovery, including two-tab logout without rebroadcast loops
and a delayed previous-session401. Both bundles and lint pass at the checkpoint.

The full #89 acceptance remains open until actual #100 integration/browser
journeys verify text entry interrupted by Wi-Fi or ambiguous saves, recovery
choices against real note versions, team switching and current authority. That
later integration must not claim scoring is available offline.
