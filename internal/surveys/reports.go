package surveys

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

type ReportCommand struct {
	Action       string    `json:"action"`
	ReviewID     uuid.UUID `json:"review_id"`
	Summary      string    `json:"summary"`
	ReviewedText bool      `json:"reviewed_text"`
}

type ReviewedReport struct {
	ReviewID    uuid.UUID  `json:"review_id"`
	Version     int64      `json:"version"`
	Report      Report     `json:"report"`
	ReviewerID  uuid.UUID  `json:"reviewer_id"`
	ReviewedAt  time.Time  `json:"reviewed_at"`
	ApprovedAt  *time.Time `json:"approved_at"`
	PublishedAt *time.Time `json:"published_at"`
}

func reportSnapshot(ctx context.Context, q *dbgen.Queries, eventID, id uuid.UUID) (Report, error) {
	var out Report
	row, err := q.SurveyReportSnapshot(
		ctx,
		dbgen.SurveyReportSnapshotParams{EventID: eventID, SurveyID: id},
	)
	if err != nil {
		return out, fmt.Errorf("read frozen survey aggregate: %w", err)
	}
	if err = json.Unmarshal(row.Document, &out); err != nil {
		return out, fmt.Errorf("decode frozen survey aggregate: %w", err)
	}
	return out, nil
}

func (s Service) freezeReport(
	ctx context.Context,
	q *dbgen.Queries,
	e dbgen.Event,
	id uuid.UUID,
) (Report, error) {
	if out, err := reportSnapshot(ctx, q, e.ID, id); err == nil {
		return out, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	if err := s.privateCurrent(ctx, q, e); err != nil {
		return Report{}, err
	}
	head, cfg, err := current(ctx, q, e.ID, id)
	if err != nil {
		return Report{}, err
	}
	if !head.PublishedVersion.Valid || s.now().Before(cfg.Definition.ClosesAt) {
		return Report{}, access.ErrConflict
	}
	rows, err := q.CurrentSurveyResponses(
		ctx,
		dbgen.CurrentSurveyResponsesParams{EventID: e.ID, SurveyID: id},
	)
	if err != nil {
		return Report{}, fmt.Errorf("read closed response cohort: %w", err)
	}
	if len(rows) > 10000 {
		return Report{}, access.ErrInvalid
	}
	responses := []Response{}
	people, err := s.people(ctx, q, e.ID, uuid.Nil, cfg.Definition.Form.Audience)
	if err != nil {
		return Report{}, err
	}
	eligible := map[uuid.UUID]bool{}
	for _, person := range people {
		eligible[person] = true
	}
	for _, row := range rows {
		if !eligible[row.PersonID] {
			continue
		}
		response, readErr := decodeResponse(row)
		if readErr != nil {
			return Report{}, readErr
		}
		responses = append(responses, response)
	}
	out := aggregate(cfg, responses)
	body, err := json.Marshal(out)
	if err != nil {
		return out, fmt.Errorf("encode suppressed snapshot: %w", err)
	}
	// Never retain a dictionary-checkable hash of identifiable raw answers after
	// privacy erasure. The snapshot binds only the already suppressed projection.
	digest := sha256.Sum256(body)
	if err = q.InsertSurveyReportSnapshot(
		ctx,
		dbgen.InsertSurveyReportSnapshotParams{
			EventID:         e.ID,
			SurveyID:        id,
			SurveyVersion:   cfg.Version,
			AggregateDigest: hex.EncodeToString(digest[:]),
			Document:        body,
			CreatedAt:       s.now(),
		},
	); err != nil {
		return out, fmt.Errorf("freeze closed survey aggregate: %w", err)
	}
	return out, nil
}

func projectReview(
	ctx context.Context,
	q *dbgen.Queries,
	eventID, id uuid.UUID,
	row dbgen.SurveyReportReview,
) (ReviewedReport, error) {
	report, err := reportSnapshot(ctx, q, eventID, id)
	if err != nil {
		return ReviewedReport{}, err
	}
	report.ReviewedSummary = row.Summary
	out := ReviewedReport{
		ReviewID:   row.ID,
		Version:    row.Version,
		Report:     report,
		ReviewerID: row.ActorID,
		ReviewedAt: row.ReviewedAt,
	}
	approval, e := q.SurveyReportApproval(
		ctx,
		dbgen.SurveyReportApprovalParams{EventID: eventID, SurveyID: id, ReviewID: row.ID},
	)
	if e == nil {
		out.ApprovedAt = &approval.ApprovedAt
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return out, fmt.Errorf("read reviewed text approval: %w", e)
	}
	publication, e := q.SurveyReportPublication(
		ctx,
		dbgen.SurveyReportPublicationParams{EventID: eventID, SurveyID: id},
	)
	if e == nil && publication.ReviewID == row.ID {
		out.PublishedAt = &publication.PublishedAt
	} else if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return out, fmt.Errorf("read report publication: %w", e)
	}
	return out, nil
}

