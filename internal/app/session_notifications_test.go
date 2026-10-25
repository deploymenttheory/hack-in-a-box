package app

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/forms"
	"github.com/deploymenttheory/hack-in-a-box/internal/registration"
	"github.com/deploymenttheory/hack-in-a-box/internal/sessionbooking"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type sessionNotificationFixture struct {
	pool               *pgxpool.Pool
	runtime            communicationRuntime
	events             event.Service
	registration       registration.Service
	owner, person      dbgen.Person
	eventID, sessionID uuid.UUID
	registrationID     uuid.UUID
	now                time.Time
}

func newSessionNotificationFixture(t *testing.T) *sessionNotificationFixture {
	t.Helper()
	pool := testdb.New(t, true)
	ctx := t.Context()
	owner, person := testdb.Person(t, pool), testdb.Person(t, pool)
	q := dbgen.New(pool)
	if _, err := q.BootstrapAdmin(ctx, owner.ID); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(func(key string) string {
		if key == "HACKBOX_ENV" {
			return "test"
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := newCommunicationRuntime(
		pool,
		cfg,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	if err != nil {
		t.Fatal(err)
	}
	f := &sessionNotificationFixture{
		pool:      pool,
		runtime:   runtime,
		owner:     owner,
		person:    person,
		now:       time.Now().UTC().Truncate(time.Microsecond),
		eventID:   uuid.New(),
		sessionID: uuid.New(),
	}
	clock := func() time.Time { return f.now }
	f.runtime.SessionBookings.Now = clock
	f.runtime.Inbox.Now = clock
	f.runtime.Communications.Now = clock
	f.runtime.SMTP.Now = clock
	f.runtime.SessionBookings.WorkspaceOrigin = "https://hack.example.test"
	f.runtime.SMTP.Origin = "https://hack.example.test"
	f.events = event.Service{
		Pool:              pool,
		Clock:             clock,
		ReconcileSessions: f.runtime.SessionBookings.ReconcileEventInTransaction,
	}
	f.registration = registration.Service{
		Pool:              pool,
		Now:               clock,
		ReconcileSessions: f.runtime.SessionBookings.ReconcileEventInTransaction,
	}
	c := event.DefaultConfiguration()
	c.Timezone = "Europe/London"
	c.DSTChoice = "later"
	c.LiveStart = f.now.AddDate(0, 1, 0)
	_, c.DSTChoice = testdb.WallTimeForInstant(t, c.LiveStart, c.Timezone)
	c.LiveEnd = c.LiveStart.Add(8 * time.Hour)
	c.SubmissionCutoff = c.LiveStart.Add(6 * time.Hour)
	c.TeamFreeze = c.LiveStart.AddDate(0, 0, -8)
	c.AdmissionCutoff = c.TeamFreeze.Add(-time.Hour)
	c.Schedule = []event.AgendaItem{
		{
			ID:       f.sessionID,
			Title:    "PRIVATE SESSION TITLE",
			Audience: []string{"attendee"},
			Anchor:   "live_start",
		},
	}
	v, err := f.events.Create(
		ctx,
		owner.ID,
		event.Input{
			ID:                                f.eventID,
			Name:                              "PRIVATE EVENT TITLE",
			Slug:                              "session-notification-" + uuid.NewString(),
			Configuration:                     c,
			AcknowledgeCompetitionRestriction: true,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	form := forms.Definition{
		ID:       uuid.New(),
		Name:     "Synthetic session registration",
		Audience: "attendee",
		Fields:   []forms.Field{},
	}
	draft, err := f.registration.SaveForm(ctx, owner.ID, f.eventID, 0, form)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.registration.PublishForm(
		ctx,
		owner.ID,
		f.eventID,
		form.ID,
		draft.Revision,
		draft.Version,
		true,
	); err != nil {
		t.Fatal(err)
	}
	v, err = f.events.Read(ctx, owner.ID, f.eventID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.events.Transition(
		ctx,
		owner.ID,
		f.eventID,
		v.Revision,
		"registration",
		"",
		false,
	); err != nil {
		t.Fatal(err)
	}
	r, err := f.registration.Apply(
		ctx,
		person.ID,
		f.eventID,
		registration.Application{
			ID:             uuid.New(),
			FormID:         form.ID,
			FormVersion:    1,
			Answers:        forms.Answers{},
			TeamPreference: "solo",
		},
	)
	if err != nil || r.State != "confirmed" {
		t.Fatal("native fixture admission", err)
	}
	f.registrationID = r.ID
	zone, _ := time.LoadLocation(c.Timezone)
	d := sessionbooking.Definition{
		Capacity:        1,
		DurationMinutes: 60,
		Room:            "PRIVATE ROOM",
		MeetingURL:      "https://example.test/PRIVATE-MEETING",
		FacilitatorIDs:  []uuid.UUID{},
		BookingOpens: sessionbooking.LocalTime{
			Wall: f.now.In(zone).Format(time.DateOnly) + "T00:00:00",
		},
		BookingCloses: sessionbooking.LocalTime{
			Wall: c.LiveStart.In(zone).Format(time.DateOnly) + "T00:00:00",
		},
	}
	if _, err = f.runtime.SessionBookings.Configure(
		ctx,
		owner.ID,
		f.eventID,
		f.sessionID,
		0,
		d,
	); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *sessionNotificationFixture) approve(t *testing.T, channels []string) {
	t.Helper()
	page, err := f.runtime.SessionBookings.Read(t.Context(), f.owner.ID, f.eventID, f.sessionID)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := f.runtime.SessionBookings.SaveNotificationConfiguration(
		t.Context(),
		f.owner.ID,
		f.eventID,
		f.sessionID,
		page.Revision,
		channels,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !preview.CanApprove {
		t.Fatal("synthetic notification channel unavailable", preview)
	}
	if _, err = f.runtime.SessionBookings.ApproveNotifications(
		t.Context(),
		f.owner.ID,
		f.eventID,
		f.sessionID,
		preview.Configuration.Version,
		preview.Fingerprint,
	); err != nil {
		t.Fatal(err)
	}
}

func (f *sessionNotificationFixture) reserve(t *testing.T) sessionbooking.Command {
	t.Helper()
	current, err := f.runtime.SessionBookings.Read(t.Context(), f.person.ID, f.eventID, f.sessionID)
	if err != nil {
		t.Fatal(err)
	}
	command := sessionbooking.Command{
		SessionRevision: current.Revision,
		RequestID:       uuid.New(),
		Action:          "reserve",
	}
	if _, err := f.runtime.SessionBookings.Change(
		t.Context(),
		f.person.ID,
		f.eventID,
		f.sessionID,
		command,
	); err != nil {
		t.Fatal(err)
	}
	return command
}

func TestSessionRuntimeWithdrawalGetsOnlyMinimalOwnCancellation(t *testing.T) {
	f := newSessionNotificationFixture(t)
	ctx := t.Context()
	f.approve(t, []string{"in_app"})
	command := f.reserve(t)
	page, err := f.runtime.Inbox.List(ctx, f.person.ID, nil, nil, 50)
	if err != nil || len(page.Items) != 1 {
		t.Fatal("actual native session inbox not queued", page, err)
	}
	first := page.Items[0].ID
	if _, err = f.registration.Respond(
		ctx,
		f.person.ID,
		f.eventID,
		f.registrationID,
		1,
		"withdraw",
	); err != nil {
		t.Fatal(err)
	}
	if _, err = f.runtime.SessionBookings.Read(
		ctx,
		f.person.ID,
		f.eventID,
		f.sessionID,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("withdrawal granted event reader", err)
	}
	if _, err = f.runtime.Inbox.Open(
		ctx,
		f.person.ID,
		first,
		nil,
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatal("withdrawn person retained ordinary notification", err)
	}
	page, err = f.runtime.Inbox.List(ctx, f.person.ID, nil, nil, 50)
	if err != nil || len(page.Items) != 1 || page.Items[0].EventName != "" {
		t.Fatal("minimal cancellation absent or leaked event name", page, err)
	}
	detail, err := f.runtime.Inbox.Open(ctx, f.person.ID, page.Items[0].ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(detail)
	for _, private := range []string{"PRIVATE", "meeting", "room", "assessment", "facilitator"} {
		if strings.Contains(string(raw), private) {
			t.Fatal("cancellation disclosed private session/event payload", string(raw))
		}
	}
	if detail.Body != "Your session booking has been cancelled." || detail.Destination != nil {
		t.Fatal("cancellation invented access or destination", detail)
	}
	cancellation, err := f.runtime.SessionBookings.OwnCancellation(
		ctx,
		f.person.ID,
		f.eventID,
		f.sessionID,
	)
	if err != nil || cancellation.PersonID != f.person.ID || cancellation.Sequence != 2 {
		t.Fatal("safe own cancellation projection", cancellation, err)
	}
	other := testdb.Person(t, f.pool)
	if _, err = f.runtime.SessionBookings.OwnCancellation(
		ctx,
		other.ID,
		f.eventID,
		f.sessionID,
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatal("another person read cancellation", err)
	}
	var records int
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM session_notification_occurrences WHERE event_id=$1`, f.eventID).
		Scan(&records); err != nil ||
		records != 2 {
		t.Fatal("notification source identity count", records, err)
	}
	// Current state cannot fabricate success for another original command. The
	// actual retained reserve ACK remains the reserve, even after withdrawal.
	var retained sessionbooking.Receipt
	var receipt []byte
	if err = f.pool.QueryRow(ctx, `SELECT result FROM session_commands WHERE event_id=$1 AND request_id=$2`, f.eventID, command.RequestID).
		Scan(&receipt); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(
		receipt,
		&retained,
	); err != nil || retained.Action != "reserve" ||
		retained.State != "booked" {
		t.Fatal("withdrawal rewrote original acknowledgement", retained, err)
	}
}

func TestSessionRuntimeParentCancellationSuppressesEffectsAndCannotReplay(t *testing.T) {
	f := newSessionNotificationFixture(t)
	ctx := t.Context()
	f.approve(t, []string{"in_app"})
	f.reserve(t)
	current, err := f.events.Read(ctx, f.owner.ID, f.eventID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.events.Transition(
		ctx,
		f.owner.ID,
		f.eventID,
		current.Revision,
		"cancelled",
		"Synthetic deliberate cancellation",
		false,
	); err != nil {
		t.Fatal(err)
	}
	var suppressed, notifications int
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM session_notification_occurrences WHERE event_id=$1 AND source_state='suppressed' AND reason='parent_event_readonly'`, f.eventID).
		Scan(&suppressed); err != nil ||
		suppressed != 1 {
		t.Fatal("missing explicit no-writes suppression", suppressed, err)
	}
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM inbox_notifications WHERE event_id=$1`, f.eventID).
		Scan(&notifications); err != nil ||
		notifications != 1 {
		t.Fatal("cancelled parent sent/implied a cancellation", notifications, err)
	}
	page, err := f.runtime.SessionBookings.Read(ctx, f.owner.ID, f.eventID, f.sessionID)
	if err != nil || !page.Cancelled || page.CanReserve || page.MeetingURL != nil {
		t.Fatal("cancelled parent retained booking access", page, err)
	}
	if _, err = f.runtime.SessionBookings.ApproveNotifications(
		ctx,
		f.owner.ID,
		f.eventID,
		f.sessionID,
		1,
		"old preview",
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("read-only parent reauthorized delivery", err)
	}
}

func TestWithdrawnSessionCancellationDoesNotFollowFutureAgendaEdits(t *testing.T) {
	f := newSessionNotificationFixture(t)
	ctx := t.Context()
	f.approve(t, []string{"in_app"})
	f.reserve(t)
	if _, err := f.registration.Respond(
		ctx,
		f.person.ID,
		f.eventID,
		f.registrationID,
		1,
		"withdraw",
	); err != nil {
		t.Fatal(err)
	}
	original, err := f.runtime.SessionBookings.OwnCancellation(
		ctx,
		f.person.ID,
		f.eventID,
		f.sessionID,
	)
	if err != nil {
		t.Fatal(err)
	}
	current, err := f.events.Read(ctx, f.owner.ID, f.eventID)
	if err != nil {
		t.Fatal(err)
	}
	c := current.Configuration
	c.LiveStart = c.LiveStart.AddDate(0, 0, 1)
	c.LiveEnd = c.LiveEnd.AddDate(0, 0, 1)
	c.SubmissionCutoff = c.SubmissionCutoff.AddDate(0, 0, 1)
	if _, err = f.events.Update(
		ctx,
		f.owner.ID,
		f.eventID,
		current.Revision,
		event.Input{
			ID:            f.eventID,
			Name:          current.Name,
			Slug:          current.Slug,
			Configuration: c,
			Reason:        "Synthetic later session date",
		},
	); err != nil {
		t.Fatal(err)
	}
	page, err := f.runtime.Inbox.List(ctx, f.person.ID, nil, nil, 50)
	if err != nil || len(page.Items) != 1 {
		t.Fatal("withdrawn person received a future session update", len(page.Items), err)
	}
	after, err := f.runtime.SessionBookings.OwnCancellation(
		ctx,
		f.person.ID,
		f.eventID,
		f.sessionID,
	)
	if err != nil || after != original {
		t.Fatal(
			"unrelated later agenda edit changed retained personal cancellation",
			original,
			after,
			err,
		)
	}
}

func TestGenuineSessionRebookingUsesHigherPersonalCalendarSequence(t *testing.T) {
	f := newSessionNotificationFixture(t)
	ctx := t.Context()
	f.approve(t, []string{"in_app"})
	f.reserve(t)
	if _, err := f.registration.Respond(
		ctx,
		f.person.ID,
		f.eventID,
		f.registrationID,
		1,
		"withdraw",
	); err != nil {
		t.Fatal(err)
	}
	old, err := f.runtime.SessionBookings.OwnCancellation(ctx, f.person.ID, f.eventID, f.sessionID)
	if err != nil {
		t.Fatal(err)
	}
	registrationRow, err := dbgen.New(f.pool).
		Registration(ctx, dbgen.RegistrationParams{EventID: f.eventID, ID: f.registrationID})
	if err != nil {
		t.Fatal(err)
	}
	admitted, err := f.registration.Apply(
		ctx,
		f.person.ID,
		f.eventID,
		registration.Application{
			ID:             uuid.New(),
			FormID:         registrationRow.FormID,
			FormVersion:    registrationRow.FormVersion,
			Answers:        forms.Answers{},
			TeamPreference: "solo",
		},
	)
	if err != nil || admitted.State != "confirmed" {
		t.Fatal("native readmission fixture", err)
	}
	before, err := f.runtime.SessionBookings.Read(ctx, f.person.ID, f.eventID, f.sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.runtime.SessionBookings.Change(
		ctx,
		f.person.ID,
		f.eventID,
		f.sessionID,
		sessionbooking.Command{
			SessionRevision:  before.Revision,
			RequestID:        uuid.New(),
			Action:           "reserve",
			ExpectedRevision: before.OwnBooking.Revision,
		},
	); err != nil {
		t.Fatal(err)
	}
	after, err := f.runtime.SessionBookings.Read(ctx, f.person.ID, f.eventID, f.sessionID)
	if err != nil || after.OwnBooking.State != "booked" ||
		after.OwnBooking.CalendarSequence <= old.Sequence {
		t.Fatal("genuine later booking reused cancelled calendar generation", after, old, err)
	}
}
