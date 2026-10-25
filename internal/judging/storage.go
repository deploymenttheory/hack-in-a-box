package judging

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func failure(op string, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return access.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("judge %s: %w", op, err)
	}
	return nil
}
func digest(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", failure("encode fingerprint", err)
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}

type authority struct {
	row              dbgen.Event
	organiser, judge bool
	dates            event.Configuration
}

func (s Service) guard(ctx context.Context, tx pgx.Tx, actor, eventID uuid.UUID, write bool) (authority, error) {
	var a authority
	q := dbgen.New(tx)
	row, err := q.LockEvent(ctx, eventID)
	if err != nil {
		return a, failure("lock event", err)
	}
	a.row = row
	if err = (access.EventPolicy{Queries: q, Clock: s.now}).Authorize(ctx, actor, eventID, eventID, access.EventRead); err != nil {
		return a, failure("current actor", err)
	}
	roles, err := q.ActiveRoles(ctx, dbgen.ActiveRolesParams{EventID: eventID, PersonID: actor})
	if err != nil {
		return a, failure("roles", err)
	}
	a.organiser = slices.Contains(roles, "organiser")
	a.judge = slices.Contains(roles, "judge")
	if !a.organiser && !a.judge {
		return a, access.ErrForbidden
	}
	if write && (!row.WritesAllowed || row.Stage == "cancelled" || row.Stage == "archived" || row.Stage == "completed") {
		return a, access.ErrForbidden
	}
	err = json.Unmarshal(row.Configuration, &a.dates)
	return a, failure("dates", err)
}
func loadRun(ctx context.Context, tx pgx.Tx, eventID, runID uuid.UUID) (dbgen.RubricAssessmentRun, rubric.Definition, error) {
	q := dbgen.New(tx)
	run, err := q.RubricRun(ctx, dbgen.RubricRunParams{EventID: eventID, ID: runID})
	if err != nil {
		return run, rubric.Definition{}, failure("run", err)
	}
	if run.Status != "current" || run.Purpose != "judge_final" {
		return run, rubric.Definition{}, access.ErrConflict
	}
	if err = rubric.ValidateRunPublicationInTransaction(ctx, tx, eventID, runID); err != nil {
		return run, rubric.Definition{}, failure("run publication", err)
	}
	v, err := q.RubricVersion(ctx, dbgen.RubricVersionParams{EventID: eventID, RubricID: run.RubricID, Version: run.RubricVersion})
	if err != nil {
		return run, rubric.Definition{}, failure("criteria", err)
	}
	var d rubric.Definition
	err = json.Unmarshal(v.Document, &d)
	return run, d, failure("decode criteria", err)
}
func round(ctx context.Context, tx pgx.Tx, eventID, runID uuid.UUID) (int64, bool, error) {
	var version int64
	var open bool
	err := tx.QueryRow(ctx, "SELECT revision,deliberation FROM judge_rounds WHERE event_id=$1 AND run_id=$2", eventID, runID).Scan(&version, &open)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	return version, open, failure("round", err)
}
func ensureRound(ctx context.Context, tx pgx.Tx, eventID, runID uuid.UUID) error {
	_, err := tx.Exec(ctx, "INSERT INTO judge_rounds(event_id,run_id) VALUES($1,$2) ON CONFLICT DO NOTHING", eventID, runID)
	return failure("initialize round", err)
}
func readCard(ctx context.Context, tx pgx.Tx, eventID, runID, teamID, author uuid.UUID) (*Card, error) {
	var raw []byte
	err := tx.QueryRow(ctx, "SELECT document FROM judge_cards WHERE event_id=$1 AND run_id=$2 AND team_id=$3 AND author_id=$4", eventID, runID, teamID, author).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, failure("card", err)
	}
	var c Card
	err = json.Unmarshal(raw, &c)
	return &c, failure("decode card", err)
}
func history(ctx context.Context, tx pgx.Tx, actor, eventID, runID, teamID uuid.UUID, action, reason string, value any, s Service) error {
	b, err := json.Marshal(value)
	if err != nil {
		return failure("history document", err)
	}
	_, err = tx.Exec(ctx, "INSERT INTO judge_history(event_id,run_id,team_id,actor_id,action,reason,document,occurred_at) VALUES($1,NULLIF($2,'00000000-0000-0000-0000-000000000000'::uuid),NULLIF($3,'00000000-0000-0000-0000-000000000000'::uuid),$4,$5,$6,$7,$8)", eventID, runID, teamID, actor, action, reason, b, s.now())
	return failure("retain history", err)
}
func original(ctx context.Context, tx pgx.Tx, eventID, actor, request uuid.UUID, fingerprint string) (*Receipt, error) {
	var oldActor uuid.UUID
	var hash string
	var raw []byte
	err := tx.QueryRow(ctx, "SELECT actor_id,fingerprint,result FROM judge_commands WHERE event_id=$1 AND request_id=$2", eventID, request).Scan(&oldActor, &hash, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, failure("original command", err)
	}
	if oldActor != actor || hash != fingerprint {
		return nil, access.ErrConflict
	}
	var r Receipt
	err = json.Unmarshal(raw, &r)
	return &r, failure("original receipt", err)
}
func retain(ctx context.Context, tx pgx.Tx, eventID uuid.UUID, r Receipt) error {
	b, err := json.Marshal(r)
	if err != nil {
		return failure("encode receipt", err)
	}
	_, err = tx.Exec(ctx, "INSERT INTO judge_commands(event_id,request_id,actor_id,fingerprint,result) VALUES($1,$2,$3,$4,$5)", eventID, r.RequestID, r.ActorID, r.Fingerprint, b)
	return failure("retain receipt", err)
}
func reason(value string) bool { return strings.TrimSpace(value) != "" && len(value) <= 4000 }
func invalidate(ctx context.Context, tx pgx.Tx, eventID, runID, actor uuid.UUID, why string, s Service) error {
	_, err := tx.Exec(ctx, "INSERT INTO judge_result_invalidations(approval_id,actor_id,reason,created_at) SELECT a.id,$3,$4,$5 FROM judge_result_approvals a WHERE a.event_id=$1 AND a.run_id=$2 ON CONFLICT DO NOTHING", eventID, runID, actor, why, s.now())
	return failure("invalidate unannounced approvals", err)
}
