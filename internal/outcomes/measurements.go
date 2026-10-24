package outcomes

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/objectives"
)

type (
	ObservationMeasure struct {
		Reference  objectives.MeasureReference `json:"reference"`
		Definition objectives.Measure          `json:"definition"`
	}
	Observation struct {
		OutcomeID      uuid.UUID            `json:"outcome_id"`
		OutcomeVersion int64                `json:"outcome_version"`
		Evidence       Evidence             `json:"evidence"`
		RecordedAt     time.Time            `json:"recorded_at"`
		Measures       []ObservationMeasure `json:"measures"`
	}
)

type ObservationPage struct {
	Items      []Observation `json:"items"`
	NextCursor *string       `json:"next_cursor"`
}

func observationCursor(value string) (uuid.UUID, uuid.UUID, error) {
	if value == "" {
		return uuid.Nil, uuid.Nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return uuid.Nil, uuid.Nil, access.ErrInvalid
	}
	parts := strings.Split(string(raw), ":")
	if len(parts) != 2 {
		return uuid.Nil, uuid.Nil, access.ErrInvalid
	}
	outcome, err := uuid.Parse(parts[0])
	if err != nil {
		return uuid.Nil, uuid.Nil, access.ErrInvalid
	}
	evidence, err := uuid.Parse(parts[1])
	if err != nil {
		return uuid.Nil, uuid.Nil, access.ErrInvalid
	}
	return outcome, evidence, nil
}

// Observations exposes no inferred aggregate or return-on-investment figure.
// Each nullable value keeps its original basis, source/date and exact native
// measure snapshot; coincidentally equal IDs across events never imply equality.
func (s Service) Observations(
	ctx context.Context,
	actor, eventID uuid.UUID,
	cursor string,
	limit int32,
) (ObservationPage, error) {
	var out ObservationPage
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		var err error
		out, err = s.ObservationsInTransaction(ctx, tx, actor, eventID, cursor, limit)
		return err
	})
	if err != nil {
		return out, wrap("read observations transaction", err)
	}
	return out, nil
}

// ObservationsInTransaction preserves this owner's current field and resource policy
// while composing a read in the caller's event-locked transaction.
func (s Service) ObservationsInTransaction(ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	cursor string,
	limit int32,
) (ObservationPage, error) {
	out := ObservationPage{Items: []Observation{}}
	a, b, err := observationCursor(cursor)
	if err != nil || limit < 1 || limit > 100 {
		return out, access.ErrInvalid
	}
	q := dbgen.New(tx)
	err = func() error {
		if err := (access.EventPolicy{Queries: q}).Authorize(
			ctx,
			actor,
			eventID,
			eventID,
			access.StaffRead,
		); err != nil {
			return wrap("authorize restricted outcome observations", err)
		}
		rows, err := q.OutcomeObservationsPage(
			ctx,
			dbgen.OutcomeObservationsPageParams{
				EventID:         eventID,
				ActorID:         actor,
				AtTime:          s.now(),
				AfterOutcomeID:  a,
				AfterEvidenceID: b,
				PageSize:        limit + 1,
			},
		)
		if err != nil {
			return wrap("page authorised outcome observations", err)
		}
		more := len(rows) > int(limit)
		if more {
			rows = rows[:limit]
		}
		for _, row := range rows {
			var evidence Evidence
			if err = json.Unmarshal(row.Document, &evidence); err != nil {
				return wrap("read retained observation evidence", err)
			}
			if evidence.ID != row.ID {
				return access.ErrConflict
			}
			item := Observation{
				OutcomeID:      row.OutcomeID,
				OutcomeVersion: row.OutcomeVersion,
				Evidence:       evidence,
				RecordedAt:     row.CreatedAt,
				Measures:       []ObservationMeasure{},
			}
			for _, ref := range evidence.Measures {
				definition, e := objectives.ResolveMeasureInTransaction(
					ctx,
					tx,
					actor,
					eventID,
					ref,
				)
				if e != nil {
					return wrap("resolve exact observation measure", e)
				}
				item.Measures = append(
					item.Measures,
					ObservationMeasure{Reference: ref, Definition: definition},
				)
			}
			out.Items = append(out.Items, item)
		}
		if more {
			last := rows[len(rows)-1]
			value := base64.RawURLEncoding.EncodeToString(
				[]byte(last.OutcomeID.String() + ":" + last.ID.String()),
			)
			out.NextCursor = &value
		}
		return nil
	}()
	return out, wrap("outcome observations", err)
}
