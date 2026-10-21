# Organiser workspace verification

The approved follow-on batch is #34, #33, #39, #45, #40, #35, #37, #41, #36 and #38. Changes are delivered as dependent PRs so the policy, lifecycle, interface and privacy work can be reviewed separately.

| Issue | Implementation | Evidence |
| --- | --- | --- |
| #34 | Current event/resource policies, transactional role and owner changes, irreversible competition exclusions, registration eligibility boundary | PostgreSQL role/resource matrix, cross-event IDs, revocation, last-organiser protection, registration/grant races, generated HTTP response validation. |
| #33 | Validated local login return paths, expired-session feedback, account-scoped safe drafts and logout cleanup | Signed OIDC callback returns to its original page; desktop/mobile return-path, logout and account-switch tests. |

## Domain boundaries

Registration writes `competition_claims` through `permissions.SetRegistrationState` inside its own transaction. The function shares the event row lock with role changes. #51 supplies real confirmed-attendee facts; no API in this batch grants a competitor place.

Resource owners register `(event_id, id, kind)` in `policy_resources` and maintain `resource_assignments` transactionally. Mentor/judge and credential access requires current scoped authority; platform administration does not implicitly grant private event data. Future team, content and assessment modules extend this boundary and its test matrix.

Existing foundation databases have no event ownership model. Platform administrators can explicitly assign an existing event owner through the protected ownership endpoint; the new owner receives an organiser grant and permanent event-specific competition exclusion. Acknowledgment is required. Existing events are not silently assigned to every administrator.

## Verification commands

```sh
HACKBOX_INTEGRATION=1 go test -race -timeout 5m ./cmd/... ./internal/...
golangci-lint run ./cmd/... ./internal/...
npm --prefix web run lint
npm --prefix web run build
npm --prefix web test
npm --prefix web run test:e2e
sh scripts/check-generated.sh
```
