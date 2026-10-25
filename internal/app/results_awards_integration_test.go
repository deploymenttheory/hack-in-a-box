package app

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/results"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/google/uuid"
)

func TestResultsNativeConfirmationAndAwardPublicationKeepIndependentEmbargoes(t *testing.T) {
	for _, mode := range []string{rubric.AllTeams, rubric.MentorQualification} {
		t.Run(mode, func(t *testing.T) {
			first, second := uuid.New(), uuid.New()
			var category uuid.UUID
			f := newResultsSourceFixtureWithSetup(t, mode, func(d *rubric.Definition) {
				category = d.Categories[0].ID
				d.Prizes = []rubric.Prize{{ID: first, Name: "Overall award"}, {ID: second, Name: "Demo award", CategoryID: &category}}
			}, nil)
			ctx := t.Context()
			eventID := f.runtime.eventID
			actor := f.runtime.owner.ID
			p, err := f.service.Read(ctx, actor, eventID)
			resultsSourceRequire(t, err)
			earlier := 0
			if mode == rubric.MentorQualification {
				p, err = f.service.Change(ctx, actor, eventID, p.Revision, results.Command{RequestID: uuid.New(), Version: p.Round.Version, Action: "publish_finalists", SourceBinding: p.Preview.FinalistsBinding, Reason: "Finalists announced while final judges are pending"})
				resultsSourceRequire(t, err)
				earlier = 1
			}
			f.complete(t)
			p, err = f.service.Read(ctx, actor, eventID)
			resultsSourceRequire(t, err)
			p, err = f.service.Change(ctx, actor, eventID, p.Revision, results.Command{RequestID: uuid.New(), Version: p.Round.Version, Action: "prepare", SourceBinding: p.Preview.Binding, Reason: "Private exact judge preparation"})
			resultsSourceRequire(t, err)
			awards := []results.Award{{PrizeID: first, Name: "Overall award", TeamID: f.runtime.teamID, Amount: "12345678901234567890.001", Currency: "GBP"}, {PrizeID: second, Name: "Demo award", CategoryID: &category, TeamID: f.runtime.teamID}}
			p, err = f.service.Change(ctx, actor, eventID, p.Revision, results.Command{RequestID: uuid.New(), Version: p.Round.Version, Action: "confirm", SourceBinding: p.Preview.Binding, Reason: "Explicit private approval of actual independent judges", Awards: awards})
			resultsSourceRequire(t, err)
			if p.Round.Confirmation == nil || len(p.Round.Confirmation.Awards) != 2 {
				t.Fatal("missing native exact approval", p)
			}
			released, err := f.service.Released(ctx, f.runtime.competitor.ID, eventID)
			resultsSourceRequire(t, err)
			if len(released) != earlier {
				t.Fatal("confirmation leaked category awards", released)
			}
			command := results.Command{RequestID: uuid.New(), Version: p.Round.Version, Action: "publish_awards", SourceBinding: p.Preview.Binding, Reason: "Explicitly announce confirmed awards"}
			expected := p.Revision
			announced, err := f.service.Change(ctx, actor, eventID, expected, command)
			resultsSourceRequire(t, err)
			if len(announced.Announcements) != earlier+1 || announced.Receipt.PublicationID == nil {
				t.Fatal("missing released award event", announced)
			}
			released, err = f.service.Released(ctx, f.runtime.competitor.ID, eventID)
			resultsSourceRequire(t, err)
			var awardEvent *results.Announcement
			for _, item := range released {
				if item.Kind == "awards" {
					value := item
					awardEvent = &value
				}
			}
			if awardEvent == nil || len(awardEvent.Awards) != 2 || awardEvent.Awards[0].Team.ID != awardEvent.Awards[1].Team.ID || awardEvent.Awards[0].Amount != awards[0].Amount {
				t.Fatal("award metadata rounded or multi-award team lost", awardEvent)
			}
			raw, err := json.Marshal(awardEvent)
			resultsSourceRequire(t, err)
			for _, private := range []string{resultsMentorPrivateNote, "PRIVATE-INDEPENDENT-JUDGE-PROSE", "source_binding", "evidence", "votes", "confirmation", "reason"} {
				if strings.Contains(string(raw), private) {
					t.Fatal("private preparation in released award event", private)
				}
			}
			replay, err := f.service.Change(ctx, actor, eventID, expected, command)
			resultsSourceRequire(t, err)
			if replay.Round.Version != announced.Round.Version || replay.Receipt.PublicationID == nil || *replay.Receipt.PublicationID != *announced.Receipt.PublicationID {
				t.Fatal("award retry repeated publication")
			}
			duplicate := command
			duplicate.RequestID = uuid.New()
			duplicate.Version = announced.Round.Version
			if _, err = f.service.Change(ctx, actor, eventID, announced.Revision, duplicate); !errors.Is(err, access.ErrConflict) {
				t.Fatal("new request repeated same confirmed announcement", err)
			}
			f.reopen(t, 0)
			current, err := f.service.Read(ctx, actor, eventID)
			resultsSourceRequire(t, err)
			if current.Current || current.Round.Confirmation != nil || len(current.Round.Source.Tallies) != 0 {
				t.Fatal("reopen retained current private approval", current)
			}
			duplicate.RequestID = uuid.New()
			duplicate.Version = current.Round.Version
			if _, err = f.service.Change(ctx, actor, eventID, current.Revision, duplicate); !errors.Is(err, access.ErrConflict) {
				t.Fatal("stale confirmation published after native reopen", err)
			}
			retained, err := f.service.Released(ctx, f.runtime.competitor.ID, eventID)
			resultsSourceRequire(t, err)
			if len(retained) != earlier+1 {
				t.Fatal("explicit historical release silently removed", retained)
			}
		})
	}
}
