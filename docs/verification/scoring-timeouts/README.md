# Native scoring timeout investigation (#319)

The retained synthetic lifecycle and qualification browser failures exposed
HTTP503 responses at the unchanged10-second request budget. Disk-full500 was
a separate failure, not an explanation for all503s. The read-only R6 observer
recorded55 lock-wait samples; a worker waited behind an API-held transaction
for more than8seconds. Retained databases/traces remain private.

## Short reproduction

```sh
HACKBOX_INTEGRATION=1 GOMAXPROCS=2 go test -v -p 1 ./internal/app \
  -run '^TestQualificationFiveTeamSaveWithDatabaseRoundTripCost$' \
  -count=1 -timeout=2m
```

This uses the real five-team native fixture, current generated HTTP router,
current role/source policy and a fresh original moderated-card command. A pgx
query tracer adds a fixed4ms per-query round-trip cost; no provider call, fake
HTTP handler, accepted-write retry or wider request timeout is substituted.
The normal router timeout remains10seconds. Setup is a component fixture, not
the complete #120 lifecycle. The original normal-OIDC retained-database probe
also showed thousands of SQL calls for a single source read.

Before the production changes, the short4ms case failed twice with HTTP503 at
10seconds. A successful1ms control completed2,770 queries. Bulk settlement alone
and focused mentor authority alone were insufficient; their failing probes
remain failures. With the combined changes, the unchanged4ms case passed twice
in6.586s and6.548s with1,320 queries, preserving its exact original receipt.
These are development results before final formatting/rebase; final-head owner,
browser/lifecycle and CI verification remains required.

## Change and preserved boundaries

- Submission settlement reads the current event-locked entry/state census in
  one query, creates only missing states and updates derived gate facts only
  when needed. It still records each actual outcome/freeze change exactly once.
  The scoped reader uses the exact event lock it already acquired.
- Basic submission authority and content role/attendance checks each read their
  current facts together. No decision is cached across actors, requests or
  transactions; native mentor/reviewer/judge assignment checks remain separate.
  An absent competition claim still means non-competing membership, not an error
  or fabricated confirmed place.
- A panel actor check queries its current published team, own-team exclusion,
  declared conflicts and assignment directly. Full capacity/availability/coverage
  validation remains in `ReviewCoverageInTransaction` and publication owners.
  The new policy matrix preserves revoked/inactive, draft-member, conflict and
  missing/unpublished-team denials.

Public API and database schema are unchanged. No timeout, authority gate,
cutoff rule, audience or unknown-write policy has been relaxed. No production
query logging or diagnostic endpoint is added. Explicitly gated retained-data
and query-census probes are diagnostics, not substitutes for the regression.

Initial affected-owner race suites passed before the final content-reader
batching; the final access-policy race suite passed7.849s. All affected suites
must pass again on the frozen integrated candidate. Full original scenarios and
final CI remain pending; #319 stays open.
