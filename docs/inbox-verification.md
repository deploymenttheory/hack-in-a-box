# Recipient inbox and source-owner activation

Issue #81 owns recipient persistence/read state, current-authority filtered inbox
projections and safe resource navigation. Schema29 follows SMTP27 and Teams28.
Final ordered28→29 upgrade evidence and app-runtime composition must be recorded
before this implementation can merge; local fresh-schema tests alone do not prove
that sequence.

Every list, body-open and read-state mutation rechecks the active recipient and
current source authority under event locks. Hidden notifications contribute no
previews, unread counts or pagination cursor. Lists contain fixed labels rather
than message subjects, source titles or bodies. There is no HTTP endpoint accepting
a recipient or destination URL. Current typed destinations contain only a released
content UUID, navigated through the existing reader, which checks current access
again.

Campaign dispatch uses the strict approved campaign resolver. Actual delivery
retains exact immutable template/selector version identifiers and the personal
message. Historical reads separately apply the retained selector against current
roles, status, team and official source/embargo authority. Later cancellation,
source edits, due-time passage and archive write freeze do not erase an otherwise
authorised historical message. Losing its current source audience still hides it.
Read state belongs only to the fixed recipient.

`QueueApprovedInTransaction` atomically stores local in-app recipient messages;
#82 owns scheduling and pause/missed-action policy. The additive occurrence method
intersects the approved selector with the source policy's current recipients and
retains occurrence ID/binding. Repeated delivery of one occurrence deduplicates;
different reviewed occurrences remain separate. A missing scope policy fails
closed. Future expected-publication and archived-follow-up plans require #82's
canonical scoped approval resolver; the current adapter grants no bypass.

The real `communication_test_in_app` handler resolves #78's fixed requesting
organiser, persists one labelled test transactionally, and reconciles safely using
the same unique identity. It never reuses a competitor's personalised preview.

## Source journeys

| Source | Current support | Activation ownership |
| --- | --- | --- |
| Approved campaign | Current-recipient transactional local persistence; no unattended scheduling | #78 approval, #81 persistence, #82 automation |
| Organiser-only test | Actual local job delivery/reconciliation | #78 intent, #81 handler |
| Released content | Authorised typed resource reference and real participant/admin reader route; trusted source-owner transaction can record a fixed-recipient notification | Existing #69/#71 resource policy |
| Help-request update | Explicitly unavailable; no invented item or destination | #159 source/claiming and #160 helper UI |
| Reviewed common clarification | Explicitly unavailable; no invented item or destination | #161 official reviewed source and #162 feed |
| Outcome-review reminder | Explicitly unavailable; no invented item or destination | #117 owner/review schedule/evidence and #82 approved trigger |

The last three named journeys are unmet #81 acceptance until their actual owning
resources and routes exist and the following integration is verified. This core
implementation must not close #81 or assert those journeys pass.

An owning domain must record its official fact/source in its authorised source
transaction, supply a typed resource adapter that rechecks current ownership,
audience and publication, and call `RecordResourceInTransaction` for an eligible
fixed recipient. Activating a destination requires an explicit API allowlist and
an implemented browser route whose own read operation rechecks current authority.
Facts alone never create an implemented journey. No adapter accepts arbitrary URLs
or copies private titles, evidence, scores or credentials into previews.

## Verification

Real PostgreSQL race tests cover overlapping memberships, revoked recipient
status, concurrent duplicate delivery, independent read state, local River
self-test/reconciliation, cancelled/edited/archived readable history, withdrawn
resources, unavailable future owners and independent scoped occurrences. HTTP
contracts check recipient isolation, CSRF, no-store, open/read revocation and
filtered counts. Six desktop/mobile participant browser flows verify keyboard
opening/focus, safe hostile text, read/unread, event filters and the actual resource
route's revoked-access response. Both application bundles and lint pass locally.
Final exact-head CI and ordered migrations remain merge gates.
