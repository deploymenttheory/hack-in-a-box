# Support, independent review and huddle dashboards

Issue #105 adds `/events/:eventID/review`, scoped team review/agenda, and the explicit organiser-only factual judge handoff preview. Both the public participant/mentor build and the admin build use the same current-authority API. There is no new schema or migration.

## Current contracts

- Operational support risk uses native check-in status, current shared-action metadata and permitted help queue summaries. Assessment completeness is a separate projection. Neither uses commit counts, competitive help ranks, averaged checkpoint scores or invented zero scores.
- Organisers receive score-free coordination metadata; a generic organiser does not join a private panel. Before an owning huddle authorises the panel, mentors receive only their own independent completion metadata. Generic judges, competitors and logistics roles cannot use these dashboard endpoints.
- Native checkpoint windows/current rubric runs remain authoritative. Reading coordination metadata does not start or freeze an assessment. Live checkpoint completion and final frozen-output completeness/sign-off remain separate.
- Missing entries, disbanded teams and native withdrawn/not-submitted output are explicit, non-assessment states. The dedicated agenda endpoint agrees with the aggregate projection.
- The factual preview includes only native formal submission outcome, timestamps, frozen revision and counts of judge-visible frozen artifact kinds. It excludes submitted labels/URLs, qualification scores, private observations, ranks and comparative prose. It does not grant future assigned judges access.
- Help metrics use permitted native summaries: waiting, claimed, unresolved, current waiting/open ages, actual first-response samples, and resolved-only completion samples. Absent measurements remain absent. Links use the native authorised help detail route; descriptions, locations and meeting URLs are absent from metadata.
- Current huddle metadata provides named moderator/primary/second, due/completion, blockers, gaps/proposals and exact canonical current-run sign-off status. UUID keysets are bounded with no lifetime 1,000-row cap. Requested-team filtering occurs before the page bound.
- A planned huddle can expose a separate explicit reconfiguration capability after native mentor coverage replacement, without loading the obsolete private snapshot. The native configure command still validates current actor/conflicts, exact prior version, new published primary/distinct second, checkpoint and due time.
- Private panel reads compose the owning huddle and exact submitted-card reads as one view. A failed current source read unmounts the private editor. All private input lives in page memory, is scoped to person/session/event/team, and is excluded from browser storage and print. Decision/history query keys include current draft scope; historical snapshots retain native per-snapshot current reader checks.
- Writes bind an explicitly reviewed event revision and exact huddle version. An event change requires explicit fresh review and input reset; there is no implicit version rebase. Uncertain responses preserve an in-memory immutable intent and inspect exact immutable history without replay. Definite current authority denial advances the pending generation so a deferred preflight cannot later send a decision. Already-sent effects remain server-authoritative.
- Formal gaps use actual currently published #103 actions. The integration repair separates retained-source disclosure permission from exact-current operational completeness: reopening an action leaves its panel able to read and explicitly repair the huddle, but cannot sign until current-version formal evidence is explicitly re-resolved. Completing an action alone never resolves a gap.
- Proposed changes display original author/card/category/value/evidence alongside proposed evidence/reasons, decisions and immutable huddle history. Independent cards remain unchanged. Final sign-off uses the actual #104/#101 same-transaction canonical proof, never a client-computed total or an automatic approval.

## Verification evidence

All integration fixtures use synthetic people, output and prose in isolated real PostgreSQL. Browser network fixtures are deterministic API mocks; they prove UI behavior, while separate real PostgreSQL and authenticated HTTP tests prove current native policy and transaction/source composition. No real provider sends are involved.

