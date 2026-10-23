package objectives_test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/objectives"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func retainedTables(
	t *testing.T,
	pool *pgxpool.Pool,
	names []string,
) (map[string][]byte, []string) {
	t.Helper()
	if names == nil {
		rows, err := pool.Query(
			t.Context(),
			"SELECT tablename FROM pg_tables WHERE schemaname='public' AND tablename<>'goose_db_version' ORDER BY tablename",
		)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var name string
			if err = rows.Scan(&name); err != nil {
				t.Fatal(err)
			}
			names = append(names, name)
		}
		if err = rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows.Close()
	}
	out := map[string][]byte{}
	for _, name := range names {
		sql := "SELECT COALESCE(jsonb_agg(to_jsonb(t) ORDER BY to_jsonb(t)::text),'[]'::jsonb) FROM " + pgx.Identifier{
			"public",
			name,
		}.Sanitize() + " t"
		var raw []byte
		if err := pool.QueryRow(t.Context(), sql).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		out[name] = raw
	}
	return out, names
}

func TestObjectivesOrdered38To39PreservesExistingHistoryAndGuardsRollback(t *testing.T) {
	pool := testdb.New(t, false)
	if err := database.Migrate(t.Context(), pool, 38); err != nil {
		t.Fatal(err)
	}
	owner := testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, owner.ID)
	// Historical fixture SQL deliberately uses only the actual38 schema. It must
	// not invoke a current snapshot/clone service that requires objective tables.
	if _, err := pool.Exec(
		t.Context(),
		"INSERT INTO event_definitions(event_id,definitions) VALUES($1,'{\"forms\":[],\"tasks\":[],\"deliverables\":[],\"role_briefs\":[],\"rubrics\":[],\"messages\":[],\"branding\":{\"primary\":\"#173a35\"}}'::jsonb)",
		eventID,
	); err != nil {
		t.Fatal(err)
	}
	pageID := uuid.New()
	if _, err := (content.Service{Pool: pool}).Save(
		t.Context(),
		owner.ID,
		eventID,
		pageID,
		0,
		content.Input{
			Kind: "page",
			Document: content.Document{
				Title:    "Retained private historical planning",
				Markdown: "PRIVATE HISTORICAL CONTENT",
				Audience: access.ContentAudience{Roles: []string{"organiser"}},
			},
		},
	); err != nil {
		t.Fatal(err)
	}
	before, names := retainedTables(t, pool, nil)
	if err := database.Migrate(t.Context(), pool, 39); err != nil {
		t.Fatal(err)
	}
	after, _ := retainedTables(t, pool, names)
	for _, name := range names {
		if !bytes.Equal(before[name], after[name]) {
			t.Fatalf("normal upgrade changed retained38 table %s", name)
		}
	}
	db := stdlib.OpenDBFromPool(pool)
	defer func() { _ = db.Close() }()
	migrator, err := goose.NewProvider(
		goose.DialectPostgres,
		db,
		os.DirFS("../database/migrations"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if version, err := migrator.GetDBVersion(t.Context()); err != nil || version != 39 {
		t.Fatal("explicit39 upgrade boundary", version, err)
	}
	if err := database.Migrate(t.Context(), pool, 0); err != nil {
		t.Fatal(err)
	}
	if err := database.CheckSchema(t.Context(), pool); err != nil {
		t.Fatal(err)
	}
	s := objectives.Service{Pool: pool}
	id := uuid.New()
	d := objectives.Definition{
		Description: "Retained objective definition",
		Measures:    []objectives.Measure{},
	}
	if _, err := s.Change(
		t.Context(),
		owner.ID,
		eventID,
		id,
		0,
		objectives.Command{
			Action:     "save",
			Definition: &d,
			Reason:     "Create retained planning history",
		},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := migrator.DownTo(
		t.Context(),
		38,
	); err == nil ||
		!strings.Contains(err.Error(), "Objective definition history") {
		t.Fatal("populated objective rollback not guarded", err)
	}
	if version, err := migrator.GetDBVersion(t.Context()); err != nil || version != 39 {
		t.Fatal("failed rollback lost explicit39 preservation boundary", version, err)
	}
	if err := database.Migrate(context.WithoutCancel(t.Context()), pool, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Read(t.Context(), owner.ID, eventID, id, 1); err != nil {
		t.Fatal("guard lost retained version", err)
	}
}

func TestObjectivesEmpty39RollbackSupportsOrderedReupgrade(t *testing.T) {
	pool := testdb.New(t, true)
	db := stdlib.OpenDBFromPool(pool)
	defer func() { _ = db.Close() }()
	migrator, err := goose.NewProvider(
		goose.DialectPostgres,
		db,
		os.DirFS("../database/migrations"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = migrator.DownTo(t.Context(), 38); err != nil {
		t.Fatal("empty objective rollback", err)
	}
	if version, err := migrator.GetDBVersion(t.Context()); err != nil || version != 38 {
		t.Fatal("empty rollback boundary", version, err)
	}
	if err = database.Migrate(t.Context(), pool, 0); err != nil {
		t.Fatal(err)
	}
	if err = database.CheckSchema(t.Context(), pool); err != nil {
		t.Fatal(err)
	}
}
