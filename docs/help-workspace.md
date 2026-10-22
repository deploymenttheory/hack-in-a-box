# Team help and mobile helper workspace (#160)

The actual `/events/:event/help` queue and `/events/:event/help/:request` detail
routes use the existing authenticated shell independently of Today and the
progress dashboard. Existing event navigation links to the help workspace.
The future #88/#105 surfaces can use these real routes; no placeholder dashboard
or dependency cycle is needed.

## Native current-authority projections

The queue exposes only the actor's current confirmed published request team and
creation capability, obtained through the native help authority. It is not a
team browser. Helpers/organisers do not fetch a participant roster to create a
form. Request detail includes its current owner's display name; only actors
currently allowed to transfer receive eligible helper IDs/names. Enumeration
uses the same native159 authority under the event lock, including role/person,
assignment window, availability and declared conflicts. POST rechecks eligibility
and revision, so a stale select option cannot grant a transfer.

Mentor profile Active is derived from people.active and the event mentor role;
there is no independent profile activation column. No new profile state or SQL
policy exception was introduced. Real role/person deactivation and conflict
regressions exercise claim/read/transfer eligibility and unavailable-assignee
reconciliation.

## Forms and truthful operational state

Request description/location/meeting route stay only in the open form, outside
browser persistence. Leaving its event/account or losing current membership
clears that state. A location or approved HTTPS meeting route is explained and
validated locally; approved-host enforcement remains on the server. Setup and
logistics remain distinct from competition-output support, which requires actual
common publication and scheduled live start. The interface promises no arrival
estimate, completed formal review or score.

Create/action intents retain stable request/command UUIDs for deliberate retries.
An uncertain transport response is never labelled saved. Exact-request checking
can confirm matching created input; a hidden404 is not proof nothing committed.
Retries are explicit and reuse their idempotency key, never an offline outbox.
Claim conflicts fetch the native current owner/state while preserving unrelated
local work. Helper controls use current native action flags. Reopening returns
the request to Waiting with its explicit history entry and authoritative time.
Transfer selections are labelled unsent until a server result confirms them.

Reader/action state keys include session, event and request identity. Event-scoped
UUIDs may repeat across events without carrying an old uncertain command or
transfer selection to the new context. Current native reads refresh every15
seconds and on focus; current denial hides private prose/actions and removes the
requester's in-memory form. Queue status/category/owner/overdue filters operate
after native authorization; neither counts nor summaries include hidden details.

## Verification

Real PostgreSQL/race native help tests pass19.907s and HTTP4.288s; added current
context/transfer privacy checks pass help3.035s and HTTP3.330s. Browser coverage
exercises actual routes/forms on desktop/mobile: keyboard create/cancel/reopen,
claim/release/transfer/resolve, competing claimers, scoped filters, uncertain
create checking/idempotent retry, pre-live output refusal with setup still usable,
role/account/event/session cleanup, cached two-event same-ID intent isolation
and useful location/meeting validation. Existing safe inbox-to-help navigation
and inert hostile request rendering remain covered.
