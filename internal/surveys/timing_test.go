package surveys

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/forms"
)

// Change the authoritative dates through the native event service, not a survey
// shadow or direct SQL date mutation. The fixture represents a writable event
// with no common challenge publication; this is a current-end edit, not a claim
// to reverify the event owner's separately tested post-publication postponement.
func (f *fixture) moveEventEnd(end time.Time) {
	f.t.Helper()
	service := event.Service{Pool: f.pool, Clock: f.s.Now}
	current, err := service.Read(f.t.Context(), f.owner.ID, f.event)
	if err != nil {
		f.t.Fatal(err)
	}
	dates := current.Configuration
	dates.LiveEnd = end
	_, err = service.Update(f.t.Context(), f.owner.ID, f.event, current.Revision, event.Input{
		Name: current.Name, Slug: current.Slug, Configuration: dates, Reason: "Extend current native live end",
	})
	if err != nil {
		f.t.Fatal(err)
	}
}

func TestSurveyApprovalAndPublicationRecheckCurrentNativeEnd(t *testing.T) {
	for _, action := range []string{"approve", "publish"} {
		t.Run(action, func(t *testing.T) {
			f := setup(t)
			cfg, err := f.s.Change(t.Context(), f.owner.ID, f.event, f.id, 0, Command{Action: "save", Definition: &f.definition})
			if err != nil {
				t.Fatal(err)
			}
			if action == "publish" {
				if _, err = f.s.Change(t.Context(), f.owner.ID, f.event, f.id, cfg.Version, Command{Action: "approve"}); err != nil {
					t.Fatal(err)
				}
			}
			f.moveEventEnd(f.s.now().Add(30 * time.Minute))
			if _, err = f.s.Change(t.Context(), f.owner.ID, f.event, f.id, cfg.Version, Command{Action: action}); !errors.Is(err, access.ErrInvalid) {
				t.Fatal("stale approved opening preceded current native end", err)
			}
		})
	}
}

