package registration

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/forms"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func formFixture() forms.Definition {
	return forms.Definition{
		ID:       uuid.New(),
		Name:     "Attendee registration",
		Audience: "attendee",
		Fields: []forms.Field{
			{
				ID:       uuid.New(),
				Label:    "Experience",
				Type:     "single",
				Required: true,
				Choices:  []string{"Starting out", "Experienced"},
			},
		},
	}
}

func TestFormPublicationResponseVersionsIsolationAndRetention(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	q := dbgen.New(pool)
	owner := testdb.Person(t, pool)
	id := testdb.Event(t, pool, owner.ID)
	person := testdb.Person(t, pool)
	other := testdb.Event(t, pool, person.ID)
	s := Service{Pool: pool}
	d := formFixture()
	v, err := s.SaveForm(ctx, owner.ID, id, 0, d)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveForm(
		ctx,
		person.ID,
		id,
		v.Revision,
		d,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("unauthorised form edit")
	}
	if _, err := s.PublishForm(ctx, owner.ID, id, d.ID, v.Revision, v.Version, false); err == nil {
		t.Fatal("publication without response warning")
	}
	v, err = s.PublishForm(ctx, owner.ID, id, d.ID, v.Revision, v.Version, true)
	if err != nil {
		t.Fatal(err)
	}
	answers := forms.Answers{d.Fields[0].ID.String(): json.RawMessage(`"Starting out"`)}
	store := func(eventID uuid.UUID, version int32, revision int64) error {
		return database.Within(ctx, pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
			return StoreResponse(ctx, tx, eventID, person.ID, d.ID, version, revision, answers)
		})
	}
	if err := store(id, 1, 0); err != nil {
		t.Fatal(err)
	}
	if err := store(other, 1, 0); err == nil {
		t.Fatal("cross-event form accepted")
	}
	d.Fields = append(
		d.Fields,
		forms.Field{
			ID:       uuid.New(),
			Label:    "New required field",
			Type:     "text",
			Required: true,
			Choices:  []string{},
		},
	)
	v, err = s.SaveForm(ctx, owner.ID, id, v.Revision, d)
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.PublishForm(ctx, owner.ID, id, d.ID, v.Revision, v.Version, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := store(id, 2, 1); err == nil {
		t.Fatal("new required field ignored")
	}
	if err := store(id, 1, 1); err != nil {
		t.Fatal("original response could not be corrected against original version", err)
	}
	if err := store(id, 1, 1); !errors.Is(err, access.ErrConflict) {
		t.Fatal("stale response correction accepted")
	}
	old, err := q.RegistrationFormVersion(
		ctx,
		dbgen.RegistrationFormVersionParams{EventID: id, FormID: d.ID, Version: 1},
	)
	if err != nil {
		t.Fatal(err)
	}
	var original forms.Definition
	if err := json.Unmarshal(old.Definition, &original); err != nil {
		t.Fatal(err)
	}
	if len(original.Fields) != 1 {
		t.Fatal("published form mutated")
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM form_response_versions WHERE event_id=$1", id).
		Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatal("response history lost")
	}
	if _, err := s.Forms(ctx, person.ID, id); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("private forms leaked")
	}
	if _, err := pool.Exec(
		ctx,
		"UPDATE events SET stage='completed',completed_at=$2 WHERE id=$1",
		id,
		time.Now().AddDate(0, 0, -91),
	); err != nil {
		t.Fatal(err)
	}
	if err := s.Retain(ctx); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM form_response_versions WHERE event_id=$1", id).
		Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("expired answers retained")
	}
}
