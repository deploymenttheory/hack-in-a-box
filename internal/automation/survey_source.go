package automation

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/surveys"
)

// SurveySource delegates invitation authority to the actual published native
// questionnaire. It never reads or interpolates an authored response.
type SurveySource struct{ Surveys *surveys.Service }

func (s SurveySource) DiscoverInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
	d Definition,
) ([]Source, error) {
	if s.Surveys == nil {
		return nil, communications.ErrUnresolved
	}
	out := []Source{}
	after := uuid.Nil
	for {
		rows, next, err := s.Surveys.DiscoverNotificationSourcesPageInTransaction(
			ctx,
			tx,
			eventID,
			d.SourceID,
			after,
			200,
		)
		if err != nil {
			return nil, fmt.Errorf("discover native survey source: %w", err)
		}
		for _, row := range rows {
			if d.SourceVersion == 0 || d.SourceVersion == row.Version {
				out = append(
					out,
					Source{
						Kind:       "survey.open",
						ID:         row.ID,
						Revision:   row.Version,
						OccurredAt: row.OpensAt,
					},
				)
			}
		}
		if next == nil {
			return out, nil
		}
		after = *next
	}
}

func (s SurveySource) ResolveInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
	source Source,
) (SourceAudience, error) {
	if s.Surveys == nil || source.Kind != "survey.open" {
		return SourceAudience{}, communications.ErrUnresolved
	}
	people, err := s.Surveys.ResolveInvitationAudienceInTransaction(
		ctx,
		tx,
		eventID,
		source.ID,
		source.Revision,
	)
	if err != nil {
		return SourceAudience{}, fmt.Errorf("resolve native survey invitations: %w", err)
	}
	return SourceAudience{
		RecipientIDs: people,
		Resource: &communications.ResourceReference{
			Kind:    "survey",
			ID:      source.ID,
			Version: source.Revision,
		},
	}, nil
}