func TestPublishedSurveyCurrentEndBlocksEffectsWithoutMovingWindowOrHistory(t *testing.T) {
	f := setup(t)
	cfg := f.publish()
	command := ResponseCommand{CommandID: uuid.New(), FormVersion: cfg.Version, Answers: forms.Answers{f.choice.String(): json.RawMessage(`"no"`)}}
	original, err := f.s.Respond(t.Context(), f.person.ID, f.event, f.id, 0, command)
	if err != nil {
		t.Fatal(err)
	}
	end := f.s.now().Add(30 * time.Minute)
	f.moveEventEnd(end)
	own, err := f.s.ReadOwn(t.Context(), f.person.ID, f.event, f.id)
	if err != nil || own.CanRespond || (own.Response == nil || own.Response.Revision != original.Revision || own.Response.FormVersion != original.FormVersion || !own.Response.SubmittedAt.Equal(original.SubmittedAt) || !reflect.DeepEqual(own.Response.Answers, original.Answers)) || !own.OpensAt.Equal(f.definition.OpensAt) || !own.ClosesAt.Equal(f.definition.ClosesAt) {
		t.Fatal("current end lost history, moved window or allowed response", own, err)
	}
	directory, err := f.s.OwnDirectory(t.Context(), f.person.ID, f.event, uuid.Nil, 100)
	if err != nil || len(directory.Items) != 1 || directory.Items[0].CanRespond {
		t.Fatal("directory bypass", directory, err)
	}
	if _, err = f.s.Respond(t.Context(), f.person.ID, f.event, f.id, 0, command); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("replay bypassed current event time", err)
	}
	err = database.Within(t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		sources, next, e := f.s.DiscoverNotificationSourcesPageInTransaction(t.Context(), tx, f.event, nil, uuid.Nil, 100)
		if e != nil || len(sources) != 0 || next != nil {
			t.Fatalf("pre-end sources: %v %v %v", sources, next, e)
		}
		_, e = f.s.ResolveInvitationAudienceInTransaction(t.Context(), tx, f.event, f.id, cfg.Version)
		if !errors.Is(e, access.ErrForbidden) {
			t.Fatalf("pre-end audience: %v", e)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ChangeReport(t.Context(), f.owner.ID, f.event, f.id, 0, ReportCommand{Action: "review_text", ReviewID: uuid.New(), ReviewedText: true}); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("pre-end report effect", err)
	}
	f.clock.Store(end.UnixNano())
	own, err = f.s.ReadOwn(t.Context(), f.person.ID, f.event, f.id)
	if err != nil || !own.CanRespond {
		t.Fatal("current end boundary did not reopen retained window", err)
	}
	command.CommandID = uuid.New()
	if response, e := f.s.Respond(t.Context(), f.person.ID, f.event, f.id, original.Revision, command); e != nil || response.Revision != 2 {
		t.Fatal("response at current end", response, e)
	}
	err = database.Within(t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		sources, _, e := f.s.DiscoverNotificationSourcesPageInTransaction(t.Context(), tx, f.event, nil, uuid.Nil, 100)
		if e != nil || len(sources) != 1 {
			t.Fatalf("post-end sources: %v %v", sources, e)
		}
		people, e := f.s.ResolveInvitationAudienceInTransaction(t.Context(), tx, f.event, f.id, cfg.Version)
		if e != nil || len(people) != 1 || people[0] != f.other.ID {
			t.Fatalf("current nonrespondent audience: %v %v", people, e)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// If the live end moves past the retained close, no replacement window opens.
	end = f.definition.ClosesAt.Add(30 * time.Minute)
	f.moveEventEnd(end)
	f.clock.Store(f.definition.ClosesAt.UnixNano())
	if _, err = f.s.ChangeReport(t.Context(), f.owner.ID, f.event, f.id, 0, ReportCommand{Action: "review_text", ReviewID: uuid.New(), ReviewedText: true}); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("closed survey created report before current live end", err)
	}
	f.clock.Store(end.UnixNano())
	own, err = f.s.ReadOwn(t.Context(), f.person.ID, f.event, f.id)
	if err != nil || own.CanRespond || !own.ClosesAt.Equal(f.definition.ClosesAt) {
		t.Fatal("missed window shifted", own, err)
	}
	review, err := f.s.ChangeReport(t.Context(), f.owner.ID, f.event, f.id, 0, ReportCommand{Action: "review_text", ReviewID: uuid.New(), ReviewedText: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"approve", "publish"} {
		if _, err = f.s.ChangeReport(t.Context(), f.owner.ID, f.event, f.id, review.Version, ReportCommand{Action: action, ReviewID: review.ReviewID}); err != nil {
			t.Fatal(err)
		}
	}
	released, err := f.s.PublishedReport(t.Context(), f.person.ID, f.event, f.id)
	if err != nil {
		t.Fatal(err)
	}
	f.moveEventEnd(end.Add(time.Hour))
	retained, err := f.s.PublishedReport(t.Context(), f.person.ID, f.event, f.id)
	if err != nil || !reflect.DeepEqual(retained, released) {
		t.Fatal("date change rewrote or unpublished history", retained, err)
	}
	if _, err = f.s.ChangeReport(t.Context(), f.owner.ID, f.event, f.id, review.Version, ReportCommand{Action: "review_text", ReviewID: uuid.New(), ReviewedText: true}); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("new correction before current end", err)
	}
}

func TestResponseCommandReceiptUsesPersistedTimeAndBody(t *testing.T) {
	f := setup(t)
	cfg := f.publish()
	// A ticking clock exposes separate timestamp reads and PostgreSQL precision;
	// the first acknowledgement must match the exact persisted retry receipt.
	f.s.Now = func() time.Time { return time.Unix(0, f.clock.Add(1234567)).UTC() }
	command := ResponseCommand{CommandID: uuid.New(), FormVersion: cfg.Version, Answers: forms.Answers{f.choice.String(): json.RawMessage(`"no"`)}}
	first, err := f.s.Respond(t.Context(), f.person.ID, f.event, f.id, 0, command)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := f.s.Respond(t.Context(), f.person.ID, f.event, f.id, 0, command)
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, _ := json.Marshal(first)
	replayJSON, _ := json.Marshal(replay)
	if string(firstJSON) != string(replayJSON) {
		t.Fatalf("initial acknowledgement differs from persisted exact receipt:\n%s\n%s", firstJSON, replayJSON)
	}
}
