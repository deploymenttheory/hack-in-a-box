package outcomes

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestReviewedProjectPositiveProjectionSearchWithdrawalAndExactReplay(t *testing.T) {
	pool := testdb.New(t, true)
	org, author, reader := testdb.Person(t, pool), testdb.Person(t, pool), testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, org.ID)
	s := Service{Pool: pool, Now: func() time.Time { return time.Now().UTC() }}
	id := uuid.New()
	if _, err := s.Change(t.Context(), author.ID, id, 0, proposal()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Change(
		t.Context(),
		org.ID,
		id,
		1,
		Command{
			ID:         uuid.New(),
			Action:     "adopt",
			Provenance: &Provenance{EventID: &eventID},
			Reason:     "Deliberate review",
		},
	); err != nil {
		t.Fatal(err)
	}
	fields := PublicationFields{
		Problem:  "Reviewed searchable problem",
		Solution: "Reviewed solution",
		Audience: "organisation",
		Links: []ProjectLink{
			{ID: uuid.New(), Label: "Approved documentation", URL: "https://example.org/project"},
		},
	}
	save := PublicationCommand{
		ID:            uuid.New(),
		Action:        "save",
		SourceVersion: 2,
		Fields:        &fields,
		Reason:        "Review exactly these public fields",
	}
	if _, err := s.ChangePublication(
		t.Context(),
		author.ID,
		id,
		0,
		save,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("author self-published", err)
	}
	if _, err := s.ChangePublication(t.Context(), org.ID, id, 0, save); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Public(t.Context(), reader.ID, id); !errors.Is(err, access.ErrNotFound) {
		t.Fatal("draft visible", err)
	}
	publish := PublicationCommand{
		ID:            uuid.New(),
		Action:        "publish",
		SourceVersion: 2,
		Reason:        "Explicitly publish reviewed fields",
	}
	if _, err := s.ChangePublication(t.Context(), org.ID, id, 1, publish); err != nil {
		t.Fatal(err)
	}
	public, err := s.Public(t.Context(), reader.ID, id)
	if err != nil || public.Version != 2 || public.EventName != nil || public.TeamName != nil ||
		len(public.Links) != 1 {
		t.Fatal(public, err)
	}
	raw, _ := json.Marshal(public)
	for _, secret := range []string{"PRIVATE original", author.ID.String(), "proposed_value", "owner_id", "evidence", "Review exactly"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("private field in project: %s", secret)
		}
	}
	page, err := s.Projects(t.Context(), reader.ID, uuid.Nil, "SEARCHABLE", 1)
	if err != nil || len(page.Items) != 1 {
		t.Fatal("public search", page, err)
	}
	privateSearch, err := s.Projects(t.Context(), reader.ID, uuid.Nil, "PRIVATE original", 1)
	if err != nil || len(privateSearch.Items) != 0 {
		t.Fatal("private prose searchable", privateSearch, err)
	}
	if _, err = s.ChangePublication(
		t.Context(),
		org.ID,
		id,
		2,
		PublicationCommand{
			ID:            uuid.New(),
			Action:        "withdraw",
			SourceVersion: 2,
			Reason:        "Withdraw deliberately",
		},
	); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Public(t.Context(), reader.ID, id); !errors.Is(err, access.ErrNotFound) {
		t.Fatal("withdrawn visible", err)
	}
	replay, err := s.ChangePublication(t.Context(), org.ID, id, 1, publish)
	if err != nil || replay.Version != 2 || replay.CurrentVersion != 3 ||
		replay.PublishedVersion != nil {
		t.Fatal("replay must retain exact result without republishing", replay, err)
	}
	if _, err = s.Public(t.Context(), reader.ID, id); !errors.Is(err, access.ErrNotFound) {
		t.Fatal("replay republished", err)
	}
	fields.Audience = "event"
	if _, err = s.ChangePublication(
		t.Context(),
		org.ID,
		id,
		3,
		PublicationCommand{
			ID:            uuid.New(),
			Action:        "save",
			SourceVersion: 2,
			Fields:        &fields,
			Reason:        "Narrow to event members",
		},
	); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ChangePublication(
		t.Context(),
		org.ID,
		id,
		4,
		PublicationCommand{
			ID:            uuid.New(),
			Action:        "publish",
			SourceVersion: 2,
			Reason:        "Publish narrow audience",
		},
	); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Public(t.Context(), author.ID, id); !errors.Is(err, access.ErrNotFound) {
		t.Fatal("own private resource granted event audience", err)
	}
	if page, err = s.Projects(
		t.Context(),
		reader.ID,
		uuid.Nil,
		"",
		1,
	); err != nil || len(page.Items) != 0 ||
		page.NextCursor != nil {
		t.Fatal("hidden project leaked via pagination", page, err)
	}
	if _, err = s.Public(t.Context(), org.ID, id); err != nil {
		t.Fatal("current organiser audience", err)
	}
}

func TestPublicationRejectsStaleSourceAndUnavailableNativeMediaOwner(t *testing.T) {
	pool := testdb.New(t, true)
	org, author := testdb.Person(t, pool), testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, org.ID)
	s := Service{Pool: pool}
	id := uuid.New()
	if _, err := s.Change(t.Context(), author.ID, id, 0, proposal()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Change(
		t.Context(),
		org.ID,
		id,
		1,
		Command{
			ID:         uuid.New(),
			Action:     "adopt",
			Provenance: &Provenance{EventID: &eventID},
			Reason:     "Deliberate adoption",
		},
	); err != nil {
		t.Fatal(err)
	}
	fields := PublicationFields{
		Problem:  "Reviewed",
		Solution: "Reviewed",
		Audience: "organisation",
		Media:    []MediaReference{{EventID: eventID, ID: uuid.New(), Version: 1}},
	}
	if _, err := s.ChangePublication(
		t.Context(),
		org.ID,
		id,
		0,
		PublicationCommand{
			ID:            uuid.New(),
			Action:        "save",
			SourceVersion: 2,
			Fields:        &fields,
			Reason:        "Propose native media reference",
		},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ChangePublication(
		t.Context(),
		org.ID,
		id,
		1,
		PublicationCommand{
			ID:            uuid.New(),
			Action:        "publish",
			SourceVersion: 2,
			Reason:        "Try unavailable owner",
		},
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatal("unavailable native media claimed approved", err)
	}
	if _, err := s.Change(
		t.Context(),
		org.ID,
		id,
		2,
		Command{
			ID:     uuid.New(),
			Action: "moderate",
			Status: "reviewing",
			Reason: "Source changed after review",
		},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ChangePublication(
		t.Context(),
		org.ID,
		id,
		1,
		PublicationCommand{
			ID:            uuid.New(),
			Action:        "publish",
			SourceVersion: 2,
			Reason:        "Stale reviewed source",
		},
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatal("stale source published", err)
	}
}
