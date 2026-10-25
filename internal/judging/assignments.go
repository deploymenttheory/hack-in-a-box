package judging

import (
	"context"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

func eligibleJudge(ctx context.Context, tx pgx.Tx, eventID, teamID, judge uuid.UUID) (bool, error) {
	q := dbgen.New(tx)
	a, err := access.TeamPermissions(ctx, q, judge, eventID, teamID)
	if err != nil {
		return false, failure("judge eligibility", err)
	}
	var eligible bool
	err = tx.QueryRow(ctx, `SELECT p.active AND EXISTS(SELECT 1 FROM event_memberships m WHERE m.event_id=$1 AND m.person_id=p.id AND m.role='judge' AND m.active) AND NOT EXISTS(SELECT 1 FROM hack_team_members tm WHERE tm.event_id=$1 AND tm.team_id=$2 AND (tm.person_id=p.id OR EXISTS(SELECT 1 FROM mentor_conflicts c WHERE c.event_id=$1 AND c.mentor_id=p.id AND c.competitor_id=tm.person_id))) FROM people p WHERE p.id=$3`, eventID, teamID, judge).
		Scan(&eligible)
	return eligible && a.Active && !a.Member, failure("declared judge conflict", err)
}

func (s Service) Round(ctx context.Context, actor, eventID, runID uuid.UUID) (Round, error) {
	var out Round
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		a, err := s.guard(ctx, tx, actor, eventID, false)
		if err != nil {
			return err
		}
		_, d, err := loadRun(ctx, tx, eventID, runID)
		if err != nil {
			return err
		}
		version, open, err := round(ctx, tx, eventID, runID)
		if err != nil {
			return err
		}
		out = Round{
			RunID:          runID,
			Revision:       version,
			Deliberation:   open,
			RequiredJudges: d.RequiredJudgeCount,
			Assignments:    []Assignment{},
		}
		entries, err := dbgen.New(tx).SubmissionEntries(ctx, eventID)
		if err != nil {
			return failure("assignment entries", err)
		}
		for _, entry := range entries {
			if !entry.Active || entry.TeamState != "published" {
				continue
			}
			judges, err := assigned(ctx, tx, eventID, entry.TeamID)
			if err != nil {
				return failure("current assignments", err)
			}
			ids := []uuid.UUID{}
			for _, id := range judges {
				ok, err := eligibleJudge(ctx, tx, eventID, entry.TeamID, id)
				if err != nil {
					return err
				}
				if ok {
					ids = append(ids, id)
				}
			}
			if !a.organiser {
				if !slices.Contains(ids, actor) {
					continue
				}
				ids = []uuid.UUID{actor}
			}
			missing := max(0, d.RequiredJudgeCount-len(ids))
			if !a.organiser {
				missing = 0
			}
			out.Assignments = append(
				out.Assignments,
				Assignment{TeamID: entry.TeamID, JudgeIDs: ids, Missing: missing},
			)
		}
		return nil
	})
	return out, failure("round read", err)
}

func (s Service) ChangeRound(
	ctx context.Context,
	actor, eventID, runID uuid.UUID,
	expected int64,
	c RoundCommand,
) (Receipt, error) {
	var out Receipt
	if c.RequestID == uuid.Nil || expected < 0 || !reason(c.Reason) {
		return out, access.ErrInvalid
	}
	hash, err := digest(struct {
		Run     uuid.UUID
		Version int64
		Command RoundCommand
	}{runID, expected, c})
	if err != nil {
		return out, err
	}
	err = database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		a, err := s.guard(ctx, tx, actor, eventID, true)
		if err != nil {
			return err
		}
		if !a.organiser {
			return access.ErrForbidden
		}
		if _, _, err = loadRun(ctx, tx, eventID, runID); err != nil {
			return err
		}
		old, err := original(ctx, tx, eventID, actor, c.RequestID, hash)
		if err != nil {
			return err
		}
		if old != nil {
			out = *old
			return nil
		}
		version, _, err := round(ctx, tx, eventID, runID)
		if err != nil {
			return err
		}
		if version != expected {
			return access.ErrConflict
		}
		if err = ensureRound(ctx, tx, eventID, runID); err != nil {
			return err
		}
		switch c.Action {
		case "assign":
			err = s.assign(ctx, tx, eventID, c)
		case "declare_conflict":
			err = s.declare(ctx, tx, eventID, c)
		case "open_deliberation":
			if _, err = s.CompletionInTransaction(ctx, tx, actor, eventID, runID); err == nil {
				_, err = tx.Exec(
					ctx,
					"UPDATE judge_rounds SET deliberation=true WHERE event_id=$1 AND run_id=$2",
					eventID,
					runID,
				)
			}
		default:
			return access.ErrInvalid
		}
		if err != nil {
			return failure("round decision", err)
		}
		if c.Action != "open_deliberation" {
			if err = invalidate(ctx, tx, eventID, runID, actor, c.Reason, s); err != nil {
				return err
			}
			if _, err = tx.Exec(
				ctx,
				"UPDATE judge_rounds SET deliberation=false WHERE event_id=$1 AND run_id=$2",
				eventID,
				runID,
			); err != nil {
				return failure("close deliberation", err)
			}
		}
		if _, err = tx.Exec(
			ctx,
			"UPDATE judge_rounds SET revision=revision+1 WHERE event_id=$1 AND run_id=$2",
			eventID,
			runID,
		); err != nil {
			return failure("advance round", err)
		}
		out = Receipt{
			RequestID:   c.RequestID,
			ActorID:     actor,
			RunID:       runID,
			TeamID:      c.TeamID,
			Action:      c.Action,
			Revision:    version + 1,
			State:       "recorded",
			Fingerprint: hash,
		}
		if err = history(
			ctx,
			tx,
			actor,
			eventID,
			runID,
			c.TeamID,
			c.Action,
			c.Reason,
			c,
			s,
		); err != nil {
			return err
		}
		return retain(ctx, tx, eventID, out)
	})
	return out, failure("round change", err)
}

