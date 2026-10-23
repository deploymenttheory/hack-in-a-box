package media_test

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/files"
	"github.com/deploymenttheory/hack-in-a-box/internal/media"
	"github.com/deploymenttheory/hack-in-a-box/internal/privacy"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type fixture struct {
	t                                   *testing.T
	pool                                *pgxpool.Pool
	s                                   media.Service
	privacy                             privacy.Service
	owner, photographer, subject, other uuid.UUID
	event, second                       uuid.UUID
	now                                 time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	pool := testdb.New(t, true)
	f := &fixture{t: t, pool: pool, now: time.Date(2027, 6, 20, 9, 0, 0, 0, time.UTC)}
	f.owner = testdb.Person(t, pool).ID
	f.photographer = testdb.Person(t, pool).ID
	f.subject = testdb.Person(t, pool).ID
	f.other = testdb.Person(t, pool).ID
	f.event = testdb.Event(t, pool, f.owner)
	f.second = testdb.Event(t, pool, f.owner)
	f.exec(
		`INSERT INTO event_memberships(event_id,person_id,role)VALUES($1,$2,'photographer')`,
		f.event,
		f.photographer,
	)
	f.exec(
		`INSERT INTO competition_claims(event_id,person_id,state)VALUES($1,$2,'confirmed'),($1,$3,'confirmed')`,
		f.event,
		f.subject,
		f.other,
	)
	f.exec(`UPDATE events SET published_at=$2 WHERE id=$1`, f.event, f.now.Add(-time.Hour))
	storage, err := files.OpenLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storage.Close() })
	f.privacy = privacy.Service{Pool: pool, Clock: func() time.Time { return f.now }}
	f.s = media.Service{
		Pool:    pool,
		Clock:   func() time.Time { return f.now },
		Privacy: f.privacy,
		Files: files.Service{
			Pool:       pool,
			Storage:    storage,
			Policy:     access.EventPolicy{Queries: dbgen.New(pool)},
			MaxBytes:   8 << 20,
			EventQuota: 64 << 20,
		},
	}
	return f
}

func (f *fixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.pool.Exec(f.t.Context(), sql, args...); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) revision() int64 {
	f.t.Helper()
	row, err := dbgen.New(f.pool).Event(f.t.Context(), f.event)
	if err != nil {
		f.t.Fatal(err)
	}
	return row.Revision
}

func (f *fixture) consent(choice string) {
	f.t.Helper()
	current, err := f.privacy.OwnPhotoConsent(f.t.Context(), f.subject, f.event)
	if err != nil {
		f.t.Fatal(err)
	}
	if _, err = f.privacy.SetOwnPhotoConsent(
		f.t.Context(),
		f.subject,
		f.event,
		current.Revision,
		choice,
	); err != nil {
		f.t.Fatal(err)
	}
}

func photo() []byte {
	im := image.NewNRGBA(image.Rect(0, 0, 4, 3))
	im.Set(1, 1, color.NRGBA{R: 200, A: 255})
	var b bytes.Buffer
	_ = png.Encode(&b, im)
	return b.Bytes()
}

func (f *fixture) upload(people []uuid.UUID) media.Item {
	f.t.Helper()
	item, err := f.s.CollectPhoto(
		f.t.Context(),
		media.Upload{
			ID: uuid.New(),
			Metadata: media.Metadata{
				Kind:             "photo",
				ChallengeRelated: true,
				Title:            "Hack photo",
				People:           people,
			},
			Reason: "Photographer upload",
			File: files.Upload{
				Actor:     f.photographer,
				Event:     f.event,
				Key:       uuid.New(),
				Revision:  f.revision(),
				Filename:  "hack.png",
				MediaType: "image/png",
				Content:   photo(),
			},
		},
	)
	if err != nil {
		f.t.Fatal(err)
	}
	return item
}

func (f *fixture) change(item media.Item, action string, review *media.Review) (media.Item, error) {
	return f.s.Change(
		f.t.Context(),
		f.owner,
		f.event,
		item.ID,
		f.revision(),
		media.Command{
			Action:    action,
			Version:   item.Version,
			CommandID: uuid.New(),
			Review:    review,
			Reason:    "Explicit organiser " + action,
		},
	)
}

func (f *fixture) release(item media.Item) media.Item {
	f.t.Helper()
	item, err := f.change(
		item,
		"review",
		&media.Review{
			ContentReviewed:        true,
			IdentificationReviewed: true,
			NoIdentifiablePeople:   len(item.People) == 0,
		},
	)
	if err != nil {
		f.t.Fatal(err)
	}
	item, err = f.change(item, "publish", nil)
	if err != nil {
		f.t.Fatal(err)
	}
	return item
}

