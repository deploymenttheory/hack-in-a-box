package mentors

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
)

func mentorFixture() Snapshot {
	start := time.Date(2027, 6, 20, 9, 0, 0, 0, time.UTC)
	live := Window{Start: start, End: start.Add(8 * time.Hour)}
	return Snapshot{DefaultCapacity: 2, Live: live, Profiles: []Profile{
		{
			PersonID:     uuid.MustParse("00000000-0000-4000-8000-000000000001"),
			Active:       true,
			Skills:       []string{"Design"},
			Availability: []Window{live},
		},
		{
			PersonID:     uuid.MustParse("00000000-0000-4000-8000-000000000002"),
			Active:       true,
			Skills:       []string{"API"},
			Availability: []Window{live},
		},
	}, Teams: []Team{
		{
			ID:        uuid.MustParse("00000000-0000-4000-8000-000000000011"),
			Published: true,
			Skills:    []string{"design"},
			Members:   []uuid.UUID{uuid.New()},
		},
		{
			ID:        uuid.MustParse("00000000-0000-4000-8000-000000000012"),
			Published: true,
			Skills:    []string{"api"},
			Members:   []uuid.UUID{uuid.New()},
		},
	}}
}

func hasGap(gaps []Gap, code string) bool {
	return slices.ContainsFunc(gaps, func(g Gap) bool { return g.Code == code })
}

func TestMentorPlanDeterminismCapacityAndDeclaredSkills(t *testing.T) {
	fixture := mentorFixture()
	out := Suggest(fixture)
	if len(out.Gaps) != 0 || len(out.Assignments) != 4 {
		t.Fatalf("full roster not covered: %+v", out)
	}
	if out.Assignments[0].PersonID != fixture.Profiles[0].PersonID ||
		out.Assignments[1].PersonID != fixture.Profiles[1].PersonID {
		t.Fatalf("declared skills ignored: %+v", out)
	}
	slices.Reverse(fixture.Teams)
	slices.Reverse(fixture.Profiles)
	if !reflect.DeepEqual(out, Suggest(fixture)) {
		t.Fatal("input order changed proposal")
	}
	fixture.DefaultCapacity = 1
	out = Suggest(fixture)
	primaries := 0
	for _, a := range out.Assignments {
		if a.Slot == "primary" {
			primaries++
		}
	}
	if primaries != 2 || hasGap(out.Gaps, "mentor_capacity") ||
		!hasGap(out.Gaps, "missing_second") {
		t.Fatalf("primary coverage not prioritised within capacity: %+v", out)
	}
}

func TestMentorPlanAvailabilityAndConflicts(t *testing.T) {
	fixture := mentorFixture()
	// Adjacent availability is continuous; separated availability never bridges a gap.
	mid := fixture.Live.Start.Add(4 * time.Hour)
	fixture.Profiles[0].Availability = []Window{
		{Start: fixture.Live.Start, End: mid},
		{Start: mid, End: fixture.Live.End},
	}
	if !available(fixture.Profiles[0].Availability, fixture.Live) {
		t.Fatal("adjacent windows rejected")
	}
	fixture.Profiles[0].Availability[1].Start = mid.Add(time.Hour)
	if available(fixture.Profiles[0].Availability, fixture.Live) {
		t.Fatal("unavailable hour treated as covered")
	}
	fixture.Profiles[1].Availability = []Window{}
	out := Suggest(fixture)
	if len(out.Assignments) != 2 || !hasGap(out.Gaps, "partial_primary") ||
		!hasGap(out.Gaps, "missing_second") {
		t.Fatalf("partial availability not explained: %+v", out)
	}
	fixture = mentorFixture()
	fixture.Conflicts = []Conflict{
		{
			MentorID:     fixture.Profiles[0].PersonID,
			CompetitorID: fixture.Teams[0].Members[0],
			Kind:         "reporting",
		},
	}
	out = Suggest(fixture)
	for _, a := range out.Assignments {
		if a.TeamID == fixture.Teams[0].ID && a.PersonID == fixture.Profiles[0].PersonID {
			t.Fatal("declared direct report assigned")
		}
	}
	fixture.Conflicts = nil
	fixture.Teams[0].Members = append(fixture.Teams[0].Members, fixture.Profiles[0].PersonID)
	out = Suggest(fixture)
	for _, a := range out.Assignments {
		if a.TeamID == fixture.Teams[0].ID && a.PersonID == fixture.Profiles[0].PersonID {
			t.Fatal("own-team reviewer assigned")
		}
	}
}

func TestMentorValidationRejectsInvalidManualEdits(t *testing.T) {
	fixture := mentorFixture()
	assignments := []Assignment{
		{
			TeamID:   fixture.Teams[0].ID,
			PersonID: fixture.Profiles[0].PersonID,
			Slot:     "primary",
			Window:   fixture.Live,
		},
		{
			TeamID:   fixture.Teams[0].ID,
			PersonID: fixture.Profiles[0].PersonID,
			Slot:     "second",
			Window:   fixture.Live,
		},
	}
	if !hasGap(Validate(fixture, assignments), "distinct_reviewer") {
		t.Fatal("same primary and second allowed")
	}
	assignments[1].Slot = "primary"
	if !hasGap(Validate(fixture, assignments), "duplicate_slot") {
		t.Fatal("duplicate primary allowed")
	}
	fixture.Profiles[0].Active = false
	if !hasGap(Validate(fixture, assignments), "inactive_assignment") {
		t.Fatal("inactive mentor allowed")
	}
	fixture = mentorFixture()
	assignments[0].Window.End = fixture.Live.End.Add(time.Hour)
	if !hasGap(Validate(fixture, assignments[:1]), "unavailable") {
		t.Fatal("out-of-window assignment accepted")
	}
	fixture.Teams[0].Published = false
	if !hasGap(Validate(fixture, nil), "unpublished_team") {
		t.Fatal("draft competitor team publishable")
	}
}
