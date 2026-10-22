# Communication automation verification (#82)

This branch is in progress. It is not a claim that all #82 journeys are complete. Broader lifecycle/failure coverage, final ordered-main rebasing and actual help/clarification integration remain to finish. HTTP contracts, transactional queue reports and production SMTP/inbox composition have local verification below. Help requests (#159) and official clarification publication (#161) have no actual implemented source journey yet, so their aggregate acceptance remains unmet. Outcome-specific 30/60/90-day production remains owned by #117.

## Tested domain boundaries

- Durable campaign bindings preserve exact approved campaign version, typed trigger, source identity/revision, deadline and explicit organiser review. Schedules do not silently retime generic approvals. Source discovery produces deduplicated persisted occurrences and transactional outbox work.
- Pause prevents execution. Late work becomes a durable missed action; resume does not emit an uncontrolled burst. Explicit individual review rechecks current approval/source and permits the reviewed window. Current withdrawals/new confirmed competitors are resolved at execution.
- Generic cancellation/version changes prevent the old queued occurrence from producing provider effects. Dispatch itself only records provider intents in its outer transaction, never sends external messages.
- An expected-publication plan is a separate preview and explicit approval. Its preview shows literal template syntax, unresolved future variable names and the exact already-approved content ID/version. It does not return a privileged title/body or mark the generic campaign draft approved.
- The typed plan binds the complete immutable source document JSON, exact approved metadata, complete content-item set, source/current organiser authority, event dates/timezone/timing, template/selector versions/digests and provider revisions. Delivery requires the same content version's actual released state and recorded successful publication transition. Only that exact reviewed metadata transition is expected; changed templates, documents, dates, providers, other content or source authority require fresh review. Full-document hashing includes future additive rubric references/digests.
- Only after actual release does the canonical scoped resolver invoke the ordinary current-reader renderer. Explicit source audience routing excludes implicit organiser preview privilege. A participant still cannot read the source after plan approval but before release.
- Archive retains the normal write gate. An operation must additionally carry a durable retained occurrence marker, its handler must explicitly implement current read-only eligibility, and the current persisted occurrence must satisfy the event's privacy retention deadline. The canonical follow-up resolver separately requires an open exact-version owner-scoped official outcome fact, current authority/audience/resource access, explicit approved retained follow-up and bounded retain-until. The ordinary generic resolver still denies archived events.

## Executed checks

- Full current `internal/automation` real PostgreSQL/race suite: pass, 14.911s.
- Full affected `internal/jobs` real PostgreSQL/race suite: pass, 22.873s.
- Full affected `internal/communications` real PostgreSQL/race suite: pass, 30.296s.
- Direct participant-reader embargo addition: pass, 3.003s.
- Scoped Go lint: zero issues after extracting source, scope, proof and retained-projection boundaries.

The archive test uses the actual event transition and jobs execution path: an ordinary operation is cancelled while a specifically authorised retained follow-up survives; a resolved source prevents effects. Approval beyond the event privacy deadline fails. Publication tests use the real content Save/review/approve/Dispatch workflow: planned clock time alone creates no notification, the recorded guarded release produces one scoped notification, the generic campaign remains draft, and unknown variables are rejected by the existing template boundary.

These are local checkpoint results. Final ordered migration checks must include merged #78/schema26, #79/schema27, #80/schema28 and #81/schema29 before schema 30. Temporary dependency commits and the separately merged worker snooze fix must be removed on the final main rebase. Exact-head CI and the remaining journeys are still required before merge/readiness claims.

## HTTP and queue-report checkpoint

The organiser-only HTTP API and generated browser client expose durable bindings, review generation, pause/review/cancellation, named unresolved publication values and per-channel local queue evidence. Real HTTP checks cover current organiser authority, anonymous/outsider/platform-admin denial, wrong CSRF, valid stale revision conflict, arbitrary recipient rejection, cross-event denial and later role revocation: race PASS 5.841s. Generated contracts pass the repository generation command.

Queue reports distinguish new provider intents, local in-app persistence, retained provider statuses and unavailable-person counts. `accepted` means relay/provider acceptance only. An all-unavailable report commits its evidence and records `blocked`; a mixed report records `partially_queued`, and already-persisted valid intents remain eligible under current authority. Explicit review increments the durable scope generation and requires every selected provider's current outstanding-effect guard before changing the binding. Provider identities retain deduplication across those reviewed generations.

