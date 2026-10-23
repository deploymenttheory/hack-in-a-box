package surveys

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// ErasePersonInTransaction is called by the native privacy owner after its
// current self/administrative authority check and event lock, or by the retention
// worker after checking the same event privacy deadline. It retains no raw answer
// hash, respondent mapping or command identity. Reviewed suppressed snapshots
// are independent, immutable and contain no respondent identifiers.
func ErasePersonInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor *uuid.UUID,
	eventID, person uuid.UUID,
	reason audit.Reason,
) error {
	q := dbgen.New(tx)
	if _, err := q.DeleteSurveyPersonCommands(
		ctx,
		dbgen.DeleteSurveyPersonCommandsParams{EventID: eventID, PersonID: person},
	); err != nil {
		return fmt.Errorf("erase survey command identities: %w", err)
	}
	if _, err := q.DeleteSurveyPersonResponses(
		ctx,
		dbgen.DeleteSurveyPersonResponsesParams{EventID: eventID, PersonID: person},
	); err != nil {
		return fmt.Errorf("erase authored survey revisions: %w", err)
	}
	if _, err := q.DeleteSurveyPersonHeads(
		ctx,
		dbgen.DeleteSurveyPersonHeadsParams{EventID: eventID, PersonID: person},
	); err != nil {
		return fmt.Errorf("erase current survey response: %w", err)
	}
	removed, err := q.DeleteSurveyPersonInvitations(
		ctx,
		dbgen.DeleteSurveyPersonInvitationsParams{EventID: eventID, PersonID: person},
	)
	if err != nil {
		return fmt.Errorf("erase private survey invitations: %w", err)
	}
	if removed == 0 {
		return nil
	}
	err = audit.Append(
		ctx,
		q,
		audit.Record{
			ActorID:  actor,
			EventID:  &eventID,
			TargetID: person,
			Action:   audit.SurveyPersonalRemoved,
			Reason:   reason,
		},
	)
	return wrap("record survey erasure", err)
}

// Retain runs on the existing privacy maintenance boundary. It grants no user
// write exception in archived events and rechecks policy under the event lock.
func (s Service) Retain(ctx context.Context) error {
	after := uuid.Nil
	for {
		events, err := dbgen.New(s.Pool).SurveyPrivacyEvents(ctx, after)
		if err != nil {
			return fmt.Errorf("page survey retention events: %w", err)
		}
		for _, eventID := range events {
			err = database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
				e, readErr := q.LockEvent(ctx, eventID)
				if readErr != nil {
					return fmt.Errorf("lock survey retention event: %w", readErr)
				}
				if checkErr := s.privateCurrent(
					ctx,
					q,
					e,
				); !errors.Is(
					checkErr,
					access.ErrForbidden,
				) {
					return checkErr
				}
				personAfter := uuid.Nil
				for {
					people, readErr := q.SurveyPrivacyPeoplePage(
						ctx,
						dbgen.SurveyPrivacyPeoplePageParams{
							EventID:     eventID,
							AfterPerson: personAfter,
						},
					)
					if readErr != nil {
						return fmt.Errorf("page expired survey identities: %w", readErr)
					}
					for _, person := range people {
						if readErr = ErasePersonInTransaction(
							ctx,
							tx,
							nil,
							eventID,
							person,
							audit.RetentionPolicy,
						); readErr != nil {
							return readErr
						}
					}
					if len(people) < 100 {
						return nil
					}
					personAfter = people[len(people)-1]
				}
			})
			if err != nil {
				return fmt.Errorf("retain private survey records: %w", err)
			}
		}
		if len(events) < 100 {
			return nil
		}
		after = events[len(events)-1]
	}
}
