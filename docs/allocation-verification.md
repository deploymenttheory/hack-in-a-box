# Explainable allocation suggestions

Issue #64 supplies organiser-only proposals; #65 supplies the review interface.
Suggestions never publish, move a competitor, create a competition entry, retire
a team or enqueue a notification. Every proposal needs explicit organiser review.

## API and inputs

Use the current confirmed roster and team IDs from `GET /api/v1/events/{event_id}/teams`.
The organiser posts to `/api/v1/events/{event_id}/allocation-suggestions` with the
current event revision in `If-Match` and the session's CSRF token. For example:

```json
{
  "seed": "review-1",
  "limits": {"maximum_size": null, "maximum_teams": null},
  "declarations": []
}
```

Declarations optionally supply a confirmed person's `id`, selected `skills`,
`experience` (0 unknown, 1 starting out, 2 some experience, 3 experienced) and
`prefer_with` confirmed-person IDs. Callers deliberately select these declared
features; the service does not infer question meaning from labels or import
arbitrary form answers. Missing declarations remain unknown. It always includes
the complete current confirmed roster, rather than trusting a caller-supplied
participant list. Unknown, duplicate and cross-event identities are rejected.
Neither operational profiles nor their dietary/accessibility fields are read.
The contract rejects extra private fields in the request.

The event row lock serialises the snapshot with registration, membership and
freeze changes. Current organiser authority, the event revision, permitted stage,
recorded freeze and freeze deadline are checked on every request. Platform
administration alone grants no access. Responses are `no-store`.

## Locks and constraints

Published teams are locked automatically, including teams present when upgrading
to schema 20. A locked cohort remains together; additional confirmed people can
join without displacing the cohort, subject to its actual capacity. Moving a
locked member requires an organiser's explicit `unlock` team command with a
reason and current event revision. Previously edited drafts cannot silently
replace a locked published cohort at publication or while generating suggestions.
Publish reviewed changes before locking a divergent cohort again.

The `lock`, `unlock` and `capacity` actions use the existing team command endpoint.
`capacity` is nullable: null clears the per-team hard cap; a number from 3 to
10,000 sets it. Lowering it below the current draft membership is rejected rather
than evicting anyone. Manual moves and publication enforce stored capacities.
All these actions are prohibited after the competitor freeze; existing captain
remediation and withdrawal behavior remain separate.

Preferences are exposed independently: target five, preferred four to six, hard
publication minimum three. Six is not a universal maximum. Optional request
limits constrain that proposal's maximum team size/count; persisted per-team
capacities remain additional hard constraints. A proposal exposes both the
preferences and its request limits for review.

## Deterministic behavior

The engine normalises declared skill names, orders candidates by experience,
scarce skills and a seeded hash, and uses stable IDs for ties and proposed new
teams. Reordering input slices or map iteration does not change the result.
It preserves locked cohorts, chooses a feasible team count that first maximises
assigned people and then favours sizes near five, and greedily balances sizes,
declared skills, experience and soft pairings. This is an explainable heuristic,
not a promise of globally optimal packing or an LLM decision.

A locked depleted/over-capacity group needs an explicit unlock or constraint
decision. Unlocked groups below three are proposed for retirement and their
survivors are reassigned to another or a newly formed group. Full destinations
can require explicitly unlocked rebalancing and new viable groups. Impossible
combinations return unresolved people and explanations rather than illegal
assignments. Sparse skill/experience coverage and unmet soft pairings are also
reported. Nothing acts after freeze; depleted frozen teams continue or withdraw
under the existing lifecycle rules.

## Verification

`internal/teams/allocation_test.go` covers balanced, sparse-skill, locked,
over-capacity and impossible-size fixtures; reversed-input determinism; unchanged
inputs; preferred versus hard limits; depleted-team remediation; and 400 bounded
population/capacity/team-count invariant combinations.

`internal/teams/allocation_service_test.go` uses real PostgreSQL to check current
rosters, lock/unlock and capacities, unchanged membership/revision/audit/delivery
state after suggestion generation, privacy-field independence, cross-event and
role denial, stale revisions, divergent published/draft cohorts, and races with
the hard freeze. A schema-19 upgrade fixture checks existing published teams
become locked without changing either membership snapshot. Existing registration lifecycle tests now explicitly unlock a
depleted published team before its reviewed reassignment.

`internal/httpapi/allocation_test.go` validates actual HTTP responses against the
generated OpenAPI contract. It checks organiser/competitor/administrator/outsider
access, CSRF, foreign identities, rejection of private input fields, cache headers,
current revisions, lock commands and the freeze deadline.

```sh
HACKBOX_INTEGRATION=1 go test -race -timeout 5m ./internal/teams ./internal/registration ./internal/httpapi
sh scripts/check-generated.sh
```

The implementation PR records the full checks, including Docker migration and
persistence verification. Deploy schema 20 explicitly with the matching image;
the application never upgrades its database implicitly.
