package scorecards

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
)

func decodeCard(raw []byte) (Card, error) {
	var c Card
	if err := json.Unmarshal(raw, &c); err != nil {
		return c, fmt.Errorf("decode immutable scorecard: %w", err)
	}
	return c, nil
}

func cardID(checkpoint Checkpoint, team, author uuid.UUID) uuid.UUID {
	return uuid.NewSHA1(
		checkpoint.Definition.ID,
		[]byte(fmt.Sprintf("%s:%s:%d", team, author, checkpoint.Version)),
	)
}

func (s Service) Change(
	ctx context.Context,
	actor, eventID, teamID uuid.UUID,
	revision int64,
	input CardCommand,
) (Card, error) {
	if !slices.Contains([]string{"save", "submit"}, input.Action) ||
		input.CheckpointID == uuid.Nil ||
		input.CheckpointVersion < 1 ||
		input.Version < 0 {
		return Card{}, access.ErrInvalid
	}
	var out Card
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		row, err := s.guard(ctx, q, actor, eventID, teamID)
		if err != nil {
			return err
		}
		if !row.WritesAllowed {
			return access.ErrForbidden
		}
		if row.Revision != revision {
			return access.ErrConflict
		}
		cp, err := checkpoint(ctx, q, eventID, input.CheckpointID)
		if err != nil {
			return err
		}
		if cp.Version != input.CheckpointVersion {
			return access.ErrConflict
		}
		if err = s.checkWindow(row, cp); err != nil {
			return err
		}

		// The native rubric owner rechecks publication, current run, assignment,
		// live/final clocks and frozen output inside THIS immutable-card transaction.
		snapshot, err := (rubric.Service{Pool: s.Pool, Clock: s.Now}).StartAssessmentInTransaction(
			ctx,
			tx,
			actor,
			eventID,
			teamID,
			cp.Definition.RunID,
		)
		if err != nil {
			return fmt.Errorf("authorize native scorecard assessment: %w", err)
		}
		if snapshot.Reference != cp.Rubric || snapshot.Purpose != cp.Purpose {
			return access.ErrConflict
		}
		values, err := normalizeScores(input.Scores, snapshot.Definition, false)
		if err != nil {
			return err
		}
		if err = s.evidence(
			ctx,
			q,
			eventID,
			teamID,
			values,
			input.Action == "submit",
			false,
			snapshot.SubmissionRevision,
		); err != nil {
			return err
		}
		id := cardID(cp, teamID, actor)
		prior, err := q.LatestMentorScorecard(
			ctx,
			dbgen.LatestMentorScorecardParams{EventID: eventID, ID: id},
		)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("read scorecard original author: %w", err)
		}
		if prior.Version != input.Version {
			return access.ErrConflict
		}
		created := s.now()
		if err == nil {
			old, decodeErr := decodeCard(prior.Document)
			if decodeErr != nil {
				return decodeErr
			}
			if old.AuthorID != actor || old.RunID != snapshot.RunID ||
				old.Rubric != snapshot.Reference {
				return access.ErrForbidden
			}
			created = old.CreatedAt
		}
		state := "draft"
		if input.Action == "submit" {
			state = "submitted"
		}
		out = Card{
			Revision:           revision + 1,
			Reference:          CardReference{ID: id, Version: input.Version + 1},
			TeamID:             teamID,
			CheckpointID:       cp.Definition.ID,
			CheckpointVersion:  cp.Version,
			AuthorID:           actor,
			RunID:              snapshot.RunID,
			Rubric:             snapshot.Reference,
			Criteria:           snapshot.Definition.PublicDefinition,
			Purpose:            snapshot.Purpose,
			SubmissionRevision: snapshot.SubmissionRevision,
			State:              state,
			Scores:             values,
			CreatedAt:          created,
			ChangedAt:          s.now(),
		}
		raw, err := json.Marshal(out)
		if err != nil {
			return fmt.Errorf("encode private scorecard: %w", err)
		}
		submission := pgtype.Int8{}
		if out.SubmissionRevision != nil {
			submission = pgtype.Int8{Int64: *out.SubmissionRevision, Valid: true}
		}
		if err = q.InsertMentorScorecard(
			ctx,
			dbgen.InsertMentorScorecardParams{
				EventID:            eventID,
				TeamID:             teamID,
				CheckpointID:       cp.Definition.ID,
				CheckpointVersion:  cp.Version,
				ID:                 id,
				Version:            out.Reference.Version,
				AuthorID:           actor,
				RunID:              out.RunID,
				RubricID:           out.Rubric.ID,
				RubricVersion:      out.Rubric.Version,
				SubmissionRevision: submission,
				Document:           raw,
				CreatedAt:          s.now(),
			},
		); err != nil {
			return fmt.Errorf("retain independent scorecard revision: %w", err)
		}
		return recordChange(ctx, q, eventID, actor, id, revision, audit.ScorecardChanged)
	})
	if err != nil {
		return Card{}, fmt.Errorf("change private scorecard: %w", err)
	}
	return out, nil
}

func (s Service) evidence(
	ctx context.Context,
	q *dbgen.Queries,
	eventID, teamID uuid.UUID,
	scores []CategoryScore,
	submitted, complete bool,
	frozen *int64,
) error {
	for _, score := range scores {
		if complete && score.Pending {
			return access.ErrConflict
		}
		if submitted && !score.Pending && len(score.Evidence) == 0 {
			return access.ErrInvalid
		}
		for _, reference := range score.Evidence {
			switch reference.Kind {
			case "observation":
				rows, err := q.MentorObservationHistory(
					ctx,
					dbgen.MentorObservationHistoryParams{
						EventID:       eventID,
						TeamID:        teamID,
						ID:            *reference.ID,
						BeforeVersion: pgtype.Int8{Int64: *reference.Version + 1, Valid: true},
						PageSize:      1,
					},
				)
				if err != nil {
					return fmt.Errorf("check observation evidence reference: %w", err)
				}
				if len(rows) != 1 || rows[0].Version != *reference.Version {
					return access.ErrNotFound
				}
			case "submission":
				if frozen != nil && *reference.Version != *frozen {
					return access.ErrConflict
				}
				if *reference.ID != teamID {
					return access.ErrNotFound
				}
				if _, err := q.SubmissionRevision(
					ctx,
					dbgen.SubmissionRevisionParams{
						EventID:  eventID,
						TeamID:   teamID,
						Revision: *reference.Version,
					},
				); err != nil {
					return fmt.Errorf("check submission evidence reference: %w", err)
				}
			}
		}
	}
	return nil
}

func (s Service) checkWindow(row dbgen.Event, cp Checkpoint) error {
	current, err := resolveWindow(row, cp)
	if err != nil {
		return err
	}
	if current.Timezone != cp.Timezone || !current.OpensAt.Equal(cp.OpensAt) ||
		!current.ClosesAt.Equal(cp.ClosesAt) {
		return access.ErrConflict
	}
	if s.now().Before(cp.OpensAt) || !s.now().Before(cp.ClosesAt) {
		return access.ErrForbidden
	}
	return nil
}
