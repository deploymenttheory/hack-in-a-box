# Backlog delivery

GitHub issues are the acceptance criteria and the source of completion status.
Each implementation ticket receives its own pull request. Merge only after that
PR's checks pass, then reconcile its acceptance checklist and close the ticket.
Epic and index issues close only after their children and aggregate acceptance
criteria have been verified. A deferred ticket needs a specific recorded reason;
an unmet dependency alone is a sequencing decision, not a completed deliverable.

## Batch 1 — preparation, logistics and submission foundations

| Order | Issue | Deliverable | Why it is here |
| --- | --- | --- | --- |
| 1 | [#47](https://github.com/deploymenttheory/hack-in-a-box/issues/47) | Preparation task APIs | Scheduling, permissions and templates are already available. |
| 2 | [#48](https://github.com/deploymenttheory/hack-in-a-box/issues/48) | Preparation board and run sheet | Makes task ownership and event readiness actionable for organisers. |
| 3 | [#52](https://github.com/deploymenttheory/hack-in-a-box/issues/52) | Attendance and restricted logistics APIs | Builds on the merged registration and privacy boundaries. |
| 4 | [#53](https://github.com/deploymenttheory/hack-in-a-box/issues/53) | Logistics and check-in screens | Combines the task board with accurate operational headcounts. |
| 5 | [#60](https://github.com/deploymenttheory/hack-in-a-box/issues/60) | Versioned team deliverables | Establishes live-start acceptance and cutoff outcomes needed by judging. |

Delivered in PRs #145–#149. All five issues are closed after green CI.

## Batch 2 — challenge publication and team preparation

| Order | Issue | Deliverable | Why it is here |
| --- | --- | --- | --- |
| 1 | [#69](https://github.com/deploymenttheory/hack-in-a-box/issues/69) | Guarded content publication APIs | Supplies actual challenge publication using the merged schedule and team-freeze gates. |
| 2 | [#70](https://github.com/deploymenttheory/hack-in-a-box/issues/70) | Organiser authoring and release controls | Makes version approval and release scheduling usable. |
| 3 | [#71](https://github.com/deploymenttheory/hack-in-a-box/issues/71) | Attendee resource library | Gives each current audience its released challenge, briefs and resources. |
| 4 | [#61](https://github.com/deploymenttheory/hack-in-a-box/issues/61) | Team deliverable screens | Connects challenge preparation to the versioned submission APIs from #60. |
| 5 | [#72](https://github.com/deploymenttheory/hack-in-a-box/issues/72) | Release embargo verification | Checks the complete author-to-reader path and copied attachment links. |

All prerequisite foundations are merged. Existing manual allocation supplies frozen
teams; suggested allocation and mentor coverage can follow this usable preparation
and submission path.

Delivered in PRs #150–#154. All five issues are closed after green CI, including
the real PostgreSQL embargo matrix and 146 desktop/mobile browser tests in #154.

## Batch 3 — reviewed allocations and mentor coverage

| Order | Issue | Deliverable | Why it is here |
| --- | --- | --- | --- |
| 1 | [#64](https://github.com/deploymenttheory/hack-in-a-box/issues/64) | Explainable team allocation suggestions | Uses confirmed membership from #59 and supplies proposals without changing published assignments. |
| 2 | [#65](https://github.com/deploymenttheory/hack-in-a-box/issues/65) | Allocation review and explicit publication | Makes #64 usable while enforcing locked groups, current revisions and the hard competitor freeze. |
| 3 | [#66](https://github.com/deploymenttheory/hack-in-a-box/issues/66) | Mentor capacity and conflict-aware assignment | Adds scoped primary/second-review assignments to the reviewed teams without reopening competitor membership. |
| 4 | [#67](https://github.com/deploymenttheory/hack-in-a-box/issues/67) | Mentor coverage administration | Combines #65 and #66 into workload previews, explicit gap decisions and permitted published rosters. |
| 5 | [#68](https://github.com/deploymenttheory/hack-in-a-box/issues/68) | Allocation and publication race verification | Exercises the complete suggestion-to-publication path, withdrawals, hard freeze and mentor access changes. |

The dependencies outside this batch (#59, #63 and #34) are merged. Completing
reviewed allocation and mentor coverage supplies assignment facts used by later
resource inventory and mentor workspaces. Readiness exports (#62) remain an open
deliverable; sequencing them later does not mark them complete or skipped.
