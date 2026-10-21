package templates

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
)

type Starter struct {
	Key           string `json:"key"`
	Name          string `json:"name"`
	Configuration Bundle `json:"configuration"`
}

func (s Service) Starters(ctx context.Context, actor uuid.UUID) ([]Starter, error) {
	allowed, err := dbgen.New(s.Pool).CanCreateEvent(ctx, actor)
	if err != nil {
		return nil, fmt.Errorf("authorize starter templates: %w", err)
	}
	if !allowed {
		return nil, access.ErrForbidden
	}
	return []Starter{starter(false), starter(true)}, nil
}

// Starters contain process definitions only. Runtime judging belongs to #107.
func starter(qualification bool) Starter {
	key, name := "all-team", "All-team judging"
	if qualification {
		key, name = "qualification-final", "Mentor qualification and final"
	}
	id := func(label string) uuid.UUID {
		return uuid.NewSHA1(uuid.NameSpaceURL, []byte("hackbox/starter/"+key+"/"+label))
	}
	c := event.DefaultConfiguration()
	c.Description = name + ". Target five competitors per team, with four to six preferred. One competition entry per team."
	c.LiveStart = time.Date(2027, 6, 20, 9, 0, 0, 0, time.UTC)
	c.LiveEnd = c.LiveStart.Add(8 * time.Hour)
	c.SubmissionCutoff = c.LiveStart.Add(5 * time.Hour)
	c.TeamFreeze = c.LiveStart.AddDate(0, 0, -8)
	c.AdmissionCutoff = c.TeamFreeze.Add(-time.Hour)
	item := func(label, title, anchor string, minutes int, audience string) event.AgendaItem {
		days := 0
		switch anchor {
		case "admission_cutoff":
			anchor, days, minutes = "live_start", -8, -60
		case "team_freeze":
			anchor, days = "live_start", -8
		case "challenge_publication":
			anchor, days = "live_start", -7
		case "live_end":
			anchor, minutes = "live_start", minutes+480
		}
		return event.AgendaItem{
			Days:     days,
			ID:       id(label),
			Title:    title,
			Audience: []string{audience},
			Anchor:   anchor,
			Minutes:  minutes,
		}
	}
	c.Schedule = []event.AgendaItem{
		item("admission", "Final admission", "admission_cutoff", 0, "organiser"),
		item("freeze", "Confirm and freeze teams", "team_freeze", 0, "organiser"),
		item("challenge", "Common challenge publication", "challenge_publication", 0, "attendee"),
		item(
			"welcome",
			"Live hack starts; submissions and mentor reviews open",
			"live_start",
			0,
			"all",
		),
		item("submission", "Final submission cutoff", "submission_cutoff", 0, "attendee"),
		item("judging", "Independent final judging", "submission_cutoff", 30, "judge"),
		item("awards", "Approve and publish awards", "live_end", -30, "organiser"),
	}
	if qualification {
		c.Schedule[5].Minutes = 90
		c.Schedule = append(
			c.Schedule,
			item(
				"moderation",
				"Moderate final mentor cards; select three finalists and a reserve",
				"submission_cutoff",
				15,
				"organiser",
			),
		)
	}
	d := EmptyDefinitions()
	d.Forms = []Form{
		{
			ID:       id("attendee-form"),
			Name:     "Attendee registration",
			Audience: "attendee",
			Fields: []Field{
				{
					ID:      id("skills"),
					Label:   "Skills you would like to contribute",
					Type:    "multiple",
					Choices: []string{"Development", "Design", "Research", "Presentation"},
				},
				{
					ID:      id("experience"),
					Label:   "AI-development experience",
					Type:    "single",
					Choices: []string{"Starting out", "Some experience", "Experienced"},
				},
				{
					ID:       id("preference"),
					Label:    "Team preference",
					Type:     "single",
					Required: true,
					Choices:  []string{"Captain", "Join a team", "Solo placement"},
				},
			},
		},
		{
			ID:       id("mentor-form"),
			Name:     "Mentor application",
			Audience: "mentor",
			Fields: []Field{
				{
					ID:       id("mentor-skills"),
					Label:    "Skills and mentoring experience",
					Type:     "text",
					Required: true,
					Choices:  []string{},
				},
			},
		},
	}
	d.Tasks = []Task{
		{
			ID:        id("task-places"),
			Title:     "Review confirmed attendance and make the explicit go decision",
			OwnerRole: "organiser",
			DependsOn: []uuid.UUID{},
		},
		{
			ID:         id("task-teams"),
			Title:      "Assign every confirmed attendee; resolve teams below three before freeze",
			OwnerRole:  "organiser",
			DependsOn:  []uuid.UUID{id("task-places")},
			ScheduleID: new(id("freeze")),
		},
		{
			ID:         id("task-challenge"),
			Title:      "Approve the challenge for common publication seven calendar days before live start",
			OwnerRole:  "organiser",
			DependsOn:  []uuid.UUID{id("task-teams")},
			ScheduleID: new(id("challenge")),
		},
		{
			ID:        id("task-venue"),
			Title:     "Check venue, connectivity and accessible facilities",
			OwnerRole: "logistics",
			DependsOn: []uuid.UUID{},
		},
		{
			ID:         id("task-judging"),
			Title:      "Confirm judge coverage and resolve declared conflicts",
			OwnerRole:  "organiser",
			DependsOn:  []uuid.UUID{},
			ScheduleID: new(id("judging")),
		},
	}
	d.Deliverables = []Deliverable{
		{
			ID:          id("demo"),
			Title:       "Final competition entry",
			Description: "One final entry per team, including demonstration and supporting evidence. Formal submission opens at live start and freezes at the shared cutoff.",
			Required:    true,
			ScheduleID:  new(id("submission")),
		},
	}
	d.RoleBriefs = []RoleBrief{
		{
			ID:   id("brief-attendee"),
			Role: "attendee",
			Body: "Register individually. Target five competitors per team, with four to six preferred. Final admission closes before team freeze. After freeze no team moves are allowed. The challenge is published seven calendar days before live start at the same event-local time; you may begin building then. Formal submissions and mentor reviews open at live start. After freeze, withdrawals leave a smaller team that may continue or withdraw.",
		},
		{
			ID:   id("brief-organiser"),
			Role: "organiser",
			Body: "Only confirmed competitors count toward the minimum. Make an explicit go decision; do not automatically cancel after withdrawals. Reassign members of teams below three before freeze. Confirm and freeze every team before challenge publication. Missed publication requires postponement preserving the preparation week or cancellation. Publish awards explicitly after sign-off.",
		},
		{
			ID:   id("brief-mentor"),
			Role: "mentor",
			Body: "Mentor reviews and provisional checkpoint scores begin at live start. Record evidence and conflicts. Checkpoint scores do not accumulate into final totals.",
		},
		{
			ID:   id("brief-judge"),
			Role: "judge",
			Body: "Judge frozen final submissions after the cutoff, independently. Score demo, technical approach, UX, feature fit and innovation from 1 to 10, equally weighted. Resolve missing evidence and ties explicitly before publication.",
		},
		{
			ID:   id("brief-mc"),
			Role: "mc",
			Body: "Use the approved run sheet. Announce only deliberately published teams, finalists and awards.",
		},
		{
			ID:   id("brief-logistics"),
			Role: "logistics",
			Body: "Use authorised attendance and logistics fields to prepare the venue, catering and accessibility arrangements.",
		},
	}
	if qualification {
		d.RoleBriefs[2].Body += " Submit final mentor cards for moderation after the cutoff; qualification uses moderated final cards, not checkpoint totals."
		d.RoleBriefs[3].Body += " The final is a fresh assessment of three finalists, with a reserve by default. Judges do not receive mentor scores or rankings. All eligible teams remain eligible for category awards."
	}
	criteria := make([]Criterion, 0, 5)
	for _, label := range []string{"Demo", "Technical approach", "UX", "Feature fit", "Innovation"} {
		criteria = append(
			criteria,
			Criterion{
				ID:       id("criterion-" + label),
				Label:    label,
				Guidance: "Use recorded evidence against the challenge and explain the score.",
				MaxScore: 10,
				Weight:   1,
			},
		)
	}
	d.Rubrics = []Rubric{{ID: id("rubric"), Name: name, Criteria: criteria}}
	d.Messages = []Message{
		{
			ID:         id("message-challenge"),
			Name:       "Challenge available",
			Subject:    "Your challenge is ready",
			Body:       "Your team's challenge is now available in the event workspace. You may begin building. Formal submissions and mentor reviews open at live start.",
			Audience:   []string{"attendee"},
			ScheduleID: new(id("challenge")),
		},
	}
	return Starter{Key: key, Name: name, Configuration: Bundle{Event: c, Definitions: d}}
}
