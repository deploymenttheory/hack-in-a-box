# Communication automation verification (#82)

This branch is in progress. It is not a claim that all #82 journeys are complete. Provider composition, production occurrence-aware SMTP/in-app/Teams routing and lifecycle integration coverage remain to finish. HTTP contracts and transactional queue-report orchestration have local verification below; production adapter composition is still required. Help requests (#159) and official clarification publication (#161) have no actual implemented source journey yet, so their aggregate acceptance remains unmet. Outcome-specific 30/60/90-day production remains owned by #117.

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

These are local checkpoint results. Final ordered migration checks must include merged #78/schema26, #79/schema27, #80/schema28 and #81/schema29 before schema30. Temporary dependency commits and the separately merged worker snooze fix must be removed on the final main rebase. Exact-head CI and the remaining journeys are still required before merge/readiness claims.

## HTTP and queue-report checkpoint

The organiser-only HTTP API and generated browser client expose durable bindings, review generation, pause/review/cancellation, named unresolved publication values and per-channel local queue evidence. Real HTTP checks cover current organiser authority, anonymous/outsider/platform-admin denial, wrong CSRF, valid stale revision conflict, arbitrary recipient rejection, cross-event denial and later role revocation: race PASS 5.841s. Generated contracts pass the repository generation command.

Queue reports distinguish new provider intents, local in-app persistence, retained provider statuses and unavailable-person counts. `accepted` means relay/provider acceptance only. An all-unavailable report commits its evidence and records `blocked`; a mixed report records `partially_queued`, and already-persisted valid intents remain eligible under current authority. Explicit review increments the durable scope generation and requires every selected provider's current outstanding-effect guard before changing the binding. Provider identities retain deduplication across those reviewed generations.

Real PostgreSQL recording-adapter orchestration checks for all-unavailable evidence and partial current-scope eligibility pass under race detection in 4.500s. These tests exercise transaction ownership at the automation seam; they do not substitute for actual SMTP verified-mailbox, immutable omission and accepted/ambiguous receipt integration, which is still pending adapter composition. Current existing domain regressions pass 13.560s and scoped lint has 0 issues at this checkpoint. The shared jobs callback lock-order defect is tracked separately in PR #191; it is not hidden in this feature branch.
