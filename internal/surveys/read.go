package surveys

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

type ConfigurationSummary struct {
	ID        uuid.UUID `json:"id"`
	Version   int64     `json:"version"`
	Name      string    `json:"name"`
	Audience  string    `json:"audience"`
	OpensAt   time.Time `json:"opens_at"`
	ClosesAt  time.Time `json:"closes_at"`
	Approved  bool      `json:"approved"`
	Published bool      `json:"published"`
	Cancelled bool      `json:"cancelled"`
}
type DirectoryItem struct {
	ID           uuid.UUID `json:"id"`
	Version      int64     `json:"version"`
	Name         string    `json:"name"`
	OpensAt      time.Time `json:"opens_at"`
	ClosesAt     time.Time `json:"closes_at"`
	CanRespond   bool      `json:"can_respond"`
	HasResponded bool      `json:"has_responded"`
}

type ConfigurationPage struct {
	Items     []ConfigurationSummary `json:"items"`
	NextID    *uuid.UUID             `json:"next_id"`
	CanManage bool                   `json:"can_manage"`
}

type Directory struct {
	Items              []DirectoryItem `json:"items"`
	NextID             *uuid.UUID      `json:"next_id"`
	ResponseVisibility string          `json:"response_visibility"`
}

func (s Service) Configuration(
	ctx context.Context,
	actor, eventID, id uuid.UUID,
) (Configuration, error) {
	var out Configuration
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		if _, err := q.LockEvent(ctx, eventID); err != nil {
			return fmt.Errorf("lock survey configuration reader: %w", err)
		}
		if err := authorize(ctx, q, actor, eventID); err != nil {
			return err
		}
		_, cfg, err := current(ctx, q, eventID, id)
		out = cfg
		return err
	})
	return out, wrap("read survey projection", err)
}

func (s Service) Configurations(
	ctx context.Context,
	actor, eventID, after uuid.UUID,
	limit int32,
) (ConfigurationPage, error) {
	out := ConfigurationPage{Items: []ConfigurationSummary{}}
	if limit < 1 || limit > 100 {
		return out, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		e, err := q.LockEvent(ctx, eventID)
		if err != nil {
			return fmt.Errorf("lock survey configuration page: %w", err)
		}
		if err = authorize(ctx, q, actor, eventID); err != nil {
			return err
		}
		out.CanManage = e.WritesAllowed && s.privateCurrent(ctx, q, e) == nil
		rows, err := q.SurveyHeadsPage(
			ctx,
			dbgen.SurveyHeadsPageParams{EventID: eventID, AfterID: after, PageSize: limit + 1},
		)
		if err != nil {
			return fmt.Errorf("page survey configurations: %w", err)
		}
		if len(rows) > int(limit) {
			last := rows[limit-1].ID
			out.NextID = &last
			rows = rows[:limit]
		}
		for _, row := range rows {
			cfg, readErr := load(ctx, q, eventID, row.ID, row.CurrentVersion)
			if readErr != nil {
				return readErr
			}
			cfg.Cancelled = row.Cancelled
			out.Items = append(
				out.Items,
				ConfigurationSummary{
					ID:        cfg.ID,
					Version:   cfg.Version,
					Name:      cfg.Definition.Form.Name,
					Audience:  string(cfg.Definition.Form.Audience),
					OpensAt:   cfg.Definition.OpensAt,
					ClosesAt:  cfg.Definition.ClosesAt,
					Approved:  cfg.ApprovedAt != nil,
					Published: cfg.PublishedAt != nil,
					Cancelled: cfg.Cancelled,
				},
			)
		}
		return nil
	})
	return out, wrap("read survey projection", err)
}

