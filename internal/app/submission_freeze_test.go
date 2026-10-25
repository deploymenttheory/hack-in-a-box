package app

import (
	"encoding/json"
	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/secretbox"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissionfreeze"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
	"github.com/google/uuid"
	"io"
	"log/slog"
	"testing"
	"time"
)

func TestRuntimeRegistersAndConsumesNativeSubmissionCutoff(t *testing.T) {
	pool := testdb.New(t, true)
	owner := testdb.Person(t, pool)
	id := testdb.Event(t, pool, owner.ID)
	team := uuid.New()
	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	c := event.DefaultConfiguration()
	c.LiveStart = now.Add(-time.Hour)
	c.SubmissionCutoff = now
	c.LiveEnd = now.Add(time.Hour)
	raw, _ := json.Marshal(c)
	for _, command := range []struct {
		sql  string
		args []any
	}{{"UPDATE events SET configuration=$2,published_at=$3,stage='preparation' WHERE id=$1", []any{id, raw, now.Add(-7 * 24 * time.Hour)}}, {"INSERT INTO hack_teams(event_id,id,name,state,frozen) VALUES($1,$2,'Synthetic runtime team','published',true)", []any{id, team}}, {"INSERT INTO competition_entries(event_id,team_id) VALUES($1,$2)", []any{id, team}}} {
		if _, err := pool.Exec(t.Context(), command.sql, command.args...); err != nil {
			t.Fatal(err)
		}
	}
	keys, err := secretbox.Parse("")
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{PublicOrigin: "https://example.test", CredentialKeys: keys}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	producer, err := newCommunicationRuntime(pool, cfg, logger, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	worker, err := newCommunicationRuntime(pool, cfg, logger, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if err = producer.SubmissionFreeze.Maintain(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err = producer.SubmissionFreeze.Maintain(t.Context()); err != nil {
		t.Fatal(err)
	}
	var op uuid.UUID
	var count int
	if err = pool.QueryRow(t.Context(), "SELECT count(*) FROM operations WHERE event_id=$1 AND kind=$2", id, submissionfreeze.HandlerKind).Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate cutoff scheduling", count, err)
	}
	if err = pool.QueryRow(t.Context(), "SELECT id FROM operations WHERE event_id=$1 AND kind=$2", id, submissionfreeze.HandlerKind).Scan(&op); err != nil {
		t.Fatal(err)
	}
	if err = worker.Jobs.Execute(t.Context(), op); err != nil {
		t.Fatal(err)
	}
	p, err := producer.SubmissionFreeze.Read(t.Context(), owner.ID, id, team)
	if err != nil || p.Source == nil || p.NativeOutcome != "not_submitted" || p.Source.Status != "not_submitted" || !p.Source.Cutoff.Equal(now) || !p.Source.RecordedAt.Equal(now) {
		t.Fatal(p, err)
	}
	var state string
	if err = pool.QueryRow(t.Context(), "SELECT state FROM operations WHERE id=$1", op).Scan(&state); err != nil || state != "delivered" {
		t.Fatal("registered handler did not consume", state, err)
	}
}