func (s Service) ChangeReport(
	ctx context.Context,
	actor, eventID, id uuid.UUID,
	expected int64,
	c ReportCommand,
) (ReviewedReport, error) {
	var out ReviewedReport
	c.Summary = strings.TrimSpace(c.Summary)
	if err := c.validate(expected); err != nil {
		return out, err
	}

	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		e, err := q.LockEvent(ctx, eventID)
		if err != nil {
			return fmt.Errorf("lock reviewed survey report: %w", err)
		}
		if err = authorize(ctx, q, actor, eventID); err != nil {
			return err
		}
		if !e.WritesAllowed {
			return access.ErrForbidden
		}
		ended, err := eventEnded(e, s.now())
		if err != nil {
			return err
		}
		if !ended {
			return access.ErrForbidden
		}
		snapshot, err := s.freezeReport(ctx, q, e, id)
		if err != nil {
			return err
		}
		latest, err := q.LatestSurveyReportReview(
			ctx,
			dbgen.LatestSurveyReportReviewParams{EventID: eventID, SurveyID: id},
		)
		if errors.Is(err, pgx.ErrNoRows) {
			latest = dbgen.SurveyReportReview{}
			err = nil
		}
		if err != nil {
			return fmt.Errorf("read latest reviewed report: %w", err)
		}
		var review dbgen.SurveyReportReview
		if c.Action == "review_text" {
			if snapshot.Suppressed && c.Summary != "" {
				return access.ErrInvalid
			}
			prior, readErr := q.SurveyReportReview(
				ctx,
				dbgen.SurveyReportReviewParams{EventID: eventID, SurveyID: id, ID: c.ReviewID},
			)
			if readErr == nil {
				if prior.ActorID != actor || prior.Version != expected+1 ||
					prior.Summary != c.Summary {
					return access.ErrConflict
				}
				out, err = projectReview(ctx, q, eventID, id, prior)
				return err
			}
			if !errors.Is(readErr, pgx.ErrNoRows) {
				return fmt.Errorf("read exact report review identity: %w", readErr)
			}
			if latest.Version != expected {
				return access.ErrConflict
			}
			review = dbgen.SurveyReportReview{
				EventID:    eventID,
				SurveyID:   id,
				ID:         c.ReviewID,
				Version:    expected + 1,
				Summary:    c.Summary,
				ActorID:    actor,
				ReviewedAt: s.now(),
			}
			if err = q.InsertSurveyReportReview(
				ctx,
				dbgen.InsertSurveyReportReviewParams(review),
			); err != nil {
				return fmt.Errorf("retain reviewed report prose: %w", err)
			}
		} else {
			if expected != latest.Version || latest.ID != c.ReviewID {
				return access.ErrConflict
			}
			review = latest
			if c.Action == "approve" {
				if err = q.ApproveSurveyReport(
					ctx,
					dbgen.ApproveSurveyReportParams{
						EventID:    eventID,
						SurveyID:   id,
						ReviewID:   review.ID,
						ActorID:    actor,
						ApprovedAt: s.now(),
					},
				); err != nil {
					return fmt.Errorf("approve exact report prose: %w", err)
				}
			} else {
				duplicate, publishErr := s.publishReport(ctx, q, actor, eventID, id, review)
				if publishErr != nil {
					return publishErr
				}
				if duplicate {
					out, err = projectReview(ctx, q, eventID, id, review)
					return err
				}
			}
		}
		if err = record(
			ctx,
			q,
			actor,
			eventID,
			id,
			audit.SurveyReportChanged,
			expected,
		); err != nil {
			return err
		}
		out, err = projectReview(ctx, q, eventID, id, review)
		return err
	})
	if err != nil {
		return out, fmt.Errorf("review survey report: %w", err)
	}
	return out, nil
}

