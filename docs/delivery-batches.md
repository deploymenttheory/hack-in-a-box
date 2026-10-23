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
or reported as delivered. #78 and #79 are delivered in PRs #189 and #194.
#80's implementation merged in PR #196; its required live shared-channel check
remains outstanding. #81's core merged in PR #198, while the named help,
clarification and outcome-review journeys (#159/#161/#117) remain open. #82's implementation merged in PR #207; aggregate retry and confidentiality
verification remains #84.

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
#158 is delivered in PR #195. #107 merged in PR #197 and is closed; #99 merged
in PR #200 and is closed. #159 merged in PR #201 after exact-head green CI. #161 merged in PR #205; its complete workspace remains #162. Outstanding provider and aggregate acceptance
is not implied complete by these implementation merges.

Each issue keeps its own PR and acceptance reconciliation. Migration numbers
follow actual ready delivery order rather than inventing feature dependencies;
normal ordered upgrade verification and exact-head green CI remain merge gates.


## Batch 8 — recoverable mentor work and complete support surfaces

| Issue | Deliverable | Why it is here |
| --- | --- | --- |
| [#89](https://github.com/deploymenttheory/hack-in-a-box/issues/89) | Connection recovery and private local text | Establishes session-scoped recovery before the actual mentor form, preserving current authority and honest unsent/server states. |
| [#100](https://github.com/deploymenttheory/hack-in-a-box/issues/100) | Mobile mentor workspace and observation timeline | Uses #99's real owning API and #89's recovery controller for the actual interrupted-save and conflict journeys; it has its own addressable route. |
| [#101](https://github.com/deploymenttheory/hack-in-a-box/issues/101) | Mentor checkpoint assessments | Uses merged #99 and #107 contracts to bind provisional checkpoints to the common rubric and current assignments/live gate, without waiting for the full workspace frontend. |
| [#160](https://github.com/deploymenttheory/hack-in-a-box/issues/160) | Team help request and helper controls | Uses #159's scoped state machine and real source/destination contract for the complete request/queue/action experience. |
| [#162](https://github.com/deploymenttheory/hack-in-a-box/issues/162) | Reviewed clarification feed | Uses #161's actual publication/current-audience boundary to present common answers equally without disclosing private questions. |

Keep three independent workstreams active with separate PRs and exact-head CI.
Dependencies follow the actual owning contracts:89→100,99+107→101,159→160 and
161→162. The recovery core merged in #202 and the actual #100 workspace merged
in #204 with interrupted-save, team-switch, conflict and recovered pre-live
denial verification. #101 checkpoint assessments merged in #203 and #160 help
controls merged in #206. #162 is under final workspace verification. Full support
surfaces use the real owning APIs independently of the aggregate Today dashboard;
no fake saved observation or unimplemented deep link substitutes for those
journeys.


## Batch9 — complete the mentor review workflow

| Issue | Work | Why it follows now |
| --- | --- | --- |
| #102 | Mentor scorecard UI | Uses the merged #99/#100/#101 observation, recovery and versioned checkpoint contracts for an actual deliberate scoring surface. |
| #103 | Reviewed shared feedback and actions | Separates private mentor evidence from reviewed participant-facing feedback, supplying the publication boundary before #104. |
| #104 | Huddles, second reviews and sign-off owner | Uses #103's reviewed outputs and explicit decision ownership; private notes never become released feedback implicitly. |
| #105 | Organiser/panel/progress UI | Follows #102/#104/#160 so its links and state come from actual scoring, review and help owners. |
| #106 | Aggregate mentor privacy and fairness verification | Verifies the complete workflow and current authority across all those owners before aggregate acceptance. |

This batch completes mentor workflows on merged #99/#100/#101/#159. #103
precedes #104; #105 follows #102/#104/#160; #106 verifies all of them. Keep
three independent implementation/PR lanes and require final exact-head CI plus
reviewed evidence for each merge. Git-dependent #108 and aggregate Today #88
wait for real #96/#93 contracts: no fabricated Git receipts or readiness claims.
These are selected work, not delivery claims.

## Batch10 — communication recovery and measurable outcomes

| Issue | Work | Why it follows now |
| --- | --- | --- |
| #83 | Delivery history and failure review | Uses the actual approved campaign, provider receipts, private recipient intents and automation occurrences to support truthful history and deliberate recovery. |
| #84 | Automation retry and confidentiality verification | Exercises the completed provider and recovery boundaries together after #83, including uncertain sends and current-authority changes. |
| #156 | Versioned event objectives and success measures | Establishes the owning objective/version contract before its editor and later outcome reporting. |
| #157 | Objective setup and measure editing | Uses #156's real versions and authority for a deliberate setup workflow. |
| #115 | Private feedback surveys and aggregates | Supplies response privacy and aggregate contracts before the outcome-review surfaces consume them. |

Keep three independent workstreams active with separate PRs. Actual automation
merged in #207 after #208's mentor scorecard UI; delivery recovery is under #83
implementation. #209's clarification workspace is merged after final native and browser
composition verification. #103 shared feedback is delivered in merged PR #210;
#104's native moderation owner is delivered in merged PR #211.
Provider history must preserve acceptance versus delivery and immutable unknown
outcomes; a recovery decision cannot manufacture a successful receipt. Migration
numbers follow actual ready merges. These are selected tasks, not completed
acceptance claims; the external Teams shared-channel verification is still open.
