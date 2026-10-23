package files_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/files"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestNativeProtectedAssociationFailureRollsBackFileAndAudit(t *testing.T) {
	f := testdb.NewAuthorization(t)
	storage, err := files.OpenLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	service := files.Service{
		Pool:       f.Pool,
		Storage:    storage,
		Policy:     f.Policy,
		MaxBytes:   1024,
		EventQuota: 8192,
	}
	input := files.Upload{
		Actor:     f.Owner.ID,
		Event:     f.First,
		Key:       uuid.New(),
		Revision:  1,
		Filename:  "native.txt",
		MediaType: "text/plain",
		Content:   []byte("Synthetic owner association"),
	}
	failure := errors.New("synthetic native association persistence failure")
	called := false
	_, err = service.UploadTransactional(
		t.Context(),
		input,
		func(ctx context.Context, tx pgx.Tx, file dbgen.File, created bool) error {
			called = true
			if !created || file.ID == uuid.Nil {
				t.Fatal("native new file callback shape lost")
			}
			var retained int
			if e := tx.QueryRow(ctx, `SELECT count(*) FROM files WHERE id=$1`, file.ID).
				Scan(&retained); e != nil ||
				retained != 1 {
				t.Fatal("association does not share file transaction", retained, e)
			}
			return failure
		},
	)
	if !called || !errors.Is(err, failure) {
		t.Fatal("native callback failure ignored", called, err)
	}
	var count int
	if err = f.Pool.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM files WHERE event_id=$1)+(SELECT count(*) FROM audit_events WHERE event_id=$1 AND action='file.uploaded')`, f.First).
		Scan(&count); err != nil ||
		count != 0 {
		t.Fatal("failed association leaked file metadata/audit", count, err)
	}
	// The ordinary API wrapper still works and exact file idempotency reruns the
	// owning association under current authority without creating a second file.
	file, err := service.Upload(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.UploadTransactional(
		t.Context(),
		input,
		func(_ context.Context, _ pgx.Tx, got dbgen.File, created bool) error {
			if created || got.ID != file.ID {
				t.Fatal("exact existing callback lost immutable file identity", got, created)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
}
