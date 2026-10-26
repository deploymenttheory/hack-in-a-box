package certificates_test

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/attendance"
	"github.com/deploymenttheory/hack-in-a-box/internal/certificates"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/resultpublication"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type fixture struct {
	pool                 *pgxpool.Pool
	s                    certificates.Service
	owner, person, other dbgen.Person
	event, template      uuid.UUID
	day                  string
	now                  time.Time
}

func require(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func setup(t *testing.T) fixture {
	t.Helper()
	pool := testdb.New(t, true)
	f := fixture{
		pool:     pool,
		owner:    testdb.Person(t, pool),
		person:   testdb.Person(t, pool),
		other:    testdb.Person(t, pool),
		template: uuid.New(),
		day:      "2027-06-20",
		now:      time.Date(2027, 6, 22, 12, 0, 0, 0, time.UTC),
	}
	f.event = testdb.Event(t, pool, f.owner.ID)
	f.s = certificates.Service{Pool: pool, Now: func() time.Time { return f.now }}
	cfg := event.DefaultConfiguration()
	cfg.Timezone = "UTC"
	cfg.LiveStart = time.Date(2027, 6, 20, 8, 0, 0, 0, time.UTC)
	cfg.LiveEnd = cfg.LiveStart.Add(8 * time.Hour)
	raw, err := json.Marshal(cfg)
	require(t, err)
	_, err = pool.Exec(t.Context(), "UPDATE events SET configuration=$2 WHERE id=$1", f.event, raw)
	require(t, err)
	_, err = pool.Exec(
		t.Context(),
		"INSERT INTO competition_claims(event_id,person_id,state) VALUES($1,$2,'confirmed'),($1,$3,'confirmed')",
		f.event,
		f.person.ID,
		f.other.ID,
	)
	require(t, err)
	return f
}

func (f fixture) configure(t *testing.T, kind string) certificates.Settings {
	t.Helper()
	v, err := f.s.Configure(
		t.Context(),
		f.owner.ID,
		f.event,
		certificates.SettingsCommand{
			RequestID: uuid.New(),
			Reviewed:  true,
			Templates: []certificates.Template{
				{
					ID:          f.template,
					Kind:        kind,
					Recognition: "Verified synthetic recognition <script>alert(1)</script>",
					Issuer:      "Synthetic organiser",
					Accent:      "#007e67",
				},
			},
			Attendance: &certificates.AttendancePolicy{Days: []string{f.day}, MinimumPresent: 1},
			Reason:     "Explicit reviewed recognition",
		},
	)
	require(t, err)
	return v
}

func (f fixture) mark(t *testing.T, person uuid.UUID, version int64, present bool) {
	t.Helper()
	_, err := (attendance.Service{Pool: f.pool, Now: func() time.Time { return f.now }}).Mark(
		t.Context(),
		f.owner.ID,
		f.event,
		person,
		f.day,
		version,
		attendance.Command{
			RequestID: uuid.New(),
			Present:   present,
			Reason:    "Actual corrected synthetic check-in",
		},
	)
	require(t, err)
}

func TestCertificatesRecordedAttendanceConcurrentIssueCorrectionAndRevocation(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	f.configure(t, "attendance")
	bad := certificates.SettingsCommand{
		RequestID: uuid.New(),
		Version:   1,
		Reviewed:  false,
		Templates: []certificates.Template{
			{
				ID:          f.template,
				Kind:        "attendance",
				Recognition: "Not reviewed",
				Issuer:      "Synthetic",
				Accent:      "#007e67",
			},
		},
		Reason: "Unreviewed policy",
	}
	if _, err := f.s.Configure(ctx, f.owner.ID, f.event, bad); !errors.Is(err, access.ErrInvalid) {
		t.Fatal("unreviewed template accepted", err)
	}
	target := certificates.Target{PersonID: f.person.ID, TemplateID: f.template}
	preview, err := f.s.Preview(ctx, f.owner.ID, f.event, target)
	require(t, err)
	if preview.Eligible {
		t.Fatal("registration alone granted attendance")
	}
	id := uuid.New()
	cmd := certificates.Command{
		RequestID: uuid.New(),
		Action:    "issue",
		Target:    target,
		Binding:   preview.Binding,
		Reason:    "Explicit recorded attendance",
	}
	if _, err = f.s.Change(ctx, f.owner.ID, f.event, id, cmd); !errors.Is(err, access.ErrConflict) {
		t.Fatal("ineligible issue", err)
	}
	f.mark(t, f.person.ID, 0, true)
	if _, err = f.s.Change(ctx, f.owner.ID, f.event, id, cmd); !errors.Is(err, access.ErrConflict) {
		t.Fatal("old preview accepted after check-in changed", err)
	}
	preview, err = f.s.Preview(ctx, f.owner.ID, f.event, target)
	require(t, err)
	cmd.Binding = preview.Binding
	const parallel = 8
	results := make(chan error, parallel)
	var wg sync.WaitGroup
	for range parallel {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := f.s.Change(ctx, f.owner.ID, f.event, id, cmd)
			if err == nil &&
				(v.Certificate.ID != id || v.Certificate.Version != 1 || v.Receipt == nil || v.Receipt.RequestID != cmd.RequestID) {
				err = access.ErrConflict
			}
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	for err := range results {
		require(t, err)
	}
	var versions int
	require(
		t,
		f.pool.QueryRow(ctx, "SELECT count(*) FROM certificate_versions WHERE event_id=$1 AND id=$2", f.event, id).
			Scan(&versions),
	)
	if versions != 1 {
		t.Fatal("duplicate issuance", versions)
	}
	tampered := cmd
	tampered.Reason = "Changed idempotency body"
	if _, err = f.s.Change(
		ctx,
		f.owner.ID,
		f.event,
		id,
		tampered,
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatal("request body mismatch", err)
	}
	body, err := f.s.Print(ctx, f.person.ID, f.event, id)
	require(t, err)
	if strings.Contains(string(body), "<script>") ||
		!strings.Contains(string(body), "&lt;script&gt;") ||
		!strings.Contains(string(body), "<html lang=\"en\">") {
		t.Fatal("unsafe/inaccessible printable HTML")
	}
	own, err := f.s.Read(ctx, f.person.ID, f.event, id)
	require(t, err)
	if len(own.History) != 0 || !own.Current {
		t.Fatal("recipient private history/current", own)
	}
	if _, err = f.s.Read(ctx, f.other.ID, f.event, id); !errors.Is(err, access.ErrNotFound) {
		t.Fatal("other person enumerated certificate", err)
	}
	f.mark(t, f.person.ID, 1, false)
	if _, err = f.s.Print(ctx, f.person.ID, f.event, id); !errors.Is(err, access.ErrConflict) {
		t.Fatal("corrected absence still printed", err)
	}
	correction := certificates.Command{
		RequestID: uuid.New(),
		Action:    "correct",
		Version:   1,
		Target:    target,
		Binding:   cmd.Binding,
		Reason:    "Correct current facts",
	}
	if _, err = f.s.Change(
		ctx,
		f.owner.ID,
		f.event,
		id,
		correction,
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatal("absence correction issued", err)
	}
	f.mark(t, f.person.ID, 2, true)
	_, err = f.pool.Exec(
		ctx,
		"UPDATE people SET display_name='Corrected <recipient>' WHERE id=$1",
		f.person.ID,
	)
	require(t, err)
	preview, err = f.s.Preview(ctx, f.owner.ID, f.event, target)
	require(t, err)
	correction.Binding = preview.Binding
	v, err := f.s.Change(ctx, f.owner.ID, f.event, id, correction)
	require(t, err)
	if v.Certificate.ID != id || v.Certificate.Version != 2 || len(v.History) != 2 {
		t.Fatal("correction lost stable identity/provenance", v)
	}
	body, err = f.s.Print(ctx, f.person.ID, f.event, id)
	require(t, err)
	if !strings.Contains(string(body), "Corrected &lt;recipient&gt;") {
		t.Fatal("corrected approved name missing")
	}
	revoke := certificates.Command{
		RequestID: uuid.New(),
		Version:   2,
		Action:    "revoke",
		Target:    target,
		Reason:    "Explicit certificate revocation",
	}
	v, err = f.s.Change(ctx, f.owner.ID, f.event, id, revoke)
	require(t, err)
	if v.Certificate.State != "revoked" || v.Current || len(v.History) != 3 {
		t.Fatal("revocation provenance", v)
	}
	if _, err = f.s.Print(ctx, f.person.ID, f.event, id); !errors.Is(err, access.ErrConflict) {
		t.Fatal("revoked certificate printed", err)
	}
	replay, err := f.s.Change(ctx, f.owner.ID, f.event, id, cmd)
	require(t, err)
	if replay.Current || replay.Certificate.Version != 1 {
		t.Fatal("original replay impersonates current certificate", replay)
	}
}

func TestCertificatesDistinctRequestsRaceOneStableRecognition(t *testing.T) {
	f := setup(t)
	f.configure(t, "attendance")
	f.mark(t, f.person.ID, 0, true)
	target := certificates.Target{PersonID: f.person.ID, TemplateID: f.template}
	preview, err := f.s.Preview(t.Context(), f.owner.ID, f.event, target)
	require(t, err)
	errorsCh := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.s.Change(
				t.Context(),
				f.owner.ID,
				f.event,
				uuid.New(),
				certificates.Command{
					RequestID: uuid.New(),
					Action:    "issue",
					Target:    target,
					Binding:   preview.Binding,
					Reason:    "Concurrent explicit issue",
				},
			)
			errorsCh <- err
		}()
	}
	wg.Wait()
	close(errorsCh)
	success, conflict := 0, 0
	for err := range errorsCh {
		if err == nil {
			success++
		} else if errors.Is(err, access.ErrConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal("multiple effective recognitions", success, conflict)
	}
}

func TestCertificatesPublishedAwardPolicyAndCurrentIsolation(t *testing.T) {
	f := setup(t)
	f.configure(t, "award")
	ctx := t.Context()
	publication, prize, team := uuid.New(), uuid.New(), uuid.New()
	target := certificates.Target{
		PersonID:      f.person.ID,
		TemplateID:    f.template,
		PublicationID: publication,
		PrizeID:       prize,
	}
	v, err := f.s.Preview(ctx, f.owner.ID, f.event, target)
	require(t, err)
	if v.Eligible {
		t.Fatal("unpublished award qualifies")
	}
	_, err = f.pool.Exec(
		ctx,
		"INSERT INTO hack_teams(event_id,id,name,state,frozen) VALUES($1,$2,'Synthetic winner','published',true)",
		f.event,
		team,
	)
	require(t, err)
	_, err = f.pool.Exec(
		ctx,
		"INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot) VALUES($1,$2,$3,'published')",
		f.event,
		team,
		f.person.ID,
	)
	require(t, err)
	// Canonical safe published projection is seeded here; award computation and
	// explicit publication commands remain the separately tested #112 owner.
	award := resultpublication.Announcement{
		ID:      publication,
		Kind:    "awards",
		Version: 1,
		Awards: []resultpublication.AnnouncedAward{
			{
				PrizeID: prize,
				Name:    "Published synthetic award",
				Team:    resultpublication.NamedTeam{ID: team, Name: "Synthetic winner"},
			},
		},
		PublishedAt: f.now.Add(-time.Hour),
	}
	raw, err := json.Marshal(award)
	require(t, err)
	_, err = f.pool.Exec(
		ctx,
		"INSERT INTO result_versions(event_id,version,actor_id,action,reason,document,changed_at) VALUES($1,1,$2,'confirm','Synthetic published source','{}',$3)",
		f.event,
		f.owner.ID,
		f.now,
	)
	require(t, err)
	_, err = f.pool.Exec(
		ctx,
		"INSERT INTO result_publications(event_id,id,kind,result_version,source_binding,document,actor_id,published_at) VALUES($1,$2,'awards',1,$3,$4,$5,$6)",
		f.event,
		publication,
		strings.Repeat("a", 64),
		raw,
		f.owner.ID,
		f.now.Add(-time.Hour),
	)
	require(t, err)
	v, err = f.s.Preview(ctx, f.owner.ID, f.event, target)
	require(t, err)
	if !v.Eligible || v.Fields.Award != "Published synthetic award" {
		t.Fatal("published award absent", v)
	}
	id := uuid.New()
	_, err = f.s.Change(
		ctx,
		f.owner.ID,
		f.event,
		id,
		certificates.Command{
			RequestID: uuid.New(),
			Action:    "issue",
			Target:    target,
			Binding:   v.Binding,
			Reason:    "Explicit published award recognition",
		},
	)
	require(t, err)
	otherTarget := target
	otherTarget.PersonID = f.other.ID
	other, err := f.s.Preview(ctx, f.owner.ID, f.event, otherTarget)
	require(t, err)
	if other.Eligible {
		t.Fatal("other competitor inherited team award")
	}
	second := testdb.Event(t, f.pool, f.owner.ID)
	if _, err = f.s.Read(ctx, f.owner.ID, second, id); !errors.Is(err, access.ErrNotFound) {
		t.Fatal("cross-event certificate", err)
	}
	_, err = f.pool.Exec(
		ctx,
		"UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2",
		f.event,
		f.person.ID,
	)
	require(t, err)
	if _, err = f.s.Print(ctx, f.person.ID, f.event, id); !errors.Is(err, access.ErrNotFound) {
		t.Fatal("revoked recipient authority retained", err)
	}
	if _, err = f.s.Print(ctx, f.owner.ID, f.event, id); !errors.Is(err, access.ErrConflict) {
		t.Fatal("ineligible recipient still printable", err)
	}
}

func TestCertificatesCheckInCorrectionRaceAndCurrentStaffRevocation(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	f.configure(t, "attendance")
	f.mark(t, f.person.ID, 0, true)
	target := certificates.Target{PersonID: f.person.ID, TemplateID: f.template}
	preview, err := f.s.Preview(ctx, f.owner.ID, f.event, target)
	require(t, err)
	id := uuid.New()
	start := make(chan struct{})
	issued := make(chan error, 1)
	corrected := make(chan error, 1)
	go func() {
		<-start
		_, err := f.s.Change(
			ctx,
			f.owner.ID,
			f.event,
			id,
			certificates.Command{
				RequestID: uuid.New(),
				Action:    "issue",
				Target:    target,
				Binding:   preview.Binding,
				Reason:    "Race explicit issuance with corrected check-in",
			},
		)
		issued <- err
	}()
	go func() {
		<-start
		_, err := (attendance.Service{Pool: f.pool, Now: func() time.Time { return f.now }}).Mark(
			ctx,
			f.owner.ID,
			f.event,
			f.person.ID,
			f.day,
			1,
			attendance.Command{
				RequestID: uuid.New(),
				Present:   false,
				Reason:    "Concurrent corrected absence",
			},
		)
		corrected <- err
	}()
	close(start)
	require(t, <-corrected)
	err = <-issued
	if err != nil && !errors.Is(err, access.ErrConflict) {
		t.Fatal(err)
	}
	_, err = f.s.Print(ctx, f.person.ID, f.event, id)
	if !errors.Is(err, access.ErrNotFound) && !errors.Is(err, access.ErrConflict) {
		t.Fatal("absence/issuance race left printable recognition", err)
	}
	_, err = f.pool.Exec(
		ctx,
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='organiser'",
		f.event,
		f.owner.ID,
	)
	require(t, err)
	if _, err = f.s.Settings(
		ctx,
		f.owner.ID,
		f.event,
	); !errors.Is(err, access.ErrNotFound) &&
		!errors.Is(err, access.ErrForbidden) {
		t.Fatal("revoked organiser retained certificate settings", err)
	}
}
