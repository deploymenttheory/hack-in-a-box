# Survey completion and organiser results

The survey directory links current invitations and organiser configuration. A
published question set stays fixed. Organisers deliberately save, approve and
publish a version, then separately approve its real campaign and exact native
`survey.open` binding. Survey publication alone sends nothing. The invitation
surface uses the existing automation owner for missed occurrences and cancellation;
provider recovery stays in its native delivery-history surface.

Respondents see required/optional fields, conditional questions, the event-local
opening/closing times and the explicit identifiable-response policy. Answers stay
in page memory until a native save. Close, current eligibility loss, archive,
session/account changes and native denial clear editing state. Temporary read
failures retain the local draft and exact uncertain command while pausing writes.
An explicit retry preserves its original command and expected response revision;
the returned receipt identifies that operation, while the separately refreshed
current response may be newer. Neither is substituted for the other.

Organisers have a separate private response/history surface and an explicit,
audited CSV export. The export includes author, response revision, question version
and answer state, with spreadsheet formula prefixes neutralised. Raw responses
and private workspaces are excluded from print. No private survey text uses the
persisted mentor draft store, browser storage, URLs or notification payloads.
Local section changes require explicit consent before discarding dirty/unknown
state. Native authority loss clears it immediately.

The private administrative dashboard distinguishes retained invitation/response
counts from their currently eligible subsets. Personal removal can remove an
invitation mapping, so these are not presented as the original published cohort.
These current organiser-only counts never replace a published report denominator.
See [the native disclosure and retention policy](survey-privacy.md) for the frozen
small-cohort and complementary suppression rules. Publication requires deliberate
reviewed prose, approval and publication; no raw answer is copied automatically.
Charts have equivalent accessible tables. A known zero remains zero; suppressed
or missing values remain unavailable.

Edition comparison requires the same stable question identity, equivalent field
and conditional context, audience and self-report construct. Linked measures also
require their exact retained definition, unit, baseline, target and source to
match. Merely matching a label or an event-local UUID/version number is
insufficient. Counts keep separate cohorts, denominators and publication dates;
they are never pooled. Reusing questions creates a new unapproved draft and never
silently retargets another edition's success measure.

The overview also lists the native paginated current planning objectives, including
objectives with no survey link. A later planning version is labelled separately
from an older exact survey or outcome snapshot; no new definition retargets a
retained observation. Definition creation time is not an observation date.

The follow-through panel consumes the native outcome owner's paginated
`/events/{event_id}/outcomes/observations` projection. It displays estimates,
self-reports and demonstrated-adoption records separately. Every record keeps its
exact retained objective/measure definition, unit, baseline, target, evidence
source, nullable value/denominator and observation date. Record creation time is
labelled separately and does not fill a missing observation date. No observed
record means no available record, not zero achievement. The dashboard does not
infer monetary ROI or turn a planning target/survey self-report into demonstrated
adoption. Native organiser and resource permission still apply on each page.

## Verification

The PostgreSQL/race dashboard and CSV tests verify current organiser authority,
privacy expiry, native withdrawal/removal, archive reads, safe logs/audits and
spreadsheet-safe exports. The measurement HTTP journey creates a real native
pitch, adopts it, assigns its owner, attaches an exact objective, changes the later
planning definition and records real immutable evidence. It then checks paged
HTTP results, retained units/nulls, current organiser revocation and no-store.

Browser cases exercise desktop/mobile completion, conditional pruning, distinct
publication steps, private export, source-bound invitation approval, uncertain
save/reconcile, current authority/close/archive cleanup, transient read failure,
small cohorts, equivalent/changed editions and actual outcome DTO pagination.
They capture the published report and native observation layouts for inspection.
The question-equivalence unit cases include changed conditional parents and
coincidentally identical IDs with different retained measure definitions.

## Acceptance evidence for #116

| Acceptance criterion | Native/browser evidence |
| --- | --- |
| Required/optional questions and response visibility | `survey-workspace.spec.ts`: keyboard/mobile completion and conditional-answer pruning; `survey-reader.spec.ts`: invited private reader. |
| Accessible counts without restricted respondents | Reviewed report chart/table journey and small-cohort suppression journey; native survey privacy/disclosure fixtures. Raw authored responses and audited CSV have separate current-organiser checks. |
| Equivalent versus changed editions | Browser comparison cases and `survey-question-equivalence.test.ts` cover separate retained versions, changed definitions, conditional context and exact measure provenance. |
| Configured objectives, participation, completion and follow-through with provenance | Current objectives and native observation pagination browser cases; `TestSurveyMeasurementDashboardConsumesNativeRetainedOutcomeEvidence` creates actual outcome evidence and changes the later planning unit without changing the retained observation. Dashboard HTTP/service fixtures prove native invitation/response denominators. |
| Unknown baselines/pending reviews and unsupported ROI | Observation fixtures preserve nullable value, denominator, observation date and baseline; browser cases distinguish a recorded zero from unknown/pending and keep estimates, self-reports and demonstrated records separate. No monetary ROI calculation exists. |

Fresh validation on the current outcomes/schema-43 main baseline includes the
canonical SQLC/OpenAPI generator, full Go build/vet, Go lint (zero issues), web
lint, both portal builds, 60 web unit cases and 60 desktop/mobile survey browser
cases against the admin build. Real PostgreSQL survey service and HTTP journeys
ran with `HACKBOX_INTEGRATION=1 go test -race -timeout 10m
./internal/surveys ./internal/httpapi -run Survey -count=1` and passed.
Synthetic browser screenshots show the reviewed report, configured objectives and
retained native observations at desktop/mobile widths.
