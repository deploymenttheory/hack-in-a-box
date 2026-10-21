# Organiser workspace verification

The approved follow-on batch is #34, #33, #39, #45, #40, #35, #37, #41, #36 and #38. Changes are delivered as dependent PRs so the policy, lifecycle, interface and privacy work can be reviewed separately.

| Issue | Implementation | Evidence |
| --- | --- | --- |
| #34 | Current event/resource policies, transactional role and owner changes, irreversible competition exclusions, registration eligibility boundary | PostgreSQL role/resource matrix, cross-event IDs, revocation, last-organiser protection, registration/grant races, generated HTTP response validation. |
| #33 | Validated local login return paths, expired-session feedback, account-scoped safe drafts and logout cleanup | Signed OIDC callback returns to its original page; desktop/mobile return-path, logout and account-switch tests. |
| #39 | Event directory APIs, configuration, optimistic revisions, explicit lifecycle transitions and transactional domain gate contracts | Real PostgreSQL checks for confirmed-only thresholds, explicit go action, withdrawal continuity, post-publication attendance exceptions, cutoff/outcome gates, reopening reasons and archive write/job protection; HTTP schema validation. |
| #45 | Event-local calendar arithmetic, business calendars, explicit DST resolution, schedule preview/approval and obsolete-work cancellation | Spring/autumn seven-calendar-day tests (167/169 elapsed hours), gap/fold handling, excluded dates, stale edits, cancelled approvals and preservation of executed actions. |

## Domain boundaries

Registration writes `competition_claims` through `permissions.SetRegistrationState` inside its own transaction. The function shares the event row lock with role changes. #51 supplies real confirmed-attendee facts; no API in this batch grants a competitor place.

Resource owners register `(event_id, id, kind)` in `policy_resources` and maintain `resource_assignments` transactionally. Mentor/judge and credential access requires current scoped authority; platform administration does not implicitly grant private event data. Future team, content and assessment modules extend this boundary and its test matrix.

Existing foundation databases have no event ownership model. Platform administrators can explicitly assign an existing event owner through the protected ownership endpoint; the new owner receives an organiser grant and permanent event-specific competition exclusion. Acknowledgment is required. Existing events are not silently assigned to every administrator.

Event creation starts in draft. #51 marks attendance facts ready and supplies confirmed claims; #59 records frozen team facts; #60 records frozen/not-submitted/withdrawn outcomes; #69 checks publication readiness in its publication transaction and owns the exact scheduled-time enforcement. Missing facts block the relevant transition. These are internal transaction contracts, never client-editable counters.

Post-publication postponement stores an immutable reason and an exact revised-date notice with a durable `event.rescheduled` operation. Communication delivery is owned by #82; a pending operation is not evidence that anyone has been notified. Schedule approvals similarly become delivery inputs when automation is installed. Replanning invalidates approvals and cancels linked unsent operations; executed actions retain their execution state.

Publication uses seven calendar days at the same event-local clock time. A DST gap requires choosing a different time; a repeated time requires an explicit earlier/later choice. Business-day rules apply to relative agenda/offer calculations and never change the challenge-publication interval.

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
