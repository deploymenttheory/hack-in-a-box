package app

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/forms"
	"github.com/deploymenttheory/hack-in-a-box/internal/registration"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

// Admission owns a fresh database and static pre-admission clock. It never
// rewinds the live rehearsal or changes its frozen/published competition facts.
type accessibilityAdmission struct {
	runtime *liveRehearsal
	public  rehearsalSurface
	admin   rehearsalSurface
	actors  map[string]dbgen.Person
	formID  uuid.UUID
}

func newAccessibilityAdmission(t *testing.T) *accessibilityAdmission {
	t.Helper()
	r := &liveRehearsal{
		pool:  testdb.New(t, true),
		clock: &rehearsalClock{now: time.Date(2027, 3, 1, 9, 0, 0, 0, time.UTC)},
	}
	r.owner = testdb.Person(t, r.pool)
	if _, err := dbgen.New(r.pool).BootstrapAdmin(t.Context(), r.owner.ID); err != nil {
		t.Fatal(err)
	}
	configuration := event.DefaultConfiguration()
	configuration.Capacity = 8
	configuration.MinimumAttendance = 3
	configuration.LiveStart = time.Date(2027, 3, 29, 9, 0, 0, 0, time.UTC)
	configuration.LiveEnd = configuration.LiveStart.Add(8 * time.Hour)
	configuration.SubmissionCutoff = configuration.LiveStart.Add(6 * time.Hour)
	configuration.TeamFreeze = configuration.LiveStart.AddDate(0, 0, -8)
	configuration.AdmissionCutoff = configuration.TeamFreeze.Add(-time.Hour)
	events := event.Service{Pool: r.pool, Clock: r.clock.read}
	created, err := events.Create(t.Context(), r.owner.ID, event.Input{
		ID: uuid.New(), Name: "Accessibility admission rehearsal",
		Slug: "accessibility-admission-" + uuid.NewString(), Configuration: configuration,
		AcknowledgeCompetitionRestriction: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	r.eventID = created.ID
	service := registration.Service{Pool: r.pool, Now: r.clock.read}
	question := uuid.New()
	definition := forms.Definition{
		ID:       uuid.New(),
		Name:     "Accessibility attendee registration",
		Audience: "attendee",
		Fields: []forms.Field{
			{ID: question, Label: "Accessibility learning goal", Type: "text", Required: true},
		},
	}
	saved, err := service.SaveForm(t.Context(), r.owner.ID, r.eventID, 0, definition)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.PublishForm(
		t.Context(),
		r.owner.ID,
		r.eventID,
		definition.ID,
		saved.Revision,
		saved.Version,
		true,
	); err != nil {
		t.Fatal(err)
	}
	current, err := events.Read(t.Context(), r.owner.ID, r.eventID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := events.Transition(
		t.Context(),
		r.owner.ID,
		r.eventID,
		current.Revision,
		"registration",
		"",
		false,
	); err != nil {
		t.Fatal(err)
	}
	actors := map[string]dbgen.Person{"owner": r.owner}
	for _, role := range []string{"applicant", "captain", "teammate", "unknown-joiner"} {
		person := testdb.Person(t, r.pool)
		person.DisplayName = "Synthetic " + role
		if _, err := r.pool.Exec(
			t.Context(),
			"UPDATE people SET display_name=$2 WHERE id=$1",
			person.ID,
			person.DisplayName,
		); err != nil {
			t.Fatal(err)
		}
		actors[role] = person
		if role == "applicant" {
			continue
		}
		registered, err := service.Apply(
			t.Context(),
			person.ID,
			r.eventID,
			registration.Application{
				ID:             uuid.New(),
				FormID:         definition.ID,
				FormVersion:    saved.Version,
				TeamPreference: "solo",
				Answers: forms.Answers{
					question.String(): json.RawMessage(`"Synthetic confirmed setup"`),
				},
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if registered.State != "confirmed" {
			t.Fatal("native admission setup did not confirm its team actor", role, registered.State)
		}
	}
	return &accessibilityAdmission{
		runtime: r,
		public:  r.surface(t, "public"),
		admin:   r.surface(t, "admin"),
		actors:  actors,
		formID:  definition.ID,
	}
}

func (a *accessibilityAdmission) session(
	w http.ResponseWriter,
	request *http.Request,
	actor, surface string,
) {
	person, ok := a.actors[actor]
	if !ok {
		http.Error(w, "unknown admission actor", http.StatusBadRequest)
		return
	}
	server := a.public
	if surface == "admin" {
		server = a.admin
	} else if surface != "public" {
		http.Error(w, "unknown admission surface", http.StatusBadRequest)
		return
	}
	cookie, err := server.sessionCookie(request.Context(), person)
	if err != nil {
		http.Error(w, "admission session unavailable", http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(cookie)
}
