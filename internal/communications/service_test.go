package communications

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/forms"
	"github.com/deploymenttheory/hack-in-a-box/internal/registration"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type fixture struct {
	pool          *pgxpool.Pool
	s             Service
	owner         dbgen.Person
	eventID       uuid.UUID
	now           time.Time
	configuration event.Configuration
	t             Template
	a             Audience
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	pool := testdb.New(t, true)
	owner := testdb.Person(t, pool)
	id := testdb.Event(t, pool, owner.ID)
	now := time.Date(2027, 5, 1, 9, 0, 0, 0, time.UTC)
	c := event.DefaultConfiguration()
	c.LiveStart = now.AddDate(0, 1, 0)
	c.LiveEnd = c.LiveStart.Add(8 * time.Hour)
	c.SubmissionCutoff = c.LiveEnd.Add(-time.Hour)
	c.TeamFreeze = c.LiveStart.AddDate(0, 0, -8)
	c.AdmissionCutoff = c.TeamFreeze.Add(-time.Hour)
	if _, err := pool.Exec(
		t.Context(),
		"INSERT INTO event_gate_facts(event_id) VALUES($1)",
		id,
	); err != nil {
		t.Fatal(err)
	}
	f := &fixture{pool: pool, owner: owner, eventID: id, now: now, configuration: c}
	f.s = Service{Pool: pool, Now: func() time.Time { return f.now }}
	data, _ := json.Marshal(c)
	f.exec(t, "UPDATE events SET configuration=$2,stage='registration' WHERE id=$1", id, data)
	f.t = Template{
		ID:      uuid.New(),
		Name:    "Synthetic reminder",
		Purpose: "reminder",
		Format:  "html",
		Subject: "Hello {{person.name}}",
		Body:    "{{event.name}} {{person.name}}",
	}
	f.a = Audience{
		ID:                 uuid.New(),
		Name:               "Confirmed competitors",
		Roles:              []string{"attendee"},
		RegistrationStates: []string{},
		Teams:              []uuid.UUID{},
	}
	f.change(t, Command{Action: "save_template", Template: &f.t})
	f.t.Version = 1
	f.change(t, Command{Action: "save_audience", Audience: &f.a})
	f.a.Version = 1
	return f
}

func (f *fixture) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := f.pool.Exec(t.Context(), sql, args...); err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) revision(t *testing.T) int64 {
	t.Helper()
	row, err := dbgen.New(f.pool).Event(t.Context(), f.eventID)
	if err != nil {
		t.Fatal(err)
	}
	return row.Revision
}

func (f *fixture) change(t *testing.T, c Command) Page {
	t.Helper()
	c.Reason = "Synthetic organiser review"
	page, err := f.s.Change(t.Context(), f.owner.ID, f.eventID, f.revision(t), c)
	if err != nil {
		t.Fatal(err)
	}
	return page
}

func (f *fixture) preview(t *testing.T) Preview {
	t.Helper()
	return *f.change(t, Command{Action: "preview", TemplateID: f.t.ID, AudienceID: f.a.ID}).Preview
}

func (f *fixture) approve(t *testing.T, p Preview) {
	t.Helper()
	f.change(t, Command{Action: "approve", PreviewID: p.ID})
}

func (f *fixture) person(t *testing.T) dbgen.Person {
	t.Helper()
	p := testdb.Person(t, f.pool)
	f.exec(
		t,
		"INSERT INTO competition_claims(event_id,person_id,state) VALUES($1,$2,'confirmed')",
		f.eventID,
		p.ID,
	)
	return p
}

func (f *fixture) fact(t *testing.T, fact dbgen.SaveCommunicationFactParams) {
	t.Helper()
	fact.EventID = f.eventID
	if err := database.Within(
		t.Context(),
		f.pool,
		func(tx pgx.Tx, _ *dbgen.Queries) error { return RecordOfficialFact(t.Context(), tx, fact) },
	); err != nil {
		t.Fatal(err)
	}
}

