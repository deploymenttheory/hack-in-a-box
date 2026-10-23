package privacy_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/privacy"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestNativePhotoConsentIncarnationRevisionAndArchivedSelfRevocation(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	owner, photographer, subject := testdb.Person(
		t,
		pool,
	), testdb.Person(
		t,
		pool,
	), testdb.Person(
		t,
		pool,
	)
	eventID := testdb.Event(t, pool, owner.ID)
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(
		`INSERT INTO event_memberships(event_id,person_id,role)VALUES($1,$2,'photographer')`,
		eventID,
		photographer.ID,
	)
	exec(
		`INSERT INTO competition_claims(event_id,person_id,state)VALUES($1,$2,'confirmed')`,
		eventID,
		subject.ID,
	)
	now := time.Date(2027, 6, 20, 9, 0, 0, 0, time.UTC)
	service := privacy.Service{Pool: pool, Clock: func() time.Time { return now }}
	fields := privacy.Fields{
		PhotoConsent:  "yes",
		Dietary:       "PRIVATE-DIETARY",
		Accessibility: "PRIVATE-ACCESSIBILITY",
		ContactEmail:  "private@example.test",
	}
	profile, err := service.Correct(ctx, subject.ID, eventID, 0, fields)
	if err != nil {
		t.Fatal(err)
	}
	read := func(actor uuid.UUID) ([]privacy.PhotoConsent, error) {
		var out []privacy.PhotoConsent
		e := database.Within(ctx, pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
			var readErr error
			out, readErr = service.PhotoConsentsInTransaction(
				ctx,
				tx,
				actor,
				eventID,
				[]uuid.UUID{subject.ID},
			)
			return readErr
		})
		return out, e
	}
	states, err := read(photographer.ID)
	if err != nil || len(states) != 1 || states[0].Choice != "yes" || states[0].Token == "" {
		t.Fatal(states, err)
	}
	binding := privacy.PhotoConsentBinding{PersonID: subject.ID, Token: states[0].Token}
	validate := func(b privacy.PhotoConsentBinding) error {
		return database.Within(ctx, pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
			return service.ValidatePhotoConsentsInTransaction(
				ctx,
				tx,
				eventID,
				[]privacy.PhotoConsentBinding{b},
			)
		})
	}
	if err = validate(binding); err != nil {
		t.Fatal(err)
	}
	wire, _ := json.Marshal(states)
	for _, private := range []string{"PRIVATE-", "private@example.test", "dietary", "accessibility", "contact_email", binding.Token} {
		if strings.Contains(string(wire), private) {
			t.Fatal("photographer guide leaked operational data or internal proof", private)
		}
	}
	if _, err = read(subject.ID); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("attendee read another photographer guide", err)
	}
	fields.Dietary = "PRIVATE-DIETARY-EDIT"
	profile, err = service.Correct(ctx, subject.ID, eventID, profile.Revision, fields)
	if err != nil || validate(binding) != nil {
		t.Fatal("unrelated personal field edit invalidated photo-only proof", err)
	}
	choice, err := service.SetOwnPhotoConsent(ctx, subject.ID, eventID, profile.Revision, "no")
	if err != nil || !errors.Is(validate(binding), access.ErrConflict) {
		t.Fatal("opt-out left consent binding valid", choice, err)
	}
	choice, err = service.SetOwnPhotoConsent(ctx, subject.ID, eventID, choice.Revision, "yes")
	if err != nil || !errors.Is(validate(binding), access.ErrConflict) {
		t.Fatal("no-to-yes resurrected old reviewed permission", choice, err)
	}
	states, err = read(owner.ID)
	if err != nil || states[0].Token == binding.Token {
		t.Fatal("photo-specific revision did not advance", states, err)
	}
	current := privacy.PhotoConsentBinding{PersonID: subject.ID, Token: states[0].Token}
	if err = validate(current); err != nil {
		t.Fatal(err)
	}
	if err = service.Delete(ctx, subject.ID, eventID, subject.ID); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(validate(current), access.ErrConflict) {
		t.Fatal("removed profile retained media consent")
	}
	choice, err = service.SetOwnPhotoConsent(ctx, subject.ID, eventID, 0, "yes")
	if err != nil || !errors.Is(validate(current), access.ErrConflict) {
		t.Fatal("delete/recreate resurrected old incarnation proof", choice, err)
	}
	exec(
		`UPDATE events SET writes_allowed=false,stage='archived',completed_at=$2 WHERE id=$1`,
		eventID,
		now,
	)
	if _, err = service.SetOwnPhotoConsent(
		ctx,
		subject.ID,
		eventID,
		choice.Revision,
		"yes",
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("archived event allowed new opt-in", err)
	}
	choice, err = service.SetOwnPhotoConsent(ctx, subject.ID, eventID, choice.Revision, "no")
	if err != nil || choice.Choice != "no" {
		t.Fatal("archived self revocation denied", choice, err)
	}
	own, err := service.Read(ctx, subject.ID, eventID, subject.ID)
	if err != nil || own.Dietary == nil || *own.Dietary != "" {
		t.Fatal("recreated consent-only profile has unexpected personal fields", own, err)
	}
	var safeAudit string
	if err = pool.QueryRow(ctx, `SELECT jsonb_agg(to_jsonb(a))::text FROM audit_events a WHERE event_id=$1`, eventID).
		Scan(&safeAudit); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(safeAudit, "photo_consent.revoked") ||
		strings.Contains(safeAudit, "PRIVATE-") ||
		strings.Contains(safeAudit, "private@example.test") {
		t.Fatal("choice audit absent or unrelated sensitive data retained", safeAudit)
	}
	now = now.AddDate(0, 0, 91)
	states, err = read(owner.ID)
	if err != nil || states[0].Eligible || states[0].Choice != "unknown" {
		t.Fatal("expired native consent leaked/stayed eligible before worker purge", states, err)
	}
}
