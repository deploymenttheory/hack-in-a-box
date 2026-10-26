package app

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/teamreports"
	"github.com/google/uuid"
)

func (r *lifecycleJourney) feedbackReaders(t *testing.T, item *lifecycleJourneyEvent, teamID uuid.UUID) (own, other, withdrawn *lifecycleClient) {
	t.Helper()
	current := map[uuid.UUID]bool{}
	for _, team := range item.teams.Teams {
		if team.State != "published" {
			continue
		}
		for _, member := range team.Published {
			current[member.ID] = true
			if team.ID == teamID && own == nil {
				own = r.participant(t, member.ID)
			}
			if team.ID != teamID && other == nil {
				other = r.participant(t, member.ID)
			}
		}
	}
	for _, client := range item.cohort.clients {
		if !current[client.person] {
			withdrawn = client
			break
		}
	}
	if own == nil || other == nil || withdrawn == nil {
		t.Fatal("report privacy lacks actual current/other-team/withdrawn native identities")
	}
	return
}

func (r *lifecycleJourney) feedbackUnscoredTeam(t *testing.T, item *lifecycleJourneyEvent) (uuid.UUID, string) {
	t.Helper()
	if item.rubric.Definition.Mode == rubric.MentorQualification {
		if item.review.Qualification == nil || len(item.review.Qualification.ReserveIDs) != 1 {
			t.Fatal("unscored report lacks its actual signed reserve")
		}
		return item.review.Qualification.ReserveIDs[0], "unscored_frozen_reserve"
	}
	for _, source := range item.cutoff {
		if source.Outcome == "not_submitted" {
			return source.TeamID, "not_submitted"
		}
	}
	t.Fatal("unscored report lacks an actual not-submitted outcome")
	return uuid.Nil, ""
}