func TestVersionedTemplatesBlockUnknownAndUnresolvedApproval(t *testing.T) {
	f := newFixture(t)
	f.person(t)
	bad := f.t
	bad.Body = "{{person.email}}"
	_, err := f.s.Change(
		t.Context(),
		f.owner.ID,
		f.eventID,
		f.revision(t),
		Command{Action: "save_template", Template: &bad, Reason: "Unknown field"},
	)
	if !errors.Is(err, event.ErrBlocked) {
		t.Fatalf("unknown field: %v", err)
	}
	f.t.Body = "{{team.name}}"
	f.change(t, Command{Action: "save_template", Template: &f.t})
	f.t.Version++
	p := f.preview(t)
	if !slices.Contains(p.Blockers, "unresolved_placeholders") {
		t.Fatalf("unresolved team: %+v", p)
	}
	_, err = f.s.Change(
		t.Context(),
		f.owner.ID,
		f.eventID,
		f.revision(t),
		Command{Action: "approve", PreviewID: p.ID, Reason: "Attempted approval"},
	)
	if !errors.Is(err, event.ErrBlocked) {
		t.Fatalf("missing value approved: %v", err)
	}
	var versions int
	f.pool.QueryRow(t.Context(), "SELECT count(*) FROM communication_versions WHERE event_id=$1 AND id=$2", f.eventID, f.t.ID).
		Scan(&versions)
	if versions != 2 {
		t.Fatalf("immutable versions: %d", versions)
	}
	f.exec(t, "UPDATE people SET platform_admin=true WHERE id=$1", f.person(t).ID)
	outsider := testdb.Person(t, f.pool)
	if _, err = f.s.Read(
		t.Context(),
		outsider.ID,
		f.eventID,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal(err)
	}
}

