package scorecards

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

func (s Service) Read(
	ctx context.Context,
	actor, eventID, teamID uuid.UUID,
	after uuid.UUID,
	count int,
) (Page, error) {
	var out Page
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		var err error
		out, err = s.ReadInTransaction(ctx, tx, actor, eventID, teamID, after, count)
		return err
	})
	if err != nil {
		return out, fmt.Errorf("read scorecards transaction: %w", err)
	}
	return out, nil
}

// ReadInTransaction preserves this owner's current field and resource policy
// while composing a read in the caller's event-locked transaction.
func (s Service) ReadInTransaction(ctx context.Context,
	tx pgx.Tx,
	actor, eventID, teamID uuid.UUID,
	after uuid.UUID,
	count int,
) (Page, error) {
	out := Page{Items: []Card{}}
	size, err := limit(count)
	if err != nil {
		return out, err
	}
	q := dbgen.New(tx)
	err = func() error {
		row, err := s.guard(ctx, q, actor, eventID, teamID)
		if err != nil {
			return err
		}
		out.Revision = row.Revision
		cards, err := q.OwnScorecards(
			ctx,
			dbgen.OwnScorecardsParams{
				EventID:  eventID,
				TeamID:   teamID,
				AuthorID: actor,
				AfterID:  after,
				PageSize: size + 1,
			},
		)
		if err != nil {
			return fmt.Errorf("page independent scorecards: %w", err)
		}
		if len(cards) > int(size) {
			cards = cards[:size]
			id := cards[len(cards)-1].ID
			out.NextCursor = &id
		}
		for _, row := range cards {
			card, err := decodeCard(row.Document)
			if err != nil {
				return err
			}
			if err = s.cardAuthority(ctx, q, actor, eventID, teamID, card); err != nil {
				return err
			}
			out.Items = append(out.Items, card)
		}
		return nil
	}()
	if err != nil {
		return Page{}, fmt.Errorf("read own independent cards: %w", err)
	}
	return out, nil
}

func (s Service) History(
	ctx context.Context,
	actor, eventID, teamID, id uuid.UUID,
	before *int64,
	count int,
) (History, error) {
	out := History{Items: []Card{}}
	size, err := limit(count)
	if err != nil {
		return out, err
	}
	if before != nil && *before < 1 {
		return out, access.ErrInvalid
	}
	err = database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		if _, err := s.guard(ctx, q, actor, eventID, teamID); err != nil {
			return err
		}
		row, err := q.LatestMentorScorecard(
			ctx,
			dbgen.LatestMentorScorecardParams{EventID: eventID, ID: id},
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return access.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("read own card identity: %w", err)
		}
		if row.AuthorID != actor || row.TeamID != teamID {
			return access.ErrForbidden
		}
		cursor := pgtype.Int8{}
		if before != nil {
			cursor = pgtype.Int8{Int64: *before, Valid: true}
		}
		rows, err := q.MentorScorecardHistory(
			ctx,
			dbgen.MentorScorecardHistoryParams{
				EventID:       eventID,
				ID:            id,
				BeforeVersion: cursor,
				PageSize:      size + 1,
			},
		)
		if err != nil {
			return fmt.Errorf("page retained private scores: %w", err)
		}
		if len(rows) > int(size) {
			rows = rows[:size]
			v := rows[len(rows)-1].Version
			out.NextCursor = &v
		}
		for _, row := range rows {
			card, err := decodeCard(row.Document)
			if err != nil {
				return err
			}
			if err = s.cardAuthority(ctx, q, actor, eventID, teamID, card); err != nil {
				return err
			}
			out.Items = append(out.Items, card)
		}
		return nil
	})
	if err != nil {
		return History{}, fmt.Errorf("read independent scorecard history: %w", err)
	}
	return out, nil
}

// Panel returns ONLY the exact current submitted versions the actual owning
// huddle policy grants. It never returns independent drafts or another author's
// revision history merely because the caller is an organiser or mentor.
func (s Service) Panel(
	ctx context.Context,
	actor, eventID, teamID, contextID uuid.UUID,
) ([]Card, error) {
	var out PanelReport
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		var err error
		out, err = s.PanelReportInTransaction(ctx, tx, actor, eventID, teamID, contextID)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("read recorded moderation card context: %w", err)
	}
	return out.Cards, nil
}

