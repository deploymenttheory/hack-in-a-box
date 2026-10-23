package outcomes

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/forms"
	"github.com/deploymenttheory/hack-in-a-box/internal/media"
	"github.com/deploymenttheory/hack-in-a-box/internal/privacy"
	"github.com/deploymenttheory/hack-in-a-box/internal/surveys"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func outcomeMigrator(t *testing.T, pool *pgxpool.Pool) *goose.Provider {
	t.Helper()
	db := stdlib.OpenDBFromPool(pool)
	t.Cleanup(func() { _ = db.Close() })
	m, err := goose.NewProvider(goose.DialectPostgres, db, os.DirFS("../database/migrations"))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func retained41(t *testing.T, pool *pgxpool.Pool) map[string]string {
	t.Helper()
	out := map[string]string{}
	// Fixed predecessor-owner tables, never caller-supplied SQL identifiers.
	for _, table := range []string{"event_media", "media_versions", "media_commands", "media_files", "operational_profiles", "survey_heads", "survey_versions", "survey_approvals", "survey_publications", "survey_invitations", "survey_commands", "survey_responses", "survey_response_heads", "objective_versions", "audit_events"} {
		var rows string
		query := fmt.Sprintf(
			`SELECT COALESCE(jsonb_agg(to_jsonb(t) ORDER BY to_jsonb(t)::text),'[]'::jsonb)::text FROM %s t`,
			table,
		) // #nosec G201 -- Fixed native owner table names above.
		if err := pool.QueryRow(t.Context(), query).Scan(&rows); err != nil {
			t.Fatal(table, err)
		}
		out[table] = rows
	}
	return out
}

func TestOutcomes42Actual41UpgradePreservesNativeMediaSurveyAndGuardedHistory(t *testing.T) {
	pool := testdb.New(t, false)
	ctx := t.Context()
	if err := database.Migrate(ctx, pool, 41); err != nil {
		t.Fatal(err)
	}
	org, subject := testdb.Person(t, pool), testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, org.ID)
	if _, err := pool.Exec(
		ctx,
		`INSERT INTO competition_claims(event_id,person_id,state)VALUES($1,$2,'confirmed')`,
		eventID,
		subject.ID,
	); err != nil {
		t.Fatal(err)
	}
	privacyService := privacy.Service{Pool: pool}
	if _, err := privacyService.SetOwnPhotoConsent(ctx, subject.ID, eventID, 0, "yes"); err != nil {
		t.Fatal(err)
	}
	row, err := dbgen.New(pool).Event(ctx, eventID)
	if err != nil {
		t.Fatal(err)
	}
	item, err := (media.Service{Pool: pool, Privacy: privacyService}).Change(
		ctx,
		org.ID,
		eventID,
		uuid.New(),
		row.Revision,
		media.Command{
			Action:    "save",
			Version:   0,
			CommandID: uuid.New(),
			Reason:    "Preserve actual native41 history",
			Metadata: &media.Metadata{
				Kind:         "recording",
				Title:        "Predecessor reviewed source",
				RecordingURL: "https://recordings.example.test/retained",
				People:       []uuid.UUID{subject.ID},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	surveyID, fieldID := uuid.New(), uuid.New()
	survey := surveys.Service{Pool: pool}
	if _, err = survey.Change(
		ctx,
		org.ID,
		eventID,
		surveyID,
		0,
		surveys.Command{
			Action: "save",
			Definition: &surveys.Definition{
				Form: forms.Definition{
					ID:       surveyID,
					Name:     "Retained native40 definition",
					Audience: "attendee",
					Fields:   []forms.Field{{ID: fieldID, Label: "Private response", Type: "text"}},
				},
				OpensAt:       now.Add(time.Hour),
				ClosesAt:      now.Add(48 * time.Hour),
				MinimumCohort: 5,
				Measures:      []surveys.QuestionMeasure{},
			},
		},
	); err != nil {
		t.Fatal(err)
	}
	before := retained41(t, pool)
	if err = database.Migrate(ctx, pool, 42); err != nil {
		t.Fatal(err)
	}
	if after := retained41(t, pool); !reflect.DeepEqual(before, after) {
		t.Fatal("42 upgrade changed native41 history", before, after)
	}
	// Empty-owned rollback must preserve preceding media/consent/survey data.
	migrator := outcomeMigrator(t, pool)
	if _, err = migrator.DownTo(ctx, 41); err != nil {
		t.Fatal(err)
	}
	if after := retained41(t, pool); !reflect.DeepEqual(before, after) {
		t.Fatal("empty42 rollback changed predecessors")
	}
	if _, err = (media.Service{Pool: pool, Privacy: privacyService}).Read(
		ctx,
		org.ID,
		eventID,
		item.ID,
	); err != nil {
		t.Fatal("native41 reader lost", err)
	}
	if err = database.Migrate(ctx, pool, 42); err != nil {
		t.Fatal(err)
	}
	author := testdb.Person(t, pool)
	if _, err = (Service{Pool: pool}).Change(
		ctx,
		author.ID,
		uuid.New(),
		0,
		proposal(),
	); err != nil {
		t.Fatal(err)
	}
	if _, err = migrator.DownTo(
		ctx,
		41,
	); err == nil ||
		!strings.Contains(err.Error(), "Outcome, publication or approved review history") {
		t.Fatal("populated42 rollback discarded outcome history", err)
	}
	if version, e := migrator.GetDBVersion(ctx); e != nil || version != 42 {
		t.Fatal("guard lost concrete42", version, e)
	}
	// Future migrations may have rolled down before this concrete guard blocked;
	// restore the full current chain before subsequent owner/query checks.
	if err = database.Migrate(ctx, pool, 0); err != nil {
		t.Fatal(err)
	}
	if err = database.CheckSchema(ctx, pool); err != nil {
		t.Fatal(err)
	}
}
