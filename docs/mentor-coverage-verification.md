# Mentor coverage and scoped review access

Issue #66 owns backend mentor planning; #67 owns the administration matrix.
Competitor freeze never prevents permitted mentor replacement. None of the
mentor commands writes competitor memberships, captain selection or freeze state.

## Contract

`GET /api/v1/events/{event_id}/mentor-coverage` returns full planning state only to
current organisers. Confirmed competitors see their own published team's mentors;
mentors see their own published slots. Those views exclude draft assignments,
relationship declarations, coverage planning gaps and other people's complete
availability/capacity. Unassigned platform administrators have no implicit access.
The response includes the current event revision and uses `Cache-Control: no-store`.

Organisers use `POST` to the same route with CSRF, current `If-Match` and a recorded
reason. Supported actions are `settings`, `profile`, `team_skills`, `conflict`,
`assign`, `apply_suggestion` and `publish`. Profiles explicitly declare skills,
availability windows and an optional capacity override; names, active state and
membership authority always come from current server records. Profile updates
replace those declared features. A missing/null maximum clears the override.

The default maximum is two assigned teams per mentor and can be configured from
one to 10,000; a mentor-specific maximum overrides it. Both primary and second
review slots count toward workload. Capacity cannot be reduced below current
published workload. Assignment windows must lie within the live hack and be fully
covered by declared availability. Adjacent availability intervals are continuous;
a gap is never treated as available time. Partial primary cover is permitted only
with an explicit publication decision acknowledging the resulting coverage gap.

Conflict commands declare a known reporting or own-team relationship between an
active event mentor and a confirmed competitor in that event. Own-team membership
is also checked directly against current draft/published rosters. The service does
not infer organisational relationships from names, domains or private profiles.
These conflicts, mentor capacity, availability and distinct primary/second people
are hard assignment constraints. They cannot be waived by allowing coverage gaps.

`POST /mentor-coverage/suggestions` on the event route returns a deterministic,
read-only proposal tied to the event revision. The engine prioritises primary
coverage across all teams, then distinct second reviewers; it favours declared
skill overlap, available duration and lower workload with stable identity ties.
It returns missing/partial coverage rather than inventing availability or exceeding
capacity. `apply_suggestion` explicitly recomputes that proposal from the current
locked roster and replaces the draft. It does not publish or grant access.

Publication revalidates current roles, teams, declared availability, constraints
and revision in the same transaction as replacement of the published snapshot.
Competitor teams must already be published. Missing primary/second coverage or
partial primary coverage requires `allow_gaps: true` and the organiser's recorded
reason. Invalid assignments always block publication. Unrelated stale draft
problems do not prevent incremental repair of a different slot.

## Access and history

Published primary mentors receive their assigned team scope. Published second
reviewers receive the assigned competition entry's review scope, without general
team-resource access. They can read only formal mentor-audience submissions,
submitted files and formal history from live start. Private preparation, submitting
as the team and organiser approval remain unavailable. No review is opened early.

Current role revocation, own-team conflicts and declared reporting conflicts are
checked on every resource request. Mentor roster views also remove newly
conflicted assignments. Publication of a replacement removes the old grant
immediately because policy reads the current published snapshot. Authored history
is retained; neither publication nor revocation deletes audit records.

Draft edits append a narrow audit action and record the organiser's reason but
emit no delivery intent. Changed publication inserts one `mentors.published`
operation keyed `mentors-published:<committed event revision>`, targeting the event.
Unchanged publication inserts none. This is a durable reconciliation intent for
communication/repository consumers, not a claim that a provider delivered it.

## Verification

Pure race tests cover stable ordering, declared skills, capacity, primary-first
coverage, adjacent and gapped availability, partial cover, conflicts, own teams,
distinct reviewers, duplicate slots and unpublished competitor teams.

PostgreSQL tests exercise schema-20 upgrades without changing competing teams,
cross-event and distinct-slot database constraints, private roster visibility,
current-role/withdrawal revocation, read-only proposals, real primary versus
second-review permissions, and denial of private preparation. Mutation scenarios
cover capacity, selected partial availability, explicit gap approval, conflict
blocking, unpublished drafts, replacement access revocation, preserved audits,
unchanged publication and unchanged frozen competitor state.

The actual HTTP router is checked against generated OpenAPI responses for real
sessions, CSRF, organiser-only writes, foreign event/person IDs, unknown/private
fields, no-store headers, current revisions, stale publication and cancellation.
Deploy schema 21 explicitly with the matching image; services never migrate on
startup.
