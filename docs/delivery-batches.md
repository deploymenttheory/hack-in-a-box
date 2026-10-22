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

Delivered in PRs #168–#171 and #174. Independent aggregate team verification
followed in #176 / PR #178; epic #13 is closed after that evidence was reconciled.

## Batch 4 — exports and reusable integration foundations

| Issue | Deliverable | Why it is here |
| --- | --- | --- |
| [#62](https://github.com/deploymenttheory/hack-in-a-box/issues/62) | Readiness reports and printable run sheets | Completes the existing preparation/logistics/submission views using current audience checks. |
| [#73](https://github.com/deploymenttheory/hack-in-a-box/issues/73) | Encrypted team credentials | Establishes protected storage, current-member reveal and audited operations for resource UI and GitHub App configuration. |
| [#77](https://github.com/deploymenttheory/hack-in-a-box/issues/77) | Versioned message templates and audience resolution | Supplies safe variables, retained previews and current recipient selection for later delivery channels. |
| [#85](https://github.com/deploymenttheory/hack-in-a-box/issues/85) | Presentation cues and protected display streams | Establishes the scoped, revocable server projection needed by MC and room-screen interfaces. |
| [#91](https://github.com/deploymenttheory/hack-in-a-box/issues/91) | GitHub App installation configuration | Supplies encrypted App identity and trusted installation/template verification before repository provisioning. |

#62, #73 and #77 are delivered in PRs #175, #177 and #179. #85 remains in
PR #182. #91 remains in draft PR #181: deterministic provider and PostgreSQL
checks do not replace its required live verification. The sandbox/App do not yet
exist; the setup ticket/resource preview is awaiting approval. This sequencing
condition does not mark #91 complete or skipped.

The planning aggregate audit found an inconsistent prepublication definition
projection after #62. Priority follow-up [#180](https://github.com/deploymenttheory/hack-in-a-box/issues/180)
was delivered in PR #183. Epic #11 is closed after the corrected boundary and
aggregate acceptance were verified against merged commit `2f51475`.

## Batch 5 — usable resource and presentation workflows

| Issue | Deliverable | Why it is here |
| --- | --- | --- |
| [#74](https://github.com/deploymenttheory/hack-in-a-box/issues/74) | Organiser resource inventory | Makes the merged credential storage usable for assignment, safe availability/expiry metadata and provider follow-up. |
| [#75](https://github.com/deploymenttheory/hack-in-a-box/issues/75) | Deliberate participant reveal and copy | Connects assigned resources to the participant library with temporary visibility and no persisted secrets. |
| [#76](https://github.com/deploymenttheory/hack-in-a-box/issues/76) | Credential confidentiality and recovery verification | Tests both completed resource interfaces and restored-key failure behavior before the content epic can close. |
| [#86](https://github.com/deploymenttheory/hack-in-a-box/issues/86) | MC console and rehearsal controls | Uses #85's approved revision/state contract to activate cues and urgent overrides deliberately. |
| [#87](https://github.com/deploymenttheory/hack-in-a-box/issues/87) | Branded room displays | Uses the same allowlisted stream for legible cues, safe QR destinations and honest disconnected state. |

Run three implementation workstreams concurrently in isolated worktrees. #74 and
#75 use the merged #73 contract and coordinate any shared metadata extension;
#76 follows both. #86 and #87 require #85's final backend contract. Each task keeps
its own PR, relevant verification and exact-head CI merge gate. A waiting live
provider setup does not prevent unrelated ready resource or presentation work.