func TestActualWithdrawalBetweenPreviewApprovalAndExecution(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	person := testdb.Person(t, f.pool)
	survivor := f.person(t)
	d := forms.Definition{
		ID:       uuid.New(),
		Name:     "Synthetic registration",
		Audience: "attendee",
		Fields:   []forms.Field{},
	}
	reg := registration.Service{Pool: f.pool, Now: func() time.Time { return f.now }}
	form, err := reg.SaveForm(ctx, f.owner.ID, f.eventID, 0, d)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = reg.PublishForm(
		ctx,
		f.owner.ID,
		f.eventID,
		d.ID,
		form.Revision,
		form.Version,
		true,
	); err != nil {
		t.Fatal(err)
	}
	applied, err := reg.Apply(
		ctx,
		person.ID,
		f.eventID,
		registration.Application{
			ID:             uuid.New(),
			FormID:         d.ID,
			FormVersion:    form.Version,
			TeamPreference: "solo",
			Answers:        forms.Answers{},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	f.exec(
		t,
		"UPDATE people SET display_name='<script>private-value</script>' WHERE id=$1",
		person.ID,
	)
	preview := f.preview(t)
	f.approve(t, preview)
	if _, err = reg.Respond(
		ctx,
		person.ID,
		f.eventID,
		applied.ID,
		applied.Revision,
		"withdraw",
	); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.Change(
		ctx,
		f.owner.ID,
		f.eventID,
		f.revision(t),
		Command{Action: "approve", PreviewID: preview.ID, Reason: "Stale recipient approval"},
	); !errors.Is(
		err,
		event.ErrBlocked,
	) {
		t.Fatalf("stale audience approved: %v", err)
	}
	resolved, err := f.s.ResolveApproved(ctx, f.eventID, preview.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Recipients) != 1 || resolved.Recipients[0].PersonID != survivor.ID {
		t.Fatalf("withdrawn recipient retained: %+v", resolved)
	}
	f.t.Body = "Revised reminder {{person.name}}"
	f.change(t, Command{Action: "save_template", Template: &f.t})
	f.t.Version++
	if _, err = f.s.ResolveApproved(ctx, f.eventID, preview.ID); !errors.Is(err, ErrUnresolved) {
		t.Fatalf("new template version reused old approval: %v", err)
	}
	f.exec(
		t,
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='organiser'",
		f.eventID,
		f.owner.ID,
	)
	if _, err = f.s.ResolveApproved(
		ctx,
		f.eventID,
		preview.ID,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatalf("revoked approver retained execution authority: %v", err)
	}
}

func TestPublishedTeamAndCurrentResourceGrants(t *testing.T) {
	f := newFixture(t)
	person := f.person(t)
	other := f.person(t)
	team := uuid.New()
	f.exec(
		t,
		"INSERT INTO hack_teams(id,event_id,name,state) VALUES($1,$2,'Published team','published')",
		team,
		f.eventID,
	)
	f.exec(t, "INSERT INTO competition_entries(event_id,team_id) VALUES($1,$2)", f.eventID, team)
	f.exec(
		t,
		"INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot) VALUES($1,$2,$3,'published')",
		f.eventID,
		team,
		person.ID,
	)
	f.a.Roles = []string{}
	f.a.Teams = []uuid.UUID{team}
	f.change(t, Command{Action: "save_audience", Audience: &f.a})
	f.a.Version++
	f.t.Body = "{{team.name}}"
	f.change(t, Command{Action: "save_template", Template: &f.t})
	f.t.Version++
	p := f.preview(t)
	if len(p.Recipients) != 1 || p.Recipients[0].PersonID != person.ID ||
		!strings.Contains(p.Recipients[0].Body, "Published team") {
		t.Fatalf("team scope: %+v", p)
	}
	f.approve(t, p)
	f.exec(
		t,
		"DELETE FROM hack_team_members WHERE event_id=$1 AND person_id=$2",
		f.eventID,
		person.ID,
	)
	if actual, err := f.s.ResolveApproved(
		t.Context(),
		f.eventID,
		p.ID,
	); err != nil ||
		len(actual.Recipients) != 0 {
		t.Fatalf("old team access: %+v %v", actual, err)
	}
	f.a.Teams = []uuid.UUID{}
	f.a.Roles = []string{"attendee"}
	f.change(t, Command{Action: "save_audience", Audience: &f.a})
	f.a.Version++
	factID, resource := uuid.New(), uuid.New()
	f.exec(
		t,
		"INSERT INTO policy_resources(event_id,id,kind) VALUES($1,$2,'assessment')",
		f.eventID,
		resource,
	)
	f.exec(
		t,
		"INSERT INTO resource_assignments(event_id,resource_id,person_id) VALUES($1,$2,$3)",
		f.eventID,
		resource,
		other.ID,
	)
	f.fact(
		t,
		dbgen.SaveCommunicationFactParams{
			ID:         factID,
			Kind:       "outcome_review",
			Revision:   1,
			OwnerID:    &other.ID,
			ResourceID: &resource,
			Status:     "open",
		},
	)
	f.a.Outstanding = "outcome_review"
	f.a.FactID = &factID
	f.change(t, Command{Action: "save_audience", Audience: &f.a})
	f.a.Version++
	f.t.FactID = &factID
	f.t.Body = "Review {{outcome_review.status}} revision {{outcome_review.revision}}"
	f.change(t, Command{Action: "save_template", Template: &f.t})
	f.t.Version++
	p = f.preview(t)
	if len(p.Recipients) != 1 || p.Recipients[0].PersonID != other.ID {
		t.Fatalf("owned reminder: %+v", p)
	}
	f.approve(t, p)
	f.exec(
		t,
		"DELETE FROM resource_assignments WHERE event_id=$1 AND resource_id=$2",
		f.eventID,
		resource,
	)
	actual, err := f.s.ResolveApproved(t.Context(), f.eventID, p.ID)
	if err != nil || len(actual.Recipients) != 0 {
		t.Fatalf("revoked resource grant: %+v %v", actual, err)
	}
}

func TestConcurrentVersionSavesHaveOneReviewedWinner(t *testing.T) {
	f := newFixture(t)
	revision := f.revision(t)
	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			_, err := f.s.Change(
				context.Background(),
				f.owner.ID,
				f.eventID,
				revision,
				Command{Action: "save_template", Template: &f.t, Reason: "Concurrent review"},
			)
			results <- err
		}()
	}
	close(start)
	success, conflict := 0, 0
	for range 2 {
		err := <-results
		if err == nil {
			success++
		} else if errors.Is(err, access.ErrConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("concurrent save: %d %d", success, conflict)
	}
}

func TestChallengeEmbargoAndSeparateDateVariables(t *testing.T) {
	f := newFixture(t)
	person := f.person(t)
	ctx := t.Context()
	f.t.Purpose = "challenge_release"
	f.t.Body = "{{challenge.title}}"
	f.change(t, Command{Action: "save_template", Template: &f.t})
	f.t.Version++
	challenge := uuid.New()
	document := []byte(
		`{"title":"SECRET-THEME","markdown":"PRIVATE-BODY","audience":{"confirmed":true,"roles":[],"teams":[]}}`,
	)
	f.exec(
		t,
		"INSERT INTO content_items(event_id,id,kind,revision,version,state) VALUES($1,$2,'challenge',1,1,'draft')",
		f.eventID,
		challenge,
	)
	f.exec(
		t,
		"INSERT INTO content_versions(event_id,content_id,version,document,actor_id,created_at) VALUES($1,$2,1,$3,$4,$5)",
		f.eventID,
		challenge,
		document,
		f.owner.ID,
		f.now,
	)
	p := f.preview(t)
	if !slices.Contains(p.Blockers, "unresolved_placeholders") ||
		strings.Contains(p.Recipients[0].Body, "SECRET") {
		t.Fatalf("early theme: %+v", p)
	}
	f.exec(
		t,
		"UPDATE content_items SET state='released',released_version=1,released_at=$3 WHERE event_id=$1 AND id=$2",
		f.eventID,
		challenge,
		f.now,
	)
	f.exec(t, "UPDATE events SET published_at=$2 WHERE id=$1", f.eventID, f.now)
	p = f.preview(t)
	if !slices.Contains(p.Blockers, "unresolved_placeholders") {
		t.Fatal("incorrect early source timestamp bypassed planned release")
	}
	f.now = f.configuration.LiveStart.AddDate(0, 0, -7).Add(time.Second)
	p = f.preview(t)
	if len(p.Blockers) != 0 || !strings.Contains(p.Recipients[0].Body, "SECRET-THEME") ||
		strings.Contains(p.Recipients[0].Body, "PRIVATE-BODY") {
		t.Fatalf("released title: %+v", p)
	}
	f.approve(t, p)
	f.exec(
		t,
		"UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2",
		f.eventID,
		person.ID,
	)
	resolved, err := f.s.ResolveApproved(ctx, f.eventID, p.ID)
	if err != nil || len(resolved.Recipients) != 0 {
		t.Fatalf("current challenge audience: %+v %v", resolved, err)
	}
	values := eventValues(dbgen.Event{ID: f.eventID, Name: "Synthetic"}, f.configuration)
	for _, name := range []string{"event.final_admission_cutoff", "event.team_freeze", "event.challenge_publication", "event.live_start", "event.submission_cutoff"} {
		if values[name] == "" {
			t.Fatalf("missing distinct deadline: %s", name)
		}
	}
	if values["event.challenge_publication"] == values["event.live_start"] ||
		values["event.team_freeze"] == values["event.final_admission_cutoff"] {
		t.Fatal("distinct product deadlines conflated")
	}
}

func TestPostponementNeedsApprovedCurrentDatesAndChangedFactsNeedNewApproval(t *testing.T) {
	f := newFixture(t)
	f.person(t)
	ctx := t.Context()
	f.t.Purpose = "postponement"
	f.t.Body = "Revised live dates: {{ event.live_start }} to {{event.live_end}}. Submit by {{event.submission_cutoff}}."
	f.change(t, Command{Action: "save_template", Template: &f.t})
	f.t.Version++
	p := f.preview(t)
	if !slices.Contains(p.Blockers, "unresolved_placeholders") {
		t.Fatal("invented postponement approved")
	}
	c := f.configuration
	f.exec(
		t,
		"INSERT INTO event_notices(id,event_id,revision,live_start,live_end,submission_cutoff) VALUES($1,$2,1,$3,$4,$5)",
		uuid.New(),
		f.eventID,
		c.LiveStart,
		c.LiveEnd,
		c.SubmissionCutoff,
	)
	p = f.preview(t)
	if len(p.Blockers) != 0 {
		t.Fatalf("approved postponement dates unresolved: %+v", p)
	}
	f.approve(t, p)
	c.LiveStart = c.LiveStart.AddDate(0, 0, 1)
	data, _ := json.Marshal(c)
	f.exec(t, "UPDATE events SET configuration=$2 WHERE id=$1", f.eventID, data)
	if _, err := f.s.ResolveApproved(ctx, f.eventID, p.ID); !errors.Is(err, ErrUnresolved) {
		t.Fatalf("changed dates reused approval: %v", err)
	}
}

func TestOfficialFactsExcludePrivateBodiesAndRequireCurrentRevision(t *testing.T) {
	f := newFixture(t)
	person := f.person(t)
	ctx := t.Context()
	id := uuid.New()
	f.t.Purpose = "clarification"
	f.t.FactID = &id
	f.t.Body = "Official clarification {{clarification.revision}}"
	f.change(t, Command{Action: "save_template", Template: &f.t})
	f.t.Version++
	p := f.preview(t)
	if !slices.Contains(p.Blockers, "unresolved_placeholders") {
		t.Fatal("missing official fact accepted")
	}
	f.fact(
		t,
		dbgen.SaveCommunicationFactParams{
			ID:       id,
			Kind:     "clarification",
			Revision: 1,
			Status:   "published",
		},
	)
	p = f.preview(t)
	if len(p.Blockers) != 0 || !strings.Contains(p.Recipients[0].Body, "1") {
		t.Fatalf("official fact: %+v", p)
	}
	f.approve(t, p)
	f.fact(
		t,
		dbgen.SaveCommunicationFactParams{
			ID:       id,
			Kind:     "clarification",
			Revision: 2,
			Status:   "published",
		},
	)
	if _, err := f.s.ResolveApproved(ctx, f.eventID, p.ID); !errors.Is(err, ErrUnresolved) {
		t.Fatalf("stale official revision: %v", err)
	}
	f.a.Outstanding = "help"
	f.change(t, Command{Action: "save_audience", Audience: &f.a})
	f.a.Version++
	help := uuid.New()
	f.fact(
		t,
		dbgen.SaveCommunicationFactParams{
			ID:       help,
			Kind:     "help",
			Revision: 1,
			OwnerID:  &person.ID,
			Status:   "open",
		},
	)
	f.t.FactID = &help
	f.t.Purpose = "help"
	f.t.Body = "Help request {{help.status}} revision {{help.revision}}"
	f.change(t, Command{Action: "save_template", Template: &f.t})
	f.t.Version++
	p = f.preview(t)
	if len(p.Recipients) != 1 || len(p.Blockers) != 0 {
		t.Fatalf("owned help status: %+v", p)
	}
	bytes, _ := json.Marshal(p)
	if strings.Contains(string(bytes), "question") ||
		strings.Contains(string(bytes), "assessment") {
		t.Fatalf("private fields exposed: %s", bytes)
	}
}

func TestOutstandingPreparationAndCurrentStaffAudience(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	staff := testdb.Person(t, f.pool)
	f.exec(
		t,
		"INSERT INTO event_memberships(event_id,person_id,role) VALUES($1,$2,'mentor')",
		f.eventID,
		staff.ID,
	)
	f.a.Roles = []string{"mentor"}
	f.a.Outstanding = "preparation"
	f.change(t, Command{Action: "save_audience", Audience: &f.a})
	f.a.Version++
	f.exec(
		t,
		`INSERT INTO preparation_tasks(event_id,id,revision,document) VALUES($1,$2,1,'{"definition":{"status":"in_progress","owner_role":"mentor","owner_id":null}}')`,
		f.eventID,
		uuid.New(),
	)
	p := f.preview(t)
	if len(p.Recipients) != 1 || p.Recipients[0].PersonID != staff.ID {
		t.Fatalf("current outstanding task owner: %+v", p)
	}
	f.approve(t, p)
	f.exec(
		t,
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2",
		f.eventID,
		staff.ID,
	)
	actual, err := f.s.ResolveApproved(ctx, f.eventID, p.ID)
	if err != nil || len(actual.Recipients) != 0 {
		t.Fatalf("revoked staff audience: %+v %v", actual, err)
	}
}

func TestOfferAudienceExpiresBeforeExecution(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	person := testdb.Person(t, f.pool)
	definition := forms.Definition{
		ID:       uuid.New(),
		Name:     "Offer fixture",
		Audience: "attendee",
		Fields:   []forms.Field{},
	}
	registrationService := registration.Service{
		Pool: f.pool,
		Now:  func() time.Time { return f.now },
	}
	form, err := registrationService.SaveForm(ctx, f.owner.ID, f.eventID, 0, definition)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = registrationService.PublishForm(
		ctx,
		f.owner.ID,
		f.eventID,
		definition.ID,
		form.Revision,
		form.Version,
		true,
	); err != nil {
		t.Fatal(err)
	}
	applied, err := registrationService.Apply(
		ctx,
		person.ID,
		f.eventID,
		registration.Application{
			ID:             uuid.New(),
			FormID:         definition.ID,
			FormVersion:    form.Version,
			TeamPreference: "solo",
			Answers:        forms.Answers{},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	// Synthetic producer state: the existing offer domain owns these transitions.
	f.exec(
		t,
		"UPDATE registrations SET state='offered' WHERE event_id=$1 AND id=$2",
		f.eventID,
		applied.ID,
	)
	f.exec(
		t,
		"UPDATE competition_claims SET state='waitlisted' WHERE event_id=$1 AND person_id=$2",
		f.eventID,
		person.ID,
	)
	f.exec(
		t,
		"INSERT INTO registration_offers(event_id,registration_id,expires_at) VALUES($1,$2,$3)",
		f.eventID,
		applied.ID,
		f.now.Add(time.Hour),
	)
	f.a.Roles = []string{}
	f.a.RegistrationStates = []string{"offered"}
	f.a.Outstanding = "offer"
	f.change(t, Command{Action: "save_audience", Audience: &f.a})
	f.a.Version++
	p := f.preview(t)
	if len(p.Recipients) != 1 {
		t.Fatalf("current offer: %+v", p)
	}
	f.approve(t, p)
	f.now = f.now.Add(2 * time.Hour)
	actual, err := f.s.ResolveApproved(ctx, f.eventID, p.ID)
	if err != nil || len(actual.Recipients) != 0 {
		t.Fatalf("expired offer remained eligible: %+v %v", actual, err)
	}
}
