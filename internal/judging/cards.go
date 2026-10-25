package judging

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissionfreeze"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
)

func (s Service) assessment(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, runID, teamID uuid.UUID,
) (rubric.AssessmentSnapshot, submissions.Page, error) {
	var empty rubric.AssessmentSnapshot
	a, err := s.guard(ctx, tx, actor, eventID, true)
	if err != nil {
		return empty, submissions.Page{}, err
	}
	if !a.judge {
		return empty, submissions.Page{}, access.ErrForbidden
	}
	ids, err := assigned(ctx, tx, eventID, teamID)
	if err != nil {
		return empty, submissions.Page{}, err
	}
	if !slices.Contains(ids, actor) {
		return empty, submissions.Page{}, access.ErrForbidden
	}
	native := submissions.Service{Pool: s.Pool, Clock: s.now}
	p, err := native.ReadInTransaction(ctx, tx, actor, eventID, teamID)
	if err != nil {
		return empty, p, failure("current frozen final", err)
	}
	snapshot, err := (rubric.Service{Pool: s.Pool, Clock: s.now, Finalists: s.Finalists}).StartAssessmentInTransaction(
		ctx,
		tx,
		actor,
		eventID,
		teamID,
		runID,
	)
	if err != nil {
		return empty, p, failure("current final assessment", err)
	}
	if snapshot.Purpose != "judge_final" || snapshot.SubmissionRevision == nil || p.Final == nil ||
		p.Outcome != "frozen" ||
		p.Final.Revision != *snapshot.SubmissionRevision {
		return empty, p, access.ErrConflict
	}
	if _, err = s.codeEvidence(ctx, tx, actor, eventID, teamID, p); err != nil {
		return empty, p, err
	}
	return snapshot, p, nil
}

func (s Service) Brief(
	ctx context.Context,
	actor, eventID, runID, teamID uuid.UUID,
) (Brief, error) {
	var out Brief
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		snapshot, p, err := s.assessment(ctx, tx, actor, eventID, runID, teamID)
		if err != nil {
			return err
		}
		team, err := q.HackTeam(ctx, dbgen.HackTeamParams{EventID: eventID, ID: teamID})
		if err != nil {
			return failure("brief team", err)
		}
		facts, err := (submissions.Service{Pool: s.Pool, Clock: s.now}).SummariesInTransaction(
			ctx,
			tx,
			actor,
			eventID,
			[]uuid.UUID{teamID},
		)
		if err != nil {
			return failure("factual output", err)
		}
		evidence, err := (submissionfreeze.Service{Pool: s.Pool, Now: s.now}).EvidenceForFinalInTransaction(
			ctx,
			tx,
			actor,
			eventID,
			teamID,
			p.EntryID,
			p.Final.Revision,
		)
		if err != nil {
			return failure("exact final repository evidence", err)
		}
		mentorSummary, err := summaryInTransaction(ctx, tx, eventID, teamID)
		if err != nil {
			return err
		}
		if mentorSummary.Summary != nil &&
			(mentorSummary.Summary.ApprovedBy == nil || mentorSummary.Summary.SubmissionRevision != p.Final.Revision || !sameFreeze(mentorSummary.Summary.Repository, freezeEvidence(evidence))) {
			mentorSummary.Summary = nil
		}
		_, open, err := round(ctx, tx, eventID, runID)
		if err != nil {
			return err
		}
		card, err := readCard(ctx, tx, eventID, runID, teamID, actor)
		if err != nil {
			return err
		}
		if card != nil &&
			(card.SubmissionRevision != p.Final.Revision || !sameFreeze(card.Repository, freezeEvidence(evidence))) {
			copy := *card
			copy.State = "source_changed"
			if card.SubmissionRevision != p.Final.Revision {
				copy.Repository = FreezeEvidence{Status: "unavailable"}
			}
			copy.Scores = []Score{}
			card = &copy
		}
		out = Brief{
			MentorSummary:        mentorSummary.Summary,
			TeamID:               teamID,
			TeamName:             team.Name,
			RunID:                runID,
			Criteria:             snapshot.Definition.PublicDefinition,
			Reference:            snapshot.Reference,
			PrivateJudgeGuidance: snapshot.Definition.PrivateJudgeGuidance,
			Submission:           p.Final.Document,
			SubmissionRevision:   p.Final.Revision,
			Output:               facts[0],
			Repository:           freezeEvidence(evidence),
			OwnCard:              card,
			SubmittedCards:       []Card{},
			Deliberation:         open,
		}
		if open {
			ids, err := assigned(ctx, tx, eventID, teamID)
			if err != nil {
				return err
			}
			for _, id := range ids {
				ok, err := eligibleJudge(ctx, tx, eventID, teamID, id)
				if err != nil {
					return err
				}
				if !ok {
					continue
				}
				c, err := readCard(ctx, tx, eventID, runID, teamID, id)
				if err != nil {
					return err
				}
				if c != nil && c.State == "submitted" && c.SubmissionRevision == p.Final.Revision &&
					sameFreeze(c.Repository, freezeEvidence(evidence)) {
					out.SubmittedCards = append(out.SubmittedCards, *c)
				}
			}
		}
		if open && len(out.SubmittedCards) < snapshot.Definition.RequiredJudgeCount {
			out.Deliberation = false
			out.SubmittedCards = []Card{}
		}
		return nil
	})
	return out, failure("brief", err)
}

