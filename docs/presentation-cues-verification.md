# Presentation cues and display capabilities

Issue #85 adds a backend for an organiser or current event MC to approve ordered room cues, advance manually, hold/resume server timers and pair/revoke independent display sessions. The frontend controller/display clients remain the separate UI backlog deliverables.

## Controller contract

`GET/POST /api/v1/events/{event_id}/presentation` uses the normal person session. Mutations require CSRF, the current presentation revision in `If-Match`, a decision reason and current organiser/MC authority. Platform administration alone grants no presentation access. Saving changes only the draft. Publishing explicitly approves the limited room fields and resets the display; a separate advance selects the first cue. Advance never runs automatically when a timer reaches zero. Simultaneous reviewed commands have one winner and stale commands return 409.

Cue fields are ID, title, plain text, optional HTTPS URL, duration and optional content reference. The display has a separate allowlisted DTO: event name/ID/timezone, safe branding colours, current/next approved room cues, current released common-content title, sequence, server time and server-derived timer values. It never includes the controller account, registration/attendance details, drafts, full content bodies, attachments, scores, pairing metadata or credentials. Linked unpublished/withdrawn content or role/team-private content withholds its whole cue. Challenge references additionally require actual event challenge publication. Later private edits cannot replace the last released source version.

`POST /presentation/pairings` returns an opaque event-scoped secret once. Only its SHA-256 hash is stored. Pairing lifetime is explicit (5–1440 minutes), and metadata can be inspected without retrieving the secret. `POST /presentation/pairings/{pairing_id}/revoke` invalidates it. Both operations retain normal session/CSRF/current-role/revision checks. Capability checks also require the issuer to remain an active current event organiser/MC and the event to permit operations.

## Display snapshot and streaming

Display clients use `GET /api/v1/events/{event_id}/display/snapshot` or `/display/stream` with `Authorization: Bearer <opaque secret>`. All URL query parameters are rejected. A person cookie alone cannot open a display; a display token cannot call person/controller endpoints. The display does not create or inherit a person session.

Clients must use authenticated fetch streaming because native `EventSource` cannot set an Authorization header:

```typescript
const response = await fetch(`/api/v1/events/${eventID}/display/stream`, {
  headers: { Authorization: `Bearer ${displayToken}` },
  credentials: 'omit',
  signal,
})
// Decode text/event-stream frames from response.body. Replace the complete
// snapshot on every snapshot event, including updates with the same cue revision.
```

Every connection sends the latest complete snapshot immediately; `Last-Event-ID` never causes historical cue replay. Further snapshots arrive every second with the server clock and computed remaining time. Snapshot IDs identify the presentation revision; ticking timers or source withdrawal may change the projection without advancing that revision. Reconnect therefore always replaces the prior view rather than replaying missed cues.

Only the exact display GET routes bypass person-session middleware, after requiring a bearer header. Only the exact stream route bypasses the buffering finite JSON timeout. It instead has a five-minute lifetime, five-second database-operation bounds and five-second per-write deadlines using `ResponseController`; this also supersedes the HTTP server's ordinary write deadline for the stream. Ordinary JSON timeouts and CSRF policies remain intact. Capability expiry, revocation and issuer/event authority are checked before opening and on every snapshot. An active stream sends `event: revoked` and closes when authority is lost, or `event: reconnect` on a dependency failure. Clients reconnect only with a still-valid scoped header capability. Logs contain request category/status/duration, never tokens, URL query data, headers or payloads.

## Verification so far

- Real PostgreSQL/race domain tests cover draft approval, current/next order, hold/resume timers, simultaneous MC changes, stale commands, hashed/event-scoped/expired/revoked tokens, current issuer authority, source release/withdrawal and private draft/role-content exclusion.
- Real HTTP/SSE tests cover generated contracts, CSRF, stale/private input, capability/session separation, no-store, URL-token rejection, cross-event rejection, immediate fresh snapshots on stale reconnect, active-stream pairing/MC/organiser revocation, and payload/log privacy.
- The SSE test uses a 40ms JSON timeout and an 80ms HTTP write timeout; later snapshots still flush, while a separate finite JSON handler still times out.

Migration 24 is reserved after credential-resource migration 22 and messaging migration 23. Final verification and merge must rebase on both predecessors and verify normal Goose upgrade order; migration policy is not changed. The PR remains draft until final checks and this dependency order are satisfied.
