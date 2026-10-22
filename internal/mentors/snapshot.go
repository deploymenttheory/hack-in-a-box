package mentors

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
)

type Service struct{ Pool *pgxpool.Pool }

func load(ctx context.Context, q *dbgen.Queries, row dbgen.Event) (Page, Snapshot, error) {
	page := Page{
		Revision:  row.Revision,
		Profiles:  []Profile{},
		Teams:     []Team{},
		Conflicts: []Conflict{},
		Draft:     []Assignment{},
		Published: []Assignment{},
		Gaps:      []Gap{},
	}
	var snapshot Snapshot
	configuration := event.DefaultConfiguration()
	if err := json.Unmarshal(row.Configuration, &configuration); err != nil {
		return page, snapshot, fmt.Errorf("read mentor dates: %w", err)
	}
	page.Live = Window{Start: configuration.LiveStart, End: configuration.LiveEnd}
	capacity, err := q.MentorDefaultCapacity(ctx, row.ID)
	if err != nil {
		return page, snapshot, fmt.Errorf("read mentor capacity: %w", err)
	}
	page.DefaultCapacity = int(capacity)
	profiles, err := q.MentorProfiles(ctx, row.ID)
	if err != nil {
		return page, snapshot, fmt.Errorf("read mentor profiles: %w", err)
	}
	for _, r := range profiles {
		p := Profile{
			PersonID:     r.PersonID,
			Name:         r.DisplayName,
			Active:       r.Active.Valid && r.Active.Bool,
			Skills:       r.Skills,
			Availability: []Window{},
		}
		if r.MaximumTeams.Valid {
			v := int(r.MaximumTeams.Int32)
			p.MaximumTeams = &v
		}
		if err = json.Unmarshal(r.Availability, &p.Availability); err != nil {
			return page, snapshot, fmt.Errorf("read mentor availability: %w", err)
		}
		page.Profiles = append(page.Profiles, p)
	}
	if err = loadTeams(ctx, q, row.ID, &page); err != nil {
		return page, snapshot, err
	}
	conflicts, err := q.MentorConflicts(ctx, row.ID)
	if err != nil {
		return page, snapshot, fmt.Errorf("read declared mentor conflicts: %w", err)
	}
	for _, c := range conflicts {
		page.Conflicts = append(
			page.Conflicts,
			Conflict{MentorID: c.MentorID, CompetitorID: c.CompetitorID, Kind: c.Kind},
		)
	}
	assignments, err := q.MentorAssignments(ctx, row.ID)
	if err != nil {
		return page, snapshot, fmt.Errorf("read mentor assignments: %w", err)
	}
	for _, a := range assignments {
		if !slices.ContainsFunc(page.Teams, func(t Team) bool { return t.ID == a.TeamID }) {
			continue
		}
		assignment := Assignment{
			TeamID:   a.TeamID,
			PersonID: a.PersonID,
			Slot:     a.Slot,
			Window:   Window{Start: a.StartsAt, End: a.EndsAt},
		}
		if a.Snapshot == "draft" {
			page.Draft = append(page.Draft, assignment)
		} else {
			page.Published = append(page.Published, assignment)
		}
	}
	snapshot = Snapshot{
		DefaultCapacity: page.DefaultCapacity,
		Live:            page.Live,
		Profiles:        page.Profiles,
		Teams:           page.Teams,
		Conflicts:       page.Conflicts,
	}
	page.Gaps = Validate(snapshot, page.Draft)
	page.PublishedGaps = Validate(snapshot, page.Published)
	return page, snapshot, nil
}

func loadTeams(ctx context.Context, q *dbgen.Queries, eventID uuid.UUID, page *Page) error {
	teams, err := q.HackTeams(ctx, eventID)
	if err != nil {
		return fmt.Errorf("read mentor teams: %w", err)
	}
	members, err := q.HackTeamMembers(ctx, eventID)
	if err != nil {
		return fmt.Errorf("read mentor team members: %w", err)
	}
	skills, err := q.MentorTeamSkills(ctx, eventID)
	if err != nil {
		return fmt.Errorf("read declared team skills: %w", err)
	}
	for _, r := range teams {
		if r.State == "disbanded" {
			continue
		}
		team := Team{
			ID:        r.ID,
			Name:      r.Name,
			Published: r.State == "published",
			Skills:    []string{},
			Members:   []uuid.UUID{},
		}
		for _, s := range skills {
			if s.TeamID == team.ID {
				team.Skills = s.Skills
			}
		}
		for _, m := range members {
			if m.TeamID == team.ID && !slices.Contains(team.Members, m.PersonID) {
				team.Members = append(team.Members, m.PersonID)
			}
		}
		page.Teams = append(page.Teams, team)
	}
	return nil
}