- `TestDashboardKeepsOperationalRiskIndependentFromPrivateAssessment`: current roles, own-only mentor completion, generic judge/competitor denial, private canary exclusion and native cutoff not-submitted state.
- `TestDashboardRequestedTeamFilterPrecedesBoundAndIncludesDisbandedWithoutScores`: requested team beyond 1,006 earlier records, unassigned mentor denial, explicit disbanded/non-entry preview state.
- `TestDashboardUsesNativeHelpAgeResponseAndCompletionWithoutPrivateRequestBody`: real native request/claim/resolve lifecycle, waiting/claimed counts, elapsed/sample truth and private request/location exclusion.
- `TestAgendaMetadataDoesNotJoinPrivatePanelOrExposeAssessment`: current organiser metadata versus intended-panel permission, canonical sign-off facts, archived write capability and role revocation.
- Actual native planned second-review replacement regression verifies repair capability while obsolete context remains closed.
- `TestCoordinationMetadataDoesNotDiscloseAnotherIndependentCard` and `TestCoordinationReadDoesNotStartOrFreezeNativeAssessment`: independent privacy, pending counts and read-only native run semantics.
- `TestFactualOutputMetadataUsesFrozenSourceWithoutAssessmentOrDraftText`: actual cutoff/frozen source, absent/withdrawn output, retained judge-visible artifact counts and privacy.
- Native reopened-action repair regression: reopen → read/panel remains possible → sign denied → operational done still denied → explicit current-version evidence resolution → native canonical sign-off.
- `TestReviewDashboardHTTPCurrentAuthorityPrivateSourcesAndFactualBoundary`: real authenticated routes, strict contract response validation, no-store/private canaries, current role/session invalidation and explicit organiser-only preview.
- `web/e2e/review-dashboard.spec.ts`: desktop/mobile support/completeness/help drilldown, unused output, factual preview and print, pending versus original numeric scores, intended-panel evidence, lost accepted proposal response and exact-history reconciliation, planned replacement repair without obsolete private reads, second-reviewer/preview boundary, deferred preflight versus definite panel denial, primary proposal-only permissions and explicit changed-revision review.

Focused results before draft publication: dashboard/native-help PG/race **5.990s**; agenda PG/race **3.381s**; authenticated HTTP PG/race **5.751s**; native coordination/history **4.883s** and scorecard metadata **4.514s**; admin desktop/mobile **16 passed in 5.1s**. Both surface builds passed. Final affected suite/public-browser/lint results are recorded in the PR; readiness requires passing final checks on the actual composed head.

## Remaining boundaries

This is #105 coordination/UI. Actual judge assignment and final results/awards remain with #108–#112; organiser preview does not stand in for them. Live external provider acceptance and unrelated automation results integration remain with their owning tickets. Existing native help, feedback, Q&A, objectives, independent scorecards and delivery history routes are preserved.

Independent review on the composed schema-39 base passed dashboard PostgreSQL/race
in 7.623 s, moderation/current-action repair in 8.328 s and authenticated HTTP in
7.262 s. Sixteen admin browser cases passed in 6.1 s. A mobile viewport regression
then reproduced a 427-pixel private panel in the configured 390-pixel viewport;
wrapping long huddle buttons fixes the overflow. Both surfaces rebuild, all
sixteen participant browser cases pass in 4.5 s, and the desktop/mobile admin
panel checks pass in 1.6 s with the viewport assertion. The assertion compares
against Playwright's configured viewport, since a mobile layout viewport can
expand along with overflowing content.

Final composition on actual main `642de7f` (merged #214, schema 39) preserves the
shared participant test selection for both review dashboards and automation
recovery. The combined participant dashboard/delivery/recovery check passes 42
cases in 16.5 s. The rebase leaves reviewed production source unchanged; its only
conflict was the additive test-selection list.

After #215 merged, final composition rebases onto actual main `5db301f` (schema
39). Additive conflicts preserve both Objectives and Review coverage navigation,
both style blocks and every participant test entry. The combined build exposed
an accidental huddle-only `can_record` addition in three unrelated schemas;
removing those fields and regenerating preserves every pre-existing schema
exactly, with 22 new dashboard schemas only. Actual objective/feedback/help
PostgreSQL/race HTTP checks pass in 14.039 s. Both surfaces build; the composed
admin objectives/templates/content/dashboard check passes 82 cases in 50.6 s,
and participant disclosure/dashboard/delivery/recovery passes 48 cases in 29.0 s.
