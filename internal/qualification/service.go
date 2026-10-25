package qualification

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/mentors"
)

func (s Service) Read(ctx context.Context, actor, eventID, runID uuid.UUID) (Page, error) {
	var out Page
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		v, err := s.guard(ctx, tx, actor, eventID, runID, currentRead)
		if err != nil {
			return failure("service", err)
		}
		out, err = s.page(ctx, tx, actor, eventID, v)
		return failure("service", err)
	})
	return out, failure("service", err)
}

func (s Service) Change(
	ctx context.Context,
	actor, eventID, runID uuid.UUID,
	expected int64,
	input Command,
) (Page, error) {
	var out Page
	if input.RequestID == uuid.Nil || expected < 0 || input.Version < 0 {
		return out, access.ErrInvalid
	}
	if strings.TrimSpace(input.Reason) == "" || len(input.Reason) > 2000 {
		return out, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		v, err := s.guard(ctx, tx, actor, eventID, runID, currentWrite)
		if err != nil {
			return failure("service", err)
		}
		fingerprint := digest(struct {
			Command          Command
			ExpectedRevision int64
		}{input, expected})
		original, err := originalReceipt(ctx, tx, actor, eventID, runID, input.RequestID)
		if err != nil {
			return err
		}
		if original != nil {
			if original.Fingerprint != fingerprint {
				return access.ErrConflict
			}
			out, err = s.page(ctx, tx, actor, eventID, v)
			out.Receipt = original
			return err
		}
		if v.row.Revision != expected || v.round.Version != input.Version {
			return access.ErrConflict
		}
		if v.round.Signoff != nil {
			return access.ErrConflict
		}
		if err = s.applyCommand(ctx, tx, actor, eventID, &v, input); err != nil {
			return err
		}
		if err = s.retain(ctx, tx, eventID, actor, v.row.Revision, &v.round, input); err != nil {
			return failure("service", err)
		}
		receipt := Receipt{
			RequestID:        input.RequestID,
			RunID:            runID,
			Action:           input.Action,
			ExpectedRevision: expected,
			ExpectedVersion:  input.Version,
			Version:          v.round.Version,
			Fingerprint:      fingerprint,
		}
		if err = retainReceipt(ctx, tx, actor, eventID, receipt); err != nil {
			return err
		}
		v.row, err = dbgen.New(tx).Event(ctx, eventID)
		if err != nil {
			return failure("service", err)
		}
		out, err = s.page(ctx, tx, actor, eventID, v)
		out.Receipt = &receipt
		return failure("service", err)
	})
	if err != nil {
		return Page{}, fmt.Errorf("change mentor qualification: %w", err)
	}
	return out, nil
}

func invalidate(round *Round) {
	round.Proposal = nil
	round.Votes = []Vote{}
	round.Decisions = []TieDecision{}
	round.Signoff = nil
}

func panelReader(actor uuid.UUID, round Round) bool {
	return actor == round.Panel.OrganiserID || slices.Contains(round.Panel.MentorIDs, actor)
}