func normalize(
	input []Score,
	d rubric.PublicDefinition,
	document submissions.Document,
	complete bool,
) ([]Score, error) {
	if len(input) > len(d.Categories) {
		return nil, access.ErrInvalid
	}
	known := map[uuid.UUID]bool{}
	for _, e := range document.Evidence {
		known[e.DeliverableID] = true
	}
	byID := map[uuid.UUID]Score{}
	for _, c := range input {
		if _, exists := byID[c.CategoryID]; exists {
			return nil, access.ErrInvalid
		}
		if !slices.ContainsFunc(
			d.Categories,
			func(v rubric.Category) bool { return v.ID == c.CategoryID },
		) {
			return nil, access.ErrInvalid
		}
		byID[c.CategoryID] = c
	}
	out := []Score{}
	for _, category := range d.Categories {
		c := byID[category.ID]
		c.CategoryID = category.ID
		if c.Evidence == nil {
			c.Evidence = []Evidence{}
		}
		if c.Value != nil && (*c.Value < category.MinScore || *c.Value > category.MaxScore) ||
			len(c.Evidence) > 30 {
			return nil, access.ErrInvalid
		}
		if complete && (c.Value == nil || len(c.Evidence) == 0) {
			return nil, access.ErrConflict
		}
		for _, e := range c.Evidence {
			if len(e.Note) > 4000 || e.DeliverableID != nil && !known[*e.DeliverableID] ||
				e.DeliverableID == nil && strings.TrimSpace(e.Note) == "" {
				return nil, access.ErrInvalid
			}
		}
		out = append(out, c)
	}
	return out, nil
}

func (s Service) ChangeCard(
	ctx context.Context,
	actor, eventID, runID, teamID uuid.UUID,
	expected int64,
	c CardCommand,
) (Receipt, error) {
	var out Receipt
	if c.RequestID == uuid.Nil || expected < 0 {
		return out, access.ErrInvalid
	}
	hash, err := digest(struct {
		Run, Team uuid.UUID
		Version   int64
		Command   CardCommand
	}{runID, teamID, expected, c})
	if err != nil {
		return out, err
	}
	err = database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		a, err := s.guard(ctx, tx, actor, eventID, true)
		if err != nil {
			return err
		}
		if _, _, err = loadRun(ctx, tx, eventID, runID); err != nil {
			return err
		}
		author, snapshot, p, err := s.cardAuthor(ctx, tx, actor, eventID, runID, teamID, a, c)
		if err != nil {
			return err
		}
		repository, err := s.codeEvidence(ctx, tx, author, eventID, teamID, p)
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
		card, err := readCard(ctx, tx, eventID, runID, teamID, author)
		if err != nil {
			return err
		}
		version := int64(0)
		if card != nil {
			version = card.Version
		}
		if !sameID(c.RepositoryDecisionID, repository.DecisionID) {
			return access.ErrConflict
		}
		if version != expected || c.SubmissionRevision != p.Final.Revision {
			return access.ErrConflict
		}
		if err = ensureRound(ctx, tx, eventID, runID); err != nil {
			return err
		}
		card, err = s.updatedCard(card, version, actor, runID, teamID, snapshot, p, repository, c)
		if err != nil {
			return err
		}
		raw, err := json.Marshal(card)
		if err != nil {
			return failure("encode card", err)
		}
		if _, err = tx.Exec(
			ctx,
			"INSERT INTO judge_cards(event_id,run_id,team_id,author_id,version,document) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(event_id,run_id,team_id,author_id) DO UPDATE SET version=EXCLUDED.version,document=EXCLUDED.document",
			eventID,
			runID,
			teamID,
			author,
			card.Version,
			raw,
		); err != nil {
			return failure("save independent card", err)
		}
		if err = invalidate(
			ctx,
			tx,
			eventID,
			runID,
			actor,
			"Judge card changed: "+c.Reason,
			s,
		); err != nil {
			return err
		}
		if _, err = tx.Exec(
			ctx,
			"UPDATE judge_rounds SET deliberation=false,revision=revision+1 WHERE event_id=$1 AND run_id=$2",
			eventID,
			runID,
		); err != nil {
			return failure("invalidate deliberation", err)
		}
		if err = history(
			ctx,
			tx,
			actor,
			eventID,
			runID,
			teamID,
			c.Action,
			c.Reason,
			card,
			s,
		); err != nil {
			return err
		}
		_ = q
		out = Receipt{
			RequestID:   c.RequestID,
			ActorID:     actor,
			RunID:       runID,
			TeamID:      teamID,
			Action:      c.Action,
			Revision:    card.Version,
			State:       card.State,
			Fingerprint: hash,
		}
		return retain(ctx, tx, eventID, out)
	})
	return out, failure("card change", err)
}

