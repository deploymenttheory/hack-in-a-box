# Reviewed repository access

Repository access composes the native published teams, current confirmed claims,
verified numeric GitHub identities and published mentor coverage. It never edits
assignments. A frozen published team and actual challenge publication are both
required before a new external grant. Setting the timetable, approving a plan or
provisioning the private repository does not publish the challenge.

An organiser reviews a versioned plan and explicitly approves that exact version.
The plan includes native identity revisions, mentor coverage windows, proposed
changes and provider flags. Native changes create an unapproved replacement plan.
The worker checks the latest approval, current approver authority, native roster,
identity revision, immutable repository mapping and publication again before a
write. Withdrawal, account-link recovery, loss of mentor coverage and disbanding
produce reviewable removals of app-managed access while retaining frozen team
history. A new team receives no transferred work or inherited approval.

The organiser API is `GET/POST /api/v1/events/{event_id}/repositories/{team_id}/access`.
Commands are `review`, `approve` and `refresh`; writes require CSRF, current event
revision and a reason. Review accepts a caller-generated plan ID, and approve
requires the reviewed ID and version. Responses distinguish the exact receipt
from current state. `GET` on the sibling `/invitations` path returns only the
current confirmed member's or current published mentor's own invitation history,
after actual publication. Organiser plans and other collaborators are excluded.

## Provider evidence and limits

The App uses short-lived installation tokens scoped to the one repository:
Metadata read and Administration read for observations, Administration write for
invitation or removal. It resolves the current login from the numeric GitHub ID
immediately before the username-addressed mutation and validates returned IDs.
GitHub's collaborator endpoint addresses a login rather than supporting an atomic
numeric-ID precondition, so provider identity conflicts remain uncertain instead
of becoming successful receipts.

A returned new invitation can prove app ownership. An existing collaborator or
HTTP 204 cannot prove that the App owns the grant; it is retained as unmanaged.
Unknown collaborators, unknown invitations and changed permission levels are
flagged. Org policy rejection is explicit. The App never removes unknown grants,
changes organisation policy, or adopts inherited organisation/team permissions.
An expired invitation is distinct from a disappeared invitation; only a current
provider expiry flag establishes expiry. Owned expired invitations are cancelled
before a reviewed replacement is queued. Missing observed access is recorded as
not currently present, without claiming why it disappeared.

Every mutation has a durable dispatch barrier committed before the provider call.
Only the invocation that won that barrier can make the call. A lost response,
process crash or failed receipt commit leaves an uncertain effect. Retries and
member webhooks request fresh provider reads; they never resend that mutation.
A pending or accepted observation does not prove ownership of an uncertain write.
Such effects remain flagged for reconciliation. A proven unsent request or an
explicit policy rejection is retained as failed and needs a new review to retry.
The schema retains reviewed definitions, approval, identity and attempt history
and rejects deleting or resetting those records or rolling back populated data.

GitHub member deliveries have no authoritative occurrence timestamp. Their action,
numeric member ID and receipt time are informational hints. Only a fresh provider
GET changes observed invitation/collaborator state; delayed or duplicate delivery
order never changes assignments, grant ownership or readiness.

Provider contracts: [collaborators](https://docs.github.com/en/rest/collaborators/collaborators),
[repository invitations](https://docs.github.com/en/rest/collaborators/invitations),
[users by ID](https://docs.github.com/en/rest/users/users?apiVersion=2022-11-28).

## Verification map

- `internal/githubapp/collaborators_test.go`: isolated HTTP provider, token scope,
  numeric ID/current login, response identity, policy rejection, cancellation and removal.
- `internal/repositoryaccess/policy_test.go`: native published/current-claim and link
  revision selection, unknown access, deduplication, expiry, uncertain ownership and
  mentor window fingerprints.
- `internal/repositoryaccess/service_test.go`: real PostgreSQL, real provisioning
  owner and jobs worker, review/approval/publication, provider faults and durable
  history. All provider effects are deterministic in-process fixtures.

No real GitHub repository, invitation, collaborator or organisation policy is
changed during these tests.
