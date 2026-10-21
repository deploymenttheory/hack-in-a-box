package files_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/files"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestFileAccessWithOverlappingRolesAndRevocation(t *testing.T) {
	f := testdb.NewAuthorization(t)
	ctx := t.Context()
	f.Grant(t, f.First, f.Person.ID, "organiser", true)
	f.Grant(t, f.Second, f.Person.ID, "photographer", true)
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
	upload := func(event uuid.UUID) uuid.UUID {
		t.Helper()
		row, err := service.Upload(
			ctx,
			files.Upload{
				Actor:     f.Owner.ID,
				Event:     event,
				Key:       uuid.New(),
				Revision:  1,
				Filename:  "private.txt",
				MediaType: "text/plain",
				Content:   []byte("private-event-content"),
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		return row.ID
	}
	first, second := upload(f.First), upload(f.Second)
	if _, _, err := service.Download(ctx, f.Person.ID, f.First, first); err != nil {
		t.Fatal(err)
	}
	for _, target := range []struct{ event, file uuid.UUID }{{f.Second, second}, {f.First, second}, {f.Second, first}} {
		_, body, err := service.Download(ctx, f.Person.ID, target.event, target.file)
		if err == nil || len(body) > 0 {
			t.Fatal("cross-event or unrelated-role file leaked")
		}
	}
	f.Grant(t, f.First, f.Person.ID, "organiser", false)
	if _, body, err := service.Download(
		ctx,
		f.Person.ID,
		f.First,
		first,
	); err == nil ||
		len(body) > 0 {
		t.Fatal("revoked organiser retained file access")
	}
}