Real PostgreSQL recording-adapter orchestration checks for all-unavailable evidence and partial current-scope eligibility pass under race detection in 4.500s. These tests exercise transaction ownership at the automation seam; they do not substitute for actual SMTP verified-mailbox, immutable omission and accepted/ambiguous receipt integration; the subsequent production composition checkpoint below adds that coverage. Current existing domain regressions pass 13.560s and scoped lint has 0 issues at this checkpoint. The shared jobs callback lock-order defect is tracked separately in PR #191; it is not hidden in this feature branch.

## Production SMTP/inbox composition checkpoint

The SMTP and inbox occurrence adapters are now temporarily stacked for direct integration. Nonzero scopes use the canonical persisted plan/scope resolver; ordinary one-shot queues keep their original strict generic approval paths. SMTP queue creation resolves the batch once and the worker renders literal text only for its current person. Immutable SMTP snapshots and inbox provenance retain typed approval kind, plan approval ID/binding and source ID/version, with generated database columns in schema 30.

Actual provider integration uses a recording relay without network delivery. Real PostgreSQL/race checks prove all-unverified SMTP omission evidence survives with no send intent, later mailbox verification cannot silently replay it, and explicit review creates its first intent. A mixed SMTP/inbox occurrence retains one SMTP intent, one immutable omission and two inbox rows atomically; no SMTP receipt exists until its worker records relay acceptance, and partial dispatch state does not strand that current intent. Initial pair: PASS 16.037s. Separate typed-publication and archived-retained SMTP cases: PASS 4.290s. The generic campaign stays draft under typed publication, and both providers retain matching exact plan/source provenance.

The retained SMTP worker implements the narrow read-only capability and its operation carries the durable occurrence marker. The separately merged PR #191 claim/recovery ordering is adapted here: both eligibility callbacks execute without event/operation row locks, then Event→Operation locking revalidates the event revision, candidate revision/identity, retained marker and current SQL retention proof. No handler callback executes inside that claim transaction. Current composed scoped lint: zero issues.

At that checkpoint, Teams occurrence routing and the shared application runtime graph were pending; the following checkpoint adds them. Additional lifecycle/failure integration and final ordered-main checks remain pending. These checkpoint tests do not satisfy the absent actual help/clarification journeys.

## Shared runtime and persisted invalidation checkpoint

API and worker construction now use one runtime graph for automation, SMTP production/self-test, in-app self-test and Teams. Automation binds each provider's own queue/report and outstanding-effect review guard. The five-second worker maintenance runs actual content publication first and then reconciles durable communication sources. A real PostgreSQL test constructs independent producer and consumer runtimes, approves an in-app campaign/schedule, discovers and executes it through the actual configured job handler, and reads one participant inbox item across retry; unselected external channels get no work.

Scoped Teams queue creation and worker eligibility re-resolve the same persisted public scope, literal broadcast restrictions and exact approval/source provenance. Queue retries retain one occurrence intent and report the previous state. Review cannot strand a pending broadcast, and a withdrawn current audience loses eligibility. The actual guarded challenge test crosses London's DST boundary using the agreed seven-calendar-day rule: clock time alone produces no occurrence, successful common publication produces one typed intent, and the generic campaign remains draft. These are queue/eligibility tests with synthetic encrypted configuration; they perform no DNS, network attempt or real channel send.

Explicit review of a settled mixed SMTP/inbox occurrence retains the accepted SMTP identity and both existing inbox identities. A newly verified previously omitted person gets one first SMTP intent after review; the accepted person's relay acceptance is reported separately and never replayed.

Pending/paused occurrences now receive a bounded keyset sweep independent of rediscovery. A withdrawn source cannot disappear from discovery while leaving stale pending work. Edited approval becomes blocked, explicit cancellation becomes cancelled, lateness becomes missed, and obsolete outer operations are cancelled before any effect. Historical queued channel reports and provider receipts remain intact. A known invalidation racing an already running outer claim terminates that obsolete attempt after persisting review state; explicit review uses a fresh orchestration generation.

Executed current focused real PostgreSQL/race checks: automation/provider/pending-invalidation/retained-callback **PASS 17.074s**; independent runtime **PASS 3.319s**. The retained callback separately fails against the previous callback-under-lock implementation (**FAIL 3.958s**) and passes event-first revalidation (**PASS 3.404s**). Actual withdrawal has its own old-implementation red proof before the pending sweep. Final broader suites, actual ordered-main migrations and the requested-source/discovery pagination correction remain pending before readiness. Actual #159/#161 help/clarification source integration is still required.

## Discovery and owning-domain approval checkpoint

