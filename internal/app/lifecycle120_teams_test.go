package app

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/registration"
	"github.com/deploymenttheory/hack-in-a-box/internal/teams"
)

func allocateLifecycleTeams(
	ctx context.Context,
	owner *lifecycleClient,
	eventID uuid.UUID,
	cohort *lifecycleAdmissions,
	control *lifecycleControl,
	reviewPublication func() error,
) (teams.Page, error) {
	return allocateLifecycleTeamsWithInitialPublication(
		ctx,
		owner,
		eventID,
		cohort,
		control,
		nil,
		reviewPublication,
	)
}

func allocateLifecycleTeamsWithInitialPublication(
	ctx context.Context,
	owner *lifecycleClient,
	eventID uuid.UUID,
	cohort *lifecycleAdmissions,
	control *lifecycleControl,
	initialPublication func(teams.Page) error,
	reviewPublication func() error,
) (teams.Page, error) {
	base := "/api/v1/events/" + eventID.String()
	var roster []int
	for i, applied := range cohort.registrations {
		if applied.State == "confirmed" {
			roster = append(roster, i)
		}
	}
	if len(roster) != 35 {
		return teams.Page{}, fmt.Errorf(
			"target-five allocation requires the actual confirmed cohort",
		)
	}
	var page teams.Page
	var groups []uuid.UUID
	for start := 0; start < len(roster); start += 5 {
		captain := cohort.clients[roster[start]].person
		var err error
		page, err = lifecycleTeamCommand(
			ctx,
			owner,
			eventID,
			teams.Command{
				Action:   "create",
				PersonID: captain,
				Name:     fmt.Sprintf("Synthetic cohort %d", start/5+1),
			},
			http.StatusOK,
		)
		if err != nil {
			return page, err
		}
		id := uuid.Nil
		for _, team := range page.Teams {
			if team.Captain != nil && *team.Captain == captain {
				id = team.ID
			}
		}
		if id == uuid.Nil {
			return page, fmt.Errorf("native team creation did not establish its captain")
		}
		groups = append(groups, id)
		for _, index := range roster[start+1 : start+5] {
			page, err = lifecycleTeamCommand(
				ctx,
				owner,
				eventID,
				teams.Command{Action: "move", TeamID: id, PersonID: cohort.clients[index].person},
				http.StatusOK,
			)
			if err != nil {
				return page, err
			}
		}
	}
	var err error
	page, err = lifecycleTeamCommand(
		ctx,
		owner,
		eventID,
		teams.Command{Action: "publish"},
		http.StatusOK,
	)
	if err != nil {
		return page, err
	}
	for _, team := range page.Teams {
		if len(team.Published) != 5 || len(team.Draft) != 5 {
			return page, fmt.Errorf("reviewed initial allocation did not publish teams of five")
		}
	}
	if initialPublication != nil {
		if err := initialPublication(page); err != nil {
			return page, err
		}
	}
	// The extra reserve's offer is issued within its normal two-working-day
	// policy before the real cutoff; neither policy nor deadline is stretched.
	if cohort.reserveID != uuid.Nil {
		var current event.View
		if err := owner.call(
			ctx,
			http.MethodGet,
			base,
			-1,
			nil,
			&current,
			http.StatusOK,
		); err != nil {
			return page, err
		}
		var phase struct {
			Now time.Time `json:"now"`
		}
		if err := control.call(ctx, http.MethodGet, "/clock", nil, &phase); err != nil {
			return page, err
		}
		target := current.Configuration.AdmissionCutoff.Add(-24 * time.Hour)
		if phase.Now.Before(target) {
			if err := control.advance(ctx, target); err != nil {
				return page, err
			}
		} else if !phase.Now.Before(current.Configuration.AdmissionCutoff) {
			return page, fmt.Errorf("reserve offer cannot be created after the admission cutoff")
		}
	}
	// Withdraw the captain and two members through the registration owner. The
	// remaining pair must be deliberately reassigned before the hard freeze.
	for _, index := range roster[:3] {
		applied := cohort.registrations[index]
		var withdrawn registration.View
		if err := cohort.clients[index].call(
			ctx,
			http.MethodPost,
			base+"/registrations/"+applied.ID.String()+"/respond",
			applied.Revision,
			map[string]string{"action": "withdraw"},
			&withdrawn,
			http.StatusOK,
		); err != nil {
			return page, err
		}
		if withdrawn.State != "withdrawn" {
			return page, fmt.Errorf("native team depletion did not withdraw the competitor")
		}
		cohort.registrations[index] = withdrawn
	}
	if err := owner.call(
		ctx,
		http.MethodGet,
		base+"/teams",
		-1,
		nil,
		&page,
		http.StatusOK,
	); err != nil {
		return page, err
	}
	var depleted *teams.Team
	for i := range page.Teams {
		if page.Teams[i].ID == groups[0] {
			depleted = &page.Teams[i]
		}
	}
	if depleted == nil || len(depleted.Draft) != 2 || len(depleted.Published) != 2 {
		return page, fmt.Errorf("native withdrawal did not leave the actual remaining pair")
	}
	if _, err := lifecycleTeamCommand(
		ctx,
		owner,
		eventID,
		teams.Command{Action: "publish"},
		http.StatusConflict,
	); err != nil {
		return page, err
	}
	for _, id := range groups {
		page, err = lifecycleTeamCommand(
			ctx,
			owner,
			eventID,
			teams.Command{Action: "unlock", TeamID: id},
			http.StatusOK,
		)
		if err != nil {
			return page, err
		}
	}
	for i, index := range roster[3:5] {
		page, err = lifecycleTeamCommand(
			ctx,
			owner,
			eventID,
			teams.Command{
				Action:   "move",
				TeamID:   groups[i+1],
				PersonID: cohort.clients[index].person,
			},
			http.StatusOK,
		)
		if err != nil {
			return page, err
		}
		for _, team := range page.Teams {
			if team.ID == groups[0] && len(team.Published) != 2 {
				return page, fmt.Errorf("draft reassignment silently changed the published pair")
			}
		}
	}
	// The corrected publication follows the first one. Keeping both at the
	// same frozen instant makes the native due-time/UUID ordering arbitrary.
	if err := control.nextAction(ctx); err != nil {
		return page, err
	}
	if reviewPublication != nil {
		if err := reviewPublication(); err != nil {
			return page, err
		}
		// Approval must precede the new publication, including in domain time.
		if err := control.nextAction(ctx); err != nil {
			return page, err
		}
	}
	page, err = lifecycleTeamCommand(
		ctx,
		owner,
		eventID,
		teams.Command{Action: "publish"},
		http.StatusOK,
	)
	if err != nil {
		return page, err
	}
	active, assigned, disbanded := 0, 0, false
	for _, team := range page.Teams {
		if team.ID == groups[0] {
			disbanded = team.State == "disbanded" && len(team.Published) == 0
		}
		if team.State == "disbanded" {
			continue
		}
		active++
		assigned += len(team.Published)
		if len(team.Published) < 4 || len(team.Published) > 6 {
			return page, fmt.Errorf("reviewed reassignment violated the four-to-six preference")
		}
	}
	if active != 6 || assigned != 32 || !disbanded {
		return page, fmt.Errorf(
			"remaining pair was not reassigned into the actual confirmed cohort",
		)
	}
	if cohort.reserveID != uuid.Nil {
		// A pending offer must reach the real admission cutoff before this
		// deliberate freeze; freezing earlier would expire it for another reason.
		return page, nil
	}
	page, err = lifecycleTeamCommand(
		ctx,
		owner,
		eventID,
		teams.Command{Action: "freeze"},
		http.StatusOK,
	)
	if err != nil {
		return page, err
	}
	if !page.Frozen {
		return page, fmt.Errorf("native team freeze did not commit")
	}
	for _, team := range page.Teams {
		if team.State != "disbanded" && !team.Frozen {
			return page, fmt.Errorf("native freeze left an active team mutable")
		}
	}
	if _, err := lifecycleTeamCommand(
		ctx,
		owner,
		eventID,
		teams.Command{
			Action:   "move",
			TeamID:   groups[2],
			PersonID: cohort.clients[roster[3]].person,
		},
		http.StatusConflict,
	); err != nil {
		return page, err
	}
	return page, nil
}

