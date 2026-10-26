package certificates_test

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/attendance"
	"github.com/deploymenttheory/hack-in-a-box/internal/certificates"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
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
	t.Run("explicit-two-recorded-days-policy", func(t *testing.T) {
		f := setup(t)
		ctx := t.Context()
		cfg := event.DefaultConfiguration()
		cfg.Timezone = "UTC"
		cfg.LiveStart = time.Date(2027, 6, 20, 8, 0, 0, 0, time.UTC)
		cfg.LiveEnd = cfg.LiveStart.Add(32 * time.Hour)
		raw, err := json.Marshal(cfg)
		require(t, err)
		_, err = f.pool.Exec(ctx, "UPDATE events SET configuration=$2 WHERE id=$1", f.event, raw)
		require(t, err)
		command := certificates.SettingsCommand{
			RequestID: uuid.New(),
			Reviewed:  true,
			Attendance: &certificates.AttendancePolicy{
				Days:           []string{f.day, "2027-06-21"},
				MinimumPresent: 2,
			},
			Templates: []certificates.Template{
				{
					ID:          f.template,
					Kind:        "attendance",
					Recognition: "Two verified days",
					Issuer:      "Reviewed organiser",
					Accent:      "#007e67",
				},
			},
			Reason: "Explicit two-day attendance floor",
		}
		_, err = f.s.Configure(ctx, f.owner.ID, f.event, command)
		require(t, err)
		target := certificates.Target{PersonID: f.person.ID, TemplateID: f.template}
		f.mark(t, f.person.ID, 0, true)
		preview, err := f.s.Preview(ctx, f.owner.ID, f.event, target)
		require(t, err)
		if preview.Eligible {
			t.Fatal("one check-in satisfied configured two-day policy")
		}
		second := f
		second.day = "2027-06-21"
		second.mark(t, f.person.ID, 0, true)
		preview, err = f.s.Preview(ctx, f.owner.ID, f.event, target)
		require(t, err)
		if !preview.Eligible {
			t.Fatal("two recorded days did not satisfy explicit policy")
		}
		command.RequestID = uuid.New()
		command.Version = 1
		command.Attendance = nil
		_, err = f.s.Configure(ctx, f.owner.ID, f.event, command)
		require(t, err)
		preview, err = f.s.Preview(ctx, f.owner.ID, f.event, target)
		require(t, err)
		if preview.Eligible {
			t.Fatal("unset attendance policy still issued recognition")
		}
	})
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
	_, err = f.pool.Exec(
		ctx,
		"UPDATE events SET writes_allowed=false,stage='archived',revision=revision+1 WHERE id=$1",
		f.event,
	)
	require(t, err)
	if _, err = f.s.Print(ctx, f.person.ID, f.event, id); err != nil {
		t.Fatal("archive hid earned certificate", err)
	}
	forbidden := correction
	forbidden.RequestID = uuid.New()
	forbidden.Version = 2
	if _, err = f.s.Change(
		ctx,
		f.owner.ID,
		f.event,
		id,
		forbidden,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("closed event accepted certificate correction write", err)
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
	retained, err := f.s.Read(ctx, f.owner.ID, f.event, id)
	require(t, err)
	if retained.Current || len(retained.History) != 3 ||
		retained.History[0].Receipt.RequestID != cmd.RequestID ||
		retained.History[0].Receipt.CertificateID != id ||
		retained.History[0].Receipt.Action != "issue" ||
		retained.History[0].Receipt.Version != 1 ||
		len(retained.History[0].Receipt.Fingerprint) != 64 ||
		retained.History[1].Receipt.Action != "correct" ||
		retained.History[2].Receipt.Action != "revoke" {
		t.Fatal("history lost exact closed-event command provenance", retained)
	}
	if _, err = f.pool.Exec(
		ctx,
		"DELETE FROM certificate_versions WHERE event_id=$1 AND id=$2",
		f.event,
		id,
	); err == nil {
		t.Fatal("certificate provenance could be erased")
	}
	db := stdlib.OpenDBFromPool(f.pool)
	defer db.Close()
	migrator, err := goose.NewProvider(goose.DialectPostgres, db, os.DirFS("../database/migrations"))
	require(t, err)
	if _, err = migrator.DownTo(ctx, 55); err == nil ||
		!strings.Contains(err.Error(), "certificate decisions must be retained") {
		t.Fatal("downgrade did not preserve issued recognition", err)
	}
	require(
		t,
		f.pool.QueryRow(ctx, "SELECT count(*) FROM certificate_versions WHERE event_id=$1 AND id=$2", f.event, id).
			Scan(&versions),
	)
	if versions != 3 {
		t.Fatal("failed downgrade damaged certificate provenance", versions)
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
