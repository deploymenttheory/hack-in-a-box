package judging

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// CardState is organiser coordination metadata, never an assessment document.
// The opaque current decision UUID is the exact write precondition for reopen;
// no historical or current code source, SHA, artifact IDs or private prose cross.
type CardState struct {
	RunID                uuid.UUID  `json:"run_id"`
	TeamID               uuid.UUID  `json:"team_id"`
	AuthorID             uuid.UUID  `json:"author_id"`
	Version              int64      `json:"version"`
	State                string     `json:"state"`
	SourceAvailable      bool       `json:"source_available"`
	SourceChanged        bool       `json:"source_changed"`
	CanReopen            bool       `json:"can_reopen"`
	SubmissionRevision   int64      `json:"submission_revision"`
	RepositoryDecisionID *uuid.UUID `json:"repository_decision_id"`
}
type CardStates struct {
	Items []CardState `json:"items"`
}

func (s Service) CardStates(
	ctx context.Context,
	actor, eventID, runID, teamID uuid.UUID,
) (CardStates, error) {
	out := CardStates{Items: []CardState{}}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		a, err := s.guard(ctx, tx, actor, eventID, false)
		if err != nil {
			return err
		}
		if !a.organiser {
			return access.ErrForbidden
		}
		if _, _, err = loadRun(ctx, tx, eventID, runID); err != nil {
			return err
		}
		entry, err := q.CompetitionEntry(
			ctx,
			dbgen.CompetitionEntryParams{EventID: eventID, TeamID: teamID},
		)
		if err != nil {
			return failure("coordination entry", err)
		}
		team, err := q.HackTeam(ctx, dbgen.HackTeamParams{EventID: eventID, ID: teamID})
		if err != nil {
			return failure("coordination team", err)
		}
		if !entry.Active || team.State != "published" {
			return access.ErrForbidden
		}
		ids, err := assigned(ctx, tx, eventID, teamID)
		if err != nil {
			return err
		}
		writable := a.row.WritesAllowed && a.row.Stage != "cancelled" &&
			a.row.Stage != "archived" &&
			a.row.Stage != "completed"
		for _, author := range ids {
			state, err := s.cardState(ctx, tx, eventID, runID, teamID, author, writable)
			if err != nil {
				return err
			}
			out.Items = append(out.Items, state)
		}
		return nil
	})
	return out, failure("organiser card states", err)
}

func (s Service) cardState(
	ctx context.Context,
	tx pgx.Tx,
	eventID, runID, teamID, author uuid.UUID,
	writable bool,
) (CardState, error) {
	state := CardState{RunID: runID, TeamID: teamID, AuthorID: author, State: "not_started"}
	card, err := readCard(ctx, tx, eventID, runID, teamID, author)
	if err != nil {
		return state, err
	}
	if card == nil {
		return state, nil
	}
	state.Version = card.Version
	state.State = card.State
	snapshot, page, err := s.assessment(ctx, tx, author, eventID, runID, teamID)
	if errors.Is(err, access.ErrForbidden) || errors.Is(err, access.ErrConflict) ||
		errors.Is(err, access.ErrNotFound) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	repository, err := s.codeEvidence(ctx, tx, author, eventID, teamID, page)
	if err != nil {
		return state, err
	}
	state.SourceAvailable = true
	state.SubmissionRevision = page.Final.Revision
	state.RepositoryDecisionID = repository.DecisionID
	state.SourceChanged = card.SubmissionRevision != page.Final.Revision ||
		card.Rubric != snapshot.Reference ||
		!sameFreeze(card.Repository, repository)
	state.CanReopen = writable && card.State == "submitted"
	return state, nil
}
