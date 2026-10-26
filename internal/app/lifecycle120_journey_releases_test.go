package app

import (
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/results"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/google/uuid"
)

type lifecycleResultReleaseReceipt struct {
	Kind              string          `json:"kind"`
	SourceBinding     string          `json:"source_binding"`
	Acknowledgement   results.Receipt `json:"acknowledgement"`
	AnnouncementID    uuid.UUID       `json:"announcement_id"`
	FinalistIDs       []uuid.UUID     `json:"finalist_ids,omitempty"`
	ReserveIDs        []uuid.UUID     `json:"reserve_ids,omitempty"`
	AwardTeamIDs      []uuid.UUID     `json:"award_team_ids,omitempty"`
	DuplicateDenied   bool            `json:"duplicate_release_denied"`
	RecipientRead     bool            `json:"current_recipient_read"`
	PrivateReadDenied bool            `json:"private_preparation_read_denied"`
}

type lifecycleResultPreparationReceipt struct {
	SourceBinding   string          `json:"source_binding"`
	Acknowledgement results.Receipt `json:"acknowledgement"`
}

func lifecycleAssertReleaseKinds(t *testing.T, item *lifecycleJourneyEvent, values []results.Announcement, awards bool) {
	t.Helper()
	expected := map[string]uuid.UUID{}
	for _, release := range item.resultReleases {
		expected[release.Kind] = release.AnnouncementID
	}
	if !awards {
		delete(expected, "awards")
	}
	if len(values) != len(expected) {
		t.Fatal("native announcement count differs from deliberate release intents")
	}
	for _, value := range values {
		if id, ok := expected[value.Kind]; !ok || id != value.ID {
			t.Fatal("native results exposed an unreleased or substituted announcement")
		}
		delete(expected, value.Kind)
	}
}

func (r *lifecycleJourney) resultRecipient(t *testing.T, item *lifecycleJourneyEvent) *lifecycleClient {
	t.Helper()
	for _, team := range item.teams.Teams {
		if team.State == "published" && len(team.Published) > 0 {
			return r.participant(t, team.Published[0].ID)
		}
	}
	t.Fatal("native release lacks a current frozen recipient")
	return nil
}

