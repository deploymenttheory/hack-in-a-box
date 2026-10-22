# Recipient inbox and source-owner activation

Issue #81 owns recipient persistence/read state, current-authority filtered inbox
projections and safe resource navigation. Schema30 follows SMTP27, Teams28 and versioned rubrics29.
The real ordered29→30 upgrade preserves the approved unpublished rubric reference,
public digest and private judge guidance, verified mailbox, immutable template history
and event state. The shared API/worker factory exercises actual organiser-only local
delivery while retaining SMTP and Teams handlers.

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
| Help-request update | Explicitly unavailable; no invented item or destination | #159 source and actual read-only help detail route; #160 owns full controls |
| Reviewed common clarification | Explicitly unavailable; no invented item or destination | #161 official reviewed source and #162 feed |
| Outcome-review reminder | Explicitly unavailable; no invented item or destination | #117 owner/review schedule/evidence and #82 approved trigger |

The last three named journeys are unmet #81 acceptance until their actual owning
resources and routes exist and the following integration is verified. This core
implementation can merge without closing #81 or asserting those journeys pass.
#159 and #161 directly depend on the merged inbox contract, so requiring full
#81 issue closure first would create an artificial cycle. Their owning domains
can use this merged core while aggregate #81 acceptance remains open; a separate
final #81 integration verification must exercise all named real journeys.

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
On the concrete SMTP27/Teams28/rubric29 base, real PostgreSQL/race checks pass:
inbox13.568s, inbox HTTP5.441s, shared runtime (including canonical SMTP factory)6.216s,
and full communications25.992s. Ordered29→30 preservation passes2.929s; historical
credential, SMTP, Teams and database upgrade/rollback suites pass4.162/3.948/4.096/4.032s.
Six admin desktop/mobile browser cases pass2.1s and the same six participant-bundle
cases pass2.2s on isolated preview ports. Current render-shaped HTML fixtures use
actual newlines after generated `<br>` tags; escaped hostile markup stays inert text.
Both bundles, Go/web lint and generated contracts pass. Final exact-head CI remains
a merge gate. Inbox30 permits empty rollback but refuses to discard recipient history.
