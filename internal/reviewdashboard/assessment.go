package reviewdashboard

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s Service) assessment(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	row *Team,
) error {
	cursor := uuid.Nil
	for {
		page, err := s.Cards.CoverageInTransaction(ctx, tx, actor, eventID, row.ID, cursor, 100)
		if err != nil {
			if expectedDenial(err) {
				row.Assessment.Availability = "unavailable"
				return nil
			}
			return fmt.Errorf("read assessment completion metadata: %w", err)
		}
		if row.Assessment.Availability != "review_required" ||
			page.Availability == "review_required" {
			row.Assessment.Availability = page.Availability
		}
		for _, checkpoint := range page.Items {
			if checkpoint.Purpose == "checkpoint" {
				addCoverage(&row.Assessment.Live, checkpoint)
			} else if checkpoint.Purpose == "mentor_final" {
				addCoverage(&row.Assessment.Final, checkpoint)
			}
		}
		if page.NextCursor == nil {
			break
		}
		cursor = *page.NextCursor
	}
	if row.Assessment.Availability == "review_required" {
		row.Assessment.Live.Complete = false
		row.Assessment.Final.Complete = false
	}
	agendaCursor := uuid.Nil
	total := 0
	for {
		agenda, err := s.Moderation.AgendaInTransaction(ctx, tx, actor, eventID, row.ID, agendaCursor, 100)
		if err != nil {
			if expectedDenial(err) {
				return nil
			}
			return fmt.Errorf("read permitted huddle agenda metadata: %w", err)
		}
		for _, item := range agenda.Items {
			total++
			if len(row.Agenda) < 10 {
				row.Agenda = append(row.Agenda, item)
			}
			if item.Purpose == "mentor_final" && item.CanonicalSigned {
				row.Assessment.CanonicalFinal = true
			}
		}
		if agenda.NextCursor == nil {
			break
		}
		agendaCursor = *agenda.NextCursor
	}
	row.MoreAgenda = total > 10

	return nil
}
