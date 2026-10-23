package surveys

import (
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/forms"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type fixture struct {
	t                       *testing.T
	pool                    *pgxpool.Pool
	s                       Service
	clock                   atomic.Int64
	owner, person, other    dbgen.Person
	event, id, choice, text uuid.UUID
	definition              Definition
}

func setup(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{
		t:      t,
		pool:   testdb.New(t, true),
		id:     uuid.New(),
		choice: uuid.New(),
		text:   uuid.New(),
	}
	f.owner = testdb.Person(t, f.pool)
	f.person = testdb.Person(t, f.pool)
	f.other = testdb.Person(t, f.pool)
	f.event = testdb.Event(t, f.pool, f.owner.ID)
	now := time.Now().UTC().Truncate(time.Second)
	f.clock.Store(now.UnixNano())
	f.s = Service{Pool: f.pool, Now: func() time.Time { return time.Unix(0, f.clock.Load()).UTC() }}
	dates := event.DefaultConfiguration()
	dates.LiveStart = now.Add(-12 * time.Hour)
	dates.LiveEnd = now.Add(-4 * time.Hour)
	dates.SubmissionCutoff = now.Add(-5 * time.Hour)
	dates.TeamFreeze = now.AddDate(0, 0, -8)
	dates.AdmissionCutoff = dates.TeamFreeze.Add(-time.Hour)
	raw, _ := json.Marshal(dates)
	f.exec(
		"UPDATE events SET configuration=$2,stage='completed',completed_at=$3 WHERE id=$1",
		f.event,
		raw,
		now.Add(-3*time.Hour),
	)
	for _, person := range []uuid.UUID{f.person.ID, f.other.ID} {
		f.exec(
			"INSERT INTO competition_claims(event_id,person_id,state) VALUES($1,$2,'confirmed')",
			f.event,
			person,
		)
	}
	f.definition = Definition{
		Form: forms.Definition{
			ID:       f.id,
			Name:     "Post-event feedback",
			Audience: "attendee",
			Fields: []forms.Field{
				{
					ID:       f.choice,
					Label:    "Did you learn something?",
					Type:     "single",
					Required: true,
					Choices:  []string{"yes", "no"},
				},
				{
					ID:          f.text,
					Label:       "Private explanation",
					Type:        "text",
					Required:    true,
					VisibleWhen: &forms.Condition{FieldID: f.choice, Equals: "yes"},
				},
			},
		},
		OpensAt:       now.Add(-time.Hour),
		ClosesAt:      now.Add(time.Hour),
		MinimumCohort: 5,
		Measures:      []QuestionMeasure{},
	}
	return f
}

func (f *fixture) exec(query string, args ...any) {
	f.t.Helper()
	if _, err := f.pool.Exec(f.t.Context(), query, args...); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) publish() Configuration {
	f.t.Helper()
	cfg, err := f.s.Change(
		f.t.Context(),
		f.owner.ID,
		f.event,
		f.id,
		0,
		Command{Action: "save", Definition: &f.definition},
	)
	if err != nil {
		f.t.Fatal(err)
	}
	for _, action := range []string{"approve", "publish"} {
		cfg, err = f.s.Change(
			f.t.Context(),
			f.owner.ID,
			f.event,
			f.id,
			cfg.Version,
			Command{Action: action},
		)
		if err != nil {
			f.t.Fatal(err)
		}
	}
	return cfg
}

func TestFixedSurveyVersionPrivateAuthorshipEligibilityAndDuplicateResponses(t *testing.T) {
	f := setup(t)
	cfg := f.publish()
	input := ResponseCommand{
		CommandID:   uuid.New(),
		FormVersion: cfg.Version,
		Answers: forms.Answers{
			f.choice.String(): json.RawMessage(`"no"`),
			f.text.String():   json.RawMessage(`"PRIVATE-HIDDEN-MUST-BE-DROPPED"`),
		},
	}
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			if _, err := f.s.Respond(
				t.Context(),
				f.person.ID,
				f.event,
				f.id,
				0,
				input,
			); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	own, err := f.s.ReadOwn(t.Context(), f.person.ID, f.event, f.id)
	if err != nil || own.Response == nil || own.Response.Revision != 1 ||
		len(own.Response.Answers) != 1 ||
		own.ResponseVisibility != ResponseVisibility {
		t.Fatal("response identity or visibility incorrect", err)
	}
	var retained int
	if err = f.pool.QueryRow(t.Context(), "SELECT count(*) FROM survey_responses WHERE survey_id=$1", f.id).
		Scan(&retained); err != nil ||
		retained != 1 {
		t.Fatal("duplicate response created another authored record", err)
	}
	input.CommandID = uuid.New()
	input.Answers[f.choice.String()] = json.RawMessage(`"yes"`)
	input.Answers[f.text.String()] = json.RawMessage(`"PRIVATE-AUTHORED-RESPONSE"`)
	if _, err = f.s.Respond(t.Context(), f.person.ID, f.event, f.id, 1, input); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.Responses(
		t.Context(),
		f.other.ID,
		f.event,
		f.id,
		uuid.Nil,
		10,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("another participant accessed authored responses", err)
	}
	page, err := f.s.Responses(t.Context(), f.owner.ID, f.event, f.id, uuid.Nil, 10)
	if err != nil || len(page.Items) != 1 || page.Items[0].PersonID != f.person.ID ||
		page.Items[0].Response.Revision != 2 {
		t.Fatal("organiser authorship projection incorrect", err)
	}
	late := testdb.Person(t, f.pool)
	f.exec(
		"INSERT INTO competition_claims(event_id,person_id,state) VALUES($1,$2,'confirmed')",
		f.event,
		late.ID,
	)
	if _, err = f.s.ReadOwn(
		t.Context(),
		late.ID,
		f.event,
		f.id,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("late role grant fabricated an invitation", err)
	}
	if _, err = f.s.Change(
		t.Context(),
		f.owner.ID,
		f.event,
		f.id,
		cfg.Version,
		Command{Action: "save", Definition: &f.definition},
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatal("published questions changed beneath responses", err)
	}
	f.exec(
		"UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2",
		f.event,
		f.person.ID,
	)
	if _, err = f.s.ReadOwn(
		t.Context(),
		f.person.ID,
		f.event,
		f.id,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("withdrawn participant retained response access", err)
	}
}

