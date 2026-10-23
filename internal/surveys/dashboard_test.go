package surveys

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/forms"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestRestrictedDashboardCountsAndAuditedExportRespectCurrentPrivacyAndAuthority(t *testing.T) {
	f := setup(t)
	cfg := f.publish()
	ctx := t.Context()
	f.exec("UPDATE people SET display_name='=UNTRUSTED-NAME' WHERE id=$1", f.person.ID)
	input := ResponseCommand{
		CommandID:   uuid.New(),
		FormVersion: cfg.Version,
		Answers: forms.Answers{
			f.choice.String(): json.RawMessage(`"yes"`),
			f.text.String():   json.RawMessage(`"PRIVATE-CSV-RESPONSE"`),
		},
	}
	if _, err := f.s.Respond(ctx, f.person.ID, f.event, f.id, 0, input); err != nil {
		t.Fatal(err)
	}
	page, err := f.s.Dashboard(ctx, f.owner.ID, f.event, f.id)
	if err != nil || page.RetainedInvitations != 2 || page.SubmittedResponses != 1 ||
		page.CurrentEligibleResponses != 1 ||
		page.CurrentEligibleInvitations != 2 ||
		page.CanReviewReport ||
		!page.ObservedAt.Equal(f.s.now()) {
		t.Fatal("restricted denominator projection", page, err)
	}
	for _, actor := range []uuid.UUID{f.person.ID, f.other.ID} {
		if _, err = f.s.Dashboard(ctx, actor, f.event, f.id); !errors.Is(err, access.ErrForbidden) {
			t.Fatal("participant accessed authored counts", err)
		}
		if _, err = f.s.ExportResponses(
			ctx,
			actor,
			f.event,
			f.id,
		); !errors.Is(
			err,
			access.ErrForbidden,
		) {
			t.Fatal("participant exported private authorship", err)
		}
	}
	raw, err := f.s.ExportResponses(ctx, f.owner.ID, f.event, f.id)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(strings.NewReader(string(raw))).ReadAll()
	if err != nil || len(rows) != 3 {
		t.Fatal("restricted CSV shape", err, len(rows))
	}
	if rows[1][1] != "'=UNTRUSTED-NAME" || !strings.Contains(string(raw), "PRIVATE-CSV-RESPONSE") ||
		rows[1][3] != "1" {
		t.Fatal("CSV lost exact private provenance or spreadsheet safety")
	}
	var evidence []byte
	if err = f.pool.QueryRow(ctx, "SELECT coalesce(jsonb_agg(to_jsonb(a)),'[]') FROM audit_events a WHERE event_id=$1", f.event).
		Scan(&evidence); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(evidence), "survey.responses_exported") ||
		strings.Contains(string(evidence), "PRIVATE-CSV-RESPONSE") ||
		strings.Contains(string(evidence), "UNTRUSTED-NAME") {
		t.Fatal("export audit missing or exposed private response")
	}
	f.exec(
		"UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2",
		f.event,
		f.person.ID,
	)
	page, err = f.s.Dashboard(ctx, f.owner.ID, f.event, f.id)
	if err != nil || page.SubmittedResponses != 1 || page.CurrentEligibleResponses != 0 ||
		page.RetainedInvitations != 2 ||
		page.CurrentEligibleInvitations != 1 {
		t.Fatal("current and retained cohorts were mixed", page, err)
	}
	if err = database.Within(ctx, f.pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		if _, e := q.LockEvent(ctx, f.event); e != nil {
			return e
		}
		return ErasePersonInTransaction(
			ctx,
			tx,
			&f.person.ID,
			f.event,
			f.person.ID,
			audit.UserRequest,
		)
	}); err != nil {
		t.Fatal(err)
	}
	page, err = f.s.Dashboard(ctx, f.owner.ID, f.event, f.id)
	if err != nil || page.RetainedInvitations != 1 || page.SubmittedResponses != 0 {
		t.Fatal("erasure fabricated original publication denominator", page, err)
	}
	f.clock.Store(f.definition.ClosesAt.UnixNano())
	page, err = f.s.Dashboard(ctx, f.owner.ID, f.event, f.id)
	if err != nil || !page.CanReviewReport {
		t.Fatal("native closed report capability", err)
	}
	var dates map[string]any
	var encoded []byte
	if err = f.pool.QueryRow(ctx, "SELECT configuration FROM events WHERE id=$1", f.event).
		Scan(&encoded); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(encoded, &dates); err != nil {
		t.Fatal(err)
	}
	dates["live_end"] = f.s.now().Add(time.Hour).Format(time.RFC3339)
	encoded, err = json.Marshal(dates)
	if err != nil {
		t.Fatal(err)
	}
	f.exec("UPDATE events SET configuration=$2 WHERE id=$1", f.event, encoded)
	page, err = f.s.Dashboard(ctx, f.owner.ID, f.event, f.id)
	if err != nil || page.CanReviewReport {
		t.Fatal("current schedule allowed premature review", err)
	}
	staff := testdb.Person(t, f.pool)
	f.exec(
		"INSERT INTO event_memberships(event_id,person_id,role,active)VALUES($1,$2,'organiser',true)",
		f.event,
		staff.ID,
	)
	if _, err = f.s.Dashboard(ctx, staff.ID, f.event, f.id); err != nil {
		t.Fatal(err)
	}
	f.exec(
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2",
		f.event,
		staff.ID,
	)
	if _, err = f.s.ExportResponses(
		ctx,
		staff.ID,
		f.event,
		f.id,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("revoked organiser exported responses", err)
	}
	f.clock.Store(f.definition.ClosesAt.AddDate(0, 0, 100).UnixNano())
	if _, err = f.s.Dashboard(
		ctx,
		f.owner.ID,
		f.event,
		f.id,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("expired dashboard implied zero achievement", err)
	}
	if _, err = f.s.ExportResponses(
		ctx,
		f.owner.ID,
		f.event,
		f.id,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("expired response export survived privacy deadline", err)
	}
}

func TestSurveyCSVLiteralNeutralisesSpreadsheetCommands(t *testing.T) {
	for _, value := range []string{"=1+1", "  =1+1", "+cmd", "-cmd", "@cmd", "\ttext", "\rtext", "\ntext"} {
		if csvLiteral(value) != "'"+value {
			t.Fatal("unsafe CSV value", value)
		}
	}
	for _, value := range []string{"plain text", "safe=equals", "1", "0", ""} {
		if csvLiteral(value) != value {
			t.Fatal("ordinary value changed", value)
		}
	}
}
