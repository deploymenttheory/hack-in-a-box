package attendance

import (
	"context"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

func (s Service) Logistics(
	ctx context.Context,
	actor, eventID, cursor uuid.UUID,
	limit int32,
	export bool,
) (OperationalPage, error) {
	var result OperationalPage
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		var err error
		result, err = s.LogisticsInTransaction(ctx, tx, actor, eventID, cursor, limit, export)
		return err
	})
	if err != nil {
		return result, fmt.Errorf("read logistics transaction: %w", err)
	}
	return result, nil
}

// LogisticsInTransaction preserves this owner's current field and resource policy
// while composing a read in the caller's event-locked transaction.
func (s Service) LogisticsInTransaction(ctx context.Context,
	tx pgx.Tx,
	actor, eventID, cursor uuid.UUID,
	limit int32,
	export bool,
) (OperationalPage, error) {
	result := OperationalPage{Items: []OperationalEntry{}}
	if limit < 1 || limit > 100 {
		return result, access.ErrInvalid
	}
	q := dbgen.New(tx)
	err := func() error {
		if _, err := lock(ctx, q, actor, eventID, access.ProfileExport); err != nil {
			return err
		}
		roster, err := q.AttendanceRoster(
			ctx,
			dbgen.AttendanceRosterParams{EventID: eventID, AttendanceDay: pgtype.Date{}},
		)
		if err != nil {
			return fmt.Errorf("load operational competitors: %w", err)
		}
		staff, err := q.AttendanceStaff(ctx, eventID)
		if err != nil {
			return fmt.Errorf("load operational staff: %w", err)
		}
		profiles, err := q.OperationalRoster(
			ctx,
			dbgen.OperationalRosterParams{EventID: eventID, Cursor: cursor, PageSize: limit},
		)
		if err != nil {
			return fmt.Errorf("load operational fields: %w", err)
		}
		people := map[uuid.UUID]OperationalEntry{}
		for _, p := range roster {
			people[p.ID] = OperationalEntry{
				PersonID:    p.ID,
				DisplayName: p.DisplayName,
				Competing:   true,
			}
		}
		for _, p := range staff {
			if _, ok := people[p.ID]; !ok {
				people[p.ID] = OperationalEntry{PersonID: p.ID, DisplayName: p.DisplayName}
			}
		}
		for _, p := range profiles {
			v, ok := people[p.PersonID]
			if !ok {
				continue
			}
			v.ContactEmail = &p.ContactEmail
			v.ContactPhone = &p.ContactPhone
			v.Dietary = &p.Dietary
			v.Accessibility = &p.Accessibility
			v.TravelDetails = &p.TravelDetails
			v.SocialAttendance = &p.SocialAttendance
			// Photo consent remains in the photographer-only projection, never inferred
			// from travel/social attendance or included in an operational export.
			people[p.PersonID] = v
		}
		for _, p := range people {
			if p.PersonID.String() > cursor.String() {
				result.Items = append(result.Items, p)
			}
		}
		sort.Slice(result.Items, func(i, j int) bool {
			return result.Items[i].PersonID.String() < result.Items[j].PersonID.String()
		})
		if len(result.Items) > int(limit) {
			result.Items = result.Items[:limit]
			last := result.Items[len(result.Items)-1].PersonID
			result.NextCursor = &last
		}
		result.RosterAsOf = s.now()
		if export {
			count := len(result.Items)
			if err := audit.Append(
				ctx,
				q,
				audit.Record{
					ActorID:  &actor,
					EventID:  &eventID,
					TargetID: eventID,
					Action:   audit.ProfileExported,
					Reason:   audit.UserRequest,
					Metadata: audit.Metadata{Count: &count},
				},
			); err != nil {
				return fmt.Errorf("audit operational export: %w", err)
			}
		}
		return nil
	}()
	if err != nil {
		return result, fmt.Errorf("read logistics roster: %w", err)
	}
	return result, nil
}
