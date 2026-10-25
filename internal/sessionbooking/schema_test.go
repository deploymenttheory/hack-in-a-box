package sessionbooking_test

import (
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/sessionbooking"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestSessionMigrationRejectsLossOfOriginalReceipts(t *testing.T) {
	t.Run("empty rollback", func(t *testing.T) {
		pool := testdb.New(t, true)
		db := stdlib.OpenDBFromPool(pool)
		defer func() { _ = db.Close() }()
		provider, err := goose.NewProvider(
			goose.DialectPostgres,
			db,
			os.DirFS("../database/migrations"),
		)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = provider.DownTo(t.Context(), 47); err != nil {
			t.Fatal(err)
		}
		if err = database.Migrate(t.Context(), pool, 49); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("retained original acknowledgement", func(t *testing.T) {
		f := setup(t, 1, []string{"attendee"})
		person := f.attendee(t)
		command := sessionbooking.Command{
			SessionRevision: 1,
			RequestID:       uuid.New(),
			Action:          "reserve",
		}
		original, err := f.s.Change(t.Context(), person, f.event, f.id, command)
		if err != nil {
			t.Fatal(err)
		}
		db := stdlib.OpenDBFromPool(f.pool)
		defer func() { _ = db.Close() }()
		provider, err := goose.NewProvider(
			goose.DialectPostgres,
			db,
			os.DirFS("../database/migrations"),
		)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = provider.DownTo(t.Context(), 47); err == nil {
			t.Fatal("rollback erased session history")
		}
		receipt, err := f.s.OriginalReceipt(t.Context(), person, f.event, f.id, command.RequestID)
		if err != nil || receipt != original {
			t.Fatal("rejected rollback changed original receipt", err)
		}
	})
}
