package loadprofile

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/mentors"
	"github.com/deploymenttheory/hack-in-a-box/internal/teams"
)

// PrepareMentors obtains real current-role profiles and publishes complete
// independent primary/second coverage. Each of eight mentors covers nine of
// the36 teams; none receives both slots on the same team. Runtime observation
// writes still read native current authority/version before every command.
func (p *NativePreparer) PrepareMentors(ctx context.Context, owner *Session, planned FactoryEvent,
	actors []CohortActor, published teams.Page,
) (mentors.Page, error) {
	path := "/api/v1/events/" + planned.Input.ID.String() + "/mentor-coverage"
	page, err := prepareCall[mentors.Page](ctx, p, owner, http.MethodGet, path, nil, nil)
	if err != nil {
		return page, err
	}
	var people []uuid.UUID
	for _, staff := range planned.Staff {
		if staff.Role != "mentor" {
			continue
		}
		for _, actor := range actors {
			if actor.ID == staff.Subject && actor.Staff && actor.Event == planned.Input.ID &&
				actor.Session != nil {
				people = append(people, actor.Session.Person())
			}
		}
	}
	if len(people) != 8 || len(published.Teams) != 36 {
		return page, fmt.Errorf(
			"%w: eight actual mentors and36 published teams required",
			errCohort,
		)
	}
	window := mentors.Window{
		Start: planned.Input.Configuration.LiveStart,
		End:   planned.Input.Configuration.LiveEnd,
	}
	for _, person := range people {
		page, err = prepareCall[mentors.Page](ctx, p, owner, http.MethodPost, path, &page.Revision,
			map[string]any{
				"action":        "profile",
				"reason":        "Actual selected load-fixture mentor availability",
				"person_id":     person,
				"skills":        []string{"design"},
				"availability":  []mentors.Window{window},
				"maximum_teams": 12,
			})
		if err != nil {
			return page, err
		}
	}
	for index, team := range published.Teams {
		for _, slot := range []struct {
			name   string
			person uuid.UUID
		}{
			{"primary", people[index%8]}, {"second", people[(index+4)%8]},
		} {
			page, err = prepareCall[mentors.Page](
				ctx,
				p,
				owner,
				http.MethodPost,
				path,
				&page.Revision,
				map[string]any{
					"action":    "assign",
					"reason":    "Distinct current native primary and second mentor",
					"team_id":   team.ID,
					"person_id": slot.person,
					"slot":      slot.name,
					"window":    window,
				},
			)
			if err != nil {
				return page, err
			}
		}
	}
	page, err = prepareCall[mentors.Page](
		ctx,
		p,
		owner,
		http.MethodPost,
		path,
		&page.Revision,
		map[string]string{
			"action": "publish",
			"reason": "Reviewed complete native mentor coverage",
		},
	)
	if err == nil && (len(page.Published) != 72 || len(page.PublishedGaps) != 0) {
		err = fmt.Errorf("%w: actual native mentor coverage is incomplete", errCohort)
	}
	return page, err
}