func (f *fixture) reference(item media.Item) error {
	return database.Within(f.t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		_, err := f.s.ResolveReferenceInTransaction(
			f.t.Context(),
			tx,
			f.other,
			f.event,
			item.ID,
			item.Version,
		)
		return err
	})
}

func TestReleasedArchivedGalleryRevocationProtectsEveryNativeRead(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	f.consent("yes")
	item := f.upload([]uuid.UUID{f.subject})
	if _, err := f.s.Read(ctx, f.other, f.event, item.ID); !errors.Is(err, access.ErrNotFound) {
		t.Fatal("private draft visible", err)
	}
	if _, err := f.change(item, "publish", nil); err == nil {
		t.Fatal("unreviewed publication accepted")
	}
	item = f.release(item)
	file, original, err := f.s.Original(ctx, f.other, f.event, item.ID)
	if err != nil || !bytes.Equal(original, photo()) {
		t.Fatal("released original unavailable", err)
	}
	if _, err = f.s.Thumbnail(ctx, f.other, f.event, item.ID); err != nil {
		t.Fatal(err)
	}
	if err = f.reference(item); err != nil {
		t.Fatal(err)
	}
	// Neither an organiser nor an attendee can bypass consent through the generic file policy.
	for _, actor := range []uuid.UUID{f.owner, f.photographer, f.other} {
		if _, _, err = f.s.Files.Download(
			ctx,
			actor,
			f.event,
			file.ID,
		); !errors.Is(
			err,
			access.ErrForbidden,
		) {
			t.Fatal("ordinary file route bypassed native media", actor, err)
		}
	}
	if err = access.RequireOrdinaryFile(
		ctx,
		dbgen.New(f.pool),
		f.event,
		file.ID,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("ordinary content could adopt protected photo", err)
	}
	if _, err = (content.Service{Pool: f.pool}).Save(
		ctx,
		f.owner,
		f.event,
		uuid.New(),
		0,
		content.Input{
			Kind: "page",
			Document: content.Document{
				Title:       "Forbidden media attachment",
				Markdown:    "Native content policy must deny protected file adoption",
				Audience:    access.ContentAudience{Confirmed: true},
				Attachments: []uuid.UUID{file.ID},
			},
		},
	); err == nil {
		t.Fatal("native content publication route adopted protected media")
	}

	if _, _, err = f.s.Original(ctx, f.other, f.second, item.ID); err == nil {
		t.Fatal("cross-event photo visible")
	}
	f.exec(
		`UPDATE events SET stage='archived',writes_allowed=false,completed_at=$2 WHERE id=$1`,
		f.event,
		f.now,
	)
	f.consent("no")
	for _, actor := range []uuid.UUID{f.owner, f.photographer, f.other} {
		if _, _, err = f.s.Original(ctx, actor, f.event, item.ID); err == nil {
			t.Fatal("revoked original visible", actor)
		}
		if _, err = f.s.Thumbnail(ctx, actor, f.event, item.ID); err == nil {
			t.Fatal("revoked thumbnail visible", actor)
		}
	}
	page, err := f.s.List(ctx, f.other, f.event, uuid.Nil, 50)
	if err != nil || len(page.Items) != 0 {
		t.Fatal("revoked gallery metadata visible", page, err)
	}
	if err = f.reference(item); err == nil {
		t.Fatal("revoked showcase reference remained available")
	}
	own, err := f.privacy.OwnPhotoConsent(ctx, f.subject, f.event)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.privacy.SetOwnPhotoConsent(
		ctx,
		f.subject,
		f.event,
		own.Revision,
		"yes",
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("archived revocation seam allowed opt-in", err)
	}
	// Consent can be granted again only after writable policy returns. That does not revive old review.
	f.exec(`UPDATE events SET stage='live',writes_allowed=true WHERE id=$1`, f.event)
	f.consent("yes")
	if _, _, err = f.s.Original(ctx, f.owner, f.event, item.ID); err == nil {
		t.Fatal("yes after revocation resurrected obsolete review")
	}
	if err = f.reference(item); err == nil {
		t.Fatal("obsolete consent binding resurrected showcase reference")
	}
}

