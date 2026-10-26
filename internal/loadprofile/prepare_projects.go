package loadprofile

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/outcomes"
)

// PreparedProject retains native restricted-source and separately reviewed
// publication acknowledgements. A team outcome is not an award or a frozen
// competition submission; those remain separate native source owners.
type PreparedProject struct {
	Source      outcomes.View            `json:"restricted_source"`
	Publication outcomes.PublicationView `json:"reviewed_publication"`
	Public      outcomes.Project         `json:"current_positive_project"`
}

// PublishTeamProject creates the source as its real member, assigns follow-up
// ownership and accepts it through the event organiser, then publishes ONLY
// positive fields in a separate two-step review. Native release/live/current
// team and organiser guards remain authoritative for every request.
func (p *NativePreparer) PublishTeamProject(
	ctx context.Context, organiser, member *Session, eventID, teamID, run uuid.UUID,
) (PreparedProject, error) {
	var prepared PreparedProject
	if organiser == nil || member == nil || eventID == uuid.Nil || teamID == uuid.Nil ||
		run == uuid.Nil {
		return prepared, fmt.Errorf(
			"%w: explicit native project source and actors required",
			errCohort,
		)
	}
	current, err := p.prepareTeamOutcome(ctx, organiser, member, eventID, teamID, run)
	if err != nil {
		return prepared, err
	}
	return p.publishPreparedProject(ctx, organiser, member, current, teamID)
}

func (p *NativePreparer) prepareTeamOutcome(
	ctx context.Context, organiser, member *Session, eventID, teamID, run uuid.UUID,
) (outcomes.View, error) {
	id := uuid.New()
	path := "/api/v1/outcomes/" + id.String()
	definition := outcomes.Definition{
		Kind: "team_outcome", Problem: "PRIVATE source problem " + run.String(),
		Audience: "Synthetic event team", ProposedValue: "Synthetic owned follow-through",
		Solution: "PRIVATE source solution " + run.String(), ThemeSensitive: true,
	}
	provenance := outcomes.Provenance{EventID: &eventID, TeamID: &teamID}
	zero := int64(0)
	current, err := prepareCall[outcomes.View](
		ctx,
		p,
		member,
		http.MethodPut,
		path,
		&zero,
		outcomes.Command{
			ID:         uuid.New(),
			Action:     "propose",
			Reason:     "Synthetic team authors its native output",
			Definition: &definition,
			Provenance: &provenance,
		},
	)
	if err != nil {
		return outcomes.View{}, err
	}
	if current.ID != id || current.Version.Version != 1 || current.AuthorID != member.Person() ||
		current.Provenance.EventID == nil ||
		*current.Provenance.EventID != eventID ||
		current.Provenance.TeamID == nil ||
		*current.Provenance.TeamID != teamID {
		return outcomes.View{}, fmt.Errorf(
			"%w: original project proposal changed exact author or source",
			errCohort,
		)
	}
	owner := member.Person()
	for _, command := range []outcomes.Command{
		{ID: uuid.New(), Action: "assign", OwnerID: &owner, Reason: "Reviewed native follow-through owner"},
		{ID: uuid.New(), Action: "moderate", Status: "accepted", Reason: "Accepted actual synthetic team output"},
	} {
		before := current.Version.Version
		current, err = prepareCall[outcomes.View](
			ctx,
			p,
			organiser,
			http.MethodPut,
			path,
			&before,
			command,
		)
		if err != nil {
			return outcomes.View{}, err
		}
		if current.ID != id || current.Version.Version != before+1 ||
			current.CommandID != command.ID ||
			current.ActorID != organiser.Person() {
			return outcomes.View{}, fmt.Errorf(
				"%w: original outcome command changed exact source/version/actor",
				errCohort,
			)
		}
	}
	if current.OwnerID == nil || *current.OwnerID != owner || current.Status != "accepted" {
		return outcomes.View{}, fmt.Errorf(
			"%w: original source ACK omitted accepted owner",
			errCohort,
		)
	}
	return current, nil
}

func (p *NativePreparer) publishPreparedProject(
	ctx context.Context, organiser, member *Session, current outcomes.View, teamID uuid.UUID,
) (PreparedProject, error) {
	prepared := PreparedProject{Source: current}
	id := current.ID
	path := "/api/v1/outcomes/" + id.String()
	var err error
	fields := outcomes.PublicationFields{
		Problem:        "Reviewed synthetic project " + teamID.String(),
		Solution:       "Reviewed demonstration of the team's synthetic service",
		Audience:       "organisation",
		ShareEventName: true,
		ShareTeamName:  true,
		Links: []outcomes.ProjectLink{
			{
				ID:    uuid.New(),
				Label: "Synthetic demonstration",
				URL:   "https://example.test/demonstrations/" + teamID.String(),
			},
		},
		Media: []outcomes.MediaReference{},
	}
	publicationPath := path + "/publication"
	revision := int64(0)
	for _, action := range []string{"save", "publish"} {
		command := outcomes.PublicationCommand{
			ID:            uuid.New(),
			Action:        action,
			SourceVersion: current.Version.Version,
			Reason:        "Reviewed exact positive synthetic project fields",
		}
		if action == "save" {
			command.Fields = &fields
		}
		publication, err := prepareCall[outcomes.PublicationView](
			ctx,
			p,
			organiser,
			http.MethodPut,
			publicationPath,
			&revision,
			command,
		)
		if err != nil {
			return prepared, err
		}
		if publication.OutcomeID != id || publication.Version != revision+1 ||
			publication.SourceVersion != current.Version.Version ||
			publication.CommandID != command.ID {
			return prepared, fmt.Errorf(
				"%w: original project publication changed exact source/review revision",
				errCohort,
			)
		}
		prepared.Publication = publication
		revision = publication.Version
	}
	if prepared.Publication.State != "published" || prepared.Publication.PublishedVersion == nil ||
		*prepared.Publication.PublishedVersion != revision {
		return prepared, fmt.Errorf(
			"%w: original publication ACK omitted current reviewed release",
			errCohort,
		)
	}
	prepared.Public, err = prepareCall[outcomes.Project](
		ctx,
		p,
		member,
		http.MethodGet,
		"/api/v1/projects/"+id.String(),
		nil,
		nil,
	)
	if err != nil {
		return prepared, err
	}
	public := prepared.Public
	if public.ID != id || public.Version != revision || public.Problem != fields.Problem ||
		public.Solution != fields.Solution ||
		public.SourceVersion != current.Version.Version ||
		len(public.Links) != 1 ||
		public.Links[0].URL != fields.Links[0].URL {
		return prepared, fmt.Errorf(
			"%w: current published project differs from positive reviewed fields",
			errCohort,
		)
	}
	return prepared, nil
}