Discovery applies a requested source ID before the page limit. Built-in metadata pages use deterministic UUID/revision keys, and production reconciliation takes at most 200 unknown source revisions per exact immutable binding version. A durable occurrence anti-join restarts at the lowest remaining key each pass, so delayed inserts with earlier keys are not skipped. The source-identity index supports that bounded lookup. Existing pending/paused reconciliation remains independent of source discovery. No historical timestamp or approved due time is rewritten.

`TestLifecycleDiscoveryFindsRequestedSourceBeyondHistoricalPage` reproduces the missing requested item after 1,205 historical sources. `TestLifecycleUnknownSourceSweepMakesBoundedProgressPerBindingVersion` reproduces an unbounded first pass, then proves 200/400/600/605 progress, late earlier-key discovery, and a separately bounded sweep for an explicitly re-reviewed binding version. Both fail before the correction (combined 3.949s) and pass with the correction (4.506s).

Declared future help, clarification, rubric and results journeys may be saved as drafts, but cannot be approved without their authoritative owning-domain adapter. All four previously returned successful approval without an adapter (red 1.924s). The new gate does not fabricate source facts or mark a future journey resolved. Actual #159/#161 runtime integration, final migration ordering and exact-head CI remain required; this is still an incomplete #82 checkpoint.

Final targeted discovery/owning-adapter PostgreSQL/race checks pass **11.831s**; scoped lint reports **0 issues**. The broader automation run completed in 105.576s with one disposable PostgreSQL startup failure: Docker's container inspect request timed out before the `partially_queued` fixture could run. Its isolated report regression rerun passes **4.180s**. All other tests in that broader run passed; this is not reported as a clean full-suite result or a final merged-base gate.

## Actual-main32 integration checkpoint

Only automation-owned commits were replayed onto merged recovery main
`ebfe86d4d6f332a7915c3ec6c9741aae41e93d2c`; old SMTP/Teams/inbox dependency
commits were dropped. Native help, observation, rubric and recovery routes,
`Session.draft_scope`, canonical attendee `WorkspaceOrigin`, and the single
shared inbox/help owner graph are retained. The additive automation schema is
explicitly provisional and unnumbered; no migration slot or final upgrade proof
is claimed yet. Focused app/HTTP fixtures apply that provisional SQL directly.

The native help adapter consumes actual latest transitions through the owner's
metadata paging and current-audience API. Automation's same-transaction source
maintenance generates a real overdue transition before discovery; stale
revisions and withdrawn recipients lose dispatch authority. Routing carries no
request description or location. Real PostgreSQL/race proofs pass automation
3.050s, app5.562s and HTTP3.722s; scoped Go lint reports zero issues and generation
succeeds. These are focused proofs, not a final full-suite/CI claim.

Actual clarification and rubric publication adapters remain pending at this
checkpoint. Results #112 has no implemented native publisher: unknown results
approval remains fail-closed, and full #82 must remain open until that real
integration is verified. Delivered historical receipts/links retain exact
provenance; corrections invalidate old pending dispatch and never replay it.

### Native help maintenance and rubric publication

Help now pages every event with writable waiting or claimed requests independently
of campaign bindings, then reuses native transaction-aware maintenance. An event
with zero automation bindings produces an actual due inbox followup; early,
archived and repeated sweeps do not add one (app2.929s). Claimed requests whose
helper departs return to the native waiting cycle (2.813s); the owner also
traverses53 events beyond its first keyset page without duplicates (4.518s).
Unknown help-source discovery passes1,005 records in bounded batches, skipping
previously recorded pages without a lifetime cap (4.396s). A real native help
transition traverses approved automation into actual once-only local inbox
persistence without copying private prose or calling the relay (5.521s).

The rubric adapter names the native actual challenge-publication tuple. Initial
common release and reviewed material corrections are sources; an unchanged
criteria challenge-only release is not a new rubric notice. Corrections use the
native immutable notice-recipient intents intersected with current confirmed
readers. The owner reuses its canonical public exact-reference/digest reader in
the same transaction. Native actual publication/correction, stale-source denial,
withdrawal, post-publication entrant exclusion and unchanged-criteria no-replay
proof passes2.885s.

The existing typed expected-publication approval also supports rubric.published
using the full exact challenge document, including its rubric reference. Its
source identity is the challenge ID/version, not a date or rubric draft. It
retains the challenge_release template purpose and never marks a generic draft
approved. A generic prepublication approval correctly blocked after actual
content changed (red: zero inbox records); the explicitly reviewed guarded
transition now produces exactly two actual local inbox records on repeated
reconciliation/execution, with no private judge guidance or external send
(2.782s). Unknown future source fields remain blocked. Actual clarification and
results integration remain outstanding; no full #82 closure is claimed.