// PanelReport binds the exact current submitted-card projection to the actual
// native recorded context that grants its cross-author disclosure.
type PanelReport struct {
	Context ModerationContext `json:"context"`
	Cards   []Card            `json:"cards"`
}

func (s Service) PanelReportInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, teamID, contextID uuid.UUID,
) (PanelReport, error) {
	out := PanelReport{Cards: []Card{}}
	proof, err := s.context(ctx, tx, actor, eventID, teamID, contextID)
	if err != nil {
		return out, err
	}
	out.Context = proof
	q := dbgen.New(tx)
	for _, reference := range proof.Cards {
		card, err := currentSubmitted(ctx, q, eventID, teamID, reference)
		if err != nil {
			return PanelReport{}, err
		}
		if card.RunID != proof.RunID {
			return PanelReport{}, access.ErrConflict
		}
		if err = s.cardAuthority(ctx, q, card.AuthorID, eventID, teamID, card); err != nil {
			return PanelReport{}, err
		}
		out.Cards = append(out.Cards, card)
	}
	return out, nil
}

func currentSubmitted(
	ctx context.Context,
	q *dbgen.Queries,
	eventID, teamID uuid.UUID,
	reference CardReference,
) (Card, error) {
	row, err := q.LatestMentorScorecard(
		ctx,
		dbgen.LatestMentorScorecardParams{EventID: eventID, ID: reference.ID},
	)
	if err != nil {
		return Card{}, fmt.Errorf("read current submitted card: %w", err)
	}
	card, err := decodeCard(row.Document)
	if err != nil {
		return Card{}, err
	}
	if card.TeamID != teamID || card.Reference != reference || card.State != "submitted" {
		return Card{}, access.ErrConflict
	}
	return card, nil
}

func (s Service) context(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, teamID, id uuid.UUID,
) (ModerationContext, error) {
	if s.Moderation == nil || id == uuid.Nil {
		return ModerationContext{}, access.ErrForbidden
	}
	q := dbgen.New(tx)
	row, err := q.LockEvent(ctx, eventID)
	if err != nil {
		return ModerationContext{}, fmt.Errorf("lock scorecard panel: %w", err)
	}
	person, err := q.Person(ctx, actor)
	if err != nil || !person.Active {
		return ModerationContext{}, access.ErrForbidden
	}
	roles, err := q.ActiveRoles(ctx, dbgen.ActiveRolesParams{EventID: eventID, PersonID: actor})
	if err != nil {
		return ModerationContext{}, fmt.Errorf("read current moderation actor role: %w", err)
	}
	if !slices.Contains(roles, "organiser") && !slices.Contains(roles, "mentor") {
		return ModerationContext{}, access.ErrForbidden
	}
	if !row.PublishedAt.Valid || s.now().Before(row.PublishedAt.Time) {
		return ModerationContext{}, access.ErrForbidden
	}
	proof, err := s.Moderation.ResolveScorecardContextInTransaction(
		ctx,
		tx,
		actor,
		eventID,
		teamID,
		id,
	)
	if err != nil {
		return proof, fmt.Errorf("authorize current recorded moderation source: %w", err)
	}
	if proof.ID != id || proof.TeamID != teamID || proof.Version < 1 || proof.Binding == "" ||
		proof.RunID == uuid.Nil ||
		len(proof.Cards) == 0 ||
		len(proof.Cards) > 100 ||
		len(proof.RequiredAuthorIDs) == 0 {
		return proof, access.ErrForbidden
	}
	seen := map[uuid.UUID]bool{}
	for _, ref := range proof.Cards {
		if ref.ID == uuid.Nil || ref.Version < 1 || seen[ref.ID] {
			return proof, access.ErrInvalid
		}
		seen[ref.ID] = true
	}
	required := map[uuid.UUID]bool{}
	if len(proof.RequiredAuthorIDs) > 100 {
		return proof, access.ErrInvalid
	}
	for _, author := range proof.RequiredAuthorIDs {
		if author == uuid.Nil || required[author] {
			return proof, access.ErrInvalid
		}
		required[author] = true
	}

	return proof, nil
}
