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

#62, #73, #77 and #85 are delivered in PRs #175, #177, #179 and #182. #91 remains in draft PR #181: deterministic provider and PostgreSQL
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

#74, #75, #76, #86 and #87 are delivered in PRs #184, #186, #192, #187 and #190. Each ticket was reconciled against its own acceptance and exact-head merge gate.
Registration epic #12 is closed after the aggregate audit in PR #185, merged at
`2baf40e`; that verification does not substitute for resource or presentation work.

## Batch 6 — reviewed campaigns and real delivery channels

| Issue | Deliverable | Why it is here |
| --- | --- | --- |
| [#78](https://github.com/deploymenttheory/hack-in-a-box/issues/78) | Campaign composer and bound approval | Gives organisers a retained review of content, current audience, channels, calendar selection and timing before delivery. |
| [#79](https://github.com/deploymenttheory/hack-in-a-box/issues/79) | SMTP, calendar attachments and verified own mailbox | Establishes real accepted/ambiguous outcomes and an organiser-only self-test destination without accepting arbitrary addresses. |
| [#80](https://github.com/deploymenttheory/hack-in-a-box/issues/80) | Teams Workflows broadcasts | Adds protected shared-channel configuration and literal public-only projection, with an explicit broadcast connection test separate from self-test. |
| [#81](https://github.com/deploymenttheory/hack-in-a-box/issues/81) | In-app inbox | Gives currently eligible recipients their own retained messages through the normal application access boundary. |
| [#82](https://github.com/deploymenttheory/hack-in-a-box/issues/82) | Automated approved reminders | Executes only reviewed current content and recipients after real provider, receipt and reconciliation contracts are available. |

Run three workstreams concurrently in isolated worktrees, beginning with the
campaign boundary, SMTP adapter and Teams adapter. Keep an individual PR and
exact-head green CI for each issue, with ordered schema upgrades. Provider work
can use the reviewed campaign contract while its final integration waits for the
prerequisite merge. Approval retains the reviewed versions and official facts;
dispatch rechecks current authority, recipients, embargoes and opaque provider
configuration revisions. These safety and receipt contracts precede unattended
scheduling. An unavailable provider remains visible and cannot be silently dropped
or reported as delivered. #78 is delivered in PR #189. #79 is in PR #194; its current exact-head CI remains a merge gate. #80 is prepared locally with its live shared-channel setup still outstanding; #81 and #82 retain their complete issue acceptance.

## Batch 7 — fair assessment contracts and event-day support

| Order | Issue | Deliverable | Why it is here |
| --- | --- | --- | --- |
| 1 | [#107](https://github.com/deploymenttheory/hack-in-a-box/issues/107) | Versioned common judging rubric | Uses merged content publication; establishes exact approved criteria and immutable assessment versions before checkpoint and final-score consumers. |
| 2 | [#99](https://github.com/deploymenttheory/hack-in-a-box/issues/99) | Private mentor observations | Uses current published assignments and live review gates, keeping operational status separate from private evidence and scores. |
| 3 | [#158](https://github.com/deploymenttheory/hack-in-a-box/issues/158) | Rules, conduct and support guidance | Reuses native content, exact-version acknowledgement and personal readiness to make event expectations and contact paths discoverable. |
| 4 | [#159](https://github.com/deploymenttheory/hack-in-a-box/issues/159) | Team help requests | Adds an independently authorised operational support workflow without waiting for private observation or score records. |
| 5 | [#161](https://github.com/deploymenttheory/hack-in-a-box/issues/161) | Private questions and common clarifications | Separates private questions from equally available reviewed official answers and preserves challenge/rubric publication gates. |

Keep three independent implementation lanes active when their prerequisites are
ready. These five tasks establish the assessment, rules and support contracts
needed by later mentor/judge workspaces and event-day views. They do not declare
future scoring, private assessment disclosure or provider integration complete.
Each issue keeps its own PR and acceptance reconciliation. Migration numbers follow actual ready delivery order rather than inventing feature dependencies;
normal ordered upgrade verification and exact-head green CI remain merge gates.
