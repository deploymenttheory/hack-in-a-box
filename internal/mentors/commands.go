package mentors

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

func hasTeam(snapshot Snapshot, id uuid.UUID) bool {
	return slices.ContainsFunc(snapshot.Teams, func(t Team) bool { return t.ID == id })
}

func hasMentor(snapshot Snapshot, id uuid.UUID) bool {
	return slices.ContainsFunc(
		snapshot.Profiles,
		func(p Profile) bool { return p.PersonID == id && p.Active },
	)
}

func cleanSkills(input []string) ([]string, error) {
	if len(input) > 100 {
		return nil, access.ErrInvalid
	}
	out := []string{}
	for _, s := range input {
		s = strings.ToLower(strings.TrimSpace(s))
		if len(s) > 200 {
			return nil, access.ErrInvalid
		}
		if s != "" {
			out = append(out, s)
		}
	}
	slices.Sort(out)
	return slices.Compact(out), nil
}

func configure(
	ctx context.Context,
	q *dbgen.Queries,
	eventID uuid.UUID,
	snapshot Snapshot,
	input Command,
) error {
	if input.Action == "settings" {
		if input.DefaultCapacity < 1 || input.DefaultCapacity > 10000 {
			return access.ErrInvalid
		}
		if err := q.SetMentorDefaultCapacity(
			ctx,
			dbgen.SetMentorDefaultCapacityParams{
				EventID:         eventID,
				DefaultCapacity: int32(input.DefaultCapacity),
			},
		); err != nil {
			return fmt.Errorf("save mentor capacity: %w", err)
		}
		return nil
	}
	if !hasMentor(snapshot, input.PersonID) || len(input.Availability) > 1000 {
		return access.ErrInvalid
	}
	maximum := pgtype.Int4{}
	if input.MaximumTeams != nil {
		if *input.MaximumTeams < 1 || *input.MaximumTeams > 10000 {
			return access.ErrInvalid
		}
		maximum = pgtype.Int4{Int32: int32(*input.MaximumTeams), Valid: true}
	}
	skills, err := cleanSkills(input.Skills)
	if err != nil {
		return err
	}
	for _, w := range input.Availability {
		if !validWindow(w) {
			return access.ErrInvalid
		}
	}
	windows := input.Availability
	if windows == nil {
		windows = []Window{}
	}
	data, err := json.Marshal(windows)
	if err != nil {
		return fmt.Errorf("encode mentor availability: %w", err)
	}
	if err = q.SetMentorProfile(
		ctx,
		dbgen.SetMentorProfileParams{
			EventID:      eventID,
			PersonID:     input.PersonID,
			Skills:       skills,
			Availability: data,
			MaximumTeams: maximum,
		},
	); err != nil {
		return fmt.Errorf("save mentor profile: %w", err)
	}
	return nil
}

func changeConflict(
	ctx context.Context,
	q *dbgen.Queries,
	eventID uuid.UUID,
	snapshot Snapshot,
	input Command,
) error {
	if !slices.Contains([]string{"reporting", "own_team"}, input.Kind) {
		return access.ErrInvalid
	}
	if !input.Enabled {
		stored, err := q.MentorConflicts(ctx, eventID)
		if err != nil {
			return fmt.Errorf("read conflict removal scope: %w", err)
		}
		if !slices.ContainsFunc(stored, func(c dbgen.MentorConflict) bool {
			return c.MentorID == input.PersonID && c.CompetitorID == input.CompetitorID &&
				c.Kind == input.Kind
		}) {
			return access.ErrInvalid
		}
		if err = q.DeleteMentorConflict(
			ctx,
			dbgen.DeleteMentorConflictParams{
				EventID:      eventID,
				MentorID:     input.PersonID,
				CompetitorID: input.CompetitorID,
				Kind:         input.Kind,
			},
		); err != nil {
			return fmt.Errorf("remove declared mentor conflict: %w", err)
		}
		return nil
	}
	if !hasMentor(snapshot, input.PersonID) {
		return access.ErrInvalid
	}
	people, err := q.ConfirmedTeamPeople(ctx, eventID)
	if err != nil {
		return fmt.Errorf("check conflict event scope: %w", err)
	}
	if !slices.ContainsFunc(
		people,
		func(p dbgen.ConfirmedTeamPeopleRow) bool { return p.ID == input.CompetitorID },
	) {
		return access.ErrInvalid
	}
	if err = q.SetMentorConflict(
		ctx,
		dbgen.SetMentorConflictParams{
			EventID:      eventID,
			MentorID:     input.PersonID,
			CompetitorID: input.CompetitorID,
			Kind:         input.Kind,
		},
	); err != nil {
		return fmt.Errorf("save declared mentor conflict: %w", err)
	}
	return nil
}

func assign(
	ctx context.Context,
	q *dbgen.Queries,
	eventID uuid.UUID,
	page Page,
	snapshot Snapshot,
	input Command,
) error {
	if !hasTeam(snapshot, input.TeamID) ||
		!slices.Contains([]string{"primary", "second"}, input.Slot) {
		return access.ErrInvalid
	}
	if input.PersonID == uuid.Nil {
		if err := q.DeleteMentorAssignment(
			ctx,
			dbgen.DeleteMentorAssignmentParams{
				EventID: eventID,
				TeamID:  input.TeamID,
				Slot:    input.Slot,
			},
		); err != nil {
			return fmt.Errorf("clear mentor draft slot: %w", err)
		}
		return nil
	}
	if !hasMentor(snapshot, input.PersonID) {
		return access.ErrInvalid
	}
	proposed := slices.DeleteFunc(
		slices.Clone(page.Draft),
		func(a Assignment) bool { return a.TeamID == input.TeamID && a.Slot == input.Slot },
	)
	a := Assignment{
		TeamID:   input.TeamID,
		PersonID: input.PersonID,
		Slot:     input.Slot,
		Window:   input.Window,
	}
	proposed = append(proposed, a)
	for _, gap := range Validate(snapshot, proposed) {
		relevant := (gap.TeamID != nil && *gap.TeamID == input.TeamID) ||
			(gap.PersonID != nil && *gap.PersonID == input.PersonID)
		if relevant && gap.Hard && gap.Code != "unpublished_team" {
			return ErrCoverage
		}
	}
	return saveAssignment(ctx, q, eventID, a)
}

func saveAssignment(ctx context.Context, q *dbgen.Queries, eventID uuid.UUID, a Assignment) error {
	if err := q.SetMentorAssignment(
		ctx,
		dbgen.SetMentorAssignmentParams{
			EventID:  eventID,
			TeamID:   a.TeamID,
			PersonID: a.PersonID,
			Slot:     a.Slot,
			StartsAt: a.Window.Start,
			EndsAt:   a.Window.End,
		},
	); err != nil {
		return fmt.Errorf("save draft mentor: %w", err)
	}
	return nil
}
