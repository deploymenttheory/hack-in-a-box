package app

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/moderation"
	"github.com/deploymenttheory/hack-in-a-box/internal/qualification"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
	"github.com/deploymenttheory/hack-in-a-box/internal/templates"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestQualificationHTTPActualFrozenFinalReviewedCardAndHistoricalACK(t *testing.T) {
	var cp scorecards.Checkpoint
	var second dbgen.Person
	r, admin, public, run := qualificationHTTPFixture(
		t,
		func(r *liveRehearsal, surface rehearsalSurface, run uuid.UUID) {
			page, err := surface.deps.Events.Read(t.Context(), r.owner.ID, r.eventID)
			if err != nil {
				t.Fatal(err)
			}
			second = testdb.Person(t, r.pool)
			if _, err = r.pool.Exec(
				t.Context(),
				"INSERT INTO event_memberships(event_id,person_id,role)VALUES($1,$2,'mentor')",
				r.eventID,
				second.ID,
			); err != nil {
				t.Fatal(err)
			}
			availability, _ := json.Marshal(
				[]map[string]time.Time{
					{"start": page.Configuration.LiveStart, "end": page.Configuration.LiveEnd},
				},
			)
			for _, person := range []dbgen.Person{r.mentor, second} {
				if _, err = r.pool.Exec(
					t.Context(),
					"INSERT INTO mentor_profiles(event_id,person_id,skills,availability)VALUES($1,$2,'{}',$3)",
					r.eventID,
					person.ID,
					availability,
				); err != nil {
					t.Fatal(err)
				}
			}
			if _, err = r.pool.Exec(
				t.Context(),
				"INSERT INTO mentor_assignments(event_id,team_id,person_id,slot,snapshot,starts_at,ends_at)VALUES($1,$2,$3,'second','published',$4,$5)",
				r.eventID,
				r.teamID,
				second.ID,
				page.Configuration.LiveStart,
				page.Configuration.LiveEnd,
			); err != nil {
				t.Fatal(err)
			}
			deliverable := uuid.New()
			defs := templates.EmptyDefinitions()
			defs.Deliverables = []templates.Deliverable{
				{
					ID:            deliverable,
					Title:         "Formal hosted final",
					Required:      true,
					EvidenceKinds: []string{"url"},
					Audience:      []string{"mentor", "judge", "organiser"},
				},
			}
			raw, _ := json.Marshal(defs)
			if err = dbgen.New(r.pool).
				SaveEventDefinitions(t.Context(), dbgen.SaveEventDefinitionsParams{EventID: r.eventID, Definitions: raw}); err != nil {
				t.Fatal(err)
			}
			native, err := surface.deps.Submissions.Read(
				t.Context(),
				r.competitor.ID,
				r.eventID,
				r.teamID,
			)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = surface.deps.Submissions.Submit(
				t.Context(),
				r.competitor.ID,
				r.eventID,
				r.teamID,
				native.Revision,
				submissions.Command{
					Evidence: []submissions.Evidence{
						{
							DeliverableID: deliverable,
							Kind:          "url",
							Label:         "Actual formal URL",
							URL:           "https://synthetic.invalid/formal-final",
						},
					},
				},
			); err != nil {
				t.Fatal(err)
			}
			page, err = surface.deps.Events.Read(t.Context(), r.owner.ID, r.eventID)
			if err != nil {
				t.Fatal(err)
			}
			zone, err := time.LoadLocation(page.Configuration.Timezone)
			if err != nil {
				t.Fatal(err)
			}
			cp, err = surface.deps.Scorecards.Configure(
				t.Context(),
				r.owner.ID,
				r.eventID,
				page.Revision,
				scorecards.CheckpointCommand{
					Reason: "Actual independent final-review context",
					Definition: scorecards.CheckpointDefinition{
						ID:    uuid.New(),
						Name:  "Frozen final review",
						RunID: run,
						OpensWallTime: page.Configuration.SubmissionCutoff.In(zone).
							Format("2006-01-02T15:04:05"),
						ClosesWallTime: page.Configuration.LiveEnd.Add(time.Hour).
							In(zone).
							Format("2006-01-02T15:04:05"),
					},
				},
			)
			if err != nil {
				t.Fatal(err)
			}
		},
	)
	revision := func() int64 {
		t.Helper()
		p, err := admin.deps.Events.Read(t.Context(), r.owner.ID, r.eventID)
		if err != nil {
			t.Fatal(err)
		}
		return p.Revision
	}
	if _, err := admin.deps.Submissions.Read(
		t.Context(),
		r.owner.ID,
		r.eventID,
		r.teamID,
	); err != nil {
		t.Fatal(err)
	}
	scores := []scorecards.CategoryScore{}
	for _, category := range cp.Criteria.Categories {
		value := 6
		scores = append(
			scores,
			scorecards.CategoryScore{
				CategoryID: category.ID,
				Value:      &value,
				Evidence: []scorecards.EvidenceReference{
					{Kind: "note", Note: "Native independent final review evidence"},
				},
			},
		)
	}
	for _, person := range []dbgen.Person{r.mentor, second} {
		if _, err := admin.deps.Scorecards.Change(
			t.Context(),
			person.ID,
			r.eventID,
			r.teamID,
			revision(),
			scorecards.CardCommand{
				Action:            "submit",
				CheckpointID:      cp.Definition.ID,
				CheckpointVersion: cp.Version,
				Scores:            scores,
			},
		); err != nil {
			t.Fatal(err)
		}
	}
	huddleID := uuid.New()
	h, err := admin.deps.Moderation.Change(
		t.Context(),
		r.owner.ID,
		r.eventID,
		r.teamID,
		huddleID,
		revision(),
		moderation.Command{
			Action: "configure",
			Reason: "Actual intended independent panel",
			Definition: &moderation.Definition{
				ID:                huddleID,
				CheckpointID:      cp.Definition.ID,
				CheckpointVersion: cp.Version,
				ModeratorID:       r.owner.ID,
				SecondReviewerID:  second.ID,
				ReviewDueAt:       cp.ClosesAt.Add(-time.Minute),
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct {
		actor  uuid.UUID
		action string
	}{{second.ID, "complete_second_review"}, {r.owner.ID, "open"}, {r.owner.ID, "sign_off"}} {
		h, err = admin.deps.Moderation.Change(
			t.Context(),
			step.actor,
			r.eventID,
			r.teamID,
			huddleID,
			revision(),
			moderation.Command{
				Action:  step.action,
				Version: h.Version,
				Reason:  "Actual current signed final source",
			},
		)
		if err != nil {
			t.Fatal(err)
		}
	}
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