// PublishedReport contains only the one frozen suppressed aggregate plus the
// latest deliberately reviewed prose correction. No filters or author rows exist.
func (s Service) PublishedReport(
	ctx context.Context,
	actor, eventID, id uuid.UUID,
) (Report, error) {
	var out Report
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		var err error
		out, err = s.PublishedReportInTransaction(ctx, tx, actor, eventID, id)
		return err
	})
	if err != nil {
		return out, fmt.Errorf("read published report transaction: %w", err)
	}
	return out, nil
}

// PublishedReportInTransaction preserves this owner's current field and resource policy
// while composing a read in the caller's event-locked transaction.
func (s Service) PublishedReportInTransaction(ctx context.Context,
	tx pgx.Tx,
	actor, eventID, id uuid.UUID,
) (Report, error) {
	var out Report
	q := dbgen.New(tx)
	err := func() error {
		if _, err := q.LockEvent(ctx, eventID); err != nil {
			return fmt.Errorf("lock published aggregate reader: %w", err)
		}
		_, cfg, err := current(ctx, q, eventID, id)
		if err != nil {
			return err
		}
		if staffErr := authorize(ctx, q, actor, eventID); staffErr != nil {
			people, readErr := s.people(ctx, q, eventID, actor, cfg.Definition.Form.Audience)
			if readErr != nil {
				return readErr
			}
			if len(people) != 1 {
				return access.ErrForbidden
			}
		}
		publication, err := q.SurveyReportPublication(
			ctx,
			dbgen.SurveyReportPublicationParams{EventID: eventID, SurveyID: id},
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return access.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("read released aggregate: %w", err)
		}
		review, err := q.SurveyReportReview(
			ctx,
			dbgen.SurveyReportReviewParams{
				EventID:  eventID,
				SurveyID: id,
				ID:       publication.ReviewID,
			},
		)
		if err != nil {
			return fmt.Errorf("read released reviewed prose: %w", err)
		}
		out, err = reportSnapshot(ctx, q, eventID, id)
		out.ReviewedSummary = review.Summary
		out.ReviewedTextVersion = publication.Version
		at := publication.PublishedAt
		out.PublishedAt = &at
		return err
	}()
	if err != nil {
		return out, fmt.Errorf("read published survey report: %w", err)
	}
	return out, nil
}

func (s Service) publishReport(
	ctx context.Context,
	q *dbgen.Queries,
	actor, eventID, id uuid.UUID,
	review dbgen.SurveyReportReview,
) (bool, error) {
	var err error
	approval, readErr := q.SurveyReportApproval(
		ctx,
		dbgen.SurveyReportApprovalParams{
			EventID:  eventID,
			SurveyID: id,
			ReviewID: review.ID,
		},
	)
	if readErr != nil {
		return false, access.ErrConflict
	}
	if err = authorize(ctx, q, approval.ActorID, eventID); err != nil {
		return false, err
	}
	publication, readErr := q.SurveyReportPublication(
		ctx,
		dbgen.SurveyReportPublicationParams{EventID: eventID, SurveyID: id},
	)
	if readErr == nil && publication.ReviewID == review.ID {
		return true, nil
	}
	if readErr != nil && !errors.Is(readErr, pgx.ErrNoRows) {
		return false, fmt.Errorf("read prior released report: %w", readErr)
	}
	if err = q.PublishSurveyReport(
		ctx,
		dbgen.PublishSurveyReportParams{
			EventID:     eventID,
			SurveyID:    id,
			Version:     review.Version,
			ReviewID:    review.ID,
			ActorID:     actor,
			PublishedAt: s.now(),
		},
	); err != nil {
		return false, fmt.Errorf("publish reviewed aggregate report: %w", err)
	}
	return false, nil
}

func (c ReportCommand) validate(expected int64) error {
	if expected < 0 || c.ReviewID == uuid.Nil ||
		!slices.Contains([]string{"review_text", "approve", "publish"}, c.Action) ||
		!validText(c.Summary, 20000) ||
		(c.Action == "review_text" && !c.ReviewedText) ||
		(c.Action != "review_text" && (c.Summary != "" || c.ReviewedText)) {
		return access.ErrInvalid
	}
	return nil
}
