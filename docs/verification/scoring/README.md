# Scoring, privacy and publication verification (#114)

Base: corrected #113 candidate plus measured CI job budget
`908a69f8b0b01b39cda87f759530667a9fc7c4cb`.
This delivery remains incomplete until native verification, independent review,
prerequisite corrections and exact-head CI are green. Compilation alone proves
neither the assertions below nor completion of the issue.

The [hand-worked fixture](hand-worked.md) gives fixed independent answers.
`TestIndependentHandWorkedScoringUsesNativeSourcesInBothModes` consumes its JSON
through actual PostgreSQL, native rubric publication, formal frozen submissions,
live checkpoints, independent cards, qualification and final result owners. It
does not inject signed decisions or replace the calculator with a fake.

## Acceptance coverage

All named tests below are existing concrete assertions unless labelled new.
The verification commands run their owning packages, rather than treating the
presence of a test name as proof it ran. Native tests must not be skipped.

| Requirement | Assertions to inspect and execute |
|---|---|
| Provisional checkpoints never accumulate; no mentor carry | **New** native both-mode golden fixture submits separate 9/10 live checkpoints, verifies signed mentor final42 and fresh judge final254/3. `scorecards/TestIndependentCheckpointLifecycleUsesNativeRubricAndKeepsPendingPrivate` separately checks pre-live rejection, actual live pending/submit/history and a fresh pending frozen final. |
| Unequal weights, missing/pending judges | **New** native golden fixture requires three independent judges, saves a pending category, blocks while any required card is missing, retains category fractions20/3,17/3,6,5,19/3 and total254/3 after completion/deliberation. `results/TestIncompleteSourcesNeverReduceTheRequiredDenominator` also rejects malformed/incomplete source shapes. |
| Exact and near ties | `results/TestExactTiesRemainTiesRegardlessOfUUIDOrder`, `TestRoundedNearTiesRemainDistinct`; native `app/TestFinalJudgeBallotsRequireEveryoneThenStrictMajorityAndExplicitRevote` and `qualification/TestTiedCutoffAndCategoriesRequireEveryMentorBallotAndRecordedCastingDecision`. The all-team category owner rule remains a prerequisite policy gap, not inferred from these final/qualification cases. |
| Multiple awards; non-finalist category winner | **New** composed `app/TestIndependentFiveTeamQualificationToNonFinalistCategoryAwardPublication` connects actual five-team qualification, three finalists/reserve, fresh35/30/25 judge final and explicit publication awarding technical to nonfinalist team5 and two prizes to team1. Native `qualification/TestNonFinalistCanWinCategoryAndOneTeamCanWinMultipleWithoutCarryingCheckpointScores` independently verifies47/42/37/32/14 totals and the signed selection handoff; native `app/TestResultsNativeConfirmationAndAwardPublicationKeepIndependentEmbargoes` verifies two awards to one team, exact money metadata and explicit release. `results/TestAwardDecisionUsesSignedQualificationCategoryWinnerOutsideFreshFinal` verifies award selection consumes that signed category source. |
| APIs cannot disclose unpublished assessments/results | Native `app/TestResultsAPIPrivateAdministratorScopeAbsentSourcesAndStrictCommands`, `TestResultsAPIExplicitQualificationReleaseOriginalACKAndSafeAudience`, `TestResultsNativeSourcesUseIndependentExactMeansAndCurrentOrganiser`; own typed DTO fields and private canaries are checked, including current role loss and both supported modes. |
| Export privacy | Native `moderation/TestArchivedAssessmentExportUsesActualRecordedHuddleAndCurrentAuthors` allows exact recorded panel context only; own-card export cannot widen to peers and current author revocation removes access. `archives/TestArchiveExportsPreserveNativeRoleAndUnavailableSources` and `TestArchiveExporterWaitingBehindRoleLossRechecksAfterEventLock` retain the actual export policy. Authorised assessment exports are deliberately private staff outputs; no claim that all exports contain no scores. |
| Notifications and room embargo | Native `app/TestResultsPublicationConsumersKeepPrivatePreparationAndIndependentReleaseBoundaries` verifies private prepare/confirm produce no publication, explicit finalist and award release remain independent, actual approved results-purpose in-app delivery, current audience loss, latest per-kind room projection and revoked pairing. `TestResultPublicationAndOfficialFactRollbackTogetherOnNativeFactFailure` verifies publication/fact/receipt atomicity. Provider delivery beyond the actual in-app path is not claimed. |
| Pre-live/live/post-cutoff/same-day assessment | `scorecards/TestIndependentCheckpointLifecycleUsesNativeRubricAndKeepsPendingPrivate`, `rubric/TestFinalAssessmentConsumerUsesCutoffFrozenEntryRequiredJudgesAndFreshQualificationPolicy`; **new** native golden fixture explicitly advances to assessment on the same hack date and verifies exact totals persist. |
| Disbanded entries/repositories excluded | Native `qualification/TestNativeURLAndFileFinalsAreNotApplicableWhileMissingWithdrawnInactiveAndDisbandedAreUnscored` and `judging/TestRepositoryRecoveryNeverMakesUnresolvedOrDisbandedBuildScoreable`. These fixtures inject lifecycle facts before invoking native assessment; they do not prove the team-disband operation itself. |
| Common challenge and exact rubric; corrected criteria/notice/history | Native `rubric/TestRubricSharesActualChallengeGateAndPublishesReviewedCorrectionAtomically`, `TestChallengeRequiresExactApprovedRubricAndCurrentIndependentApprover`, `TestRubricNotificationSourceRequiresActualCurrentPublicationAndCurrentAudience`; `scorecards/TestPublishedCriteriaRestartPreservesHistoricalCardsAndRequiresFreshCheckpointBinding` retains historical originals and rejects reinterpreting old cards. |
| Synthetic practice before live isolated in both modes | **New** `judging/TestPracticeBeforeLiveCannotEnterEitherNativeScoringMode` submits perfect fictional practice in both modes, rejects real brief/card access before live, and checks an actual practice row with zero real cards. Its qualification-only cutoff denial is explicitly the absent-policy guard, not proof of a signed qualification. The composed golden fixture additionally submits perfect practice before actual live start, retains those same judge identities through genuine native qualification/signoff, verifies two practice rows and zero real cards still block actual final completion, then proves only fresh cards enter254/3. |
| Conflicted/revoked judges; frozen/current evidence | Native `judging/TestConcurrentVersionFenceAndDeclaredConflictRevocation`, `TestCurrentAssignmentLossRevokesNativeSubmissionAndAuthoredHistoryReads`, `TestCurrentNativeFinalReplacementScrubsOldJudgeCodeAndRequiresReviewedReopen`; `app/TestResultsNativeReopenAndFinalReplacementFenceOriginalSource` covers both modes. |
| Full finalist-to-award release | Native `app/TestResultsAPIExplicitQualificationReleaseOriginalACKAndSafeAudience`, `TestResultsNativeConfirmationAndAwardPublicationKeepIndependentEmbargoes` and publication consumer sequence above; participant feedback remains separately controlled by #113. |