func TestClosingInstantAndArchiveFreezeResponseMutationEvenKnownCommand(t *testing.T) {
	f := setup(t)
	cfg := f.publish()
	input := ResponseCommand{
		CommandID:   uuid.New(),
		FormVersion: cfg.Version,
		Answers:     forms.Answers{f.choice.String(): json.RawMessage(`"no"`)},
	}
	if _, err := f.s.Respond(t.Context(), f.person.ID, f.event, f.id, 0, input); err != nil {
		t.Fatal(err)
	}
	f.clock.Store(f.definition.ClosesAt.UnixNano())
	if _, err := f.s.Respond(
		t.Context(),
		f.person.ID,
		f.event,
		f.id,
		0,
		input,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("known command bypassed exact close", err)
	}
	own, err := f.s.ReadOwn(t.Context(), f.person.ID, f.event, f.id)
	if err != nil || own.CanRespond || own.Response == nil {
		t.Fatal("closed own response is not readable or is still editable", err)
	}
	f.exec("UPDATE events SET stage='archived',writes_allowed=false WHERE id=$1", f.event)
	if _, err = f.s.Change(
		t.Context(),
		f.owner.ID,
		f.event,
		f.id,
		cfg.Version,
		Command{Action: "cancel"},
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("survey control bypassed archive freeze", err)
	}
}

func TestResponseRechecksClockAndEligibilityAfterWaitingForEventLock(t *testing.T) {
	for _, kind := range []string{"close", "withdraw"} {
		t.Run(kind, func(t *testing.T) {
			f := setup(t)
			cfg := f.publish()
			tx, err := f.pool.Begin(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(t.Context())
			if _, err = dbgen.New(tx).LockEvent(t.Context(), f.event); err != nil {
				t.Fatal(err)
			}
			started, done := make(chan struct{}), make(chan error, 1)
			go func() {
				close(started)
				_, e := f.s.Respond(
					t.Context(),
					f.person.ID,
					f.event,
					f.id,
					0,
					ResponseCommand{
						CommandID:   uuid.New(),
						FormVersion: cfg.Version,
						Answers:     forms.Answers{f.choice.String(): json.RawMessage(`"no"`)},
					},
				)
				done <- e
			}()
			<-started
			// The event lock is held while the authority/clock changes, so the writer can
			// only proceed with the newly authoritative state, regardless of scheduling.
			if kind == "close" {
				f.clock.Store(f.definition.ClosesAt.UnixNano())
			} else if _, err = tx.Exec(t.Context(), "UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2", f.event, f.person.ID); err != nil {
				t.Fatal(err)
			}
			if err = tx.Commit(t.Context()); err != nil {
				t.Fatal(err)
			}
			err = <-done
			expected := access.ErrForbidden
			if !errors.Is(err, expected) {
				t.Fatal("blocked response ignored current state", err)
			}
			var count int
			if err = f.pool.QueryRow(t.Context(), "SELECT count(*) FROM survey_responses WHERE event_id=$1", f.event).
				Scan(&count); err != nil ||
				count != 0 {
				t.Fatal("denied race persisted response", err, count)
			}
		})
	}
}

func TestDifferentResponseCommandsRaceAndNativeRoleAudienceRemainsCurrent(t *testing.T) {
	f := setup(t)
	f.definition.Form.Audience = "mentor"
	for _, id := range []uuid.UUID{f.person.ID, f.other.ID} {
		f.exec(
			"INSERT INTO event_memberships(event_id,person_id,role,active)VALUES($1,$2,'mentor',true)",
			f.event,
			id,
		)
	}
	cfg := f.publish()
	results := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := f.s.Respond(
				t.Context(),
				f.person.ID,
				f.event,
				f.id,
				0,
				ResponseCommand{
					CommandID:   uuid.New(),
					FormVersion: cfg.Version,
					Answers:     forms.Answers{f.choice.String(): json.RawMessage(`"no"`)},
				},
			)
			results <- err
		}()
	}
	a, b := <-results, <-results
	if (a == nil) == (b == nil) || (a != nil && !errors.Is(a, access.ErrConflict)) ||
		(b != nil && !errors.Is(b, access.ErrConflict)) {
		t.Fatal("competing commands both wrote or failed unexpectedly", a, b)
	}
	f.exec(
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='mentor'",
		f.event,
		f.person.ID,
	)
	// Confirmed attendance does not preserve access to a mentor-only survey.
	if _, err := f.s.ReadOwn(
		t.Context(),
		f.person.ID,
		f.event,
		f.id,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("wrong current role retained own answers", err)
	}
	if _, err := f.s.Respond(
		t.Context(),
		f.person.ID,
		f.event,
		f.id,
		1,
		ResponseCommand{
			CommandID:   uuid.New(),
			FormVersion: cfg.Version,
			Answers:     forms.Answers{f.choice.String(): json.RawMessage(`"no"`)},
		},
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("revoked native role updated survey", err)
	}
	f.exec("UPDATE people SET active=false WHERE id=$1", f.other.ID)
	if _, err := f.s.ReadOwn(
		t.Context(),
		f.other.ID,
		f.event,
		f.id,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("inactive account retained invitation", err)
	}
}
