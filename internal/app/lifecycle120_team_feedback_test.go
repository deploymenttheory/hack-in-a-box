package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/teamreports"
	"github.com/google/uuid"
)

type lifecycleTeamFeedbackReceipt struct {
	TeamID                         uuid.UUID             `json:"team_id"`
	Case                           string                `json:"case"`
	SourceBinding                  string                `json:"source_binding"`
	ScoreSourceBinding             string                `json:"score_source_binding,omitempty"`
	Commands                       []teamreports.Receipt `json:"commands"`
	ReleasedVersions               []int64               `json:"released_versions"`
	InitialBodySHA256              string                `json:"initial_body_sha256"`
	AmendedBodySHA256              string                `json:"amended_body_sha256,omitempty"`
	DraftHidden                    bool                  `json:"draft_hidden"`
	PrivatePreparationDenied       bool                  `json:"private_preparation_denied"`
	OtherTeamDenied                bool                  `json:"other_team_denied"`
	WithdrawnReaderDenied          bool                  `json:"withdrawn_reader_denied"`
	StaffReaderDenied              bool                  `json:"staff_reader_denied"`
	WrongSourceDenied              bool                  `json:"wrong_source_denied"`
	UnavailableScoresDenied        bool                  `json:"unavailable_scores_denied"`
	ScoresIncludedInitially        bool                  `json:"scores_included_initially"`
	PrivateAmendmentKeptOldRelease bool                  `json:"private_amendment_kept_old_release"`
	AmendedScoresWithheld          bool                  `json:"amended_scores_withheld"`
	OriginalACKSurvivesAmendment   bool                  `json:"original_ack_survives_amendment"`
	CompetitionUnchanged           bool                  `json:"competition_unchanged"`
}

func lifecycleTeamFeedbackBody(amendment bool) teamreports.Body {
	value := teamreports.Body{Title: "Synthetic own-team learning", Summary: "Private reviewed learning from this team's demonstrated work.", Highlights: []string{"An authored observation, separate from raw private assessments and awards."}, NextSteps: []string{}}
	if amendment {
		value.Summary = "Private revised learning; this amendment requires its own deliberate release."
	}
	return value
}

// Report revisions belong in If-Match, not this body. Save-only fields must be
// absent from release; domain structs do not encode that wire distinction.
func lifecycleTeamFeedbackCommand(id uuid.UUID, action, source string, body teamreports.Body, scoreBinding string) map[string]any {
	out := map[string]any{"request_id": id, "action": action, "source_binding": source, "reason": "Explicit current own-team feedback decision; never an automatic result release"}
	if action == "save" {
		out["body"] = body
		out["include_final_scores"] = scoreBinding != ""
		if scoreBinding != "" {
			out["score_binding"] = scoreBinding
		}
	}
	return out
}

func lifecycleFeedbackHash(t *testing.T, body teamreports.Body) string {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (r *lifecycleJourney) readTeamFeedback(t *testing.T, path string) teamreports.Page {
	t.Helper()
	var page teamreports.Page
	if err := r.owner.call(t.Context(), http.MethodGet, path, -1, nil, &page, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	if page.CurrentSource == nil || page.CurrentSource.Binding == "" || page.Receipt != nil {
		t.Fatal("current report GET lacks its source or invented an original acknowledgement")
	}
	return page
}

func lifecycleReadReleasedFeedback(t *testing.T, client *lifecycleClient, path string) *teamreports.Released {
	t.Helper()
	var value struct {
		Report *teamreports.Released `json:"report"`
	}
	if err := client.call(t.Context(), http.MethodGet, path+"/released", -1, nil, &value, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	return value.Report
}

func (r *lifecycleJourney) changeTeamFeedback(t *testing.T, path string, page teamreports.Page, action string, body teamreports.Body, scoreBinding string, proof *lifecycleTeamFeedbackReceipt) teamreports.Page {
	t.Helper()
	id := uuid.New()
	command := lifecycleTeamFeedbackCommand(id, action, page.CurrentSource.Binding, body, scoreBinding)
	var next teamreports.Page
	if err := r.owner.call(t.Context(), http.MethodPost, path, page.Version, command, &next, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	ack := next.Receipt
	if ack == nil || ack.RequestID != id || ack.ActorID != r.owner.person || ack.TeamID != proof.TeamID || ack.Action != action || ack.ExpectedVersion != page.Version || ack.Version != page.Version+1 || ack.Version != next.Version || ack.Fingerprint == "" || next.Latest == nil || next.Latest.Source.Binding != proof.SourceBinding || next.CurrentSource == nil || next.CurrentSource.Binding != proof.SourceBinding {
		t.Fatal("native own-team report command lost exact actor/source/version acknowledgement")
	}
	state := "draft"
	if action == "release" {
		state = "released"
	}
	if next.Latest.Version != next.Version || next.Latest.TeamID != proof.TeamID || next.Latest.AuthorID != r.owner.person || next.Latest.ActorID != r.owner.person || next.Latest.State != state {
		t.Fatal("native report response substituted its version, target, author or phase")
	}
	var original struct {
		Receipt *teamreports.Receipt `json:"receipt"`
	}
	if err := r.owner.call(t.Context(), http.MethodGet, path+"/requests/"+id.String(), -1, nil, &original, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(original.Receipt, ack) {
		t.Fatal("read-only original report acknowledgement changed")
	}
	proof.Commands = append(proof.Commands, *ack)
	r.persistWorkProgress(t)
	return next
}

func (r *lifecycleJourney) denyTeamFeedbackSource(t *testing.T, path string, page teamreports.Page, source, score string) {
	t.Helper()
	body := lifecycleTeamFeedbackCommand(uuid.New(), "save", source, lifecycleTeamFeedbackBody(false), score)
	if err := r.owner.call(t.Context(), http.MethodPost, path, page.Version, body, nil, http.StatusConflict); err != nil {
		t.Fatal(err)
	}
	after := r.readTeamFeedback(t, path)
	if after.Version != page.Version || !reflect.DeepEqual(after.Latest, page.Latest) {
		t.Fatal("denied report source/score choice mutated native versions")
	}
}
