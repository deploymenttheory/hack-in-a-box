# Survey rebuild inventory and delivery boundary

Issue #241 / UI-08. Initial directory base c91c52e3503f21077b519c9eae306d4dd0bd5ca9;
current continuation rebased onto 6e8ab511f00f3c3e8b63c91200a52637bee65119.

## Continuing implementation checkpoint

The response, authoring and report adapters now use controlled production decision
bars and explicit exact-snapshot review. Private answers/review prose, restricted
response/history pages and current report projections are owned by fenced mounted
React pages, never ordinary QueryClient or browser storage. Current reads cannot
settle configuration/approve/publication unknowns. Captured response UUID and exact
reviewed-text identity retries retain their original payload/revision while native
current authority and clock remain authoritative. Only safe generic operation,
original revision and UUID metadata enters the existing session safety provider.

Management/create authority, published results, comparisons and objective/observation
reads are scoped and no-store. CSV export reviews the private download, inherits
its response owner's authority lifetime and rejects a held blob after synchronous
parent denial or a same-mounted disabled permission. Object URLs are revoked;
printing/page departure cancels a pending export. Native formula protection and
auditing remain server-owned. The independently published guide imports the actual
production decision controls and definition/answer summaries with synthetic,
backend-free response/authoring/report owners and accessibility/usage guidance.

Current checkpoint proof: 54 native administrator owner journeys; 14 focused
privacy/export/equivalence unit cases; six independent guide keyboard/narrow
journeys. Both portal builds, guide build, full lint and diff-check pass. These are
bounded checkpoint checks, not full-acceptance or exact-head CI certification.

Remaining before #241 completion: broader both-portal/shared verification; additional
session/retention/version/unknown-approval and held read/write/download browser
regressions; objective/observation inspector refinements and source paging proof;
actual synthetic before/after evidence; full repository checks and independent
Standards/Spec reviews at the completed head. Keep #241 open and the PR a draft.


Owned: survey-directory.tsx; survey-reader.tsx re-export; survey-response.tsx;
survey-context.ts; survey-workspace.tsx (management and creation);
survey-authoring.tsx; survey-review.tsx (current authored responses, retained
history, report review); survey-results.tsx; survey-comparison.tsx;
survey-question-equivalence.ts; survey-objectives.tsx;
survey-outcome-observations.tsx; owned journey fixtures/tests and survey styling.

SurveyInvitations export/props and implementation belong to #239. No changes to
its implementation. Its forthcoming communication-safe metadata provider belongs
to that lane. No generic private payloads are lifted into PlanningOperations.
Outcome project follow-through belongs to #242, not survey observations.

Native APIs: current invitation directory and own participant page; paged
configuration directory/current configuration/create/version commands; own
response PUT; organiser current responses and revision history; frozen report
review/read/command and published report; organiser dashboard; audited private
CSV. Survey-linked exact objective references, configured objective cursor pages
and outcome observation cursor pages remain separate facts.

Native commands support save/approve/publish/cancel for configuration. Response
open/close is controlled by published configuration times and event live-end;
archive is an event/current-authority condition. Do not invent separate survey
close/archive mutation APIs. Respond checks current write/privacy/invitation and
published form/open clock before UUID+canonical payload digest replay. Digest
includes expected response revision. A valid exact retry returns that command's
retained response version, possibly older than current saved response. Current
GET cannot settle original unknown intent. Configuration/report current flags
also cannot be original command receipts. Raw responses/answers/report-review
text must move out of ordinary QueryClient/local caches into fenced page owners.

Original complete implementation inventory (do not drop nested scope):
- private own response editor/review/exact result/captured retry/current read;
- versioned authoring, schedules, cancellation and read-only archive states;
- restricted response inspector/history and exact scoped export/print lifecycle;
- frozen suppression/report review/approval/publication without inferred receipts;
- published factual results/comparison with explicit denominator/equivalence;
- exact configured objectives and observed outcome units/provenance/paging;
- actor/draft/event/authority denial, remount and held read/write fences;
- safe unknown metadata integration when #239 publishes its shared contract;
- both portal keyboard/mobile/error/unknown/stale/cohort/export/privacy proof;
- actual synthetic before/after evidence; independently reviewed final head/CI.

The initial directory checkpoint does not claim these remaining surfaces rebuilt.

Private survey reads now inherit only their mounted resource parent's cancellation
lifetime through `SurveyAuthority`. A management denial invalidates descendant
read/write/download tickets synchronously, even while a child read is held and
before React unmounts its private inputs. This context contains no responses,
reasons, summaries or durable command facts. Actor/draft/event keys remain local;
unknown metadata continues to use the existing safe Shell owner exclusively.

Configured objectives and follow-through observations use real cursor-backed
record directories and one deliberate Review inspector. Back restores the actual
responsive opener. Planning estimates, retained evidence bases, exact versions,
units, null values and genuine zero values remain separate native facts.

The additional private-composition inventory is `survey-privacy.spec.ts`, shared
by both portal CI configurations. It covers held CSV bytes after parent denial,
session end and print; 401/403/404 with an older held response read; minimum
cohort field correction; original held writes across close/question-version
replacement; and lost report approval whose current flags do not settle its
original command. Private payloads are not written to browser storage.