func (r *lifecycleJourney) releaseOwnTeamFeedback(t *testing.T) {
	t.Helper()
	for index := range r.events {
		item := &r.events[index]
		before := r.currentResults(t, item)
		if before.Round.Confirmation == nil || len(item.resultReleases) == 0 {
			t.Fatal("feedback lacks actual confirmed and deliberately released results")
		}
		unscored, kind := r.feedbackUnscoredTeam(t, item)
		for n, id := range []uuid.UUID{item.judgeFinal.EligibleTeamIDs[0], unscored} {
			caseName := "fresh_judge_scored"
			if n == 1 {
				caseName = kind
			}
			item.teamFeedback = append(item.teamFeedback, lifecycleTeamFeedbackReceipt{TeamID: id, Case: caseName})
			proof := &item.teamFeedback[len(item.teamFeedback)-1]
			path := "/api/v1/events/" + item.view.ID.String() + "/teams/" + id.String() + "/feedback-report"
			page := r.readTeamFeedback(t, path)
			if page.Version != 0 || page.Latest != nil || page.CurrentSource.TeamID != id || page.CurrentSource.ScoresAvailable != (n == 0) {
				t.Fatal("new report inferred an existing draft or wrong native score availability")
			}
			proof.SourceBinding = page.CurrentSource.Binding
			own, other, withdrawn := r.feedbackReaders(t, item, id)
			if lifecycleReadReleasedFeedback(t, own, path) != nil {
				t.Fatal("native member received a report before deliberate authorship/release")
			}
			if err := own.call(t.Context(), http.MethodGet, path, -1, nil, nil, http.StatusForbidden); err != nil {
				t.Fatal(err)
			}
			proof.PrivatePreparationDenied = true
			for _, client := range []*lifecycleClient{other, withdrawn} {
				if err := client.call(t.Context(), http.MethodGet, path+"/released", -1, nil, nil, http.StatusForbidden); err != nil {
					t.Fatal(err)
				}
			}
			proof.OtherTeamDenied, proof.WithdrawnReaderDenied = true, true
			for _, actor := range []string{"mentor-a", "judge-a"} {
				if err := item.cohort.staff[actor].call(t.Context(), http.MethodGet, path+"/released", -1, nil, nil, http.StatusForbidden); err != nil {
					t.Fatal(err)
				}
			}
			proof.StaffReaderDenied = true
			var foreign uuid.UUID
			for _, team := range item.teams.Teams {
				if team.State == "published" && team.ID != id {
					foreign = team.ID
					break
				}
			}
			foreignPage := r.readTeamFeedback(t, "/api/v1/events/"+item.view.ID.String()+"/teams/"+foreign.String()+"/feedback-report")
			if foreignPage.CurrentSource.Binding == proof.SourceBinding {
				t.Fatal("different native report targets share a source binding")
			}
			r.denyTeamFeedbackSource(t, path, page, foreignPage.CurrentSource.Binding, "")
			proof.WrongSourceDenied = true
			scoreBinding := ""
			if n == 0 {
				scores := page.CurrentSource.FinalScores
				if scores == nil || scores.TeamID != id || scores.JudgeRunID != item.judgeRun || scores.ConfirmationID != *item.resultDecision.ConfirmationID || scores.Binding != item.resultDecision.SourceBinding || scores.RequiredJudges != 3 {
					t.Fatal("optional report scores lack the exact current confirmed own-team source")
				}
				scoreBinding = scores.Binding
				proof.ScoreSourceBinding = scoreBinding
			} else {
				if page.CurrentSource.FinalScores != nil {
					t.Fatal("unscored native team acquired fabricated final scores")
				}
				r.denyTeamFeedbackSource(t, path, page, proof.SourceBinding, item.resultDecision.SourceBinding)
				proof.UnavailableScoresDenied = true
			}
			initial := lifecycleTeamFeedbackBody(false)
			proof.InitialBodySHA256 = lifecycleFeedbackHash(t, initial)
			page = r.changeTeamFeedback(t, path, page, "save", initial, scoreBinding, proof)
			if page.Latest.State != "draft" || page.Latest.IncludeFinalScores != (n == 0) || lifecycleFeedbackHash(t, page.Latest.Body) != proof.InitialBodySHA256 {
				t.Fatal("private report draft changed the explicit authored body/score choice")
			}
			if !reflect.DeepEqual(page.Latest.FinalScores, page.CurrentSource.FinalScores) {
				t.Fatal("initial report did not preserve its explicitly selected current own-team score source")
			}
			if lifecycleReadReleasedFeedback(t, own, path) != nil {
				t.Fatal("saving a private report implicitly released it")
			}
			proof.DraftHidden = true
			page = r.changeTeamFeedback(t, path, page, "release", teamreports.Body{}, "", proof)
			released := lifecycleReadReleasedFeedback(t, own, path)
			if released == nil || released.Version != 2 || released.TeamID != id || released.AuthorID != r.owner.person || released.ReleasedBy != r.owner.person || lifecycleFeedbackHash(t, released.Body) != proof.InitialBodySHA256 || !reflect.DeepEqual(released.FinalScores, page.Latest.FinalScores) {
				t.Fatal("explicit own-team release lost its current authored/source projection")
			}
			proof.ReleasedVersions = append(proof.ReleasedVersions, released.Version)
			proof.ScoresIncludedInitially = released.FinalScores != nil
			if n == 0 {
				old := *released
				amendment := lifecycleTeamFeedbackBody(true)
				proof.AmendedBodySHA256 = lifecycleFeedbackHash(t, amendment)
				page = r.changeTeamFeedback(t, path, page, "save", amendment, "", proof)
				if current := lifecycleReadReleasedFeedback(t, own, path); !reflect.DeepEqual(current, &old) {
					t.Fatal("a new private amendment replaced or leaked over the last released report")
				}
				proof.PrivateAmendmentKeptOldRelease = true
				page = r.changeTeamFeedback(t, path, page, "release", teamreports.Body{}, "", proof)
				revised := lifecycleReadReleasedFeedback(t, own, path)
				if revised == nil || revised.Version != 4 || revised.FinalScores != nil || !page.CurrentSource.ScoresAvailable || page.CurrentSource.FinalScores == nil || lifecycleFeedbackHash(t, revised.Body) != proof.AmendedBodySHA256 {
					t.Fatal("deliberate amendment failed to preserve organiser score-withholding choice")
				}
				proof.AmendedScoresWithheld = true
				proof.ReleasedVersions = append(proof.ReleasedVersions, revised.Version)
				var history struct {
					Items []teamreports.Record `json:"items"`
				}
				if err := r.owner.call(t.Context(), http.MethodGet, path+"/history", -1, nil, &history, http.StatusOK); err != nil {
					t.Fatal(err)
				}
				preserved := false
				for _, record := range history.Items {
					if record.Version == 2 && lifecycleFeedbackHash(t, record.Body) == proof.InitialBodySHA256 {
						preserved = true
					}
				}
				if len(history.Items) != 4 || !preserved {
					t.Fatal("native amendment mutated earlier released feedback history")
				}
				var original struct {
					Receipt *teamreports.Receipt `json:"receipt"`
				}
				if err := r.owner.call(t.Context(), http.MethodGet, path+"/requests/"+proof.Commands[1].RequestID.String(), -1, nil, &original, http.StatusOK); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(original.Receipt, &proof.Commands[1]) {
					t.Fatal("amendment changed the original release acknowledgement")
				}
				proof.OriginalACKSurvivesAmendment = true
			}
			r.persistWorkProgress(t)
		}
		after := r.currentResults(t, item)
		if !reflect.DeepEqual(before.Round, after.Round) || before.Preview.Binding != after.Preview.Binding || !reflect.DeepEqual(before.Announcements, after.Announcements) {
			t.Fatal("own-team feedback changed frozen competition decisions or announcements")
		}
		for i := range item.teamFeedback {
			item.teamFeedback[i].CompetitionUnchanged = true
		}
		r.persistWorkProgress(t)
	}
}
