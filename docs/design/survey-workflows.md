# Survey rebuild inventory and delivery boundary

Issue #241 / UI-08, base c91c52e3503f21077b519c9eae306d4dd0bd5ca9.

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

Full remaining implementation after the first real directory checkpoint:
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
