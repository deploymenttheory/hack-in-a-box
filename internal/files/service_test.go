package files_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/files"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestQuotaFinalizationIdempotencyAndOwnership(t *testing.T) {
	pool := testdb.New(t, true)
	q := dbgen.New(pool)
	ctx := t.Context()
	person, err := q.UpsertIdentity(
		ctx,
		dbgen.UpsertIdentityParams{Issuer: "test", Subject: "admin"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.BootstrapAdmin(ctx, person.ID); err != nil {
		t.Fatal(err)
	}
	event := uuid.New()
	if _, err := pool.Exec(
		ctx,
		"INSERT INTO events(id,name) VALUES ($1,'Synthetic')",
		event,
	); err != nil {
		t.Fatal(err)
	}
	storage, err := files.OpenLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	service := files.Service{
		Pool:       pool,
		Storage:    storage,
		Policy:     access.AdminPolicy{Queries: q},
		MaxBytes:   100,
		EventQuota: 12,
	}
	input := files.Upload{
		Actor:     person.ID,
		Event:     event,
		Key:       uuid.New(),
		Revision:  1,
		Filename:  "notes.txt",
		MediaType: "text/plain",
		Content:   []byte("test text"),
	}
	file, err := service.Upload(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := service.Upload(ctx, input)
	if err != nil || repeated.ID != file.ID {
		t.Fatalf("idempotent replay: %v %v", repeated, err)
	}
	input.Content = []byte("other")
	if _, err := service.Upload(ctx, input); !errors.Is(err, files.ErrIdempotency) {
		t.Fatalf("key conflict: %v", err)
	}
	input.Key = uuid.New()
	input.Revision = 9
	if _, err := service.Upload(ctx, input); !errors.Is(err, access.ErrConflict) {
		t.Fatalf("stale revision: %v", err)
	}
	input.Revision = 1
	input.Content = []byte("xx")
	var wg sync.WaitGroup
	successes := make(chan bool, 2)
	for range 2 {
		wg.Go(func() {
			attempt := input
			attempt.Key = uuid.New()
			_, err := service.Upload(ctx, attempt)
			if err != nil && !errors.Is(err, files.ErrQuota) {
				t.Error(err)
			}
			successes <- err == nil
		})
	}
	wg.Wait()
	close(successes)
	count := 0
	for success := range successes {
		if success {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("quota race admitted %d files", count)
	}
	metadata, data, err := service.Download(ctx, person.ID, event, file.ID)
	if err != nil || metadata.OwnerID != person.ID || string(data) != "test text" {
		t.Fatalf("download: %+v %q %v", metadata, data, err)
	}
	if _, _, err := service.Download(
		ctx,
		uuid.New(),
		event,
		file.ID,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatalf("unauthorized download: %v", err)
	}
	if err := storage.Remove(file.ObjectKey); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Download(
		ctx,
		person.ID,
		event,
		file.ID,
	); !errors.Is(
		err,
		files.ErrMissing,
	) {
		t.Fatalf("missing file: %v", err)
	}
}
