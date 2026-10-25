package judging

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
)

// FactualSummary is deliberately authored for judge briefing, then explicitly
// reviewed. Private observations and qualification cards are never its source.
// Approval is editorial accountability, not automatic semantic sanitization.
type FactualSummary struct {
	Version            int64          `json:"version"`
	AuthorID           uuid.UUID      `json:"author_id"`
	ApprovedBy         *uuid.UUID     `json:"approved_by"`
	SubmissionRevision int64          `json:"submission_revision"`
	Repository         FreezeEvidence `json:"repository"`
	Facts              []SummaryFact  `json:"facts"`
}
type SummaryFact struct {
	DeliverableID uuid.UUID `json:"deliverable_id"`
	Fact          string    `json:"fact"`
}
type SummaryPage struct {
	Version int64           `json:"version"`
	Summary *FactualSummary `json:"summary"`
	Receipt *Receipt        `json:"receipt"`
}
type SummaryCommand struct {
	RequestID            uuid.UUID     `json:"request_id"`
	Action               string        `json:"action"`
	SubmissionRevision   int64         `json:"submission_revision"`
	RepositoryDecisionID *uuid.UUID    `json:"repository_decision_id"`
	Facts                []SummaryFact `json:"facts"`
	Reason               string        `json:"reason"`
}

func (s Service) summaryAuthority(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, teamID uuid.UUID,
	write bool,
) (bool, error) {
	q := dbgen.New(tx)
	row, err := q.LockEvent(ctx, eventID)
	if err != nil {
		return false, failure("lock summary event", err)
	}
	if err = (access.EventPolicy{Queries: q, Clock: s.now}).Authorize(
		ctx,
		actor,
		eventID,
		eventID,
		access.EventRead,
	); err != nil {
		return false, failure("summary actor", err)
	}
	roles, err := q.ActiveRoles(ctx, dbgen.ActiveRolesParams{EventID: eventID, PersonID: actor})
	if err != nil {
		return false, failure("summary roles", err)
	}
	organiser := false
	for _, role := range roles {
		if role == "organiser" {
			organiser = true
		}
	}
	assigned, err := q.ObservationMentorAssigned(
		ctx,
		dbgen.ObservationMentorAssignedParams{EventID: eventID, TeamID: teamID, PersonID: actor},
	)
	if err != nil {
		return false, failure("summary mentor assignment", err)
	}
	if !organiser && !assigned {
		return false, access.ErrForbidden
	}
	if write &&
		(!row.WritesAllowed || row.Stage == "cancelled" || row.Stage == "archived" || row.Stage == "completed") {
		return false, access.ErrForbidden
	}
	return organiser, nil
}

func summaryInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, teamID uuid.UUID,
) (SummaryPage, error) {
	var out SummaryPage
	var raw []byte
	err := tx.QueryRow(ctx, "SELECT document FROM judge_factual_summaries WHERE event_id=$1 AND team_id=$2 ORDER BY version DESC LIMIT 1", eventID, teamID).
		Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, failure("read summary", err)
	}
	var v FactualSummary
	if err = json.Unmarshal(raw, &v); err != nil {
		return out, failure("decode summary", err)
	}
	out.Version = v.Version
	out.Summary = &v
	return out, nil
}

func (s Service) Summary(
	ctx context.Context,
	actor, eventID, teamID, requestID uuid.UUID,
) (SummaryPage, error) {
	var out SummaryPage
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		if _, err := s.summaryAuthority(ctx, tx, actor, eventID, teamID, false); err != nil {
			return err
		}
		// Require the current native team/final audience even for retained draft reads.
		p, err := (submissions.Service{Pool: s.Pool, Clock: s.now}).ReadInTransaction(
			ctx,
			tx,
			actor,
			eventID,
			teamID,
		)
		if err != nil {
			return failure("summary final audience", err)
		}
		out, err = summaryInTransaction(ctx, tx, eventID, teamID)
		if err != nil {
			return err
		}
		if out.Summary != nil {
			if p.Final == nil || p.Outcome != "frozen" ||
				out.Summary.SubmissionRevision != p.Final.Revision {
				out.Summary = nil
			} else {
				repository, readErr := s.codeEvidence(ctx, tx, actor, eventID, teamID, p)
				if readErr != nil && !errors.Is(readErr, access.ErrConflict) {
					return readErr
				}
				if readErr != nil || !sameFreeze(out.Summary.Repository, repository) {
					out.Summary = nil
				}
			}
		}
		if requestID != uuid.Nil {
			var raw []byte
			err = tx.QueryRow(ctx, "SELECT result FROM judge_commands WHERE event_id=$1 AND request_id=$2 AND actor_id=$3", eventID, requestID, actor).
				Scan(&raw)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			if err != nil {
				return failure("summary receipt", err)
			}
			var receipt Receipt
			if err = json.Unmarshal(raw, &receipt); err != nil {
				return failure("decode summary receipt", err)
			}
			if receipt.TeamID == teamID && strings.HasPrefix(receipt.Action, "summary_") {
				out.Receipt = &receipt
			}
		}
		return nil
	})
	return out, failure("summary page", err)
}