func (r *lifecycleJourney) publishResult(t *testing.T, item *lifecycleJourneyEvent, page results.Page, action, binding string) results.Page {
	t.Helper()
	if err := r.control.nextAction(t.Context()); err != nil {
		t.Fatal(err)
	}
	before := page
	page = r.changeResults(t, item, page, map[string]any{"action": action, "source_binding": binding})
	ack := page.Receipt
	var selected *results.Announcement
	for _, value := range page.Announcements {
		if value.ID == *ack.PublicationID {
			copy := value
			selected = &copy
		}
	}
	if selected == nil || selected.Version != ack.Version {
		t.Fatal("native release acknowledgement does not identify its immutable announcement")
	}
	proof := lifecycleResultReleaseReceipt{Kind: selected.Kind, SourceBinding: binding, Acknowledgement: *ack, AnnouncementID: selected.ID}
	if action == "publish_finalists" {
		if selected.Kind != "finalists" || len(selected.Awards) != 0 || before.Preview.Selection == nil {
			t.Fatal("finalist release inferred awards or lacks signed qualification")
		}
		for _, value := range selected.Finalists {
			proof.FinalistIDs = append(proof.FinalistIDs, value.ID)
		}
		for _, value := range selected.Reserves {
			proof.ReserveIDs = append(proof.ReserveIDs, value.ID)
		}
		if !reflect.DeepEqual(proof.FinalistIDs, before.Preview.Selection.FinalistIDs) || !reflect.DeepEqual(proof.ReserveIDs, before.Preview.Selection.ReserveIDs) {
			t.Fatal("released finalists/reserve differ from exact signed native panel selection")
		}
	} else {
		if selected.Kind != "awards" || before.Round.Confirmation == nil || len(selected.Awards) != len(before.Round.Confirmation.Awards) || len(selected.Finalists)+len(selected.Reserves) != 0 {
			t.Fatal("award release inferred finalists or lost exact separate confirmation")
		}
		for i, value := range selected.Awards {
			confirmed := before.Round.Confirmation.Awards[i]
			if value.PrizeID != confirmed.PrizeID || value.Team.ID != confirmed.TeamID || value.Amount != confirmed.Amount || value.Currency != confirmed.Currency || !reflect.DeepEqual(value.CategoryID, confirmed.CategoryID) {
				t.Fatal("released award differs from current exact confirmed metadata")
			}
			proof.AwardTeamIDs = append(proof.AwardTeamIDs, value.Team.ID)
		}
	}
	// A fresh intent cannot duplicate the same source publication. This is not
	// acknowledgement recovery: the original request was already GET-recovered.
	path := "/api/v1/events/" + item.view.ID.String() + "/results"
	body := lifecycleResultCommandBody(uuid.New(), page, map[string]any{"action": action, "source_binding": binding})
	if err := r.owner.call(t.Context(), http.MethodPost, path, page.Revision, body, nil, http.StatusConflict); err != nil {
		t.Fatal(err)
	}
	after := r.currentResults(t, item)
	if after.Revision != page.Revision || after.Round.Version != page.Round.Version || !reflect.DeepEqual(after.Announcements, page.Announcements) {
		t.Fatal("duplicate release changed immutable publication or result version")
	}
	proof.DuplicateDenied = true
	client := r.resultRecipient(t, item)
	var visible struct {
		Items []results.Announcement `json:"items"`
	}
	if err := client.call(t.Context(), http.MethodGet, path+"/announcements", -1, nil, &visible, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	expected, actual := slices.Clone(page.Announcements), slices.Clone(visible.Items)
	byID := func(a, b results.Announcement) int { return strings.Compare(a.ID.String(), b.ID.String()) }
	slices.SortFunc(expected, byID)
	slices.SortFunc(actual, byID)
	if !reflect.DeepEqual(actual, expected) {
		t.Fatal("current recipient lacks exact separately released safe snapshots")
	}
	proof.RecipientRead = true
	if err := client.call(t.Context(), http.MethodGet, path, -1, nil, nil, http.StatusForbidden); err != nil {
		t.Fatal(err)
	}
	proof.PrivateReadDenied = true
	item.resultReleases = append(item.resultReleases, proof)
	lifecycleAssertReleaseKinds(t, item, visible.Items, action == "publish_awards")
	r.persistWorkProgress(t)
	t.Logf("native deliberate %s release: publication=%s version=%d", selected.Kind, selected.ID, ack.Version)
	return page
}

func (r *lifecycleJourney) publishSignedFinalists(t *testing.T) {
	t.Helper()
	for i := range r.events {
		item := &r.events[i]
		if item.rubric.Definition.Mode != rubric.MentorQualification {
			continue
		}
		page := r.currentResults(t, item)
		if page.Preview.Selection == nil || page.Preview.FinalistsBinding == "" || page.Preview.Binding != "" || len(page.Announcements) != 0 || item.judgeFinal != nil {
			t.Fatal("finalists were not deliberately released before fresh independent judge cards")
		}
		page = r.changeResults(t, item, page, map[string]any{"action": "prepare", "source_binding": page.Preview.FinalistsBinding})
		if !page.Current || page.Round.PreparedBy != r.owner.person || len(page.Round.Votes) != 0 || page.Round.Confirmation != nil || len(page.Announcements) != 0 {
			t.Fatal("private signed-finalist preparation inferred judging, confirmation or release")
		}
		r.publishResult(t, item, page, "publish_finalists", page.Preview.FinalistsBinding)
	}
}

func (r *lifecycleJourney) publishConfirmedAwards(t *testing.T) {
	t.Helper()
	for i := range r.events {
		item := &r.events[i]
		page := r.currentResults(t, item)
		if page.Round.Confirmation == nil || page.Round.Confirmation.ID != *item.resultDecision.ConfirmationID {
			t.Fatal("award release lost its actual separately confirmed result")
		}
		r.publishResult(t, item, page, "publish_awards", page.Preview.Binding)
	}
}
