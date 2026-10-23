package scorecards

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/mentors"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
)

// Coverage is coordination metadata. It contains no values, rubric category
// identities, private reasons, evidence references or qualification totals.
type Coverage struct {
	Availability string               `json:"availability"`
	Items        []CheckpointCoverage `json:"items"`
	NextCursor   *uuid.UUID           `json:"next_cursor"`
}
type CheckpointCoverage struct {
	ID       uuid.UUID        `json:"id"`
	Version  int64            `json:"version"`
	Name     string           `json:"name"`
	Purpose  string           `json:"purpose"`
	RunID    uuid.UUID        `json:"run_id"`
	OpensAt  time.Time        `json:"opens_at"`
	ClosesAt time.Time        `json:"closes_at"`
	Reviews  []ReviewCoverage `json:"reviews"`
}
type ReviewCoverage struct {
	PersonID          *uuid.UUID `json:"person_id"`
	Name              string     `json:"name"`
	Slot              string     `json:"slot"`
	State             string     `json:"state"`
	PendingCategories *int       `json:"pending_categories"`
	MissingEvidence   *int       `json:"missing_evidence"`
	Overdue           bool       `json:"overdue"`
}

type coverageTarget struct {
	person     uuid.UUID
	slot, name string
}

// CoverageInTransaction allows an organiser coordination states for the current
// reviewers. A mentor sees only their own independent completion metadata;
// cross-author card contents still require the recorded moderation policy.
func (s Service) CoverageInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, teamID, after uuid.UUID,
	count int,
) (Coverage, error) {
	out := Coverage{Availability: "not_open", Items: []CheckpointCoverage{}}
	size, err := limit(count)
	if err != nil {
		return out, err
	}
	q := dbgen.New(tx)
	row, err := q.LockEvent(ctx, eventID)
	if err != nil {
		return out, fmt.Errorf("lock assessment coverage: %w", err)
	}
	if err = mentors.AuthorizeModerationPanelInTransaction(
		ctx,
		tx,
		actor,
		eventID,
		teamID,
	); err != nil {
		return out, fmt.Errorf("authorize current coordination panel scope: %w", err)
	}
	roles, _, err := access.ContentReader(ctx, q, actor, eventID)
	if err != nil {
		return out, fmt.Errorf("authorize assessment coordination reader: %w", err)
	}
	team, err := q.HackTeam(ctx, dbgen.HackTeamParams{EventID: eventID, ID: teamID})
	if err != nil {
		return out, fmt.Errorf("read assessment coverage team: %w", err)
	}
	entry, err := q.CompetitionEntry(
		ctx,
		dbgen.CompetitionEntryParams{EventID: eventID, TeamID: teamID},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		out.Availability = "not_applicable"
		return out, nil
	}
	if err != nil {
		return out, fmt.Errorf("read current assessment entry: %w", err)
	}
	if team.State != "published" || !entry.Active {
		out.Availability = "not_applicable"
		return out, nil
	}
	state, stateErr := q.SubmissionState(
		ctx,
		dbgen.SubmissionStateParams{EventID: eventID, TeamID: teamID},
	)
	if stateErr != nil && !errors.Is(stateErr, pgx.ErrNoRows) {
		return out, fmt.Errorf("read native final assessment outcome: %w", stateErr)
	}
	if stateErr == nil && (state.Outcome == "withdrawn" || state.Outcome == "not_submitted") {
		out.Availability = "not_applicable"
		return out, nil
	}
	var dates event.Configuration
	if err = json.Unmarshal(row.Configuration, &dates); err != nil {
		return out, fmt.Errorf("read current assessment coordination dates: %w", err)
	}
	if !row.PublishedAt.Valid || s.now().Before(row.PublishedAt.Time) || dates.LiveStart.IsZero() ||
		s.now().Before(dates.LiveStart) {
		return out, nil
	}
	common, err := q.EventRubric(ctx, eventID)
	if errors.Is(err, pgx.ErrNoRows) {
		out.Availability = "not_configured"
		return out, nil
	}
	if err != nil {
		return out, fmt.Errorf("read current published assessment source: %w", err)
	}
	if !common.PublishedVersion.Valid {
		return out, nil
	}
	out.Availability = "current"
	verifiedRuns := map[uuid.UUID]bool{}
	targets, err := coverageTargets(
		ctx,
		q,
		actor,
		eventID,
		teamID,
		slices.Contains(roles, "organiser"),
	)
	if err != nil {
		return out, err
	}
	rows, err := q.ScorecardCheckpoints(
		ctx,
		dbgen.ScorecardCheckpointsParams{EventID: eventID, AfterID: after, PageSize: size + 1},
	)
	if err != nil {
		return out, fmt.Errorf("page current assessment coordination: %w", err)
	}
	if len(rows) > int(size) {
		rows = rows[:size]
		id := rows[len(rows)-1].ID
		out.NextCursor = &id
	}
	for _, raw := range rows {
		item, reviewRequired, itemErr := s.coordinationCheckpoint(
			ctx,
			tx,
			row,
			common,
			teamID,
			raw.Document,
			targets,
			verifiedRuns,
		)
		if itemErr != nil {
			return out, itemErr
		}
		if reviewRequired {
			out.Availability = "review_required"
		}
		if item != nil {
			out.Items = append(out.Items, *item)
		}
	}
	return out, nil
}

