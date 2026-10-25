package app

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/qualification"
)

func TestQualificationHTTPActualFrozenFinalReviewedCardAndHistoricalACK(t *testing.T) {
	f := newQualificationFinalFixture(t)
	r, admin, public, run := f.runtime, f.admin, f.public, f.run
	second, scores := f.second, f.scores
	var err error
	request := qualificationHTTPRequest(t)
	base := fmt.Sprintf("/api/v1/events/%s/mentor-qualification/%s", r.eventID, run)
	read := func(surface rehearsalSurface, person dbgen.Person) qualification.Page {
		t.Helper()
		status, raw := request(surface, person, "GET", base, nil, 0, true)
		var p qualification.Page
		if status != 200 {
			t.Fatal("native API current read", status, string(raw))
		}
		if err = json.Unmarshal(raw, &p); err != nil {
			t.Fatal(err)
		}
		if p.Receipt != nil {
			t.Fatal("fresh read invented original ACK")
		}
		return p
	}
	p := read(admin, r.owner)
	change := func(command qualification.Command) qualification.Page {
		t.Helper()
		encoded, marshalErr := json.Marshal(command)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		var body contract.QualificationCommand
		if marshalErr = json.Unmarshal(encoded, &body); marshalErr != nil {
			t.Fatal(marshalErr)
		}
		status, raw := request(admin, r.owner, "POST", base, body, p.Revision, true)
		if status != 200 {
			t.Fatal("reviewed actual API intent", command.Action, status, string(raw))
		}
		var accepted qualification.Page
		if err = json.Unmarshal(raw, &accepted); err != nil {
			t.Fatal(err)
		}
		if accepted.Receipt == nil || accepted.Receipt.RequestID != command.RequestID ||
			accepted.Receipt.Action != command.Action ||
			accepted.Receipt.ExpectedVersion != command.Version ||
			accepted.Receipt.ExpectedRevision != p.Revision {
			t.Fatal("native ACK lost reviewed intent", accepted)
		}
		return accepted
	}
	p = change(
		qualification.Command{
			RequestID: uuid.New(),
			Version:   p.Round.Version,
			Action:    "configure_panel",
			Reason:    "Explicit private result panel",
			Panel: &qualification.Panel{
				MentorIDs:   []uuid.UUID{r.mentor.ID, second.ID},
				OrganiserID: r.owner.ID,
			},
		},
	)
	if len(p.Teams) != 1 || p.Teams[0].Source == nil || !p.Teams[0].CanSaveCard ||
		p.Teams[0].FreezeStatus != "not_applicable" {
		t.Fatal("actual URL final lacked exact writable source", p.Teams)
	}
	p = change(
		qualification.Command{
			RequestID:     uuid.New(),
			Version:       p.Round.Version,
			Action:        "save_card",
			Reason:        "Explicit moderated values in actual API",
			FreezeBinding: p.Teams[0].FreezeBinding,
			Card:          &qualification.CardInput{TeamID: r.teamID, Scores: scores},
		},
	)
	if !p.Teams[0].CanSignCard || len(p.Round.Cards) != 1 {
		t.Fatal("complete current draft not signable", p)
	}
	p = change(
		qualification.Command{
			RequestID:     uuid.New(),
			Version:       p.Round.Version,
			Action:        "sign_card",
			Reason:        "Reviewed exact final card",
			FreezeBinding: p.Teams[0].FreezeBinding,
			TeamID:        r.teamID,
			CardVersion:   p.Teams[0].CardVersion,
		},
	)
	receipt := *p.Receipt
	current := read(public, r.mentor)
	if len(current.Round.Cards) != 1 || current.Round.Cards[0].State != "signed" ||
		*current.Round.Cards[0].Scores[0].Value != 6 ||
		current.Current ||
		current.Capabilities.CanSignOff {
		t.Fatal("independent portal lost card or fabricated complete selection", current)
	}
	status, raw := request(
		admin,
		r.owner,
		"GET",
		base+"/commands/"+receipt.RequestID.String(),
		nil,
		0,
		true,
	)
	var ack qualification.Recovery
	if err = json.Unmarshal(
		raw,
		&ack,
	); err != nil || status != 200 || ack.Receipt == nil ||
		*ack.Receipt != receipt {
		t.Fatal("actual signed-card original ACK", status, ack, err)
	}
	status, raw = request(public, r.mentor, "GET", base+"/history?limit=20", nil, 0, true)
	var history qualification.History
	if err = json.Unmarshal(
		raw,
		&history,
	); err != nil || status != 200 || len(history.Items) != 3 || len(history.Items[0].Snapshot.Cards) != 1 ||
		history.Items[0].Snapshot.Cards[0].State != "signed" {
		t.Fatal("actual current audience retained card history", status, history, err)
	}
}
