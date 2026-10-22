package credentials

import (
	"bytes"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestCredentialAvailabilityBoundariesAndMetadataPreserveEncryption(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	now := time.Date(2027, 6, 20, 8, 0, 0, 0, time.UTC)
	f.s.Now = func() time.Time { return now }
	available, expires := now.Add(time.Hour), now.Add(2*time.Hour)
	page := f.change(
		Command{
			Action:       "create",
			Label:        "Synthetic account",
			Value:        "SYNTHETIC-TIMED-SECRET",
			ResourceType: "account",
			AvailableAt:  &available,
			ExpiresAt:    &expires,
			TeamIDs:      []uuid.UUID{f.team},
		},
	)
	id := page.Items[0].ID
	if page.Items[0].Readiness != "scheduled" {
		t.Fatalf("scheduled metadata: %+v", page.Items[0])
	}
	for _, tc := range []struct {
		at      time.Time
		allowed bool
	}{{available.Add(-time.Nanosecond), false}, {available, true}, {expires.Add(-time.Nanosecond), true}, {expires, false}, {expires.Add(time.Second), false}} {
		now = tc.at
		value, err := f.reveal(id)
		if tc.allowed {
			if err != nil || value.Value != "SYNTHETIC-TIMED-SECRET" {
				t.Fatalf("boundary %s: %v", now, err)
			}
		} else {
			if !errors.Is(err, access.ErrForbidden) || value.Value != "" {
				t.Fatalf("disclosed outside interval %s: %v", now, err)
			}
		}
		member, err := f.s.List(ctx, f.member.ID, f.event)
		if err != nil {
			t.Fatal(err)
		}
		if (len(member.Items) == 1) != tc.allowed {
			t.Fatalf("member metadata disclosed outside availability at %s", now)
		}
	}
	f.change(
		Command{
			Action: "replace",
			ID:     id,
			Label:  "Replacement account",
			Value:  "SYNTHETIC-REPLACEMENT",
		},
	)
	before, err := dbgen.New(f.pool).
		TeamCredential(ctx, dbgen.TeamCredentialParams{EventID: f.event, ID: id})
	if err != nil {
		t.Fatal(err)
	}
	if before.ResourceType != "account" || !before.AvailableAt.Time.Equal(available) ||
		!before.ExpiresAt.Time.Equal(expires) {
		t.Fatal("replacement silently changed public metadata")
	}
	if _, err = f.reveal(id); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("replacement restored expired access")
	}
	page = f.change(
		Command{Action: "metadata", ID: id, Label: "Updated account", ResourceType: "licence"},
	)
	after, err := dbgen.New(f.pool).
		TeamCredential(ctx, dbgen.TeamCredentialParams{EventID: f.event, ID: id})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before.Ciphertext, after.Ciphertext) || before.Version != after.Version ||
		after.AvailableAt.Valid ||
		after.ExpiresAt.Valid ||
		page.Items[0].Readiness != "ready" {
		t.Fatal("metadata edit changed encryption or failed to clear interval")
	}
	if value, err := f.reveal(id); err != nil || value.Value != "SYNTHETIC-REPLACEMENT" {
		t.Fatalf("metadata recovery: %v", err)
	}
	revision := f.revision()
	if _, err = f.s.Change(
		ctx,
		f.owner.ID,
		f.event,
		revision,
		Command{
			Action:       "metadata",
			ID:           id,
			Label:        "Invalid",
			ResourceType: "licence",
			AvailableAt:  &expires,
			ExpiresAt:    &available,
		},
	); !errors.Is(err, access.ErrInvalid) ||
		f.revision() != revision {
		t.Fatal("invalid availability committed")
	}
}

func TestCredentialInventoryUpgradeAndRollbackGate(t *testing.T) {
	pool := testdb.New(t, false)
	ctx := t.Context()
	if err := database.Migrate(ctx, pool, 24); err != nil {
		t.Fatal(err)
	}
	owner := testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, owner.ID)
	id := uuid.New()
	if _, err := pool.Exec(
		ctx,
		"INSERT INTO team_credentials(id,event_id,label,key_id,ciphertext) VALUES($1,$2,'Synthetic legacy','legacy-key','\\x01')",
		id,
		eventID,
	); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, pool, 25); err != nil {
		t.Fatal(err)
	}
	row, err := dbgen.New(pool).
		TeamCredential(ctx, dbgen.TeamCredentialParams{EventID: eventID, ID: id})
	if err != nil {
		t.Fatal(err)
	}
	if row.ResourceType != "credential" || row.AvailableAt.Valid || row.ExpiresAt.Valid ||
		!row.Active {
		t.Fatal("inventory upgrade changed existing disclosure semantics")
	}
	if _, err = pool.Exec(
		ctx,
		"UPDATE team_credentials SET available_at=now()+interval '1 hour' WHERE id=$1",
		id,
	); err != nil {
		t.Fatal(err)
	}
	sqlDB := stdlib.OpenDBFromPool(pool)
	defer func() { _ = sqlDB.Close() }()
	migrator, err := goose.NewProvider(
		goose.DialectPostgres,
		sqlDB,
		os.DirFS("../database/migrations"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = migrator.DownTo(ctx, 24); err == nil {
		t.Fatal("rollback silently removed the disclosure gate")
	}
	// This historical fixture isolates migration25's rollback gate. Later
	// migrations may independently roll down before Goose reaches the gate.
	version, err := migrator.GetDBVersion(ctx)
	if err != nil || version != 25 {
		t.Fatalf("failed rollback changed protected schema: %d %v", version, err)
	}
	retained, err := dbgen.New(pool).
		TeamCredential(ctx, dbgen.TeamCredentialParams{EventID: eventID, ID: id})
	if err != nil || !retained.AvailableAt.Valid {
		t.Fatalf("failed rollback removed disclosure metadata: %v", err)
	}
	if err = database.Migrate(ctx, pool, 0); err != nil {
		t.Fatal(err)
	}
	if err = database.CheckSchema(ctx, pool); err != nil {
		t.Fatalf("normal upgrade after protected rollback failed: %v", err)
	}
}
