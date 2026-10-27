package app

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/githubapp"
	"github.com/deploymenttheory/hack-in-a-box/internal/objectives"
	"github.com/deploymenttheory/hack-in-a-box/internal/outcomes"
	"github.com/deploymenttheory/hack-in-a-box/internal/privacy"
	"github.com/deploymenttheory/hack-in-a-box/internal/registration"
	"github.com/deploymenttheory/hack-in-a-box/internal/repositories"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/teams"
	"github.com/deploymenttheory/hack-in-a-box/internal/templates"
)

// A single connected actor/event state is retained as the remaining lifecycle
// phases are implemented. A passing preparation/live checkpoint is explicitly
// not acceptance of the complete L01–L18 matrix.
type lifecycleJourneyEvent struct {
	browserHelp *lifecycleBrowserHelpReceipt

	failedMessage lifecycleFailedMessageReceipt

	browserApplications       []lifecycleBrowserApplicationReceipt
	browserSelections         []lifecycleBrowserSelectionReceipt
	browserAdmissionDecisions []string
	browserRegistrationOpened bool
	browserEventCreated       bool
	browserBrandVerified      bool

	retentionBoundary         *lifecycleReviewBoundaryReceipt
	retiredRepositoryVerified bool
	expiry                    lifecycleExpiryReceipt
	offerCutoff               lifecycleOfferCutoffReceipt
	attendance                lifecycleAttendanceReceipt
	postponement              lifecyclePostponementReceipt
	view                      event.View
	cohort                    lifecycleAdmissions
	eligibility               []lifecycleEligibilityReceipt
	teams                     teams.Page
	challenge                 content.Page
	rubric                    rubric.Configuration
	objective                 objectives.View
	brand                     templates.Branding
	github                    githubapp.Status
	repositories              []repositories.View
	access                    []lifecycleAccessReceipt
	liveAccess                []lifecycleAccessReceipt
	teamNotice                lifecycleTeamNoticeReceipt
	accessPoll                []lifecycleAccessPollReceipt
	deliverable               uuid.UUID
	submissions               []lifecycleSubmissionReceipt
	work                      []lifecycleWorkReceipt
	lateWork                  *lifecycleWorkReceipt
	cutoff                    []lifecycleCutoffReceipt
	judgeRun                  uuid.UUID
	judgeFinal                *lifecycleJudgeFinalReceipt
	resultDecision            *lifecycleResultDecisionReceipt
	resultReleases            []lifecycleResultReleaseReceipt
	resultPreparations        []lifecycleResultPreparationReceipt
	teamFeedback              []lifecycleTeamFeedbackReceipt
	liveHelp                  lifecycleHelpReceipt
	clarification             lifecycleClarificationReceipt
	practice                  []lifecyclePracticeReceipt
	review                    lifecycleMentorReviewReceipt
	postEvent                 lifecyclePostEventReceipt
	outcomes                  []outcomes.View
	showcase                  lifecycleShowcaseReceipt
	competitionBefore         map[string]lifecycleCompetitionDigest
	archive                   lifecycleArchiveReceipt
	reviews                   []lifecycleOwnedReviewReceipt
	commonHash                string
}
type lifecycleJourney struct {
	owner      *lifecycleClient
	control    *lifecycleControl
	timeline   lifecycleTimeline
	events     []lifecycleJourneyEvent
	livePollAt time.Time
	liveRoles  *lifecycleBrowserRoleReceipt
}