func (s Service) assign(ctx context.Context, tx pgx.Tx, eventID uuid.UUID, c RoundCommand) error {
	if c.TeamID == uuid.Nil || len(c.JudgeIDs) > 50 {
		return access.ErrInvalid
	}
	q := dbgen.New(tx)
	entry, err := q.CompetitionEntry(
		ctx,
		dbgen.CompetitionEntryParams{EventID: eventID, TeamID: c.TeamID},
	)
	if err != nil {
		return failure("assign entry", err)
	}
	seen := map[uuid.UUID]bool{}
	for _, id := range c.JudgeIDs {
		if id == uuid.Nil || seen[id] {
			return access.ErrInvalid
		}
		seen[id] = true
		ok, err := eligibleJudge(ctx, tx, eventID, c.TeamID, id)
		if err != nil {
			return err
		}
		if !ok {
			return access.ErrForbidden
		}
	}
	if _, err = tx.Exec(
		ctx,
		"INSERT INTO policy_resources(event_id,id,kind) VALUES($1,$2,'assessment') ON CONFLICT DO NOTHING",
		eventID,
		entry.ID,
	); err != nil {
		return failure("assessment resource", err)
	}
	// Own judge membership is independent from any mentor assignment on the same resource.
	if _, err = tx.Exec(
		ctx,
		"UPDATE judge_assignments SET active=false WHERE event_id=$1 AND team_id=$2",
		eventID,
		c.TeamID,
	); err != nil {
		return failure("replace judge cohort", err)
	}
	for _, id := range c.JudgeIDs {
		if _, err = tx.Exec(
			ctx,
			"INSERT INTO judge_assignments(event_id,team_id,person_id,active) VALUES($1,$2,$3,true) ON CONFLICT(event_id,team_id,person_id) DO UPDATE SET active=true",
			eventID,
			c.TeamID,
			id,
		); err != nil {
			return failure("retain judge assignment", err)
		}
		if _, err = tx.Exec(
			ctx,
			"INSERT INTO resource_assignments(event_id,resource_id,person_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING",
			eventID,
			entry.ID,
			id,
		); err != nil {
			return failure("assign judge resource", err)
		}
	}
	return nil
}

func (s Service) declare(ctx context.Context, tx pgx.Tx, eventID uuid.UUID, c RoundCommand) error {
	if c.JudgeID == uuid.Nil || c.CompetitorID == uuid.Nil ||
		!slices.Contains([]string{"own_team", "reporting"}, c.ConflictKind) {
		return access.ErrInvalid
	}
	q := dbgen.New(tx)
	if c.Enabled {
		return failure(
			"declare conflict",
			q.SetMentorConflict(
				ctx,
				dbgen.SetMentorConflictParams{
					EventID:      eventID,
					MentorID:     c.JudgeID,
					CompetitorID: c.CompetitorID,
					Kind:         c.ConflictKind,
				},
			),
		)
	}
	return failure(
		"remove conflict",
		q.DeleteMentorConflict(
			ctx,
			dbgen.DeleteMentorConflictParams{
				EventID:      eventID,
				MentorID:     c.JudgeID,
				CompetitorID: c.CompetitorID,
				Kind:         c.ConflictKind,
			},
		),
	)
}

func assigned(ctx context.Context, tx pgx.Tx, eventID, teamID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := tx.Query(
		ctx,
		"SELECT person_id FROM judge_assignments WHERE event_id=$1 AND team_id=$2 AND active ORDER BY person_id",
		eventID,
		teamID,
	)
	if err != nil {
		return nil, failure("assigned judges", err)
	}
	defer rows.Close()
	ids := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		if err = rows.Scan(&id); err != nil {
			return nil, failure("judge assignment", err)
		}
		ids = append(ids, id)
	}
	return ids, failure("judge assignments", rows.Err())
}
