# Communication automation verification (#82)

This implements the automation core with actual native help (#159), official
clarification (#161), and rubric publication sources. It references #82 rather
than closing it: native published-results integration requires the actual #112
publisher, which is not implemented. Unknown results approval fails closed.
Outcome-specific 30/60/90-day production remains owned by #117.

## Current contract

Durable bindings retain exact campaign version, typed trigger/source scope,
approval provenance, deadline and organiser review. Discovery creates persisted,
deduplicated occurrences and outbox work; it never sends provider requests.
Current approval, actor authority, source version, audience, provider revision,
pause and lateness are checked again before effects. Late work becomes a durable
missed action; resume does not burst-send missed work. Explicit review uses a new
scope generation after provider outstanding-effect guards pass.

Event-wide campaign pause stops all configured campaign channels: in-app, SMTP
and Teams. Native help/clarification publication bookkeeping and their native
transactional inbox intents remain governed by their owning domains; campaign
pause does not silently drop, replay or reinterpret those independent intents.
The worker runs native help maintenance even for events with zero campaign
bindings. Waiting and claimed requests are included, with current archival/write
gates and native followup deduplication.

Actual help routing uses metadata-only latest native transitions and the native
current-audience policy. It excludes descriptions and locations. Clarification
routing uses actual current published answer tuples and the canonical released
challenge/rubric reader, without querying private questions or answer prose.
Corrections invalidate old pending work. A still-valid explicitly approved broad
literal subscription may discover a new native publication; a pinned source
version never advances. Native publication approval cannot repair an invalid
campaign approval. Historical accepted receipts and authorised inbox links keep
immutable provenance without replay.

Rubric routing consumes actual native challenge-publication tuples and material
notice recipient intents. Current readers are intersected with the immutable
notice audience. A challenge-only release with unchanged criteria creates no new
rubric notice. Native ref/version/digest checks remain authoritative.

Expected-publication approval is a separate explicit plan, not generic campaign
approval. Preview names unresolved future variables and binds the complete exact
approved challenge document, including rubric reference, metadata, authority,
provider, template, selector and timing. Delivery requires that same version's
actual guarded publication. Planned time alone grants no participant visibility
and produces no notification. Changed sources or approval guards require review.

Archive retains the ordinary write gate. Only an explicitly retained followup
with a durable occurrence marker, a handler's narrow read-only capability and
current SQL/privacy-deadline proof can execute. The canonical scoped resolver
also checks current exact source, authority and audience. Ordinary one-shot
provider queues retain their strict generic approval paths.

Queue reports distinguish new intents, local inbox persistence, immutable
unavailable-recipient evidence and retained provider states. All-unavailable
SMTP evidence commits as blocked; mixed results are partially queued and valid
existing intents remain eligible. Relay/provider acceptance is not delivery.
Explicit review cannot replay accepted identities or silently deliver a formerly
omitted recipient without a newly reviewed scope.

## Persistence and bounded progress

Normal Goose migration 00035 follows actual merged schema34. No provisional SQL
is applied by fixtures. A populated34→35 preservation test exercises native
help, clarification and independent scorecard data plus explicit historical
SMTP/Teams receipt fixtures. It compares retained rows across upgrade, empty35
rollback and re-upgrade. A real pause-only command then prevents rollback from
losing either settings or its immutable operational journal.

Bindings and occurrences have separate bounded UUID cursors. HTTP defaults to 50
and accepts at most 200 per collection; returned cursors are independent. To keep
an exhausted collection exhausted while paging the other, retain that
collection's last returned UUID. Every page rechecks current organiser authority.
Controller reconciliation scans all current binding pages; source discovery
processes bounded unknown source batches per exact binding version and resumes
at the lowest unknown key. Requested source filtering precedes limits. There is
no permanent 1,001-record ceiling or OFFSET-based skip.

## Executed evidence

These are local results, not a substitute for final exact-head CI:

- Normal populated34→35 preservation, empty rollback, pause-only guarded rollback:
  real PostgreSQL/race PASS 3.082s.
- Current bindings/occurrences beyond1,000, independent cursor traversal,
  exhausted pages and authority revocation: PASS 3.438s; HTTP cursor validation
  and current authority: PASS 3.917s.
- Native clarification metadata pages over 1,005 sources, requested-ID filtering,
  private canary exclusion and current withdrawal: PASS 3.030s.
- Actual native clarification publication→approved broad/pinned routing→dedup
  inbox, correction/stale denial and invalidated campaign approval, together
  with current native rubric/expected-publication cases: PASS 6.213s.
- Native rubric current publication/material correction/no replay: PASS 3.124s.
  Removing its owning adapter between preview and approval fails closed:
  PASS 3.018s.

Final corrected-source full PostgreSQL/race suites pass: automation 72.764s,
help 39.332s, clarification 11.300s, communications 45.609s, SMTP 73.040s and
inbox 21.395s. The actual Teams Workflows full suite passes 59.164s. The earlier unchanged jobs and rubric suites pass 42.284s and 36.264s.
Independent root PostgreSQL/race checks of normal35, retired/current operations,
original provider guards and canonical reviewed reuse also pass
(automation 17.147s/inbox 4.591s/SMTP 4.493s).
Full Go lint reports zero issues; both frontend bundles, frontend lint and
reproducible generated contracts pass. Actual current-main runtime/HTTP checks
above preserve help projections, recovery draft_scope and existing provider routes.

Broader verification exposed three integration boundaries and retained their
original assertions: historical schema fixtures now use explicit old columns
and compare retained JSON without the new nullable approval field; a non-complete
SMTP scope with an empty selector intersection returns zero effects; ordinary
inbox scope-binding changes still conflict. Only canonical persisted positive
review-generation proof permits reuse of an identical inbox identity across
bindings. Wrong occurrence/binding/nonpositive review revisions deny reuse;
source, typed provenance, body and format must still match, and retained rows
are never rewritten. Focused inbox proof passes 2.846s, actual canonical mixed
SMTP/inbox review plus retired-job guards pass 8.072s, and original SMTP scoped
and historical26 preservation regressions pass 4.151s.

A retired orchestration previously overwrote a reviewed/cancelled/queued
successor as blocked (real PostgreSQL red 4.549s). The fallback now locks the
event, rereads the occurrence, and requires the same pending operation,
review revision, scope binding and unchanged state timestamp as the failed
attempt. Retired jobs and current terminal states cannot change that row or
journal; the new reviewed generation still executes exactly once.

Native help additionally proves zero-binding actual due inbox followup
(early/archived/repeated passes add none), claimed-owner loss, 53-event keyset
progress and 1,005-source progress. Provider composition uses recording transports
and actual durable provider tables; no real SMTP or Teams message was sent.
Live Teams-channel acceptance remains separately open under #80.

Actual native #112 results integration remains outstanding even when all checks
pass; #82 stays open. Final exact-head CI is required before merge.