func TestLifecycleComposeJourney(t *testing.T) {
	if os.Getenv("HACKBOX_LIFECYCLE_ISOLATED") != "synthetic-only" {
		t.Skip("isolated native lifecycle journey only")
	}
	ctx := t.Context()
	control, err := newLifecycleControl(os.Getenv("HACKBOX_LIFECYCLE_CONTROL"))
	if err != nil {
		t.Fatal(err)
	}
	defer control.close()
	timeline, err := control.initialTimeline(ctx)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := signInLifecycle(
		ctx,
		"http://admin.localhost:8080",
		"http://provider.localhost:8080",
		"owner",
	)
	if err != nil {
		t.Fatal(err)
	}
	run := lifecycleJourney{
		owner:    owner,
		control:  control,
		timeline: timeline,
		events:   []lifecycleJourneyEvent{},
	}
	for index, mode := range []string{rubric.AllTeams, rubric.MentorQualification} {
		item := run.prepareEvent(t, mode, index)
		if index == 1 {
			first := &run.events[0]
			var unchanged event.View
			if err := owner.call(
				ctx,
				http.MethodGet,
				"/api/v1/events/"+first.view.ID.String(),
				-1,
				nil,
				&unchanged,
				http.StatusOK,
			); err != nil {
				t.Fatal(err)
			}
			if unchanged.Revision != first.view.Revision || unchanged.Stage != first.view.Stage ||
				unchanged.ConfirmedAttendees != 32 ||
				unchanged.PublishedAt != nil {
				t.Fatal("second-event preparation mutated the first event")
			}
			for i := range item.cohort.clients {
				if item.cohort.clients[i].person != first.cohort.clients[i].person {
					t.Fatal("overlapping OIDC people changed native identity across events")
				}
			}
			ids := map[uuid.UUID]bool{}
			for _, team := range first.teams.Teams {
				ids[team.ID] = true
			}
			for _, team := range item.teams.Teams {
				if ids[team.ID] {
					t.Fatal("frozen team identity crossed events")
				}
			}
		}
		run.events = append(run.events, item)
	}
	if lifecycleBrowserRequired(t) {
		for _, item := range run.events {
			if !item.browserBrandVerified {
				t.Fatal("required same-event browser branding verification was not executed")
			}
		}
	}
	run.rehearseFailedMessages(t)
	run.linkGitHubParticipants(t)
	// Let the genuine remaining offer expire at admission cutoff, then freeze
	// the published teams before repository setup or common theme disclosure.
	run.expireSurvivingOffers(t)
	run.provisionRepositories(t)
	if err := control.advance(ctx, timeline.Publication.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	for i := range run.events {
		item := &run.events[i]
		base := "/api/v1/events/" + item.view.ID.String()
		for _, path := range []string{base + "/content/" + item.challenge.ID.String(), base + "/rubric"} {
			if err := item.cohort.clients[3].call(
				ctx,
				http.MethodGet,
				path,
				-1,
				nil,
				nil,
				http.StatusNotFound,
			); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := control.advance(ctx, timeline.Publication); err != nil {
		t.Fatal(err)
	}
	for i := range run.events {
		run.readCommonPublication(t, &run.events[i])
	}
	run.postponePublishedEvents(t)
	run.depleteFrozenTeams(t)
	run.prepareMentorReview(t)
	run.planFinalMentorReview(t)
	run.prepareRepositoryAccess(t)
	run.buildDuringPreparation(t)
	run.preparePrivateDrafts(t)
	run.rehearseJudgePractice(t)
	// The original live date is now still preparation after explicit postponement.
	if err := control.advance(ctx, timeline.Live); err != nil {
		t.Fatal(err)
	}
	// Formal live entry remains guarded through the entire published preparation
	// week. A failed transition must not mutate event state or revision.
	for i := range run.events {
		item := &run.events[i]
		before := item.view
		var rejected struct {
			Error struct {
				Code     string        `json:"code"`
				Blockers []event.Issue `json:"blockers"`
			} `json:"error"`
		}
		if err := owner.call(
			ctx,
			http.MethodPost,
			"/api/v1/events/"+before.ID.String()+"/transition",
			before.Revision,
			map[string]any{
				"stage":                "live",
				"reason":               "Synthetic early-live denial proof",
				"attendance_exception": false,
			},
			&rejected,
			http.StatusConflict,
		); err != nil {
			t.Fatal(err)
		}
		early := false
		for _, blocker := range rejected.Error.Blockers {
			early = early || blocker.Field == "live_start" && blocker.Code == "too_early"
		}
		if rejected.Error.Code != "event_blocked" || !early {
			t.Fatal("live transition did not retain the real scheduled-start gate")
		}
		if err := owner.call(
			ctx,
			http.MethodGet,
			"/api/v1/events/"+before.ID.String(),
			-1,
			nil,
			&item.view,
			http.StatusOK,
		); err != nil {
			t.Fatal(err)
		}
		if item.view.Revision != before.Revision || item.view.Stage != "preparation" {
			t.Fatal("early live denial mutated native competition state")
		}
	}
	run.denyEarlyOutputHelp(t)
	if err := control.advance(ctx, run.timeline.Live); err != nil {
		t.Fatal(err)
	}
	run.enterLiveWithAttendanceException(t)
	run.prepareLiveMentorAccess(t)
	run.waitCurrentAccessPoll(t)
	run.closeJudgePracticeAtLive(t)
	run.raceLiveHelp(t)
	run.recoverLiveHelpInBrowser(t)
	run.checkLiveRolesInBrowser(t)
	run.publishCommonClarifications(t)
	run.recordLiveMentorCheckpoints(t)
	run.submitLiveEvidence(t)
	run.captureAndReconcileCutoff(t)
	run.advanceReconciledAssessment(t)
	run.reviewFrozenMentorEvidence(t)
	run.qualifyFrozenFinalists(t)
	run.publishSignedFinalists(t)
	run.scoreIndependentJudgeFinals(t)
	run.decideJudgeResults(t)
	run.publishConfirmedAwards(t)
	run.releaseOwnTeamFeedback(t)
	run.completeAndCollectFeedback(t)
	for i := range run.events {
		run.events[i].competitionBefore = lifecycleCompetitionSnapshot(
			t,
			run.events[i].view.ID,
			len(run.events[i].review.FinalHuddles),
		)
	}
	run.captureTeamOutcomesAndShowcase(t)
	run.prepareRetainedReviews(t)
	run.archiveCompetitionAndUpdateOutcome(t)
	run.deliverRetainedReviews(t)
	run.verifyRetiredRepositoriesUnscored(t)
	run.expireRetainedOutcomes(t)
	if mode := os.Getenv(
		"HACKBOX_LIFECYCLE_BROWSER_PHASES",
	); mode == "live-recovery" ||
		mode == "role-matrix" || mode == "ui-journey" {
		proof := run.events[0].browserHelp
		if proof == nil || !proof.NativeResolved || !proof.OtherTeamDenied ||
			!proof.NativeQueueExact ||
			proof.PutAttempts != 1 {
			t.Fatal("required browser recovery was not actually verified")
		}
	}
	if mode := os.Getenv(
		"HACKBOX_LIFECYCLE_BROWSER_PHASES",
	); mode == "role-matrix" ||
		mode == "ui-journey" {
		proof := run.liveRoles
		if proof == nil || proof.UIStaffGrants != 6 || len(proof.Cases) != 12 ||
			proof.NativeEventChecks != 24 ||
			!proof.MentorRevocationCleared ||
			!proof.OtherEventRetained ||
			!proof.NativeRevocationAndRestoreAudited {
			t.Fatal("required live browser role matrix was not independently verified")
		}
	}
	if os.Getenv("HACKBOX_LIFECYCLE_BROWSER_PHASES") == "ui-journey" {
		for _, item := range run.events {
			if !item.browserEventCreated || !item.browserRegistrationOpened ||
				len(item.browserApplications) != 4 || len(item.browserSelections) != 2 ||
				!reflect.DeepEqual(
					item.browserAdmissionDecisions,
					[]string{"competition-go", "withdraw", "accept"},
				) {
				t.Fatal("required native event was not created through the Fleet UI")
			}
		}
	}
	var providerProof lifecycleGitHubProof
	if err := control.call(ctx, http.MethodGet, "/github/proof", nil, &providerProof); err != nil {
		t.Fatal(err)
	}
	if providerProof.AppJWTsVerified < 4 || providerProof.InstallationTokensMinted < 2 ||
		providerProof.TemplateReads < 2 {
		t.Fatal(
			"native GitHub installation did not exercise actual verified App/scoped REST transport",
		)
	}
	if providerProof.OAuthExchangesVerified != 38 || providerProof.OAuthUserReads != 38 {
		t.Fatal(
			"participant linking did not retain exactly36 participant plus2 selected mentor PKCE/user verifications",
		)
	}
	if providerProof.RepositoryCreates != 14 {
		t.Fatal(
			"actual provider did not retain exactly fourteen original team repositories including two retired mappings",
			providerProof.RepositoryCreates,
		)
	}
	if providerProof.InvitationCreates != 82 || providerProof.InvitationAcceptances != 82 {
		t.Fatal(
			"actual external access census did not match58 surviving frozen competitor plus24 read-only mentor grants",
			providerProof.InvitationCreates,
			providerProof.InvitationAcceptances,
		)
	}
	type eventReceipt struct {
		BrowserHelp               *lifecycleBrowserHelpReceipt         `json:"browser_help_recovery,omitempty"`
		FailedMessage             lifecycleFailedMessageReceipt        `json:"failed_message"`
		BrowserApplications       []lifecycleBrowserApplicationReceipt `json:"browser_applications,omitempty"`
		BrowserSelections         []lifecycleBrowserSelectionReceipt   `json:"browser_selections,omitempty"`
		BrowserAdmissionDecisions []string                             `json:"browser_admission_decisions,omitempty"`
		BrowserRegistrationOpened bool                                 `json:"browser_registration_opened"`
		BrowserEventCreated       bool                                 `json:"browser_event_created"`
		BrowserBrandVerified      bool                                 `json:"browser_brand_verified"`

		RetentionBoundary         *lifecycleReviewBoundaryReceipt     `json:"retention_boundary,omitempty"`
		RetiredRepositoryVerified bool                                `json:"retired_repository_verified"`
		Expiry                    lifecycleExpiryReceipt              `json:"retention_expiry"`
		OfferCutoff               lifecycleOfferCutoffReceipt         `json:"surviving_offer_cutoff"`
		Attendance                lifecycleAttendanceReceipt          `json:"attendance_exception"`
		Postponement              lifecyclePostponementReceipt        `json:"postponement"`
		Eligibility               []lifecycleEligibilityReceipt       `json:"eligibility"`
		PostEvent                 lifecyclePostEventReceipt           `json:"post_event"`
		Showcase                  lifecycleShowcaseReceipt            `json:"showcase"`
		Archive                   lifecycleArchiveReceipt             `json:"archive"`
		Reviews                   []lifecycleOwnedReviewReceipt       `json:"owned_reviews"`
		ID                        uuid.UUID                           `json:"event_id"`
		Mode                      string                              `json:"mode"`
		Stage                     string                              `json:"stage"`
		Objective                 objectives.Reference                `json:"objective"`
		Primary                   string                              `json:"primary"`
		TeamIDs                   []uuid.UUID                         `json:"team_ids"`
		Confirmed                 int64                               `json:"confirmed"`
		Census                    map[string]int64                    `json:"admission_census"`
		Challenge                 uuid.UUID                           `json:"challenge_id"`
		Rubric                    rubric.Reference                    `json:"rubric"`
		CommonHash                string                              `json:"common_sha256"`
		PublishedAt               *time.Time                          `json:"published_at"`
		Repositories              []repositories.View                 `json:"repositories"`
		Access                    []lifecycleAccessReceipt            `json:"repository_access"`
		LiveAccess                []lifecycleAccessReceipt            `json:"live_mentor_repository_access"`
		TeamNotice                lifecycleTeamNoticeReceipt          `json:"team_notice"`
		AccessPoll                []lifecycleAccessPollReceipt        `json:"access_poll"`
		Submissions               []lifecycleSubmissionReceipt        `json:"submissions"`
		Work                      []lifecycleWorkReceipt              `json:"preparation_work"`
		LateWork                  *lifecycleWorkReceipt               `json:"late_work,omitempty"`
		Cutoff                    []lifecycleCutoffReceipt            `json:"cutoff_sources"`
		JudgeRun                  uuid.UUID                           `json:"planned_judge_run_id"`
		Practice                  []lifecyclePracticeReceipt          `json:"fictional_judge_practice"`
		JudgeFinal                *lifecycleJudgeFinalReceipt         `json:"independent_judge_final,omitempty"`
		ResultDecision            *lifecycleResultDecisionReceipt     `json:"result_decision,omitempty"`
		ResultReleases            []lifecycleResultReleaseReceipt     `json:"result_releases,omitempty"`
		ResultPreparations        []lifecycleResultPreparationReceipt `json:"result_preparations,omitempty"`
		TeamFeedback              []lifecycleTeamFeedbackReceipt      `json:"team_feedback,omitempty"`
		LiveHelp                  lifecycleHelpReceipt                `json:"live_help"`
		Clarification             lifecycleClarificationReceipt       `json:"common_clarification"`
		Review                    lifecycleMentorReviewReceipt        `json:"mentor_live_checkpoint"`
	}
	items := []eventReceipt{}
	for _, item := range run.events {
		if lifecycleRetentionDays(t) == 90 &&
			(len(item.reviews) != 2 || item.retentionBoundary == nil || !item.retentionBoundary.ApprovalRejected || item.retentionBoundary.Days != 90) {
			t.Fatal("default90 receipt lacks the actual rejected boundary approval")
		}
		if item.failedMessage.EventID != item.view.ID || item.failedMessage.State != "permanent" ||
			!item.failedMessage.RetryDenied || !item.failedMessage.HistoryIntact || !item.failedMessage.NoAcceptedData {
			t.Fatal("required same-event failed-message phase was not verified")
		}
		if !item.retiredRepositoryVerified || item.expiry.EventID != item.view.ID ||
			!item.expiry.OwnerReadableBefore ||
			!item.expiry.OwnerDeniedAt ||
			!item.expiry.OrganiserDeniedAt ||
			!item.expiry.NotificationDeniedAt ||
			!item.expiry.CompetitionUnchanged {
			t.Fatal(
				"final receipt lacks executed retired-repository and retention-expiry assertions",
			)
		}
		teams := []uuid.UUID{}
		for _, team := range item.teams.Teams {
			if team.State != "disbanded" {
				teams = append(teams, team.ID)
			}
		}
		items = append(
			items,
			eventReceipt{
				BrowserHelp:               item.browserHelp,
				FailedMessage:             item.failedMessage,
				BrowserBrandVerified:      item.browserBrandVerified,
				BrowserEventCreated:       item.browserEventCreated,
				BrowserAdmissionDecisions: item.browserAdmissionDecisions,
				BrowserRegistrationOpened: item.browserRegistrationOpened,
				BrowserApplications:       item.browserApplications,
				BrowserSelections:         item.browserSelections,
				RetentionBoundary:         item.retentionBoundary,
				RetiredRepositoryVerified: item.retiredRepositoryVerified,
				Expiry:                    item.expiry,
				OfferCutoff:               item.offerCutoff,
				Attendance:                item.attendance,
				Postponement:              item.postponement,
				Eligibility:               item.eligibility,
				PostEvent:                 item.postEvent,
				Showcase:                  item.showcase,
				Archive:                   item.archive,
				Reviews:                   item.reviews,
				ID:                        item.view.ID,
				Mode:                      item.rubric.Definition.Mode,
				Stage:                     item.view.Stage,
				Objective:                 item.objective.Reference,
				Primary:                   item.brand.Primary,
				TeamIDs:                   teams,
				Confirmed:                 item.view.ConfirmedAttendees,
				Census:                    item.cohort.census,
				Challenge:                 item.challenge.ID,
				Rubric:                    item.rubric.Reference,
				CommonHash:                item.commonHash,
				PublishedAt:               item.view.PublishedAt,
				Repositories:              item.repositories,
				Access:                    item.access,
				LiveAccess:                item.liveAccess,
				TeamNotice:                item.teamNotice,
				AccessPoll:                item.accessPoll,
				Submissions:               item.submissions,
				Work:                      item.work,
				LateWork:                  item.lateWork,
				Cutoff:                    item.cutoff,
				JudgeRun:                  item.judgeRun,
				Practice:                  item.practice,
				JudgeFinal:                item.judgeFinal,
				ResultDecision:            item.resultDecision,
				ResultReleases:            item.resultReleases,
				ResultPreparations:        item.resultPreparations,
				TeamFeedback:              item.teamFeedback,
				LiveHelp:                  item.liveHelp,
				Clarification:             item.clarification,
				Review:                    item.review,
			},
		)
	}
	receipt, err := json.MarshalIndent(
		map[string]any{
			"checkpoint":                             "connected-two-event-through-cutoff-partial",
			"complete_lifecycle":                     false,
			"github_transport_proof":                 providerProof,
			"events":                                 items,
			"overlapping_oidc_people":                len(run.events[0].cohort.clients),
			"retention_days":                         lifecycleRetentionDays(t),
			"retention_policy_explicitly_configured": lifecycleRetentionDays(t) == 120,
			"retention_reason":                       "Configured120 exercises retained day90 plus response window; default90 preserves the native default and rejects the exclusive boundary",
			"publication":                            timeline.Publication,
			"live_start":                             run.timeline.Live,
			"assertions": []string{
				"single-connected-native-http-journey",
				"both-judging-templates",
				"distinct-native-event-brands-objectives",
				"overlapping-native-person-identities",
				"independent-team-and-event-state",
				"actual-admission-fifo-go-remediation-and-freeze",
				"native-common-publication-in-both-events",
				"same-exact-public-rubric-within-event",
				"private-guidance-excluded",
				"exact-version-acknowledgements",
				"early-live-denied-without-mutation",
				"native-scheduled-live-transition",
				"normal-configured-encrypted-github-app-owner",
				"real-app-jwt-scoped-template-rest",
				"single-use-native-installation-callback",
				"actual-queued-private-template-repositories-for-every-frozen-team",
				"actual-native-participant-github-pkce-session-and-numeric-ownership",
				"reviewed-current-team-source-notification-and-obsolete-zero-effect",
				"actual-native-queued-access-poll-current-live-source",
			},
			"live_role_matrix": run.liveRoles,
			"remaining_matrix": "L01-L18 still require complete final integrated business/provider/browser variants",
		},
		"",
		"  ",
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join("/artifacts", "journey-start-live.json"),
		receipt,
		0o600,
	); err != nil {
		t.Fatal(err)
	}
}

func (r *lifecycleJourney) prepareEvent(
	t *testing.T,
	mode string,
	index int,
) lifecycleJourneyEvent {
	t.Helper()
	ctx := t.Context()
	item := lifecycleJourneyEvent{}
	input := lifecycleEventInput("Journey "+mode, r.timeline)
	if os.Getenv("HACKBOX_LIFECYCLE_BROWSER_PHASES") == "ui-journey" {
		r.createEventInBrowser(t, &item, input, index)
	} else if err := r.owner.call(ctx, http.MethodPost, "/api/v1/events", -1, input, &item.view, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	base := "/api/v1/events/" + item.view.ID.String()
	brand := templates.EmptyDefinitions().Branding
	brand.Assets = []templates.Asset{}
	brand.Primary = []string{"#164e63", "#5b21b6"}[index]
	item.brand = brand
	if lifecycleBrowserRequired(t) {
		r.saveBrandInBrowser(t, &item, index+1)
	} else {
		var branded templates.BrandView
		if err := r.owner.call(
			ctx,
			http.MethodPut,
			base+"/branding",
			item.view.Revision,
			brand,
			&branded,
			http.StatusOK,
		); err != nil {
			t.Fatal(err)
		}
	}
	target := "5"
	definition := objectives.Definition{
		Description:    "Synthetic planned demonstration response time for " + mode,
		OwnerID:        &r.owner.person,
		ThemeSensitive: true,
		Measures: []objectives.Measure{
			{
				ID:             uuid.New(),
				Description:    "Demonstrated response time",
				Baseline:       &objectives.Baseline{Value: "10", Basis: "estimate"},
				Target:         &target,
				Unit:           "minutes",
				Method:         "Review the synthetic demonstration log",
				EvidenceSource: "Synthetic demonstration record",
				ReviewHorizon:  "Owned 30/60/90-day reviews",
			},
		},
	}
	if err := r.owner.call(
		ctx,
		http.MethodPut,
		base+"/objectives/"+uuid.NewString(),
		0,
		objectives.Command{
			Action:     "save",
			Definition: &definition,
			Reason:     "Planning estimate only; not achieved outcome evidence",
		},
		&item.objective,
		http.StatusOK,
	); err != nil {
		t.Fatal(err)
	}
	if item.objective.Reference.Version != 1 || item.objective.Definition.OwnerID == nil ||
		*item.objective.Definition.OwnerID != r.owner.person ||
		item.objective.Definition.Measures[0].Baseline.Basis != "estimate" {
		t.Fatal("native planning objective lost its owner/estimated baseline identity")
	}
	if err := r.owner.call(
		ctx,
		http.MethodGet,
		base,
		-1,
		nil,
		&item.view,
		http.StatusOK,
	); err != nil {
		t.Fatal(err)
	}
	var policy privacy.Settings
	if lifecycleRetentionDays(t) == 120 {
		if err := r.owner.call(
			ctx,
			http.MethodPut,
			base+"/privacy",
			item.view.Revision,
			privacy.Settings{RetentionDays: 120, DraftDays: 7},
			&policy,
			http.StatusOK,
		); err != nil {
			t.Fatal(err)
		}
	} else {
		if err := r.owner.call(
			ctx,
			http.MethodGet,
			base+"/privacy",
			-1,
			nil,
			&policy,
			http.StatusOK,
		); err != nil {
			t.Fatal(err)
		}
	}
	if policy.RetentionDays != lifecycleRetentionDays(t) {
		t.Fatal("native event policy differs from the selected retention case")
	}
	var err error
	item.cohort, err = admitLifecycleCohortWithActions(
		ctx,
		r.owner,
		item.view,
		37,
		func(form registration.FormView) { r.proveLastingEligibility(t, &item, form, index == 1) },
		r.browserAdmissionActions(t, &item, index),
	)
	if err != nil {
		t.Fatal(err)
	}
	item.teams, err = allocateLifecycleTeamsWithInitialPublication(
		ctx,
		r.owner,
		item.view.ID,
		&item.cohort,
		r.control,
		func(page teams.Page) error { r.provisionInitialRepositories(t, &item, page); return nil },
		func() error { r.configureTeamNotice(t, &item); return nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	r.waitTeamNotice(t, &item)
	item.challenge, item.rubric, err = installLifecycleChallenge(ctx, r.owner, item.view.ID, mode)
	if err != nil {
		t.Fatal(err)
	}
	if item.rubric.Definition.Mode != mode ||
		item.teams.Frozen != (item.cohort.reserveID == uuid.Nil) {
		t.Fatal(
			"native event did not retain its distinct rubric and expected pending-offer freeze phase",
		)
	}
	if err := r.owner.call(
		ctx,
		http.MethodGet,
		base,
		-1,
		nil,
		&item.view,
		http.StatusOK,
	); err != nil {
		t.Fatal(err)
	}
	r.configureDeliverables(t, &item)
	return item
}

func (r *lifecycleJourney) readCommonPublication(t *testing.T, item *lifecycleJourneyEvent) {
	t.Helper()
	ctx := t.Context()
	base := "/api/v1/events/" + item.view.ID.String()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if err := r.owner.call(
			ctx,
			http.MethodGet,
			base,
			-1,
			nil,
			&item.view,
			http.StatusOK,
		); err != nil {
			t.Fatal(err)
		}
		if item.view.PublishedAt != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if item.view.PublishedAt == nil || item.view.PublishedAt.Before(r.timeline.Publication) ||
		item.view.PublishedAt.After(r.timeline.Publication.Add(time.Minute)) ||
		item.view.ConfirmedAttendees != 32 {
		t.Fatal(
			"native common publication did not use the real event readiness within its release window",
		)
	}
	var common json.RawMessage
	for _, reader := range []*lifecycleClient{item.cohort.clients[3], item.cohort.clients[5]} {
		var raw json.RawMessage
		if err := reader.call(
			ctx,
			http.MethodGet,
			base+"/rubric",
			-1,
			nil,
			&raw,
			http.StatusOK,
		); err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(raw, []byte("PRIVATE")) || common != nil && !bytes.Equal(common, raw) {
			t.Fatal("common rubric differed within event or leaked private judge guidance")
		}
		common = raw
		var page rubric.PublicPage
		if err := json.Unmarshal(
			raw,
			&page,
		); err != nil ||
			page.Reference != item.rubric.Reference {
			t.Fatal("native public rubric reference changed")
		}
		var released content.Page
		path := base + "/content/" + item.challenge.ID.String()
		if err := reader.call(
			ctx,
			http.MethodGet,
			path,
			-1,
			nil,
			&released,
			http.StatusOK,
		); err != nil {
			t.Fatal(err)
		}
		if released.ReleasedVersion == nil || *released.ReleasedVersion != item.challenge.Version {
			t.Fatal("native challenge lost exact released version")
		}
		var ack content.Page
		if err := reader.call(
			ctx,
			http.MethodPost,
			path+"/acknowledgements",
			*released.ReleasedVersion,
			nil,
			&ack,
			http.StatusOK,
		); err != nil {
			t.Fatal(err)
		}
		if ack.AcknowledgedAt == nil {
			t.Fatal("exact version acknowledgement not retained")
		}
		var branded templates.BrandView
		if err := reader.call(
			ctx,
			http.MethodGet,
			base+"/branding",
			-1,
			nil,
			&branded,
			http.StatusOK,
		); err != nil {
			t.Fatal(err)
		}
		if branded.Branding.Primary != item.brand.Primary {
			t.Fatal("event branding crossed the independent event boundary")
		}
		var planning json.RawMessage
		if err := reader.call(
			ctx,
			http.MethodGet,
			base+"/objectives",
			-1,
			nil,
			&planning,
			http.StatusForbidden,
		); err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(planning, []byte(item.objective.Definition.Description)) {
			t.Fatal("undisclosed private planning objective leaked to an attendee")
		}
	}
	digest := sha256.Sum256(common)
	item.commonHash = hex.EncodeToString(digest[:])
	if err := r.owner.call(
		ctx,
		http.MethodGet,
		base,
		-1,
		nil,
		&item.view,
		http.StatusOK,
	); err != nil {
		t.Fatal(err)
	}
}
