package judging

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissionfreeze"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
)

// Completion is the exact transactional source for the separate result owner.
// It is not a public tally, qualification result or announcement. The caller
// holds this event transaction while retaining its own approval/reference.
type Completion struct {
	RunID              uuid.UUID
	Binding            string
	Cards              []Card
	RepositoryEvidence []submissionfreeze.FinalEvidence
}

func (s Service) CompletionInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, runID uuid.UUID,
) (Completion, error) {
	out := Completion{
		RunID:              runID,
		Cards:              []Card{},
		RepositoryEvidence: []submissionfreeze.FinalEvidence{},
	}
	a, err := s.guard(ctx, tx, actor, eventID, true)
	if err != nil {
		return out, err
	}
	if !a.organiser {
		return out, access.ErrForbidden
	}
	_, d, err := loadRun(ctx, tx, eventID, runID)
	if err != nil {
		return out, err
	}
	if s.now().Before(a.dates.SubmissionCutoff) {
		return out, access.ErrForbidden
	}
	if d.Mode == rubric.MentorQualification && s.Finalists == nil {
		return out, access.ErrForbidden
	}
	q := dbgen.New(tx)
	entries, err := q.SubmissionEntries(ctx, eventID)
	if err != nil {
		return out, failure("completion entries", err)
	}
	teams := 0
	for _, entry := range entries {
		if !entry.Active || entry.TeamState != "published" {
			continue
		}
		p, err := (submissions.Service{Pool: s.Pool, Clock: s.now}).ReadInTransaction(
			ctx,
			tx,
			actor,
			eventID,
			entry.TeamID,
		)
		if err != nil {
			return out, failure("completion final", err)
		}
		if p.Outcome != "frozen" || p.Final == nil {
			continue
		}
		if d.Mode == rubric.MentorQualification {
			if err = s.Finalists.AuthorizeFinalistInTransaction(
				ctx,
				tx,
				eventID,
				entry.TeamID,
			); errors.Is(err, access.ErrForbidden) ||
				errors.Is(err, access.ErrNotFound) {
				continue
			} else if err != nil {
				return out, failure("completion finalist", err)
			}
		}
		ids, err := assigned(ctx, tx, eventID, entry.TeamID)
		if err != nil {
			return out, err
		}
		if len(ids) < d.RequiredJudgeCount {
			return out, access.ErrConflict
		}
		teams++
		cards, err := s.completedCards(ctx, tx, eventID, runID, entry.TeamID, ids, d)
		if err != nil {
			return out, err
		}
		out.Cards = append(out.Cards, cards...)
		evidence, err := (submissionfreeze.Service{Pool: s.Pool, Now: s.now}).EvidenceForFinalInTransaction(
			ctx,
			tx,
			actor,
			eventID,
			entry.TeamID,
			p.EntryID,
			p.Final.Revision,
		)
		if err != nil {
			return out, failure("completion repository evidence", err)
		}
		// A managed repository remains unresolved until its explicit cutoff decision;
		// file/URL-only entries are eligible through their native frozen final.
		if evidence.Status != "not_applicable" && evidence.Status != "completed" {
			return out, access.ErrConflict
		}
		out.RepositoryEvidence = append(out.RepositoryEvidence, evidence)
	}
	if teams == 0 {
		return out, access.ErrConflict
	}
	out.Binding, err = digest(out)
	return out, err
}

// RecordResultApprovalInTransaction is used by the result owner after its own
// tally/tie checks. Missing/current-stale judges fail before an immutable approval
// is written. Reopen/reassignment/conflict decisions retain explicit invalidations.
func (s Service) RecordResultApprovalInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, runID, approvalID uuid.UUID,
	expectedBinding, why string,
) error {
	if approvalID == uuid.Nil || !reason(why) {
		return access.ErrInvalid
	}
	current, err := s.CompletionInTransaction(ctx, tx, actor, eventID, runID)
	if err != nil {
		return err
	}
	_, open, err := round(ctx, tx, eventID, runID)
	if err != nil {
		return err
	}
	if !open || current.Binding != expectedBinding {
		return access.ErrConflict
	}
	_, err = tx.Exec(
		ctx,
		"INSERT INTO judge_result_approvals(id,event_id,run_id,binding,actor_id,reason,created_at) VALUES($1,$2,$3,$4,$5,$6,$7)",
		approvalID,
		eventID,
		runID,
		current.Binding,
		actor,
		why,
		s.now(),
	)
	return failure("record result completion approval", err)
}

func (s Service) ResultApprovalCurrentInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, runID, approvalID uuid.UUID,
) error {
	current, err := s.CompletionInTransaction(ctx, tx, actor, eventID, runID)
	if err != nil {
		return err
	}
	var binding string
	err = tx.QueryRow(ctx, "SELECT a.binding FROM judge_result_approvals a WHERE a.event_id=$1 AND a.run_id=$2 AND a.id=$3 AND NOT EXISTS(SELECT 1 FROM judge_result_invalidations i WHERE i.approval_id=a.id)", eventID, runID, approvalID).
		Scan(&binding)
	if err != nil {
		return failure("current result approval", err)
	}
	if binding != current.Binding {
		return access.ErrConflict
	}
	return nil
}

// OwnHistory preserves authored records without preserving revoked access.
func (s Service) OwnHistory(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, runID, teamID uuid.UUID,
) ([]Card, error) {
	if _, _, err := s.assessment(ctx, tx, actor, eventID, runID, teamID); err != nil {
		return nil, err
	}
	rows, err := tx.Query(
		ctx,
		"SELECT document FROM judge_history WHERE event_id=$1 AND run_id=$2 AND team_id=$3 AND action IN ('save','submit','reopen') ORDER BY id DESC LIMIT 100",
		eventID,
		runID,
		teamID,
	)
	if err != nil {
		return nil, failure("card history", err)
	}
	out := []Card{}
	revisions := []int64{}
	for rows.Next() {
		var raw []byte
		var c Card
		if err = rows.Scan(&raw); err != nil {
			rows.Close()
			return nil, failure("history row", err)
		}
		if err = json.Unmarshal(raw, &c); err != nil {
			rows.Close()
			return nil, failure("history card", err)
		}
		if c.AuthorID == actor {
			out = append(out, c)
			revisions = append(revisions, c.SubmissionRevision)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, failure("history", err)
	}
	documents, err := (submissions.Service{Pool: s.Pool, Clock: s.now}).VisibleEvidenceInTransaction(
		ctx,
		tx,
		actor,
		eventID,
		teamID,
		revisions,
	)
	if err != nil {
		return nil, failure("history current target visibility", err)
	}
	for i := range out {
		document, visible := documents[out[i].SubmissionRevision]
		if !visible || len(document.Evidence) == 0 {
			out[i].Repository = FreezeEvidence{Status: "unavailable"}
			out[i].Scores = []Score{}
			out[i].State = "source_unavailable"
		}
	}
	return out, nil
}

func (s Service) completedCards(
	ctx context.Context,
	tx pgx.Tx,
	eventID, runID, teamID uuid.UUID,
	ids []uuid.UUID,
	d rubric.Definition,
) ([]Card, error) {
	out := []Card{}
	for _, judge := range ids {
		snapshot, final, err := s.assessment(ctx, tx, judge, eventID, runID, teamID)
		if err != nil {
			return nil, err
		}
		c, err := readCard(ctx, tx, eventID, runID, teamID, judge)
		if err != nil {
			return nil, err
		}
		evidence, err := s.codeEvidence(ctx, tx, judge, eventID, teamID, final)
		if err != nil {
			return nil, err
		}
		if c == nil || c.State != "submitted" || c.SubmissionRevision != final.Final.Revision ||
			c.Rubric != snapshot.Reference || !sameFreeze(c.Repository, evidence) {
			return nil, access.ErrConflict
		}
		if _, err = normalize(
			c.Scores,
			d.PublicDefinition,
			final.Final.Document,
			true,
		); err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, nil
}