func TestExplicitUntaggedReviewCurrentRolesAndActualPublication(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	item := f.upload(nil)
	if _, err := f.change(
		item,
		"review",
		&media.Review{ContentReviewed: true, IdentificationReviewed: true},
	); err == nil {
		t.Fatal("untagged treated as automatic consent")
	}
	item, err := f.change(
		item,
		"review",
		&media.Review{
			ContentReviewed:        true,
			IdentificationReviewed: true,
			NoIdentifiablePeople:   true,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	f.exec(`UPDATE events SET published_at=NULL WHERE id=$1`, f.event)
	if _, err = f.change(item, "publish", nil); err == nil {
		t.Fatal("planned/absent challenge release accepted")
	}
	f.exec(`UPDATE events SET published_at=$2 WHERE id=$1`, f.event, f.now.Add(-time.Minute))
	item, err = f.change(item, "publish", nil)
	if err != nil {
		t.Fatal(err)
	}
	f.exec(
		`UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2`,
		f.event,
		f.other,
	)
	if _, _, err = f.s.Original(
		ctx,
		f.other,
		f.event,
		item.ID,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("withdrawn attendee retained photo", err)
	}
	f.exec(
		`UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='organiser'`,
		f.event,
		f.owner,
	)
	if _, _, err = f.s.Original(ctx, f.subject, f.event, item.ID); err == nil {
		t.Fatal("retired reviewer retained publication authority")
	}
	f.exec(
		`UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='photographer'`,
		f.event,
		f.photographer,
	)
	if _, err = f.s.Read(
		ctx,
		f.photographer,
		f.event,
		item.ID,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("retired photographer retained private workspace", err)
	}
}

func TestPublishAndConsentRevocationSerializeAndCannotLeaveReadableRelease(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	f.consent("yes")
	item := f.upload([]uuid.UUID{f.subject})
	item, err := f.change(
		item,
		"review",
		&media.Review{ContentReviewed: true, IdentificationReviewed: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	own, err := f.privacy.OwnPhotoConsent(ctx, f.subject, f.event)
	if err != nil {
		t.Fatal(err)
	}
	revision := f.revision()
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	results := make(chan error, 2)
	go func() {
		defer wg.Done()
		<-start
		_, e := f.s.Change(
			context.Background(),
			f.owner,
			f.event,
			item.ID,
			revision,
			media.Command{
				Action:    "publish",
				Version:   item.Version,
				CommandID: uuid.New(),
				Reason:    "Concurrent explicit release",
			},
		)
		results <- e
	}()
	go func() {
		defer wg.Done()
		<-start
		_, e := f.privacy.SetOwnPhotoConsent(
			context.Background(),
			f.subject,
			f.event,
			own.Revision,
			"no",
		)
		results <- e
	}()
	close(start)
	wg.Wait()
	close(results)
	success := 0
	for e := range results {
		if e == nil {
			success++
		} else if !errors.Is(e, access.ErrConflict) {
			t.Fatal(e)
		}
	}
	if success < 1 {
		t.Fatal("neither serialized operation succeeded")
	}
	if _, _, err = f.s.Original(ctx, f.other, f.event, item.ID); err == nil {
		t.Fatal("publish/revoke race left readable bytes")
	}
	if err = f.reference(item); err == nil {
		t.Fatal("publish/revoke race left public reference")
	}
}

func TestOperationalMediaBeforeCommonReleaseAndChallengeMediaEmbargo(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	f.exec(`UPDATE events SET published_at=NULL WHERE id=$1`, f.event)
	id := uuid.New()
	item, err := f.s.Change(
		ctx,
		f.photographer,
		f.event,
		id,
		f.revision(),
		media.Command{
			Action:    "save",
			Version:   0,
			CommandID: uuid.New(),
			Reason:    "Venue access recording for review",
			Metadata: &media.Metadata{
				Kind:             "recording",
				Title:            "Venue tour",
				RecordingURL:     "https://recordings.example.test/venue",
				People:           []uuid.UUID{},
				ChallengeRelated: false,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	item = f.release(item)
	if _, err = f.s.Read(ctx, f.other, f.event, item.ID); err != nil {
		t.Fatal("reviewed operational media wrongly embargoed", err)
	}
	if err = f.reference(item); err != nil {
		t.Fatal("reviewed venue native reference unavailable", err)
	}
	updated, err := f.s.Change(
		ctx,
		f.photographer,
		f.event,
		item.ID,
		f.revision(),
		media.Command{
			Action:    "save",
			Version:   item.Version,
			CommandID: uuid.New(),
			Reason:    "Link now includes challenge content",
			Metadata: &media.Metadata{
				Kind:             "recording",
				Title:            "Challenge discussion",
				RecordingURL:     "https://recordings.example.test/challenge",
				People:           []uuid.UUID{},
				ChallengeRelated: true,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	updated, err = f.change(
		updated,
		"review",
		&media.Review{
			ContentReviewed:        true,
			IdentificationReviewed: true,
			NoIdentifiablePeople:   true,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.change(updated, "publish", nil); !errors.Is(err, access.ErrConflict) {
		t.Fatal("challenge recording escaped actual common publication", err)
	}
	if _, err = f.s.Read(ctx, f.other, f.event, item.ID); !errors.Is(err, access.ErrNotFound) {
		t.Fatal("metadata edit retained obsolete gallery release", err)
	}
}

func TestConsentBeforeUploadDeleteAndRetentionNeverAllowReleaseOrBytes(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	f.consent("no")
	item := f.upload([]uuid.UUID{f.subject})
	if _, _, err := f.s.Original(ctx, f.photographer, f.event, item.ID); err == nil {
		t.Fatal("opt-out before upload exposed original")
	}
	if _, err := f.change(
		item,
		"review",
		&media.Review{ContentReviewed: true, IdentificationReviewed: true},
	); err == nil {
		t.Fatal("opt-out before upload accepted for review")
	}
	f.consent("yes")
	item = f.release(item)
	if err := f.privacy.Delete(ctx, f.subject, f.event, f.subject); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.s.Original(ctx, f.owner, f.event, item.ID); err == nil {
		t.Fatal("deleted consent profile retained download")
	}
	f.consent("yes")
	if _, _, err := f.s.Original(ctx, f.owner, f.event, item.ID); err == nil {
		t.Fatal("recreated profile revived old approval")
	}
	if err := f.s.Maintain(ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Maintain(ctx); err != nil {
		t.Fatal(err)
	}
	var revocations int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE event_id=$1 AND action='media.revoked'`, f.event).
		Scan(&revocations); err != nil ||
		revocations != 1 {
		t.Fatal("revocation journal missing/duplicated", revocations, err)
	}
	// Fresh explicit review is possible after a deliberate metadata revision.
	item, err := f.s.Read(ctx, f.owner, f.event, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	item, err = f.s.Change(
		ctx,
		f.owner,
		f.event,
		item.ID,
		f.revision(),
		media.Command{
			Action:    "save",
			Version:   item.Version,
			CommandID: uuid.New(),
			Reason:    "Fresh consent review after deletion",
			Metadata: &media.Metadata{
				Kind:   "photo",
				Title:  "Fresh permission",
				People: []uuid.UUID{f.subject},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	item = f.release(item)
	f.exec(
		`UPDATE events SET stage='completed',completed_at=$2 WHERE id=$1`,
		f.event,
		f.now.Add(-91*24*time.Hour),
	)
	if _, _, err = f.s.Original(ctx, f.owner, f.event, item.ID); err == nil {
		t.Fatal("expired personal consent allowed download before retention sweep")
	}
}

func TestPhotoIntentIdempotencyAndOrdinaryFileCannotBeAdopted(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	upload := media.Upload{
		ID: uuid.New(),
		Metadata: media.Metadata{
			Kind:   "photo",
			Title:  "Native once-only photo",
			People: []uuid.UUID{},
		},
		Reason: "Synthetic exact intent",
		File: files.Upload{
			Actor:     f.photographer,
			Event:     f.event,
			Key:       uuid.New(),
			Revision:  f.revision(),
			Filename:  "native.png",
			MediaType: "image/png",
			Content:   photo(),
		},
	}
	first, err := f.s.CollectPhoto(ctx, upload)
	if err != nil {
		t.Fatal(err)
	}
	again, err := f.s.CollectPhoto(ctx, upload)
	if err != nil || again.ID != first.ID || again.Version != first.Version {
		t.Fatal("identical photo intent did not retain immutable identity", first, again, err)
	}
	upload.Metadata.Title = "Changed intent using same key"
	if _, err = f.s.CollectPhoto(ctx, upload); !errors.Is(err, access.ErrConflict) {
		t.Fatal("changed metadata adopted prior upload receipt", err)
	}
	old := files.Upload{
		Actor:     f.owner,
		Event:     f.event,
		Key:       uuid.New(),
		Revision:  f.revision(),
		Filename:  "ordinary.png",
		MediaType: "image/png",
		Content:   photo(),
	}
	file, err := f.s.Files.Upload(ctx, old)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.CollectPhoto(
		ctx,
		media.Upload{
			ID:       uuid.New(),
			Metadata: media.Metadata{Kind: "photo", Title: "Forbidden later association"},
			Reason:   "Cannot change prior file policy",
			File:     old,
		},
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatal("existing ordinary file silently acquired new media association", err)
	}
	if err = access.RequireOrdinaryFile(ctx, dbgen.New(f.pool), f.event, file.ID); err != nil {
		t.Fatal("failed association changed original file authority", err)
	}
	var retained int
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM event_media WHERE event_id=$1`, f.event).
		Scan(&retained); err != nil ||
		retained != 1 {
		t.Fatal("retries/adoption created extra media drafts", retained, err)
	}
	f.exec(
		`UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='photographer'`,
		f.event,
		f.photographer,
	)
	upload.Metadata.Title = "Native once-only photo"
	if _, err = f.s.CollectPhoto(ctx, upload); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("existing receipt bypassed current photographer authority", err)
	}
}