func (s Service) OwnDirectory(
	ctx context.Context,
	actor, eventID, after uuid.UUID,
	limit int32,
) (Directory, error) {
	out := Directory{Items: []DirectoryItem{}, ResponseVisibility: ResponseVisibility}
	if limit < 1 || limit > 100 {
		return out, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		e, err := q.LockEvent(ctx, eventID)
		if err != nil {
			return fmt.Errorf("lock invited survey directory: %w", err)
		}
		if err = (access.EventPolicy{Queries: q}).Authorize(
			ctx,
			actor,
			eventID,
			eventID,
			access.EventRead,
		); err != nil {
			return wrap("authorize survey event reader", err)
		}
		if err = s.privateCurrent(ctx, q, e); err != nil {
			return err
		}
		ended, err := eventEnded(e, s.now())
		if err != nil {
			return err
		}
		rows, err := q.OwnSurveyInvitationsPage(
			ctx,
			dbgen.OwnSurveyInvitationsPageParams{
				EventID:  eventID,
				PersonID: actor,
				AfterID:  after,
				PageSize: limit + 1,
			},
		)
		if err != nil {
			return fmt.Errorf("page own survey invitations: %w", err)
		}
		if len(rows) > int(limit) {
			last := rows[limit-1].SurveyID
			out.NextID = &last
			rows = rows[:limit]
		}
		for _, row := range rows {
			head, cfg, readErr := s.invited(ctx, q, actor, eventID, row.SurveyID)
			if errors.Is(readErr, access.ErrForbidden) || errors.Is(readErr, access.ErrNotFound) {
				continue
			}
			if readErr != nil {
				return readErr
			}
			response, readErr := ownResponse(ctx, q, eventID, row.SurveyID, actor)
			if readErr != nil {
				return readErr
			}
			out.Items = append(out.Items, DirectoryItem{
				ID:       row.SurveyID,
				Version:  cfg.Version,
				Name:     cfg.Definition.Form.Name,
				OpensAt:  cfg.Definition.OpensAt,
				ClosesAt: cfg.Definition.ClosesAt,
				CanRespond: e.WritesAllowed && ended && !head.Cancelled &&
					!s.now().Before(cfg.Definition.OpensAt) &&
					s.now().Before(cfg.Definition.ClosesAt),
				HasResponded: response != nil,
			})
		}
		return nil
	})
	return out, wrap("read survey projection", err)
}

func (s Service) ResponseHistory(
	ctx context.Context,
	actor, eventID, id, person uuid.UUID,
	before int64,
	limit int32,
) ([]Response, error) {
	out := []Response{}
	if before < 1 || limit < 1 || limit > 100 {
		return out, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		e, err := q.LockEvent(ctx, eventID)
		if err != nil {
			return fmt.Errorf("lock response history reader: %w", err)
		}
		if err = authorize(ctx, q, actor, eventID); err != nil {
			return err
		}
		if err = s.privateCurrent(ctx, q, e); err != nil {
			return err
		}
		if _, _, err = current(ctx, q, eventID, id); err != nil {
			return err
		}
		rows, err := q.SurveyResponseHistoryPage(
			ctx,
			dbgen.SurveyResponseHistoryPageParams{
				EventID:        eventID,
				SurveyID:       id,
				PersonID:       person,
				BeforeRevision: before,
				PageSize:       limit,
			},
		)
		if err != nil {
			return fmt.Errorf("page private response revisions: %w", err)
		}
		for _, row := range rows {
			r, readErr := decodeResponse(row)
			if readErr != nil {
				return readErr
			}
			out = append(out, r)
		}
		return nil
	})
	return out, wrap("read survey projection", err)
}

func (s Service) ReportReview(
	ctx context.Context,
	actor, eventID, id uuid.UUID,
) (ReviewedReport, error) {
	var out ReviewedReport
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		if _, err := q.LockEvent(ctx, eventID); err != nil {
			return fmt.Errorf("lock reviewed report reader: %w", err)
		}
		if err := authorize(ctx, q, actor, eventID); err != nil {
			return err
		}
		review, err := q.LatestSurveyReportReview(
			ctx,
			dbgen.LatestSurveyReportReviewParams{EventID: eventID, SurveyID: id},
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return access.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("read latest reviewed report: %w", err)
		}
		out, err = projectReview(ctx, q, eventID, id, review)
		return err
	})
	return out, wrap("read survey projection", err)
}