func (s Service) page(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	v scope,
) (Page, error) {
	out := Page{
		Revision:   v.row.Revision,
		Round:      v.round,
		Criteria:   v.criteria,
		Teams:      []TeamStatus{},
		TieActions: []TieAction{},
	}
	capabilities, err := s.baseCapabilities(ctx, tx, actor, eventID, v)
	if err != nil {
		return Page{}, err
	}
	out.Capabilities = capabilities
	if v.round.Version == 0 {
		return out, nil
	}
	if err := validatePanel(ctx, dbgen.New(tx), eventID, v.round.Panel); err != nil {
		clearPrivate(&out.Round)
		p := emptyProposal()
		p.Blockers = append(p.Blockers, "current_panel_required")
		out.Preview = &p
		return out, nil
	}
	teams, err := s.cohort(ctx, tx, eventID, v)
	if err != nil {
		return Page{}, err
	}
	readable, err := s.cohortReadable(ctx, tx, actor, eventID, v, teams)
	if err != nil {
		return Page{}, err
	}
	if !readable {
		clearPrivate(&out.Round)
		preview := emptyProposal()
		preview.Blockers = append(preview.Blockers, "current_panel_scope_required")
		out.Preview = &preview
		return out, nil
	}

	for _, team := range teams {
		out.Teams = append(out.Teams, s.teamActions(ctx, tx, actor, eventID, v, team))
	}
	cards := []FinalCard{}
	for _, card := range v.round.Cards {
		if !panelReader(actor, v.round) && actor != card.ActorID {
			continue
		}
		team, err := findTeam(teams, card.TeamID)
		if err != nil {
			continue
		}
		if mentors.AuthorizeQualificationPanelInTransaction(
			ctx,
			tx,
			actor,
			eventID,
			card.TeamID,
		) != nil ||
			!s.cardCurrent(ctx, tx, eventID, card, team) {
			continue
		}
		cards = append(cards, card)
	}
	out.Round.Cards = cards
	if !panelReader(actor, v.round) {
		out.Round.Proposal = nil
		out.Round.Eligibility = []Eligibility{}
		out.Round.Votes = []Vote{}
		out.Round.Decisions = []TieDecision{}
		out.Round.Signoff = nil
		if out.Round.ActorID != actor {
			out.Round.Reason = ""
		}
		return out, s.projectRoundEvidence(ctx, tx, actor, eventID, &out.Round)
	}
	fresh, err := s.proposal(ctx, tx, eventID, v)
	if err != nil {
		return Page{}, err
	}
	out.Preview = &fresh
	out.Capabilities.CanReadPanel = true
	out.Capabilities.CanPropose = writable(v) && actor == v.round.Panel.OrganiserID
	out.Capabilities.CanSignOff = out.Capabilities.CanPropose && v.round.Proposal != nil &&
		v.round.Proposal.Binding == fresh.Binding &&
		len(fresh.Blockers) == 0
	out.TieActions = s.tieActions(ctx, tx, actor, eventID, v, fresh, teams)
	if v.round.Proposal != nil {
		sourcesCurrent := fresh.Binding == v.round.Proposal.Binding
		out.Current = v.currentRun && sourcesCurrent && len(fresh.Blockers) == 0 &&
			v.round.Signoff != nil &&
			signedBinding(fresh, v.round) == v.round.Signoff.Binding
		if !sourcesCurrent {
			out.Round.Proposal = nil
			out.Round.Votes = []Vote{}
			out.Round.Decisions = []TieDecision{}
		}
		if !out.Current {
			out.Round.Signoff = nil
		}
	}
	return out, s.projectRoundEvidence(ctx, tx, actor, eventID, &out.Round)
}

func clearPrivate(round *Round) {
	round.Cards = []FinalCard{}
	round.Eligibility = []Eligibility{}
	round.Proposal = nil
	round.Votes = []Vote{}
	round.Decisions = []TieDecision{}
	round.Signoff = nil
	round.Reason = ""
}

func (s Service) applyCommand(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	v *scope,
	input Command,
) error {
	var err error
	switch input.Action {
	case "configure_panel":
		if !v.organiser || input.Panel == nil {
			return access.ErrForbidden
		}
		if err = (access.EventPolicy{Queries: dbgen.New(tx)}).Authorize(
			ctx,
			actor,
			eventID,
			eventID,
			access.EventEdit,
		); err != nil {
			return failure("service", err)
		}
		if err = validatePanel(ctx, dbgen.New(tx), eventID, *input.Panel); err != nil {
			return failure("service", err)
		}
		v.round.Panel = *input.Panel
		invalidate(&v.round)
	case "save_card", "sign_card":
		if err = s.changeCard(ctx, tx, actor, eventID, v, input); err != nil {
			return failure("service", err)
		}
		invalidate(&v.round)
	case "decide_eligibility":
		if err = s.decideEligibility(ctx, tx, actor, eventID, v, input); err != nil {
			return failure("service", err)
		}
		invalidate(&v.round)
	case "propose":
		if actor != v.round.Panel.OrganiserID {
			return access.ErrForbidden
		}
		if err = validatePanel(ctx, dbgen.New(tx), eventID, v.round.Panel); err != nil {
			return failure("service", err)
		}
		proposal, err := s.proposal(ctx, tx, eventID, *v)
		if err != nil {
			return failure("service", err)
		}
		v.round.Proposal = &proposal
		v.round.Votes = []Vote{}
		v.round.Decisions = []TieDecision{}
	case "vote_tie", "resolve_tie":
		if err = s.changeTie(ctx, tx, actor, eventID, v, input); err != nil {
			return failure("service", err)
		}
	case "sign_off":
		if err = s.signOff(ctx, tx, actor, eventID, v, input); err != nil {
			return failure("service", err)
		}
	default:
		return access.ErrInvalid
	}
	return nil
}