func (s Service) ChangeSummary(
	ctx context.Context,
	actor, eventID, teamID uuid.UUID,
	expected int64,
	c SummaryCommand,
) (Receipt, error) {
	var out Receipt
	if c.RequestID == uuid.Nil || expected < 0 || !reason(c.Reason) {
		return out, access.ErrInvalid
	}
	hash, err := digest(struct {
		Team    uuid.UUID
		Version int64
		Command SummaryCommand
	}{teamID, expected, c})
	if err != nil {
		return out, err
	}
	err = database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		organiser, err := s.summaryAuthority(ctx, tx, actor, eventID, teamID, true)
		if err != nil {
			return err
		}
		p, err := (submissions.Service{Pool: s.Pool, Clock: s.now}).ReadInTransaction(
			ctx,
			tx,
			actor,
			eventID,
			teamID,
		)
		if err != nil {
			return failure("summary final", err)
		}
		if p.Outcome != "frozen" || p.Final == nil {
			return access.ErrConflict
		}
		repository, err := s.codeEvidence(ctx, tx, actor, eventID, teamID, p)
		if err != nil {
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
		current, err := summaryInTransaction(ctx, tx, eventID, teamID)
		if err != nil {
			return err
		}
		if current.Version != expected || c.SubmissionRevision != p.Final.Revision ||
			!sameID(c.RepositoryDecisionID, repository.DecisionID) {
			return access.ErrConflict
		}
		next, err := summaryTransition(
			actor,
			organiser,
			current.Summary,
			current.Version+1,
			p,
			repository,
			c,
		)
		if err != nil {
			return err
		}
		raw, err := json.Marshal(next)
		if err != nil {
			return failure("encode summary", err)
		}
		if _, err = tx.Exec(
			ctx,
			"INSERT INTO judge_factual_summaries(event_id,team_id,version,document)VALUES($1,$2,$3,$4)",
			eventID,
			teamID,
			next.Version,
			raw,
		); err != nil {
			return failure("retain summary version", err)
		}
		out = Receipt{
			RequestID:   c.RequestID,
			ActorID:     actor,
			TeamID:      teamID,
			Action:      "summary_" + c.Action,
			Revision:    next.Version,
			State:       "draft",
			Fingerprint: hash,
		}
		if next.ApprovedBy != nil {
			out.State = "approved"
		}
		if err = history(
			ctx,
			tx,
			actor,
			eventID,
			uuid.Nil,
			teamID,
			out.Action,
			c.Reason,
			next,
			s,
		); err != nil {
			return err
		}
		return retain(ctx, tx, eventID, out)
	})
	return out, failure("change factual summary", err)
}

func summaryTransition(
	actor uuid.UUID,
	organiser bool,
	current *FactualSummary,
	version int64,
	p submissions.Page,
	repository FreezeEvidence,
	c SummaryCommand,
) (FactualSummary, error) {
	next := FactualSummary{
		Version:            version,
		AuthorID:           actor,
		SubmissionRevision: p.Final.Revision,
		Repository:         repository,
		Facts:              c.Facts,
	}
	switch c.Action {
	case "save":
		if organiser || len(c.Facts) == 0 || len(c.Facts) > 20 {
			return next, access.ErrForbidden
		}
		known := map[uuid.UUID]bool{}
		for _, e := range p.Final.Evidence {
			known[e.DeliverableID] = true
		}
		for _, fact := range c.Facts {
			if !known[fact.DeliverableID] || strings.TrimSpace(fact.Fact) == "" ||
				len(fact.Fact) > 2000 {
				return next, access.ErrInvalid
			}
		}
	case "approve":
		if !organiser || current == nil || current.ApprovedBy != nil || len(c.Facts) > 0 {
			return next, access.ErrForbidden
		}
		if current.SubmissionRevision != p.Final.Revision ||
			!sameFreeze(current.Repository, repository) {
			return next, access.ErrConflict
		}
		next = *current
		next.Version = version
		next.ApprovedBy = &actor
	default:
		return next, access.ErrInvalid
	}
	return next, nil
}
