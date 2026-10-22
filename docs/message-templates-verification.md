# Message templates and current audiences

Issue #77 adds the organiser-only `/api/v1/events/{event_id}/communications`
contract. GET returns current immutable template/selector versions and the safe
variable catalog. POST supports `save_template`, `save_audience`, `preview` and
`approve`; every command requires CSRF, the reviewed event revision and a reason.
Template/selector saves also compare their own version. Concurrent commands share
the event lock used by registration, permission and publication changes.

Templates support facilitator selection, registration, offers, preparation,
brief/challenge releases, reminders, feedback, results, postponement, help,
clarification, rubric and outcome-review messages. Templates are plain text;
`text`, `html` and `markdown` are output formats. HTML and Markdown escape literal
text and interpolated values, rather than accepting executable composer markup.
Subject values cannot introduce header line breaks. Interpolation is single-pass.
Unknown namespaces, malformed placeholders and missing values cannot be approved.

The catalog includes event name/workspace/timezone, person name, published team
name and separate final-admission-cutoff, team-freeze, challenge-publication,
live-start/end and submission-cutoff variables. It excludes contact addresses,
credentials, registration answers, private question bodies, assessment prose and
scores. Names rendered for one recipient are never reused for another.

Saved selectors combine selected current roles, active registration states or
published teams (OR), then apply an optional outstanding-action filter (AND).
Inactive people and withdrawn competitor registrations are excluded from their
respective audiences. An attendee who withdraws and then receives an independent
staff role remains eligible for that current staff selector; withdrawal does not
revoke staff authority. Team selection requires a current
confirmed place and an active published entry. Offer selection checks the pending
offer's expiry at resolution, without waiting for a maintenance worker.
Preparation selection uses current task ownership/status and current role grants.

Help/outcome-review facts are owned status/revision records. Official clarification,
rubric and result facts must be explicitly published. Optional resource scope is
checked against current grants, including mentor conflict exclusions. Selectors
can target owned help/outcome reminders or a published clarification/rubric fact;
unauthorized people are omitted before recipient rendering. A nonexistent source
remains unresolved. There are no private-text columns in this fact contract.

Future domain owners call `RecordOfficialFact` in their own authorized source
transaction after validating actual publication, status and ownership. This is
an internal producer boundary, not an HTTP composer command, and it neither
creates an official source nor grants publication authority. Future help,
clarification, assessment and result tickets must supply genuine source facts;
#77 never invents their content or marks missing facts resolved.

Challenge/brief titles come only from a released version that the individual
recipient can currently read. Challenge interpolation also checks the planned
T−7 instant and actual common release. Release-purpose messages require that
released fact even if the composer omitted its variable. Results require an
explicit published result fact. Postponement messages require an approved notice
matching the current revised dates and must carry live start/end and submission
cutoff; merely editing dates cannot authorize such a message.

Preview snapshots retain the exact template/selector versions, rendered recipients,
unresolved fields and approved fact fingerprint. Approval rebuilds the preview
under the event lock and rejects changed recipients, content, dates or facts.
Snapshots and approvals are append-only, and each mutation records a narrow audit
reference without message text in generic audit metadata.

`ResolveApprovedInTransaction` is the reusable execution boundary for later
campaign/delivery work. It rechecks the approver's current organiser authority,
current template/selector versions, approved event/content facts, recipient
eligibility, resource grants and embargoes. Current audience membership is
resolved again, so withdrawal, expiry and revoked assignments remove recipients;
newly eligible people may match the approved selector. Changed approved content,
dates or interpolated official revisions require fresh approval. An empty current
audience returns zero recipients and grants no delivery permission.

The caller must resolve within the transaction that records recipient attempts.
#77 owns retained previews and approval; campaign/delivery tickets own execution
identities, durable actual-recipient records, recipient attempt idempotency and
provider acceptance/delivery/reconciliation receipts. Resolving a preview does
not send a message and is not proof of delivery. The composer frontend belongs
to #78, and no SMTP, Teams or in-app delivery implementation is included here.

Verification uses synthetic fixtures and real PostgreSQL with `-race`: concurrent
version saves, actual registration withdrawal between preview/approval/execution,
current team and scoped outcome-review grants, withdrawn-attendee-to-staff
transitions and staff revocation, approver revocation, planned
challenge embargo, approved postponed dates, changed official revisions, owned
help status, preparation roles and pending offer expiry. Pure rendering tests
exercise HTML/Markdown escaping, forbidden/malformed variables and single-pass
substitution. HTTP tests validate the generated contract, no-store responses,
CSRF, stale revisions, strict request fields and current organiser access. The
implementation PR records final results and complete CI for its merge head.
