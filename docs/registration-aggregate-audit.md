# Registration and waitlist aggregate audit

This audit checks epic #12 against the actual implementation and assertions for
#49, #50, #51, #54, #55, #56, #57 and #58. Closed children alone do not establish
aggregate acceptance. The reviewed starting tree is `2f51475`.

The audit found a verification gap: applicant registration and offer journeys were
running against the admin build with a platform administrator who also owned the
fixture event. Those journeys now run in the regular participant build. Their
session is a non-admin applicant, the event has a different owner, all staff/edit
capabilities are absent, and the fixture staff list is empty. Organiser form
authoring and applicant review remain in the admin browser suite.

The corrected participant checks additionally exercise conditional questions and
the safe permanent-ineligibility explanation, plus the post-freeze withdrawal
warning. This changes verification only; production registration policies and API
shapes remain unchanged.

## Child criteria and inspected evidence

| Child | Concrete implementation and assertions |
| --- | --- |
| #49 versioned forms | `internal/forms/model.go` bounds declarative fields, choices and backward-only visibility rules. `model_test.go` checks unreachable/cyclic rules, typed answers and pruning of forged answers whose parents are hidden. `registration/forms_test.go` checks organiser-only authoring, cross-event denial, immutable published definitions, original-version corrections, newly required fields, stale response revisions, retained history and eventual answer retention cleanup. |
| #50 form editor | `web/src/definition-editor.tsx` edits questions, section labels, answer types, choices and visibility conditions. `registration-forms.tsx` previews through the server form-check endpoint and separates draft save from acknowledged publication. `registration-forms.spec.ts` asserts impossible-branch feedback, required visible answers, hidden branches, separate save/publish and acknowledgement of existing/new response-version effects. |
| #51 fair allocation | `registration/capacity_test.go` races eight applicants for one place, asserts server-sequence FIFO positions, unchanged replay identity/sequence, conflicting payload/duplicate rejection and authoritative confirmed counts. Further assertions cover ineligibility, facilitator independence, capacity-reduction remediation and reasoned exceptions. `concurrency_test.go` proves new applications close at preparation while committed retries remain valid and only confirmed competitors satisfy the minimum. |
| #54 applicant journeys | `registration.spec.ts` now runs the participant build with an ordinary applicant. Assertions cover directory discovery, captain preference, stable application identity after a failed response, distinct confirmed/waitlisted/facilitator states, application closure, separate facilitator fallback preference, conditional questions and the permanent competition-ineligibility explanation. PostgreSQL and HTTP tests supply the authoritative eligibility/capacity behavior behind these UI fixtures. |
| #55 offers and cutoff | `offers_test.go` checks FIFO reservation, replayed acceptance, transactional offer history, original-version corrections and final-cutoff handling. `concurrency_test.go` races accept/withdraw/repeated workers, asserts confirmed plus reserved capacity and deduplicated notices, rechecks competing/organiser grant conflicts, proves Friday plus a Monday holiday expires on Wednesday, and races acceptance/workers exactly at final cutoff without a later backfill. |
| #56 facilitator review | `registration/review_test.go` asserts outsider denial, atomic incompatible-role rejection, selection only after explicit attendee withdrawal, no consumed competitor place, private applicant access and an invitation instead of silent fallback enrolment. `applicants.spec.ts` exercises reason-required selection, displayed role conflict, a backup decision and its retained reason/history in the organiser UI. |
| #57 own status and withdrawal | `offers.spec.ts` now runs the participant build and asserts event-local offer deadlines, authoritative expired state, accepted status, correction of original answers, pre-freeze team-impact warning and post-freeze no-reassignment/no-backfill warning. `registration_test.go` verifies another person cannot read the applicant's registration and cannot open staff review. Backend offer/team tests verify the actual state changes and retained history. |
| #58 concurrency verification | The existing real PostgreSQL suites race final-place allocation, registration/preparation, frozen membership changes, offer acceptance/withdrawal/worker retry and eligibility grants. Their assertions inspect persisted states, counts, offers/notices and histories rather than only successful responses. The participant browser suite now checks the application-to-waitlist-to-offer-to-correction-to-withdrawal flow on its actual application surface. |

## Cross-interface and recovery boundaries

The HTTP tests validate the generated OpenAPI responses with real PostgreSQL.
They cover authenticated discovery/application, current-person projections,
organiser-only review and the distinction between an organiser and a competitor.
The checked-in generated TypeScript client is consumed by both application
surfaces; moving the applicant tests does not replace backend authorisation with
hidden controls or mocked permission decisions.

Form corrections remain tied to the originally answered version. A changed
idempotency payload conflicts, while an already committed retry returns its
existing allocation even after applications close. Offers reserve capacity but
do not count as confirmed attendees. Preparation preserves existing approved
FIFO offers until final admission cutoff; subsequent withdrawal does not undo a
committed go decision. Final cutoff/team freeze still prevent later backfill.

The browser fixtures simulate server results to verify rendering and recovery.
They do not prove transaction safety, real SSO configuration or provider delivery.
The real PostgreSQL service/HTTP tests own state and policy evidence; organisation
SSO is the existing identity boundary, and notification-channel delivery remains
owned by the communications backlog. No live participant data or real identity
provider credentials are required or introduced by this audit.

## Verification and completion gate

Local checks passed for both frontend builds, frontend lint and 16 corrected
desktop/mobile applicant scenarios, including conditional ineligibility and the
frozen withdrawal branch. The final PR's exact-head CI remains required before
merge and before reconciling epic #12. Its full Go/race job exercises the existing
PostgreSQL invariants, while the web job runs organiser and ordinary participant
journeys in their respective builds.

Reviewed fixtures use synthetic people, UUIDs, dates, answers and URLs. The new
eligibility message uses the safe error code instead of exposing theme material
or another person's responses. Existing form response retention and current
person/event access checks remain authoritative.
