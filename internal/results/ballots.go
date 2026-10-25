package results

import (
	"context"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/judging"
)

// BallotPage exposes current assigned ties, the caller's own ballot and
// separately authored current mentor advisory. It cannot carry raw cards,
// qualification data or another judge's vote prose.
type BallotPage struct {
	JudgeRunID    uuid.UUID  `json:"judge_run_id"`
	Revision      int64      `json:"revision"`
	Version       int64      `json:"version"`
	SourceBinding string     `json:"source_binding"`
	Ties          []JudgeTie `json:"ties"`
	Receipt       *Receipt   `json:"receipt"`
}
type JudgeTie struct {
	State      TieState         `json:"state"`
	OwnVote    *Vote            `json:"own_vote"`
	Advisories []MentorAdvisory `json:"advisories"`
}

// ballotSource composes the source using the recorded preparer's current
// organiser authority. That internal source never becomes a judge projection.
// Every candidate is independently checked under the actual caller below.
func (s Service) ballotSource(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
	round Round,
) (assessmentSource, error) {
	if round.PreparedBy == uuid.Nil || round.Source.Binding == "" || s.Judging == nil {
		return assessmentSource{}, access.ErrConflict
	}
	a, err := s.guard(ctx, tx, round.PreparedBy, eventID, false)
	if err != nil {
		return assessmentSource{}, err
	}
	if !a.organiser {
		return assessmentSource{}, access.ErrForbidden
	}
	source, err := s.source(ctx, tx, round.PreparedBy, eventID)
	if err != nil {
		return source, err
	}
	if source.completion == nil || len(source.preview.Blockers) != 0 ||
		source.preview.Binding != round.Source.Binding {
		return source, access.ErrConflict
	}
	return source, nil
}

func (s Service) ballotPage(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	current authority,
	round Round,
	source assessmentSource,
) (BallotPage, error) {
	out := BallotPage{
		JudgeRunID:    source.preview.JudgeRunID,
		Revision:      current.event.Revision,
		Version:       round.Version,
		SourceBinding: source.preview.Binding,
		Ties:          []JudgeTie{},
	}
	states, err := resolveJudgeTies(source.preview.Ties, source.completion.Assignments, round.Votes)
	if err != nil {
		return out, err
	}
	advice, err := currentAdvisories(ctx, tx, eventID, source)
	if err != nil {
		return out, err
	}
	for _, state := range states {
		if !slices.Contains(state.RequiredJudgeIDs, actor) {
			continue
		}
		for _, team := range state.Tie.TeamIDs {
			if err := s.Judging.AuthorizeResultTargetInTransaction(
				ctx,
				tx,
				actor,
				eventID,
				source.preview.JudgeRunID,
				team,
			); err != nil {
				return BallotPage{}, failure("current ballot target", err)
			}
		}
		item := JudgeTie{State: state, Advisories: []MentorAdvisory{}}
		for _, a := range advice {
			if state.Tie.Kind == "category" && a.TieID == state.Tie.ID &&
				slices.Contains(state.Tie.TeamIDs, a.TeamID) {
				item.Advisories = append(item.Advisories, a)
			}
		}
		for _, vote := range round.Votes {
			if vote.JudgeID == actor && vote.TieID == state.Tie.ID {
				own := vote
				item.OwnVote = &own
			}
		}
		out.Ties = append(out.Ties, item)
	}
	return out, nil
}

func (s Service) Ballots(ctx context.Context, actor, eventID uuid.UUID) (BallotPage, error) {
	var out BallotPage
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		current, err := s.guard(ctx, tx, actor, eventID, false)
		if err != nil {
			return err
		}
		if !current.judge {
			return access.ErrForbidden
		}
		round, err := load(ctx, tx, eventID)
		if err != nil {
			return err
		}
		source, err := s.ballotSource(ctx, tx, eventID, round)
		if err != nil {
			return err
		}
		out, err = s.ballotPage(ctx, tx, actor, eventID, current, round, source)
		return err
	})
	if err != nil {
		return BallotPage{}, failure("read own final ballots", err)
	}
	return out, nil
}