func (s Service) cardAuthor(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, runID, teamID uuid.UUID,
	a authority,
	c CardCommand,
) (uuid.UUID, rubric.AssessmentSnapshot, submissions.Page, error) {
	author := actor
	if c.Action == "reopen" {
		if !a.organiser || c.AuthorID == uuid.Nil || !reason(c.Reason) {
			return author, rubric.AssessmentSnapshot{}, submissions.Page{}, access.ErrForbidden
		}
		author = c.AuthorID
	} else if c.AuthorID != uuid.Nil && c.AuthorID != actor {
		return author, rubric.AssessmentSnapshot{}, submissions.Page{}, access.ErrForbidden
	}
	snapshot, p, err := s.assessment(ctx, tx, author, eventID, runID, teamID)
	return author, snapshot, p, err
}

func (s Service) codeEvidence(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, teamID uuid.UUID,
	p submissions.Page,
) (FreezeEvidence, error) {
	e, err := (submissionfreeze.Service{Pool: s.Pool, Now: s.now}).EvidenceForFinalInTransaction(
		ctx,
		tx,
		actor,
		eventID,
		teamID,
		p.EntryID,
		p.Final.Revision,
	)
	if err != nil {
		return FreezeEvidence{}, failure("exact reviewed code evidence", err)
	}
	if e.Status != "not_applicable" &&
		(e.Status != "completed" || e.DecisionID == nil || e.SHA == "") {
		return FreezeEvidence{}, access.ErrConflict
	}
	return freezeEvidence(e), nil
}

func sameID(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func sameFreeze(a, b FreezeEvidence) bool {
	if a.Status == "not_applicable" && b.Status == "not_applicable" {
		return true
	}
	return a.SourceID == b.SourceID && a.Status == b.Status && a.SHA == b.SHA &&
		sameID(a.DecisionID, b.DecisionID)
}

func (s Service) updatedCard(
	card *Card,
	version int64,
	actor, runID, teamID uuid.UUID,
	snapshot rubric.AssessmentSnapshot,
	p submissions.Page,
	repository FreezeEvidence,
	c CardCommand,
) (*Card, error) {
	switch c.Action {
	case "save", "submit":
		if card != nil && card.State == "submitted" {
			return nil, access.ErrConflict
		}
		scores, err := normalize(
			c.Scores,
			snapshot.Definition.PublicDefinition,
			p.Final.Document,
			c.Action == "submit",
		)
		if err != nil {
			return nil, err
		}
		state := "draft"
		if c.Action == "submit" {
			state = "submitted"
		}
		card = &Card{
			Repository:         repository,
			RunID:              runID,
			TeamID:             teamID,
			AuthorID:           actor,
			Version:            version + 1,
			State:              state,
			SubmissionRevision: p.Final.Revision,
			Rubric:             snapshot.Reference,
			Scores:             scores,
			ChangedAt:          s.now(),
		}
	case "reopen":
		if card == nil || card.State != "submitted" || len(c.Scores) > 0 {
			return nil, access.ErrConflict
		}
		card.Version++
		if card.SubmissionRevision != p.Final.Revision ||
			!sameFreeze(card.Repository, repository) {
			card.Scores = []Score{}
			card.SubmissionRevision = p.Final.Revision
			card.Rubric = snapshot.Reference
			card.Repository = repository
		}
		card.State = "draft"
		card.ChangedAt = s.now()
	default:
		return nil, access.ErrInvalid
	}
	return card, nil
}
