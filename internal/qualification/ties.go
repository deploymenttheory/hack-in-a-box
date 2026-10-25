package qualification

import (
	"context"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/mentors"
)

func (s Service) ballots(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
	v scope,
	tie Tie,
	seat int,
	remaining []uuid.UUID,
) ([]uuid.UUID, bool) {
	counts := map[uuid.UUID]int{}
	seen := map[uuid.UUID]bool{}
	for _, vote := range v.round.Votes {
		if vote.TieID != tie.ID || vote.Seat != seat {
			continue
		}
		if !slices.Contains(v.round.Panel.MentorIDs, vote.ActorID) || seen[vote.ActorID] ||
			!slices.Contains(remaining, vote.TeamID) {
			return nil, false
		}
		for _, id := range remaining {
			if err := mentors.AuthorizeQualificationPanelInTransaction(
				ctx,
				tx,
				vote.ActorID,
				eventID,
				id,
			); err != nil {
				return nil, false
			}
		}
		teamList, err := s.cohort(ctx, tx, eventID, v)
		if err != nil {
			return nil, false
		}
		team, err := findTeam(teamList, vote.TeamID)
		if err != nil || team.source == nil {
			return nil, false
		}
		if err = s.Cards.AuthorizeModerationEvidenceInTransaction(
			ctx,
			tx,
			vote.ActorID,
			eventID,
			vote.TeamID,
			vote.Evidence,
			&team.source.SubmissionRevision,
		); err != nil {
			return nil, false
		}
		counts[vote.TeamID]++
		seen[vote.ActorID] = true
	}
	if len(seen) != len(v.round.Panel.MentorIDs) {
		return nil, false
	}
	highest := 0
	winners := []uuid.UUID{}
	for _, id := range remaining {
		count := counts[id]
		if count > highest {
			highest = count
			winners = []uuid.UUID{id}
		} else if count == highest && count > 0 {
			winners = append(winners, id)
		}
	}
	return winners, highest > 0
}

func (s Service) resolved(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
	v scope,
	tie Tie,
) ([]uuid.UUID, bool) {
	result := []uuid.UUID{}
	for seat := 1; seat <= tie.Seats; seat++ {
		index := slices.IndexFunc(
			v.round.Decisions,
			func(d TieDecision) bool { return d.TieID == tie.ID && d.Seat == seat },
		)
		if index < 0 {
			return nil, false
		}
		decision := v.round.Decisions[index]
		remaining := without(tie.TeamIDs, result)
		winners, valid := s.ballots(ctx, tx, eventID, v, tie, seat, remaining)
		if !valid || decision.ActorID != v.round.Panel.OrganiserID ||
			!slices.Contains(winners, decision.TeamID) ||
			decision.Casting != (len(winners) > 1) {
			return nil, false
		}
		for _, id := range remaining {
			if err := mentors.AuthorizeQualificationPanelInTransaction(
				ctx,
				tx,
				decision.ActorID,
				eventID,
				id,
			); err != nil {
				return nil, false
			}
		}
		teams, err := s.cohort(ctx, tx, eventID, v)
		if err != nil {
			return nil, false
		}
		team, err := findTeam(teams, decision.TeamID)
		if err != nil || team.source == nil {
			return nil, false
		}
		if err = s.Cards.AuthorizeModerationEvidenceInTransaction(
			ctx,
			tx,
			decision.ActorID,
			eventID,
			decision.TeamID,
			decision.Evidence,
			&team.source.SubmissionRevision,
		); err != nil {
			return nil, false
		}
		result = append(result, decision.TeamID)
	}
	return result, true
}

func (s Service) changeTie(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	v *scope,
	input Command,
) error {
	if v.round.Proposal == nil {
		return access.ErrConflict
	}
	fresh, err := s.proposal(ctx, tx, eventID, *v)
	if err != nil {
		return failure("ties", err)
	}
	if fresh.Binding != v.round.Proposal.Binding || !unresolvedOnly(fresh) {
		return access.ErrConflict
	}
	index := slices.IndexFunc(fresh.Ties, func(t Tie) bool { return t.ID == input.TieID })
	if index < 0 {
		return access.ErrNotFound
	}
	tie := fresh.Ties[index]
	seat := nextSeat(v.round, tie)
	if input.Seat != seat || seat > tie.Seats {
		return access.ErrConflict
	}
	resolved := []uuid.UUID{}
	for _, decision := range v.round.Decisions {
		if decision.TieID == tie.ID {
			resolved = append(resolved, decision.TeamID)
		}
	}
	remaining := without(tie.TeamIDs, resolved)
	if !slices.Contains(remaining, input.TeamID) {
		return access.ErrInvalid
	}
	for _, id := range remaining {
		if err = mentors.AuthorizeQualificationPanelInTransaction(
			ctx,
			tx,
			actor,
			eventID,
			id,
		); err != nil {
			return failure("ties", err)
		}
	}
	teams, err := s.cohort(ctx, tx, eventID, *v)
	if err != nil {
		return failure("ties", err)
	}
	team, err := findTeam(teams, input.TeamID)
	if err != nil || team.source == nil {
		return access.ErrConflict
	}
	if err = s.Cards.AuthorizeModerationEvidenceInTransaction(
		ctx,
		tx,
		actor,
		eventID,
		input.TeamID,
		input.Evidence,
		&team.source.SubmissionRevision,
	); err != nil {
		return failure("ties", err)
	}
	if input.Action == "vote_tie" {
		if !slices.Contains(v.round.Panel.MentorIDs, actor) {
			return access.ErrForbidden
		}
		if slices.ContainsFunc(
			v.round.Votes,
			func(vote Vote) bool { return vote.TieID == tie.ID && vote.Seat == seat && vote.ActorID == actor },
		) {
			return access.ErrConflict
		}
		v.round.Votes = append(
			v.round.Votes,
			Vote{
				TieID:    tie.ID,
				Seat:     seat,
				TeamID:   input.TeamID,
				ActorID:  actor,
				Reason:   input.Reason,
				Evidence: input.Evidence,
				At:       s.now(),
			},
		)
	} else {
		if actor != v.round.Panel.OrganiserID {
			return access.ErrForbidden
		}
		winners, valid := s.ballots(ctx, tx, eventID, *v, tie, seat, remaining)
		if !valid || !slices.Contains(winners, input.TeamID) {
			return access.ErrConflict
		}
		v.round.Decisions = append(
			v.round.Decisions,
			TieDecision{
				TieID:    tie.ID,
				Seat:     seat,
				TeamID:   input.TeamID,
				ActorID:  actor,
				Casting:  len(winners) > 1,
				Reason:   input.Reason,
				Evidence: input.Evidence,
				At:       s.now(),
			},
		)
	}
	fresh, err = s.proposal(ctx, tx, eventID, *v)
	if err != nil {
		return failure("ties", err)
	}
	v.round.Proposal = &fresh
	return nil
}