## Commands and evidence

```sh
HACKBOX_INTEGRATION=1 go test -v -race -p 1 -parallel 1 \
  ./internal/rubric ./internal/scorecards ./internal/moderation \
  ./internal/qualification ./internal/judging ./internal/results \
  ./internal/archives -count=1 -timeout=10m
HACKBOX_INTEGRATION=1 go test -v -race -p 1 -parallel 1 ./internal/app \
  -run '^Test(IndependentHandWorkedScoringUsesNativeSourcesInBothModes|IndependentFiveTeamQualificationToNonFinalistCategoryAwardPublication|QualificationFiveTeamNativeFinalFixture|QualificationHTTP.*|Results.*|ResultPublicationAndOfficialFactRollbackTogetherOnNativeFactFailure|FinalJudgeBallotsRequireEveryoneThenStrictMajorityAndExplicitRevote|JudgePublishedOrderUsesNativeReleaseAndAssignmentAuthority)$' \
  -count=1 -timeout=10m
```

The existing required Go CI enables `HACKBOX_INTEGRATION=1` and executes all Go
packages under the race detector, so new fixtures run in CI without a new opt-in
flag. The required native browser workflow owns the final UI matrix; this issue
does not duplicate its opt-in browser process in local owner-package commands.

Initial compile proof: app0.512s, judging0.284s; later new complete publication
fixture compiled0.467s. The first sandboxed compile could not read the external
Go cache; the authorised compile used that cache and passed.

The first native new-fixture run on initial checkpoint proved the golden both
modes (app14.060s), but the practice test incorrectly expected missing-card
conflict in qualification mode where its deliberately absent finalist adapter
correctly denies access. This was a test expectation error, not a production
isolation defect. The corrected owner test asserts that exact mode-specific
guard and an empty source, instead of accepting arbitrary errors.

Stronger composed proof then added actual pre-live perfect practice to the same
lifetime as genuine signed qualification and fresh final cards. This complete
focused run passed: judging5.792s and app12.950s (golden11.33s, both modes). The
complete five-team publication regression passed44.76s (Go package46.279s).
All seven full native owner packages passed on a277b3d4: rubric17.082s,
scorecards25.864s, moderation82.885s, qualification292.141s, judging91.201s,
results23.252s and archives37.313s. No failures or skips. Exact passed test names,
source references, command and log digest are retained in
[native-owner-summary.json](native-owner-summary.json). The native app suite is
still pending at this checkpoint.

The two owned commits were then rebased onto908a69f8, which only adds the
independently reviewed measured Go CI job-budget correction. New head92c7a568
has byte-identical Go/API/module source to testeda277b3d4 (git diff passed);
owner tests were not unnecessarily repeated for a workflow budget alone. No production code has been changed.

Remaining gates: actual native runs and any genuine failures; full independent
Standards/Spec reviews; #111 focus correction and #113 archive correction; clear
all-team category authority and complete category action path; final current-head
CI and prerequisite merges before retargeting this separate PR to main. No
unchecked criterion is replaced by a documentation-only assertion.
