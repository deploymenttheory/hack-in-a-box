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

Bindings and occurrences have separate bounded UUID cursors. HTTP defaults to50
and accepts at most200 per collection; returned cursors are independent. To keep
an exhausted collection exhausted while paging the other, retain that
collection's last returned UUID. Every page rechecks current organiser authority.
Controller reconciliation scans all current binding pages; source discovery
processes bounded unknown source batches per exact binding version and resumes
at the lowest unknown key. Requested source filtering precedes limits. There is
no permanent1,001-record ceiling or OFFSET-based skip.

## Executed evidence

These are local results, not a substitute for final exact-head CI:

- Normal populated34→35 preservation, empty rollback, pause-only guarded rollback:
  real PostgreSQL/race PASS3.082s.
- Current bindings/occurrences beyond1,000, independent cursor traversal,
  exhausted pages and authority revocation: PASS3.438s; HTTP cursor validation
  and current authority: PASS3.917s.
- Native clarification metadata pages over1,005 sources, requested-ID filtering,
  private canary exclusion and current withdrawal: PASS3.030s.
- Actual native clarification publication→approved broad/pinned routing→dedup
  inbox, correction/stale denial and invalidated campaign approval, together
  with current native rubric/expected-publication cases: PASS6.213s.
- Native rubric current publication/material correction/no replay: PASS3.124s.
  Removing its owning adapter between preview and approval fails closed:
  PASS3.018s.

Earlier affected-source checkpoint evidence remains relevant: full automation
race40.783s, native help20.271s, native rubric13.087s, composed runtime6.871s,
and scorecard HTTP/privacy5.042s. Native help additionally proves zero-binding
actual due inbox followup (early/archived/repeated passes add none), claimed-owner
loss,53-event keyset progress and1,005-source progress. Provider composition
uses recording transports and actual durable provider tables; no real SMTP or
Teams message was sent. Live Teams-channel acceptance remains separately open
under #80.

Final actual-main rebase, generated parity, affected suites and exact-head CI
must be recorded before merge readiness. Actual native #112 results integration
remains outstanding even when those checks pass; #82 stays open.
