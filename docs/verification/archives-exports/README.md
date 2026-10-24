# Issue119 archive/export implementation record

Base: main `18b23f4e3201f9ab0d10ef3875628585f0ffd74f`.
This is an early native backend checkpoint, not completion of issue119.

## First coherent slice

- Current-authority archived-event metadata directory and detail.
- Current-organiser per-event participation summary and cursor-based cross-event comparison, with independent registration records/people, confirmed attendees, recorded attendance and native frozen-entry counts.
- Explicit source/units/denominators; no global population, inferred performance, score, rank, award or return-on-investment calculation. Disbanded/inactive entries are unused/unscored, not transferred or counted as completed.
- Deliberate audited formula-safe summary CSV using the existing CSVCell owner, no credentials/person fields and the existing no-store API middleware.
- Canonical SQL queries, OpenAPI, generated Go/TypeScript boundaries, HTTP adapters and actual runtime dependency wiring.
- Missing final-judging108/published-results113 are explicit unavailable sources, never zero/empty fabricated results.

Generation completed. Bounded compile of archives/httpapi/app completed successfully; this is compile proof only, not a claimed PostgreSQL test pass. First real PostgreSQL role/cohort/paging/export-audit tests are authored and pending execution. No real provider effect or new schema migration is part of this initial slice.

## Full remaining scope (issue stays open)

1. Compose authorised roster, deliverable/submission and assessment exports through their actual field/resource owners. Current organiser status does not grant independent private scorecards or logistics/profile fields.
2. Export current published outcome/showcase projections only with underlying evidence/media/retention authorization; include exact objective version/measure/baseline/target/unit and dated/source-labelled observations and follow-up status. Distinguish continued work from demonstrated-adoption evidence and from estimates/self reports.
3. Integrate native suppressed feedback/report aggregates without respondent/raw-answer leakage or false denominators; current approved snapshots and privacy erasure remain authoritative.
4. Prove archived competition is read-only through real reachable archive transitions, ordinary reads/history and worker maintenance, including a cancelled/never-held event; fix only actual native owner mutations. Preserve the Submissions owner and explicit117 post-event follow-up, with no alternate outcome store or new grace/eligibility rule.
5. Prove event-locked archive cancellation of unintended jobs and preservation/current eligibility of explicitly retained approved review reminders; test actual queue execution and safe decision history.
6. Real PostgreSQL/race, HTTP/current-role/no-store/CSV allowlist and retention/concurrency proof; generated parity, appropriate bounded lint/compile and final independent reviews/every exact-head CI.

No smaller checkpoint is treated as satisfying the full issue. No new judging,
results, awards or financial-return owner is invented for absent dependencies.

## Archive boundary and recorded attendance checkpoint

The real native preparation → cancelled → archived path was exercised with no
formal submission states. Before the fix, advancing the clock and maintaining
submissions created two `not_submitted` outcomes and changed gate facts inside the
archive. The regression now preserves the exact event/fact/outcome snapshot;
ordinary missing archived submission reads/history return not found instead of
initialising an `open` outcome. Existing pre-archive assessment settlement remains
in the native submission owner.

The reporting metric is `recorded_attendance_people`, with no registration
population denominator: recorded people may include people without an attendee
registration. Missing records and explicit absence both produce zero recorded
presence, while physical attendance coverage remains explicitly unavailable.

Executed with real isolated PostgreSQL, `GOMAXPROCS=2 GOFLAGS=-p=1
HACKBOX_INTEGRATION=1 go test ./internal/submissions ./internal/archives -run
'TestNeverHeldArchiveDoesNotInventSubmissionOutcomes|TestArchive' -count=1`:
passed (submissions 2.107s; archives 4.441s). The initial red proof is retained in
`/private/tmp/hackbox119-archive-boundary-red.log`; green log is
`/private/tmp/hackbox119-archive-first-pg.log`. This is bounded proof, not the full
remaining export/worker/race/HTTP acceptance gate.
