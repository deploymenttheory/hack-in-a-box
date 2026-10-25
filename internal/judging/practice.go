package judging

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
)

func (s Service) practice(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
) (Practice, error) {
	var out Practice
	a, err := s.guard(ctx, tx, actor, eventID, true)
	if err != nil {
		return out, err
	}
	if !a.judge || a.dates.LiveStart.IsZero() || !s.now().Before(a.dates.LiveStart) {
		return out, access.ErrForbidden
	}
	q := dbgen.New(tx)
	h, err := q.EventRubric(ctx, eventID)
	if err != nil {
		return out, failure("practice configured rubric", err)
	}
	if !h.ApprovedVersion.Valid {
		return out, access.ErrForbidden
	}
	row, err := q.RubricVersion(
		ctx,
		dbgen.RubricVersionParams{
			EventID:  eventID,
			RubricID: h.ID,
			Version:  h.ApprovedVersion.Int64,
		},
	)
	if err != nil {
		return out, failure("practice criteria", err)
	}
	var d rubric.PublicDefinition
	if err = json.Unmarshal(row.PublicDocument, &d); err != nil {
		return out, failure("practice public criteria", err)
	}
	out = Practice{
		FictionalEntryID: uuid.NewSHA1(eventID, []byte("isolated-fictional-judge-practice")),
		Name:             "Fictional practice robot — synthetic entry",
		Rubric: rubric.Reference{
			ID:      h.ID,
			Version: row.Version,
			Digest:  row.PublicDigest,
		},
		Criteria: d,
	}
	var raw []byte
	err = tx.QueryRow(ctx, "SELECT document FROM judge_practice_cards WHERE event_id=$1 AND actor_id=$2", eventID, actor).
		Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, failure("practice card", err)
	}
	var card PracticeCard
	if err = json.Unmarshal(raw, &card); err != nil {
		return out, failure("decode practice", err)
	}
	out.Card = &card
	return out, nil
}

func (s Service) Practice(ctx context.Context, actor, eventID uuid.UUID) (Practice, error) {
	var out Practice
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		var err error
		out, err = s.practice(ctx, tx, actor, eventID)
		return err
	})
	return out, failure("practice read", err)
}

func (s Service) ChangePractice(
	ctx context.Context,
	actor, eventID uuid.UUID,
	expected int64,
	c PracticeCommand,
) (Receipt, error) {
	var out Receipt
	if c.RequestID == uuid.Nil || expected < 0 {
		return out, access.ErrInvalid
	}
	hash, err := digest(struct {
		Version  int64
		Practice PracticeCommand
	}{expected, c})
	if err != nil {
		return out, err
	}
	err = database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		p, err := s.practice(ctx, tx, actor, eventID)
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
		version := int64(0)
		if p.Card != nil {
			version = p.Card.Version
		}
		if version != expected {
			return access.ErrConflict
		}
		if p.Card != nil && p.Card.State == "submitted" && c.Action != "reset" {
			return access.ErrConflict
		}
		var scores []Score
		state := "draft"
		switch c.Action {
		case "save", "submit":
			scores, err = normalize(
				c.Scores,
				p.Criteria,
				submissions.Document{},
				c.Action == "submit",
			)
			if c.Action == "submit" {
				state = "submitted"
			}
		case "reset":
			scores = []Score{}
		default:
			return access.ErrInvalid
		}
		if err != nil {
			return err
		}
		card := PracticeCard{Version: version + 1, Rubric: p.Rubric, State: state, Scores: scores}
		raw, err := json.Marshal(card)
		if err != nil {
			return failure("practice encode", err)
		}
		if _, err = tx.Exec(
			ctx,
			"INSERT INTO judge_practice_cards(event_id,actor_id,version,document) VALUES($1,$2,$3,$4) ON CONFLICT(event_id,actor_id) DO UPDATE SET version=EXCLUDED.version,document=EXCLUDED.document",
			eventID,
			actor,
			card.Version,
			raw,
		); err != nil {
			return failure("practice save", err)
		}
		if err = history(
			ctx,
			tx,
			actor,
			eventID,
			uuid.Nil,
			uuid.Nil,
			"practice_"+c.Action,
			"Synthetic isolated practice",
			card,
			s,
		); err != nil {
			return err
		}
		out = Receipt{
			RequestID:   c.RequestID,
			ActorID:     actor,
			TeamID:      p.FictionalEntryID,
			Action:      "practice_" + c.Action,
			Revision:    card.Version,
			State:       card.State,
			Fingerprint: hash,
		}
		return retain(ctx, tx, eventID, out)
	})
	return out, failure("practice change", err)
}
