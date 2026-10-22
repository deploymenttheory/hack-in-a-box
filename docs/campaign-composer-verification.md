# Reviewed campaign composer

Issue #78 adds an organiser workspace in the separate admin application. Participant
bundles do not import the composer. Every mutation checks the current organiser,
event and source/campaign revisions, CSRF token and recorded reason. Stale writes
preserve local edits and require explicit review before another command.

Campaign approval binds the exact message and selector versions, retained audience
preview and its current approver, official facts, selected channels, calendar
selection, resolved event-local schedule and opaque provider configuration
revisions. Changing any bound value requires a new approval. Losing either
approver's current authority makes the effective state non-approved. This follows
existing communications current-authority semantics; legitimate role restoration
can restore an otherwise unchanged approval.

Self-tests are typed pending operations addressed solely to the requesting current
organiser. The provider resolves that person's own eligible destination; no request
accepts an address. Teams Workflows is broadcast-only and has no self-test control.
Its explicit shared-channel connection test belongs to #80. Missing adapters are
reported as unavailable, keep selected channels visible and prevent approval. A
queued test is never labelled accepted or delivered. Provider receipts belong to
#79–81; unattended campaign execution and missed-action policy belong to #82.

The execution resolvers recheck current authority, recipients, official sources,
challenge embargo and provider revisions under the dispatch transaction. Broadcast
projection additionally rejects personal selectors/tokens and requires identical
common content. Approval itself does not queue unattended delivery.

Local evidence on checkpoint f250426:

- Twelve desktop/mobile browser cases exercise approval invalidation, audience
  counts and personalised previews, unavailable channels, organiser-only test
  intent, cancellation, explicit stale-revision review, inert hostile previews and
  revoked access while preserving edits. All passed in 9.2 seconds.
- Real PostgreSQL race suites for campaigns and HTTP contracts passed, including
  both approver roles, provider revision changes, atomic test cancellation,
  current-recipient filtering and guarded official sources. Gap and ambiguous
  daylight-saving wall times return explicit blockers without changing revision.
- Both production UI builds, web lint and full Go lint pass. Desktop approved and
  mobile unavailable-channel captures were inspected for readable controls and
  bounded layout. Generated API and SQL outputs are committed.

Final merged-base migration and exact-head CI results must be recorded in the PR
before merge. No real SMTP message or Teams broadcast was sent for verification.