// Vote records an explicit replacement of this judge's ballot. Previous ballot
// bytes remain in immutable versions. A split never triggers an implicit vote.
func (s Service) Vote(
	ctx context.Context,
	actor, eventID uuid.UUID,
	expected int64,
	input Command,
) (BallotPage, error) {
	var out BallotPage
	if err := validateVote(expected, input); err != nil {
		return out, err
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		current, err := s.guard(ctx, tx, actor, eventID, true)
		if err != nil {
			return err
		}
		if !current.judge {
			return access.ErrForbidden
		}
		round, err := load(ctx, tx, eventID)
		if err != nil {
			return err
		}
		source, err := s.ballotSource(ctx, tx, eventID, round)
		if err != nil {
			return err
		}
		page, err := s.ballotPage(ctx, tx, actor, eventID, current, round, source)
		if err != nil {
			return err
		}
		index := slices.IndexFunc(
			page.Ties,
			func(t JudgeTie) bool { return t.State.Tie.ID == input.TieID },
		)
		if index < 0 {
			return access.ErrForbidden
		}
		tie := page.Ties[index].State.Tie
		if !slices.Contains(tie.TeamIDs, input.TeamID) ||
			input.SourceBinding != source.preview.Binding {
			return access.ErrConflict
		}
		fingerprint := digest(struct {
			Command          Command
			ExpectedRevision int64
		}{input, expected})
		original, err := originalReceipt(ctx, tx, actor, eventID, input.RequestID)
		if err != nil {
			return err
		}
		if original != nil {
			if original.Fingerprint != fingerprint {
				return access.ErrConflict
			}
			out = page
			out.Receipt = original
			return nil
		}
		if expected != current.event.Revision || input.Version != round.Version {
			return access.ErrConflict
		}
		byTeam := map[uuid.UUID][]judging.Evidence{}
		for _, e := range input.Evidence {
			if !slices.Contains(tie.TeamIDs, e.TeamID) {
				return access.ErrInvalid
			}
			byTeam[e.TeamID] = append(
				byTeam[e.TeamID],
				judging.Evidence{DeliverableID: e.DeliverableID, Note: e.Note},
			)
		}
		for team, evidence := range byTeam {
			if err := s.Judging.AuthorizeResultEvidenceInTransaction(
				ctx,
				tx,
				actor,
				eventID,
				source.preview.JudgeRunID,
				team,
				evidence,
			); err != nil {
				return failure("current ballot evidence", err)
			}
		}
		vote := Vote{
			TieID:    input.TieID,
			TeamID:   input.TeamID,
			JudgeID:  actor,
			Evidence: input.Evidence,
			Reason:   input.Reason,
			At:       s.now(),
		}
		round.Votes = slices.DeleteFunc(
			round.Votes,
			func(v Vote) bool { return v.TieID == input.TieID && v.JudgeID == actor },
		)
		round.Votes = append(round.Votes, vote)
		round.Confirmation = nil
		if err = s.retain(
			ctx,
			tx,
			eventID,
			actor,
			current.event.Revision,
			&round,
			input,
		); err != nil {
			return err
		}
		receipt := Receipt{
			RunID:            source.preview.JudgeRunID,
			TieID:            tie.ID,
			TeamIDs:          slices.Clone(tie.TeamIDs),
			RequestID:        input.RequestID,
			ActorID:          actor,
			Action:           input.Action,
			ExpectedRevision: expected,
			ExpectedVersion:  input.Version,
			Version:          round.Version,
			Fingerprint:      fingerprint,
		}
		if err = retainReceipt(ctx, tx, eventID, receipt); err != nil {
			return err
		}
		current.event, err = dbgen.New(tx).Event(ctx, eventID)
		if err != nil {
			return failure("read event after vote", err)
		}
		out, err = s.ballotPage(ctx, tx, actor, eventID, current, round, source)
		out.Receipt = &receipt
		return err
	})
	if err != nil {
		return BallotPage{}, failure("record final judge vote", err)
	}
	return out, nil
}

func validateVote(expected int64, input Command) error {
	if input.Action != "vote" || input.RequestID == uuid.Nil || expected < 0 || input.Version < 0 ||
		input.TieID == "" || input.TeamID == uuid.Nil || strings.TrimSpace(input.Reason) == "" ||
		len(
			input.Reason,
		) > 2000 || len(input.Evidence) == 0 || len(input.Evidence) > 30 || len(input.Awards) != 0 {
		return access.ErrInvalid
	}
	return nil
}
