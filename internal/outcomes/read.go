package outcomes

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

func (s Service) List(
	ctx context.Context,
	actor, cursor uuid.UUID,
	mode string,
	eventID *uuid.UUID,
	limit int32,
) (Page, error) {
	out := Page{Items: []View{}}
	if limit < 1 || limit > 100 || (mode != "own" && mode != "all" && mode != "review") ||
		mode == "review" && eventID == nil {
		return out, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		if err := active(ctx, q, actor); err != nil {
			return err
		}
		if mode == "review" {
			allowed, err := currentOrganiser(ctx, q, actor, eventID)
			if err != nil {
				return err
			}
			if !allowed {
				return access.ErrForbidden
			}
		}
		rows, err := q.OutcomeHeadsPage(
			ctx,
			dbgen.OutcomeHeadsPageParams{
				AfterID:  cursor,
				ActorID:  actor,
				AtTime:   s.now(),
				Mode:     mode,
				EventID:  eventID,
				PageSize: limit + 1,
			},
		)
		if err != nil {
			return wrap("list permitted outcomes", err)
		}
		more := len(rows) > int(limit)
		if more {
			rows = rows[:limit]
		}
		for _, row := range rows {
			v, loadErr := load(ctx, q, row.ID, row.CurrentVersion)
			if loadErr != nil {
				return loadErr
			}
			item, projectErr := project(ctx, q, actor, row, v, s.now())
			if projectErr != nil {
				return projectErr
			}
			out.Items = append(out.Items, item)
		}
		if more {
			next := rows[len(rows)-1].ID
			out.NextCursor = &next
		}
		return nil
	})
	return out, wrap("list", err)
}

func (s Service) History(
	ctx context.Context,
	actor, id uuid.UUID,
	before int64,
	limit int32,
) (History, error) {
	out := History{Items: []Version{}}
	if id == uuid.Nil || before < 0 || limit < 1 || limit > 100 {
		return out, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		if err := active(ctx, q, actor); err != nil {
			return err
		}
		head, err := q.OutcomeHead(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return access.ErrNotFound
		}
		if err != nil {
			return wrap("read history identity", err)
		}
		if _, err = capabilities(ctx, q, actor, head, s.now()); err != nil {
			return err
		}
		rows, err := q.OutcomeHistory(
			ctx,
			dbgen.OutcomeHistoryParams{OutcomeID: id, BeforeVersion: before, PageSize: limit + 1},
		)
		if err != nil {
			return wrap("list retained history", err)
		}
		more := len(rows) > int(limit)
		if more {
			rows = rows[:limit]
		}
		for _, row := range rows {
			v, loadErr := load(ctx, q, id, row.Version)
			if loadErr != nil {
				return loadErr
			}
			out.Items = append(out.Items, v)
		}
		if more {
			next := rows[len(rows)-1].Version
			out.NextCursor = &next
		}
		return nil
	})
	return out, wrap("history", err)
}

func (s Service) Owners(
	ctx context.Context,
	actor, id, cursor uuid.UUID,
	limit int32,
) (OwnerPage, error) {
	out := OwnerPage{Items: []OwnerCandidate{}}
	if id == uuid.Nil || limit < 1 || limit > 100 {
		return out, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		if err := active(ctx, q, actor); err != nil {
			return err
		}
		head, err := q.OutcomeHead(ctx, id)
		if err != nil {
			return wrap("read owner candidate context", err)
		}
		cap, err := capabilities(ctx, q, actor, head, s.now())
		if err != nil {
			return err
		}
		if !cap.CanModerate || head.EventID == nil {
			return access.ErrForbidden
		}
		rows, err := q.OutcomeOwnerCandidates(
			ctx,
			dbgen.OutcomeOwnerCandidatesParams{OutcomeID: id, AfterID: cursor, PageSize: limit + 1},
		)
		if err != nil {
			return wrap("list current scoped owner candidates", err)
		}
		more := len(rows) > int(limit)
		if more {
			rows = rows[:limit]
		}
		for _, row := range rows {
			out.Items = append(out.Items, OwnerCandidate{ID: row.ID, DisplayName: row.DisplayName})
		}
		if more {
			next := rows[len(rows)-1].ID
			out.NextCursor = &next
		}
		return nil
	})
	return out, wrap("owners", err)
}

func (s Service) Evidence(
	ctx context.Context,
	actor, id, cursor uuid.UUID,
	limit int32,
) (EvidencePage, error) {
	out := EvidencePage{Items: []EvidenceRecord{}}
	if id == uuid.Nil || limit < 1 || limit > 100 {
		return out, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		if err := active(ctx, q, actor); err != nil {
			return err
		}
		head, err := q.OutcomeHead(ctx, id)
		if err != nil {
			return wrap("read evidence resource", err)
		}
		if _, err = capabilities(ctx, q, actor, head, s.now()); err != nil {
			return err
		}
		rows, err := q.OutcomeEvidencePage(
			ctx,
			dbgen.OutcomeEvidencePageParams{OutcomeID: id, AfterID: cursor, PageSize: limit + 1},
		)
		if err != nil {
			return wrap("read restricted evidence page", err)
		}
		more := len(rows) > int(limit)
		if more {
			rows = rows[:limit]
		}
		for _, row := range rows {
			var evidence Evidence
			if err = json.Unmarshal(row.Document, &evidence); err != nil {
				return wrap("decode restricted evidence", err)
			}
			out.Items = append(
				out.Items,
				EvidenceRecord{
					Evidence:       evidence,
					OutcomeVersion: row.OutcomeVersion,
					ActorID:        row.ActorID,
					At:             row.CreatedAt,
				},
			)
		}
		if more {
			next := rows[len(rows)-1].ID
			out.NextCursor = &next
		}
		return nil
	})
	return out, wrap("evidence page", err)
}
