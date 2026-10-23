# Automation failure and confidentiality verification

#84 exercises the real PostgreSQL owners and delivery workers with synthetic
identities, controlled event clocks, a recording SMTP relay and a local TLS Teams
server. No email or shared-channel message is sent to a real destination. The
tests distinguish a committed local inbox record, queued external work, provider
acceptance and an unknown external outcome.

## Composed journeys added

| Journey | Evidence |
| --- | --- |
| Automated SMTP and inbox dispatch, relay acceptance, receipt-storage outage, fresh consumer and repeated scheduler/worker/reconciliation calls | One individual SMTP envelope, one local inbox record and one unresolved prepared attempt; no fabricated receipt or automatic second effect. Cancelling future automation preserves the unknown original. |
| Automated Teams dispatch to local TLS, provider acceptance, receipt-storage outage and fresh consumer | One physical request; repeated old jobs, clock sweeps and reconciliation retain uncertainty. Common card and safe history contain no participant addresses, endpoint or rendered private source. |
| Native official answer publication followed by a reviewed correction | The obsolete pending SMTP notice is denied, the actual current correction sends once, duplicate worker calls add no effect and local delivered history remains retained. Private question body/ID never enters source metadata or the external message. |
| Generic approved outcome-review cancellation and rescheduling | The cancelled original creates no provider intent. Edited timing requires both current campaign approval and explicit automation approval; only the reviewed replacement sends. This exercises the existing generic contract, not a future outcome-review production owner. |
| Native private data alongside an approved multi-channel reminder | An actual released rubric with private judge guidance, a native live scorecard with private evidence and an encrypted team credential remain private. The common Teams projection and inbox/SMTP prose omit their canaries and all mailboxes. Only the intended verified SMTP envelope contains its own address. Queue-only Teams evidence does not claim a send or receipt. |
| Browser recovery across both portals, desktop and mobile | A definite-failure retry becoming accepted loses generic retry; another send needs a distinct explicit duplicate-risk acknowledgement. An uncertain shared-channel decision reconciles its exact command without a second POST or invented acceptance. The existing navigation, archive, revocation and unknown-command cases remain green. |

The storage-outage fixtures deliberately fail receipt insertion after the local
provider physically accepts. This models the dangerous acceptance-to-durable-
receipt gap without claiming that a test killed a real process. A newly
constructed Jobs consumer and repeated reconciliation prove that retained
prepared evidence does not become permission to resend.

## Criterion-to-owner coverage

The aggregate reuses these real owner tests instead of reimplementing their
policies in a synthetic automation oracle:

| Requirement | Actual owning evidence |
| --- | --- |
| No uncontrolled duplicate after acceptance/crash or timeout | New `TestAutomationSMTPAcceptanceWithoutReceiptSurvivesRestartWithoutDuplicatingLocalOrExternalEffect` and `TestAutomationTeamsTLSAcceptanceReceiptFaultCannotReplayAfterConsumerRestart`; existing SMTP/Teams prepared-crash and receipt-history tests; #83 actual API→River reviewed-grant/old-lease regressions. |
| Approval edits, cancellation and current membership at execution | Automation `TestReschedulingAndCancellationInvalidateQueuedAutomation`, `TestSchedulePauseResumeMissedReviewAndCurrentAudience`, SMTP `TestApprovedSMTPCampaignWorkerRechecksAuthorityDestinationAndSource`, Teams `TestQueuedBroadcastRechecksCurrentRecipientsAndCancellation` and the new corrected-answer external journey. |
| Private scores, credentials, questions and unintended addresses excluded | New `TestApprovedMultiChannelAutomationCannotProjectNativePrivateCardsCredentialsOrOtherMailboxes`; strengthened `TestNativeClarificationBroadSubscriptionAndPinnedVersionUseActualCurrentPublication`; existing native Help/private-audience and Teams literal/common-message tests. |
| Admission cutoff and freeze serialize with real registration changes | Registration `TestBusinessCalendarAndConcurrentFinalCutoffNeverBackfill`, `TestFreezeAndMembershipMoveSerializeAgainstPublicationFacts`, `TestFreezeSerializesWithManualMovesAndQueuedPublication` and `TestFreezeRacesActualWithdrawalWithoutMovingSurvivors`. |
| Blocked/early/missed common challenge cannot send early or catch up late | Content `TestChallengePublicationGatesTimingAndRaces` covers T−7, the inclusive 60-second allowance, missed/blocked states, actual attendance/freeze gates and duplicate/cancel races. Automation expected-publication and native rubric/Teams challenge tests require the actual common release before discovering or queueing notices. |
| Missed initial publication needs postponement/cancellation; approved later postponement retains frozen teams and prior release | Content `TestContentReschedulingAndNotices`, event `TestPublishedPostponementNeedsApprovalAndKeepsFrozenTeams` and `TestScheduleEditsCancelApprovalWithoutReplayingExecutedActions`; communications `TestPostponementNeedsApprovedCurrentDatesAndChangedFactsNeedNewApproval`. |
| Duplicate Help/official answer/rubric intents, current recipients and corrected versions | Automation native Help/clarification/rubric source suites; rubric `TestRubricNotificationSourceRequiresActualCurrentPublicationAndCurrentAudience` and `TestRubricSharesActualChallengeGateAndPublishesReviewedCorrectionAtomically`. Native owners authorize current sources before delivery; retained delivered history has its separate current-reader policy. |
| Generic approved outcome-review cancellation/rescheduling | New `TestGenericApprovedOutcomeReviewCancellationAndRescheduleRequireFreshApproval`, plus retained-source expiry/archive tests. No fabricated production #117 publication is used. |

## Running the checks

The normal CI Go test job includes all these tests. For local owner verification,
set `HACKBOX_INTEGRATION=1` and run `go test -race` on `internal/automation`,
`internal/teamsworkflow`, `internal/smtpintegration`, `internal/jobs`,
`internal/communications`, `internal/rubric`, `internal/content`, `internal/event`
and `internal/registration`. PostgreSQL fixtures use disposable test databases.
`web/e2e/automation-recovery.spec.ts` and the existing delivery-history suite run
against both built portals and both desktop/mobile Playwright projects.

This verification does not complete live Teams tenant acceptance (#80), future
published-results integration (#112), or the named outcome-review journey
(#117). Those require their actual owners and acceptance evidence; #84 does not
replace or close them.

The CI race-test timeout is ten minutes per complete package, within the existing
twenty-minute job limit. The growing HTTP integration suite starts and cleans up
isolated PostgreSQL containers for its journeys; the former five-minute package
limit expired during normal container cleanup while the active test was only
five seconds old. This keeps a bounded deadlock limit without dropping race
checks, reducing coverage or extending individual provider acceptance deadlines.
