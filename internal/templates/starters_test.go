package templates

import (
	"strings"
	"testing"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/schedule"
)

func TestStartersPreserveAgreedProcess(t *testing.T) {
	for _, qualification := range []bool{false, true} {
		s := starter(qualification)
		t.Run(s.Key, func(t *testing.T) {
			b := s.Configuration
			if err := Validate(b); err != nil {
				t.Fatal(err)
			}
			if issues := event.Validate(
				event.Input{Name: s.Name, Slug: s.Key, Configuration: b.Event},
				true,
			); len(
				issues,
			) > 0 {
				t.Fatal(issues)
			}
			publication, err := schedule.ChallengePublication(
				b.Event.LiveStart,
				b.Event.Timezone,
				b.Event.DSTChoice,
			)
			if err != nil {
				t.Fatal(err)
			}
			if !b.Event.TeamFreeze.Before(publication) ||
				b.Event.AdmissionCutoff.After(b.Event.TeamFreeze) {
				t.Fatal("invalid admission/freeze order")
			}
			if qualification &&
				(b.Definitions.Rubrics[0].Mode != "mentor_qualification" || b.Definitions.Rubrics[0].FinalistCount != 3 || b.Definitions.Rubrics[0].ReserveCount != 1) {
				t.Fatal("qualification defaults missing")
			}
			if !qualification && b.Definitions.Rubrics[0].Mode != "all_teams" {
				t.Fatal("all-team defaults missing")
			}
			if len(b.Definitions.Rubrics[0].Criteria) != 5 {
				t.Fatal("missing default criteria")
			}
			for _, c := range b.Definitions.Rubrics[0].Criteria {
				if c.MinScore != 1 || c.MaxScore != 10 || c.Weight != 1 || len(c.Anchors) != 3 {
					t.Fatal("unbalanced scoring defaults")
				}
			}
			for _, item := range b.Event.Schedule {
				when, err := event.Plan(b.Event, item)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(item.Title, "Common challenge") && !when.Equal(publication) {
					t.Fatal("publication agenda differs from gate")
				}
				if strings.Contains(item.Title, "judging") &&
					when.Before(b.Event.SubmissionCutoff) {
					t.Fatal("judging before cutoff")
				}
			}
			for _, start := range []time.Time{time.Date(2027, 3, 29, 9, 0, 0, 0, time.UTC), time.Date(2027, 10, 31, 9, 0, 0, 0, time.UTC)} {
				input := cloneInput()
				input.LiveStart = start
				cloned, err := Remap(b, input)
				if err != nil {
					t.Fatal(err)
				}
				pub, err := schedule.ChallengePublication(start, cloned.Event.Timezone, "")
				if err != nil {
					t.Fatal(err)
				}
				if !cloned.Event.TeamFreeze.Before(pub) {
					t.Fatal("clone freeze moved past publication")
				}
			}
			if qualification &&
				!strings.Contains(
					b.Definitions.RoleBriefs[3].Body,
					"do not receive mentor scores",
				) {
				t.Fatal("finalist fairness guidance missing")
			}
		})
	}
}
