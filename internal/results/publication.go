package results

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

func (s Service) finalistAnnouncement(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
	source Preview,
) (*Announcement, error) {
	if source.Selection == nil {
		return nil, access.ErrConflict
	}
	finalists, err := namedTeams(ctx, tx, eventID, source.Selection.FinalistIDs)
	if err != nil {
		return nil, err
	}
	reserves, err := namedTeams(ctx, tx, eventID, source.Selection.ReserveIDs)
	if err != nil {
		return nil, err
	}
	if len(finalists) != source.Criteria.FinalistCount ||
		len(reserves) != source.Criteria.ReserveCount {
		return nil, access.ErrConflict
	}
	return &Announcement{
		ID:          uuid.New(),
		Kind:        "finalists",
		Finalists:   finalists,
		Reserves:    reserves,
		Awards:      []AnnouncedAward{},
		PublishedAt: s.now(),
	}, nil
}

func namedTeams(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
	ids []uuid.UUID,
) ([]NamedTeam, error) {
	out := make([]NamedTeam, 0, len(ids))
	q := dbgen.New(tx)
	for _, id := range ids {
		team, err := q.HackTeam(ctx, dbgen.HackTeamParams{EventID: eventID, ID: id})
		if err != nil {
			return nil, failure("released team identity", err)
		}
		if team.State != "published" {
			return nil, access.ErrConflict
		}
		out = append(out, NamedTeam{ID: id, Name: team.Name})
	}
	return out, nil
}

func (s Service) awardAnnouncement(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
	awards []Award,
) (*Announcement, error) {
	out := &Announcement{
		ID:          uuid.New(),
		Kind:        "awards",
		Finalists:   []NamedTeam{},
		Reserves:    []NamedTeam{},
		Awards:      []AnnouncedAward{},
		PublishedAt: s.now(),
	}
	for _, award := range awards {
		teams, err := namedTeams(ctx, tx, eventID, []uuid.UUID{award.TeamID})
		if err != nil {
			return nil, err
		}
		out.Awards = append(
			out.Awards,
			AnnouncedAward{
				PrizeID:    award.PrizeID,
				Name:       award.Name,
				CategoryID: award.CategoryID,
				Team:       teams[0],
				Amount:     award.Amount,
				Currency:   award.Currency,
			},
		)
	}
	return out, nil
}

func retainPublication(
	ctx context.Context,
	tx pgx.Tx,
	eventID, actor uuid.UUID,
	binding string,
	value Announcement,
) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return failure("encode released projection", err)
	}
	_, err = tx.Exec(
		ctx,
		"INSERT INTO result_publications(event_id,id,kind,result_version,source_binding,document,actor_id,published_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)",
		eventID,
		value.ID,
		value.Kind,
		value.Version,
		binding,
		raw,
		actor,
		value.PublishedAt,
	)
	var pgError *pgconn.PgError
	if errors.As(err, &pgError) && pgError.Code == "23505" {
		return access.ErrConflict
	}
	return failure("retain one released source event", err)
}
