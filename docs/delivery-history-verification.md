# Delivery history and deliberate recovery

#83 provides organiser-only campaign/recipient selection history and bounded
provider attempts at `/events/<event-id>/communications/deliveries`. Campaign
version and actual selection time remain separate from prepared-attempt and
receipt times. Never attempted, unverified-address omission, local inbox
persistence, provider acceptance, retryable failure and uncertain outcome have
distinct labels. No history query selects provider addresses, original message
content, provider endpoints, keys or raw third-party errors. Operator-supplied
reasons remain private organiser records.

The detail API calculates currently allowed actions through the owning provider's
real source, audience, configuration and mailbox policy under the event lock.
Reads remain available to current organisers after archive; every mutating
decision, including an identical-command replay, is frozen. Provider readiness
is labelled as local configuration, never live channel verification or confirmed
delivery. The separate outstanding Teams shared-channel acceptance is unchanged.

Recovery requires an explicit operator reason and request identity. New commands
take the worker's nonblocking operation advisory guard before Event→Operation
row locks. Duplicate commands retain one immutable decision. Unknown HTTP outcomes
offer only an explicit identical-command retry or exact read-only reconciliation;
the browser never replays automatically or persists operator text.

An accepted or uncertain original cannot use generic retry. An explicit resend
acknowledges duplicate risk, retains the original immutable message/recipient and
authorises one new prepared provider attempt. A separate append-only use binds
that authorisation to the new attempt in the same transaction. Current authority,
source, provider, pause and audience checks still run before effect. Prepared
attempts without receipts remain uncertain. Cancelling only an unspent queued
resend does not recall the original; resolving without resend preserves its
unknown outcome and requests no provider effect.

Retry after a definite temporary rejection is only a scheduling nudge under the
original campaign approval, which already permits automatic retry. Losing the
nudge operator's role does not revoke that independently authorised delivery;
losing the original approver or current source authority still blocks execution.
Retry never overrides accepted or uncertain evidence. An explicit resend instead
requires the recovery reviewer's current authority inside the attempt transaction.

Reviewed River arguments contain the durable review reference, validated against
the current operation generation. They can enqueue after a retained completed
original without weakening normal job uniqueness. Old lease callbacks cannot
finish a newer reviewed operation. No recovery command performs provider I/O.

## Evidence

- Recording SMTP tests first reproduced accepted→resend staying at one physical
  call and uncertain→resend failing to execute. Current native provider tests
  retain both original receipts and stable message/calendar identity, make exactly
  two total calls, consume one grant and reject replay.
- Actual HTTP→reviewed River→Jobs execution tests cover accepted and uncertain
  originals. The second provider call is held in flight while an old lease tries
  to finish it: zero rows change and the new lease remains running. Duplicate
  HTTP commands and reconciliation do not add effects. Strict OpenAPI responses,
  CSRF, current operator authority, platform-admin isolation, no-store and generic
  log/error privacy are checked against real PostgreSQL.
- Archive tests reject all five commands and an identical-command replay while
  retaining read-only history with no available actions and no new decision or
  provider effects. Separate tests cover no-resend resolution, queued-resend
  cancellation, current mailbox/provider/pause changes and revocation of a resend
  reviewer while the original approver remains active.
- Definite-rejection retry tests traverse Jobs execution and reconciliation with
  a distinct retry operator: their later role loss does not replace or revoke
  the original approval; original-approver revocation blocks the provider effect.
- Native selection keysets include actual SMTP omissions and actual private
  inbox persistence. Campaign/channel filters apply before bounds, continuation
  does not duplicate selections and local persistence invents no external attempt.
- Local TLS Teams fixtures exercise accepted and uncertain responses through
  native review/workers, retaining two immutable receipts and consuming one grant
  without repeated effects.
- Normal37→38 upgrade tests preserve all populated predecessor rows, including
  real native huddles, submitted cards, reviewed feedback/actions and private
  questions. A separate native SMTP campaign is approved, selected and accepted
  by a recording provider on37 before migration; its snapshots and original
  receipts survive unchanged, then one reviewed resend uses the new schema.
  Populated rollback refuses to discard the decision/use and both receipts;
  empty rollback and return to normal latest succeed.
- Eleven browser scenarios pass on desktop and mobile in each built portal
  (22 cases per portal). They cover
  keyboard controls, provider readiness labels, immutable evidence, explicit
  duplicate acknowledgement, uncertain commands, exact read-only reconciliation,
  archive, local history-page leave/stay, conflicts/transient reads, native
  revocation, expiry, account/event cleanup, pause and cancellation.

## Remaining merge gates

Schema38 is tested on the reviewed schema37 predecessor branch; its temporary
dependency ancestry must be dropped onto actual merged main before final parity
and CI. Exact-head composition, generated parity and CI remain required. Tests use
only synthetic identities and local recording/TLS providers; no external emails
or shared-channel messages are sent.
