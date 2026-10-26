package loadprofile

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/teams"
)

// PrepareTeams allocates the ACTUAL confirmed people into 36 native teams of
// five. It never treats a planned roster as published/frozen state. The caller
// first advances the event through its real confirmed-attendance go gate.
func (p *NativePreparer) PrepareTeams(
	ctx context.Context,
	owner *Session,
	admission PreparedAdmission,
	actors []CohortActor,
	freeze bool,
) (teams.Page, error) {
	var page teams.Page
	var roster []uuid.UUID
	people := make(map[uuid.UUID]bool)
	for _, actor := range actors {
		if actor.Staff {
			continue
		}
		view, exists := admission.Registrations[actor.ID]
		if actor.Session == nil || !exists || view.State != "confirmed" ||
			view.PersonID != actor.Session.Person() || view.EventID != admission.Event.ID || people[view.PersonID] {
			return page, fmt.Errorf(
				"%w: allocation requires distinct actually confirmed native people",
				errCohort,
			)
		}
		people[view.PersonID] = true
		roster = append(roster, view.PersonID)
	}
	if len(roster) != 180 {
		return page, fmt.Errorf("%w: full180-person native roster required", errCohort)
	}
	for start := 0; start < len(roster); start += 5 {
		captain := roster[start]
		var err error
		page, err = p.teamCommand(ctx, owner, admission.Event.ID, teams.Command{
			Action:   "create",
			PersonID: captain,
			Name:     fmt.Sprintf("Synthetic load team %02d", start/5+1),
		})
		if err != nil {
			return page, err
		}
		id := uuid.Nil
		for _, team := range page.Teams {
			if team.Captain != nil && *team.Captain == captain {
				if id != uuid.Nil {
					return page, fmt.Errorf(
						"%w: captain appears in multiple native teams",
						errCohort,
					)
				}
				id = team.ID
			}
		}
		if id == uuid.Nil {
			return page, fmt.Errorf(
				"%w: native team creation ACK omitted its exact captain",
				errCohort,
			)
		}
		for _, person := range roster[start+1 : start+5] {
			page, err = p.teamCommand(
				ctx,
				owner,
				admission.Event.ID,
				teams.Command{Action: "move", TeamID: id, PersonID: person},
			)
			if err != nil {
				return page, err
			}
		}
	}
	page, err := p.teamCommand(ctx, owner, admission.Event.ID, teams.Command{Action: "publish"})
	if err != nil {
		return page, err
	}
	if freeze {
		page, err = p.teamCommand(ctx, owner, admission.Event.ID, teams.Command{Action: "freeze"})
		if err != nil {
			return page, err
		}
	}
	return page, validatePreparedTeams(page, people, freeze)
}

func (p *NativePreparer) teamCommand(
	ctx context.Context,
	owner *Session,
	event uuid.UUID,
	command teams.Command,
) (teams.Page, error) {
	path := "/api/v1/events/" + event.String() + "/teams"
	current, err := prepareCall[teams.Page](ctx, p, owner, http.MethodGet, path, nil, nil)
	if err != nil {
		return current, err
	}
	command.Reason = "Synthetic load fixture reviewed allocation"
	next, err := prepareCall[teams.Page](
		ctx,
		p,
		owner,
		http.MethodPost,
		path,
		&current.Revision,
		command,
	)
	if err == nil && next.Revision <= current.Revision {
		err = fmt.Errorf("%w: native team command ACK did not advance revision", errCohort)
	}
	return next, err
}

func validatePreparedTeams(page teams.Page, expected map[uuid.UUID]bool, freeze bool) error {
	if len(page.Teams) != 36 || (freeze && !page.Frozen) {
		return fmt.Errorf("%w: complete36-team native publication/freeze required", errCohort)
	}
	seen := make(map[uuid.UUID]bool, len(expected))
	for _, team := range page.Teams {
		if team.State == "disbanded" || len(team.Published) != 5 || (freeze && !team.Frozen) {
			return fmt.Errorf("%w: native team is disbanded, notfive people or unfrozen", errCohort)
		}
		for _, member := range team.Published {
			if !expected[member.ID] || seen[member.ID] {
				return fmt.Errorf(
					"%w: published team membership missing, extra or duplicated",
					errCohort,
				)
			}
			seen[member.ID] = true
		}
	}
	if len(seen) != len(expected) {
		return fmt.Errorf("%w: published native teams omit confirmed people", errCohort)
	}
	return nil
}