func lifecycleTeamCommand(
	ctx context.Context,
	owner *lifecycleClient,
	eventID uuid.UUID,
	command teams.Command,
	want int,
) (teams.Page, error) {
	path := "/api/v1/events/" + eventID.String() + "/teams"
	var current teams.Page
	if err := owner.call(ctx, http.MethodGet, path, -1, nil, &current, http.StatusOK); err != nil {
		return current, err
	}
	command.Reason = "Synthetic lifecycle reviewed allocation"
	var next teams.Page
	if want == http.StatusConflict {
		var rejected struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := owner.call(
			ctx,
			http.MethodPost,
			path,
			current.Revision,
			command,
			&rejected,
			want,
		); err != nil {
			return next, err
		}
		code := "teams_unready"
		if current.Frozen {
			code = "teams_frozen"
		}
		if rejected.Error.Code != code {
			return next, fmt.Errorf(
				"native team rejection was %s, expected %s",
				rejected.Error.Code,
				code,
			)
		}
		if err := owner.call(ctx, http.MethodGet, path, -1, nil, &next, http.StatusOK); err != nil {
			return next, err
		}
		if !reflect.DeepEqual(current, next) {
			return next, fmt.Errorf("rejected team action changed the native allocation")
		}
		return next, nil
	}
	err := owner.call(ctx, http.MethodPost, path, current.Revision, command, &next, want)
	return next, err
}
