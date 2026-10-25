package qualification

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/mentors"
	"github.com/deploymenttheory/hack-in-a-box/internal/moderation"
)

func historicalBatch(
	ctx context.Context,
	tx pgx.Tx,
	eventID, runID uuid.UUID,
	before *int64,
) ([]Round, error) {
	rows, err := tx.Query(
		ctx,
		"SELECT document FROM mentor_qualification_rounds WHERE event_id=$1 AND run_id=$2 AND ($3::bigint IS NULL OR version<$3) ORDER BY version DESC LIMIT 100",
		eventID,
		runID,
		before,
	)
	if err != nil {
		return nil, failure("page historical rounds", err)
	}
	defer rows.Close()
	out := []Round{}
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			return nil, failure("read historical round", err)
		}
		var round Round
		if err = json.Unmarshal(raw, &round); err != nil {
			return nil, failure("decode historical round", err)
		}
		out = append(out, round)
	}
	return out, failure("page historical rounds", rows.Err())
}

// History is an original-audience projection of retained versions, never a
// source of finalist permission or an original request acknowledgement. Native
// source authorization remains current; historical freeze identities stay as
// authored and are never rebound to newly observed code.
func (s Service) History(
	ctx context.Context,
	actor, eventID, runID uuid.UUID,
	before *int64,
	limit int32,
) (History, error) {
	out := History{Items: []HistoricalRound{}}
	if limit < 1 || limit > 100 || before != nil && *before < 1 {
		return out, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		v, err := s.guard(ctx, tx, actor, eventID, runID, historicalRead)
		if err != nil {
			return err
		}
		if !panelReader(actor, v.round) {
			return access.ErrForbidden
		}
		latest, err := s.page(ctx, tx, actor, eventID, v)
		if err != nil {
			return err
		}
		if latest.Preview != nil &&
			(slices.Contains(latest.Preview.Blockers, "current_panel_scope_required") || slices.Contains(latest.Preview.Blockers, "current_panel_required")) {
			return nil
		}
		// This metadata cache is bounded to this same event-locked transaction.
		sources := map[uuid.UUID]*moderation.QualificationSource{}
		cursor := before
		for len(out.Items) <= int(limit) {
			batch, err := historicalBatch(ctx, tx, eventID, runID, cursor)
			if err != nil {
				return err
			}
			for _, round := range batch {
				version := round.Version
				cursor = &version
				if !panelReader(actor, round) {
					continue
				}
				if err = validatePanel(ctx, dbgen.New(tx), eventID, round.Panel); err != nil {
					if privateDenial(err) {
						continue
					}
					return err
				}
				projected, err := s.historicalRound(ctx, tx, actor, eventID, round, sources)
				if err != nil {
					return err
				}
				out.Items = append(
					out.Items,
					HistoricalRound{
						Snapshot: projected,
						Current:  round.Version == v.round.Version && latest.Current,
					},
				)
				if len(out.Items) > int(limit) {
					break
				}
			}
			if len(batch) < 100 {
				break
			}
		}
		if len(out.Items) > int(limit) {
			out.Items = out.Items[:limit]
			version := out.Items[len(out.Items)-1].Snapshot.Version
			out.NextCursor = &version
		}
		return nil
	})
	return out, failure("historical qualification", err)
}

func privateDenial(err error) bool {
	return errors.Is(err, access.ErrForbidden) || errors.Is(err, access.ErrNotFound) ||
		errors.Is(err, access.ErrConflict)
}

func (s Service) historicalRound(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	round Round,
	sources map[uuid.UUID]*moderation.QualificationSource,
) (Round, error) {
	cards := []FinalCard{}
	for _, card := range round.Cards {
		if err := mentors.AuthorizeQualificationPanelInTransaction(
			ctx,
			tx,
			actor,
			eventID,
			card.TeamID,
		); err != nil {
			if privateDenial(err) {
				continue
			}
			return Round{}, failure("historical card reader", err)
		}
		source, seen := sources[card.TeamID]
		if !seen {
			current, err := s.Moderation.QualificationSourceInTransaction(
				ctx,
				tx,
				eventID,
				card.TeamID,
				round.RunID,
			)
			if err != nil && !privateDenial(err) {
				return Round{}, failure("historical native proof", err)
			}
			if err == nil {
				source = &current
			}
			sources[card.TeamID] = source
		}
		if source == nil || source.ModeratorID != card.ActorID ||
			digest(source) != card.Source.Binding {
			continue
		}
		valid := true
		for _, score := range card.Scores {
			if len(score.Evidence) == 0 {
				continue
			}
			if err := s.Cards.AuthorizeModerationEvidenceInTransaction(
				ctx,
				tx,
				card.ActorID,
				eventID,
				card.TeamID,
				score.Evidence,
				&card.Source.SubmissionRevision,
			); err != nil {
				if !privateDenial(err) {
					return Round{}, failure("historical authored evidence", err)
				}
				valid = false
				break
			}
		}
		if valid {
			cards = append(cards, card)
		}
	}
	if len(cards) != len(round.Cards) {
		round.Reason = ""
		round.Proposal = nil
		round.Votes = []Vote{}
		round.Decisions = []TieDecision{}
		round.Signoff = nil
	}
	round.Cards = cards
	if err := s.projectRoundEvidence(ctx, tx, actor, eventID, &round); err != nil {
		return Round{}, err
	}
	return round, nil
}
