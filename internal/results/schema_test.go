package results

import (
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestPopulatedParentUpgradeAndRetainedResultHistory(t *testing.T) {
	pool := testdb.New(t, false)
	if err := database.Migrate(t.Context(), pool, 52); err != nil {
		t.Fatal(err)
	}
	actor := testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, actor.ID)
	ref := testdb.ApprovedRubric(t, pool, actor.ID, eventID)
	if err := database.Migrate(t.Context(), pool, 53); err != nil {
		t.Fatal(err)
	}
	var retained int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM event_rubrics WHERE event_id=$1 AND id=$2", eventID, ref.ID).
		Scan(&retained); err != nil ||
		retained != 1 {
		t.Fatalf("upgrade lost parent data: %d %v", retained, err)
	}
	_, err := pool.Exec(
		t.Context(),
		`INSERT INTO result_versions(event_id,version,actor_id,action,reason,document,changed_at) VALUES($1,1,$2,'prepare','Synthetic schema fixture','{}',now())`,
		eventID,
		actor.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range []struct {
		run     *uuid.UUID
		version *int64
	}{{run: ptrUUID(uuid.New())}, {version: ptrVersion(1)}} {
		if _, err = pool.Exec(t.Context(), `INSERT INTO result_versions(event_id,version,qualification_run_id,qualification_version,actor_id,action,reason,document,changed_at) VALUES($1,2,$2,$3,$4,'prepare','Synthetic incomplete native source','{}',now())`, eventID, pair.run, pair.version, actor.ID); err == nil {
			t.Fatal("incomplete native qualification reference accepted")
		}
	}
	_, err = pool.Exec(
		t.Context(),
		`INSERT INTO result_commands(event_id,request_id,actor_id,result_version,receipt) VALUES($1,$2,$3,1,'{}')`,
		eventID,
		uuid.New(),
		actor.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	binding := strings.Repeat("a", 64)
	_, err = pool.Exec(
		t.Context(),
		`INSERT INTO result_publications(event_id,id,kind,result_version,source_binding,document,actor_id,published_at) VALUES($1,$2,'finalists',1,$3,'{}',$4,now())`,
		eventID,
		uuid.New(),
		binding,
		actor.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(
		t.Context(),
		`INSERT INTO result_publications(event_id,id,kind,result_version,source_binding,document,actor_id,published_at) VALUES($1,$2,'finalists',1,$3,'{}',$4,now())`,
		eventID,
		uuid.New(),
		binding,
		actor.ID,
	); err == nil {
		t.Fatal("duplicate source emitted another publication event")
	}
	// The same confirmed source has independent finalist and category embargoes.
	if _, err = pool.Exec(
		t.Context(),
		`INSERT INTO result_publications(event_id,id,kind,result_version,source_binding,document,actor_id,published_at) VALUES($1,$2,'awards',1,$3,'{}',$4,now())`,
		eventID,
		uuid.New(),
		binding,
		actor.ID,
	); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"result_versions", "result_commands", "result_publications"} {
		for _, statement := range []string{"UPDATE " + table + " SET event_id=event_id", "DELETE FROM " + table, "TRUNCATE " + table + " CASCADE"} {
			if _, err = pool.Exec(t.Context(), statement); err == nil {
				t.Fatalf("retained history accepted %s", statement)
			}
		}
	}
	// Cross-event result and native source references must not be claimable.
	foreign := testdb.Event(t, pool, actor.ID)
	if _, err = pool.Exec(
		t.Context(),
		`INSERT INTO result_publications(event_id,id,kind,result_version,source_binding,document,actor_id,published_at) VALUES($1,$2,'awards',1,$3,'{}',$4,now())`,
		foreign,
		uuid.New(),
		binding,
		actor.ID,
	); err == nil {
		t.Fatal("cross-event result version accepted")
	}
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
	if _, err = provider.DownTo(t.Context(), 52); err == nil {
		t.Fatal("rollback removed retained result decisions")
	}
	version, err := provider.GetDBVersion(t.Context())
	if err != nil || version != 53 {
		t.Fatalf("rollback moved retained schema: %d %v", version, err)
	}
	if err = pool.QueryRow(t.Context(), "SELECT count(*) FROM result_publications WHERE event_id=$1", eventID).
		Scan(&retained); err != nil ||
		retained != 2 {
		t.Fatalf("history changed: %d %v", retained, err)
	}
}

func TestEmptyResultSchemaCanRollBack(t *testing.T) {
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
	if _, err = provider.DownTo(t.Context(), 52); err != nil {
		t.Fatal(err)
	}
	version, err := provider.GetDBVersion(t.Context())
	if err != nil || version != 52 {
		t.Fatalf("empty rollback failed: %d %v", version, err)
	}
}

func ptrUUID(v uuid.UUID) *uuid.UUID { return &v }
func ptrVersion(v int64) *int64      { return &v }
