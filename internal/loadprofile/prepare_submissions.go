package loadprofile

import (
	"context"
	"fmt"
	"net/http"
	"slices"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
	"github.com/deploymenttheory/hack-in-a-box/internal/templates"
)

// ConfigureURLDeliverable authors a real pre-publication requirement while
// preserving every other configuration owner. URL-only formal entries use the
// native capture worker's not_applicable repository/SHA result; a URL never
// becomes a fabricated repository commit receipt.
func (p *NativePreparer) ConfigureURLDeliverable(
	ctx context.Context,
	owner *Session,
	id uuid.UUID,
) (uuid.UUID, error) {
	path := "/api/v1/events/" + id.String()
	current, err := prepareCall[event.View](ctx, p, owner, http.MethodGet, path, nil, nil)
	if err != nil {
		return uuid.Nil, err
	}
	snapshot, err := prepareCall[templates.Bundle](
		ctx,
		p,
		owner,
		http.MethodGet,
		path+"/configuration-snapshot",
		nil,
		nil,
	)
	if err != nil {
		return uuid.Nil, err
	}
	deliverable := templates.Deliverable{
		ID:            uuid.New(),
		Title:         "Synthetic formal demonstration",
		Description:   "Provide the synthetic team's URL evidence during the real live submission window.",
		Required:      true,
		EvidenceKinds: []string{"url"},
		Audience:      []string{"organiser", "mentor", "judge"},
	}
	snapshot.Definitions.Deliverables = append(snapshot.Definitions.Deliverables, deliverable)
	next, err := prepareCall[templates.Bundle](
		ctx,
		p,
		owner,
		http.MethodPut,
		path+"/configuration-snapshot",
		&current.Revision,
		snapshot.Definitions,
	)
	if err != nil {
		return uuid.Nil, err
	}
	if !slices.ContainsFunc(next.Definitions.Deliverables, func(item templates.Deliverable) bool {
		return item.ID == deliverable.ID && item.Required &&
			slices.Equal(item.EvidenceKinds, deliverable.EvidenceKinds) &&
			slices.Equal(item.Audience, deliverable.Audience)
	}) {
		return uuid.Nil, fmt.Errorf("%w: native deliverable ACK changed URL requirement", errCohort)
	}
	return deliverable.ID, nil
}

func (p *NativePreparer) SubmitURL(
	ctx context.Context,
	member *Session,
	event, team, deliverable uuid.UUID,
) (submissions.Page, error) {
	path := "/api/v1/events/" + event.String() + "/teams/" + team.String() + "/submission"
	before, err := prepareCall[submissions.Page](ctx, p, member, http.MethodGet, path, nil, nil)
	if err != nil {
		return before, err
	}
	if !before.CanSubmit || before.Final != nil || before.TeamID != team {
		return before, fmt.Errorf(
			"%w: actual first formal Live submission authority required",
			errCohort,
		)
	}
	evidence := []submissions.Evidence{{
		DeliverableID: deliverable, Kind: "url", Label: "Synthetic formal demonstration",
		URL: "https://example.test/demonstrations/" + team.String(),
	}}
	formal, err := prepareCall[submissions.Page](
		ctx,
		p,
		member,
		http.MethodPut,
		path,
		&before.Revision,
		submissions.Command{Evidence: evidence},
	)
	if err != nil {
		return formal, err
	}
	if formal.TeamID != team || formal.Final == nil {
		return formal, fmt.Errorf(
			"%w: original formal URL ACK changed team or omitted final revision",
			errCohort,
		)
	}
	final := formal.Final
	if final.Revision != 1 || final.Late || final.ActorID != member.Person() ||
		len(final.Evidence) != 1 {
		return formal, fmt.Errorf(
			"%w: original formal URL ACK changed author or exact first revision",
			errCohort,
		)
	}
	actual := formal.Final.Evidence[0]
	if actual.URL != evidence[0].URL || actual.Kind != "url" ||
		actual.DeliverableID != deliverable {
		return formal, fmt.Errorf("%w: original formal URL ACK changed exact evidence", errCohort)
	}
	return formal, nil
}
