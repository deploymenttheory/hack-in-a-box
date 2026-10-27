package results

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// Read is private organiser preparation. It never returns raw independent cards
// or qualification totals, and cannot be used as a public announcement reader.
func (s Service) Read(ctx context.Context, actor, eventID uuid.UUID) (Page, error) {
	var out Page
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		current, err := s.guard(ctx, tx, actor, eventID, false)
		if err != nil {
			return err
		}
		out, err = s.page(ctx, tx, actor, eventID, current)
		return err
	})
	if err != nil {
		return Page{}, failure("read private preparation", err)
	}
	return out, nil
}

func (s Service) page(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	current authority,
) (Page, error) {
	if !current.organiser {
		return Page{}, access.ErrForbidden
	}
	round, err := load(ctx, tx, eventID)
	if err != nil {
		return Page{}, err
	}
	source, err := s.source(ctx, tx, actor, eventID)
	if err != nil {
		return Page{}, err
	}
	return s.pageFromSource(ctx, tx, eventID, current, round, source)
}

// Result writes already validate the source under their event lock. Their
// decisions update the result round, not its frozen judging evidence. Reuse is
// local to that transaction; ordinary reads and receipt replays refresh it.
func (s Service) pageFromSource(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
	current authority,
	round Round,
	source assessmentSource,
) (Page, error) {
	if !current.organiser {
		return Page{}, access.ErrForbidden
	}
	var err error
	out := Page{
		TieStates:     []TieState{},
		Advisories:    []MentorAdvisory{},
		Revision:      current.event.Revision,
		Round:         round,
		Preview:       source.preview,
		Announcements: []Announcement{},
	}
	fullCurrent := round.Source.Binding != "" && round.Source.Binding == source.preview.Binding &&
		len(source.preview.Blockers) == 0
	qualificationCurrent := round.Source.Binding == "" && round.Source.FinalistsBinding != "" &&
		round.Source.FinalistsBinding == source.preview.FinalistsBinding
	out.Current = round.Version > 0 && (fullCurrent || qualificationCurrent)
	if !out.Current {
		// Keep immutable storage; never refill current private preparation after its
		// source or assignment authority changed.
		out.Round.Source = emptyPreview()
		out.Round.Votes = []Vote{}
		out.Round.Confirmation = nil
		out.Round.Reason = ""
	}
	if source.completion != nil {
		out.TieStates, err = resolveJudgeTies(
			source.preview.Ties,
			source.completion.Assignments,
			out.Round.Votes,
		)
		if err != nil {
			return Page{}, err
		}
	}
	if fullCurrent {
		out.Advisories, err = currentAdvisories(ctx, tx, eventID, source)
		if err != nil {
			return Page{}, err
		}
	}
	out.Announcements, err = published(ctx, tx, eventID, s.now())
	return out, err
}

func published(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
	now time.Time,
) ([]Announcement, error) {
	return publishedDocuments(
		ctx,
		tx,
		eventID,
		now,
		"SELECT document FROM result_publications WHERE event_id=$1 AND published_at<=$2 ORDER BY published_at,id",
	)
}

func latestPublished(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
	now time.Time,
) ([]Announcement, error) {
	return publishedDocuments(
		ctx,
		tx,
		eventID,
		now,
		"SELECT document FROM (SELECT DISTINCT ON (kind) document,result_version,id FROM result_publications WHERE event_id=$1 AND published_at<=$2 ORDER BY kind,result_version DESC) latest ORDER BY result_version,id",
	)
}

func publishedDocuments(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
	now time.Time,
	query string,
) ([]Announcement, error) {
	rows, err := tx.Query(ctx, query, eventID, now)
	if err != nil {
		return nil, failure("read released announcements", err)
	}
	defer rows.Close()
	out := []Announcement{}
	for rows.Next() {
		var raw []byte
		var value Announcement
		if err = rows.Scan(&raw); err != nil {
			return nil, failure("announcement row", err)
		}
		if err = json.Unmarshal(raw, &value); err != nil {
			return nil, failure("decode announced projection", err)
		}
		out = append(out, value)
	}
	return out, failure("released announcement rows", rows.Err())
}