func coverageTargets(
	ctx context.Context,
	q *dbgen.Queries,
	actor, eventID, teamID uuid.UUID,
	organiser bool,
) ([]coverageTarget, error) {
	out := []coverageTarget{}
	rows, err := q.MentorAssignments(ctx, eventID)
	if err != nil {
		return out, fmt.Errorf("read current published assessment reviewers: %w", err)
	}
	for _, slot := range []string{"primary", "second"} {
		target := coverageTarget{slot: slot}
		for _, row := range rows {
			if row.TeamID == teamID && row.Snapshot == "published" && row.Slot == slot {
				target.person = row.PersonID
			}
		}
		if !organiser && target.person != actor {
			continue
		}
		if target.person != uuid.Nil {
			person, personErr := q.Person(ctx, target.person)
			if personErr != nil {
				return out, fmt.Errorf("read assessment reviewer identity: %w", personErr)
			}
			target.name = person.DisplayName
		}
		out = append(out, target)
	}
	return out, nil
}

func (s Service) reviewCoverage(
	ctx context.Context,
	q *dbgen.Queries,
	eventID, teamID uuid.UUID,
	cp Checkpoint,
	target coverageTarget,
) (ReviewCoverage, error) {
	out := ReviewCoverage{Slot: target.slot, Name: target.name, State: "missing_assignment"}
	if target.person == uuid.Nil {
		return out, nil
	}
	out.PersonID = &target.person
	out.State = "missing"
	if _, err := s.guard(ctx, q, target.person, eventID, teamID); err != nil {
		if errors.Is(err, access.ErrForbidden) || errors.Is(err, access.ErrNotFound) ||
			errors.Is(err, access.ErrConflict) {
			out.State = "unavailable"
			return out, nil
		}
		return out, err
	}
	out.Overdue = !s.now().Before(cp.ClosesAt)
	if s.now().Before(cp.OpensAt) {
		out.State = "not_open"
		out.Overdue = false
		return out, nil
	}
	row, err := q.LatestMentorScorecard(
		ctx,
		dbgen.LatestMentorScorecardParams{EventID: eventID, ID: cardID(cp, teamID, target.person)},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, fmt.Errorf("read independent completion metadata: %w", err)
	}
	card, err := decodeCard(row.Document)
	if err != nil {
		return out, err
	}
	if card.AuthorID != target.person || card.TeamID != teamID ||
		card.CheckpointVersion != cp.Version ||
		card.RunID != cp.Definition.RunID ||
		card.Rubric != cp.Rubric {
		return out, nil
	}
	if err = s.cardAuthority(ctx, q, target.person, eventID, teamID, card); err != nil {
		if errors.Is(err, access.ErrForbidden) || errors.Is(err, access.ErrNotFound) ||
			errors.Is(err, access.ErrConflict) {
			out.State = "unavailable"
			return out, nil
		}
		return out, err
	}
	pending, missing := 0, 0
	for _, category := range card.Scores {
		if category.Pending {
			pending++
		} else if category.Value == nil || len(category.Evidence) == 0 {
			missing++
		}
	}
	out.State = card.State
	out.PendingCategories = &pending
	out.MissingEvidence = &missing
	out.Overdue = out.Overdue && (card.State != "submitted" || pending > 0 || missing > 0)
	return out, nil
}

func (s Service) coordinationCheckpoint(
	ctx context.Context,
	tx pgx.Tx,
	row dbgen.Event,
	common dbgen.EventRubric,
	teamID uuid.UUID,
	document []byte,
	targets []coverageTarget,
	verifiedRuns map[uuid.UUID]bool,
) (*CheckpointCoverage, bool, error) {
	q := dbgen.New(tx)
	eventID := row.ID
	cp, decodeErr := decodeCheckpoint(document)
	if decodeErr != nil {
		return nil, false, decodeErr
	}
	run, runErr := q.RubricRun(
		ctx,
		dbgen.RubricRunParams{EventID: eventID, ID: cp.Definition.RunID},
	)
	if runErr != nil {
		return nil, false, fmt.Errorf("read coordination assessment run: %w", runErr)
	}
	if run.Status != "current" || cp.Rubric.ID != common.ID ||
		cp.Rubric.Version != common.PublishedVersion.Int64 {
		return nil, false, nil
	}
	if !verifiedRuns[cp.Definition.RunID] {
		if publicationErr := rubric.ValidateRunPublicationInTransaction(
			ctx,
			tx,
			eventID,
			cp.Definition.RunID,
		); publicationErr != nil {
			if errors.Is(publicationErr, access.ErrConflict) ||
				errors.Is(publicationErr, access.ErrForbidden) ||
				errors.Is(publicationErr, access.ErrNotFound) {
				return nil, true, nil
			}
			return nil, false, fmt.Errorf("validate coordination publication: %w", publicationErr)
		}
		verifiedRuns[cp.Definition.RunID] = true
	}
	window, windowErr := resolveWindow(row, cp)
	if windowErr != nil || !window.OpensAt.Equal(cp.OpensAt) ||
		!window.ClosesAt.Equal(cp.ClosesAt) {
		return nil, true, nil
	}
	item := CheckpointCoverage{
		ID:       cp.Definition.ID,
		Version:  cp.Version,
		Name:     cp.Definition.Name,
		Purpose:  cp.Purpose,
		RunID:    cp.Definition.RunID,
		OpensAt:  cp.OpensAt,
		ClosesAt: cp.ClosesAt,
		Reviews:  []ReviewCoverage{},
	}
	for _, target := range targets {
		review, reviewErr := s.reviewCoverage(ctx, q, eventID, teamID, cp, target)
		if reviewErr != nil {
			return nil, false, reviewErr
		}
		item.Reviews = append(item.Reviews, review)
	}
	return &item, false, nil
}
