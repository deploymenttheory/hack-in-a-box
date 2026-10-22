# Planning, logistics and deliverables aggregate audit

The independent audit of epic #11 compared each child acceptance criterion with
implementation and test assertions after #62 merged. Child closure alone was not
used as evidence. The audit found one remaining disclosure boundary inconsistency:
readiness exports withheld competition definitions before actual common challenge
publication, while normal team submission metadata returned their titles and
descriptions. Issue #180 records that defect separately from the completed children.

The #180 regression first reproduced the disclosure against real PostgreSQL with
synthetic theme/instruction canaries. The correction withholds participant
competition definitions until `published_at` is present and reached. Passing the
planned T−7 instant alone grants no access. Current organisers retain their private
authoring view. Personal prerequisites remain available through preparation; no
new early-release exception or definition classification is introduced.

## Child acceptance and evidence

| Child | Implementation and inspected assertions |
| --- | --- |
| #47 preparation backend | `internal/preparation/service.go` owns operational task state separately from lifecycle facts. `service_test.go` exercises dependency cycles/cross-event references, reasoned completion/override/waiver, immutable history, concurrent revisions, ownership and current roles, hidden evidence, due-date recomputation and minimum-attendance gate independence. `attendee_test.go` and preparation HTTP tests cover current confirmed-place/role access. |
| #48 organiser preparation UI | `web/e2e/preparation.spec.ts` asserts textual overdue/blocked/unowned labels, timezone/owners/evidence in the run sheet, actual blocker navigation, reasoned actions, retained drafts and explicit conflict review, revoked permissions, stable upload retry keys and repeated-time resolution. |
| #60 submission backend | `internal/submissions/service_test.go` asserts private draft isolation, rejection at live start minus one nanosecond, required evidence types and protected files, immutable replacement/review history, exact shared cutoff independent of lifecycle advancement, reasoned amendments, rescheduling protection, assessment-gate settlement, concurrent saves/worker replay, withdrawn/disbanded unscored outcomes and current team/reviewer authority. `submission_states` and competition-entry keys enforce one active final pointer and one entry per team/event. |
| #61 participant preparation/deliverables UI | `web/e2e/deliverables.spec.ts` asserts retained completed fields and upload idempotency after response loss, private versus formal acceptance, cutoff exception contacts, frozen outcomes, explicit conflict retries, personal task scope, no disbanded-team solicitation and revoked membership. The #180 regular participant build journey additionally checks withheld canaries, unchanged personal preparation and actual publication opening private preparation while formal submission/review remain closed. |
| #52 attendance/logistics backend | `internal/attendance/service_test.go` asserts concurrent idempotent scans, correction reasons/history, old replay not undoing a correction, per-local-day/DST behavior, confirmed-only check-in, current roles/cross-event/read-only protection, distinct/deduplicated headcounts, unchanged admission minimum, sensitive-field projections, paginated exports/audit and profile deletion. |
| #53 attendance/logistics UI | `web/e2e/attendance.spec.ts` asserts keyboard/narrow-screen check-in, stable retry IDs, conflict review and retained corrections, shared-screen absence of sensitive fields, restricted pagination/export and revocation, count labels that distinguish physical arrival/catering/minimum competitors, and venue owner/deadline/readiness requirements. |
| #62 readiness exports | `internal/preparation/export_test.go` checks organiser/mentor/participant scopes, immediate withdrawal/role/resource revocation, no staff/private/dietary/accessibility leakage, raw agenda and competition-definition embargoes, current explicit submission outcomes and empty events. `render_test.go` checks hostile CSV prefixes and escaped, printable, offline HTML with restrictive CSP and no external assets. HTTP tests assert download media/disposition, no-store, generated response contracts and unauthorised access. |

Operational preparation writes do not change authoritative attendance, admission,
freeze, publication or submission facts. Evidence URLs are bounded HTTP(S)
references without embedded credentials; preparation/submission services do not
fetch them. Submission settlement runs both during relevant operations and worker
maintenance, records the logical shared cutoff, and feeds the live-to-assessment
gate through `SettleSubmissions`. A late lifecycle transition therefore does not
extend an ordinary submission deadline.

## Definition-bearing surface review

- Normal submission metadata and successful mutation responses share
  `submissions.page`. Participant definitions now require actual common publication;
  organisers retain their authorised definitions. The OpenAPI field describes the
  same boundary.
- Private draft reads/writes require current team membership and actual publication.
  Formal writes require publication and scheduled live start; normal cutoff and
  reasoned organiser amendment policies remain authoritative.
- Formal history already requires actual publication and live start. Reviewer reads
  also enforce their live/cutoff, current role and current resource-assignment rules.
- Readiness reports use the selected current audience and formal submission view
  inside the event transaction. Their existing participant publication guard agrees
  with the corrected normal API. Historical private working evidence is not exported.
- Full event definition/configuration snapshots require current organiser access.
  Reusable template reads require the template owner's authority. Neither route
  becomes a participant release mechanism or gains implicit platform-admin access.
- Registration forms and assigned personal preparation are independent public or
  permission-filtered prerequisites. They do not carry competition definition prose
  and are deliberately unaffected by #180.

The HTTP regression checks the normal API and participant HTML export before and
after actual publication, keeps organiser authoring access, denies the participant
configuration snapshot, and verifies that private draft/history access remains
closed early. PostgreSQL regressions additionally check a future actual-release
timestamp, published due times, pre-live private-only capabilities, cross-team
access and withdrawal. Desktop/mobile scenarios run the regular participant build;
the admin regression uses the same shared component and retains its other
submission/recovery journeys.

Reviewed fixtures and documentation use synthetic people, UUIDs, URLs and canaries.
No real participant collateral, private assessment text or live credentials were
introduced by these deliveries. Browser fixtures verify presentation/recovery;
PostgreSQL tests verify transactions, current authority and clock boundaries. The
implementation PR's final CI result remains the merge gate. Epic #11 should only
be reconsidered for closure after #180 is merged and the aggregate criteria are
reconciled against that verified head.
