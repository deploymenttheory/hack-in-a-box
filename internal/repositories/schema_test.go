package repositories

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
	"github.com/deploymenttheory/hack-in-a-box/internal/forms"
	"github.com/deploymenttheory/hack-in-a-box/internal/media"
	"github.com/deploymenttheory/hack-in-a-box/internal/outcomes"
	"github.com/deploymenttheory/hack-in-a-box/internal/privacy"
	"github.com/deploymenttheory/hack-in-a-box/internal/surveys"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func retainedRepositoryPredecessors(t *testing.T, pool *pgxpool.Pool) map[string]string {
	t.Helper()
	result := map[string]string{}
	for _, table := range []string{"people", "events", "event_memberships", "competition_claims", "hack_teams", "hack_team_members", "competition_entries", "github_app_integrations", "github_app_install_attempts", "operational_profiles", "event_privacy", "event_media", "media_versions", "media_commands", "survey_heads", "survey_versions", "outcome_heads", "outcome_versions", "audit_events", "operations", "delivery_attempts"} {
		var data string
		query := fmt.Sprintf(
			`SELECT COALESCE(jsonb_agg(to_jsonb(r) ORDER BY to_jsonb(r)::text),'[]'::jsonb)::text FROM %s r`,
			table,
		)
		if err := pool.QueryRow(t.Context(), query).Scan(&data); err != nil {
			t.Fatal(table, err)
		}
		result[table] = data
	}
	return result
}

func TestRepository45Actual43And44UpgradeAndDurableBarrierRetention(t *testing.T) {
	pool := testdb.New(t, false)
	ctx := t.Context()
	if err := database.Migrate(ctx, pool, 43); err != nil {
		t.Fatal(err)
	}
	f := setup(t, pool)
	team := f.team("published")
	// Actual predecessor owners write their normal native records before upgrade.
	if _, err := (outcomes.Service{Pool: pool}).Change(
		ctx,
		f.member,
		uuid.New(),
		0,
		outcomes.Command{
			ID:     uuid.New(),
			Action: "propose",
			Reason: "Retain predecessor43 proposal",
			Definition: &outcomes.Definition{
				Kind:          "pitch",
				Problem:       "Synthetic problem",
				Audience:      "Synthetic audience",
				ProposedValue: "Proposed value, not an achieved result",
			},
		},
	); err != nil {
		t.Fatal(err)
	}
	consent := privacy.Service{Pool: pool}
	if _, err := consent.SetOwnPhotoConsent(ctx, f.member, f.event, 0, "yes"); err != nil {
		t.Fatal(err)
	}
	if _, err := (media.Service{Pool: pool, Privacy: consent}).Change(
		ctx,
		f.owner,
		f.event,
		uuid.New(),
		1,
		media.Command{
			Action:    "save",
			Version:   0,
			CommandID: uuid.New(),
			Reason:    "Retain native42 media",
			Metadata: &media.Metadata{
				Kind:         "recording",
				Title:        "Synthetic retained recording",
				RecordingURL: "https://recordings.example.test/retained",
				People:       []uuid.UUID{f.member},
			},
		},
	); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	surveyID := uuid.New()
	if _, err := (surveys.Service{Pool: pool}).Change(
		ctx,
		f.owner,
		f.event,
		surveyID,
		0,
		surveys.Command{
			Action: "save",
			Definition: &surveys.Definition{
				Form: forms.Definition{
					ID:       surveyID,
					Name:     "Retained native40 definition",
					Audience: "attendee",
					Fields: []forms.Field{
						{ID: uuid.New(), Label: "Synthetic response", Type: "text"},
					},
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
	before := retainedRepositoryPredecessors(t, pool)
	if err := database.Migrate(ctx, pool, 44); err != nil {
		t.Fatal(err)
	}
	if after := retainedRepositoryPredecessors(t, pool); !reflect.DeepEqual(before, after) {
		t.Fatal("44 upgrade changed retained43 App/media/outcome/survey/team history")
	}
	if err := database.Migrate(ctx, pool, 45); err != nil {
		t.Fatal(err)
	}
	if after := retainedRepositoryPredecessors(t, pool); !reflect.DeepEqual(before, after) {
		t.Fatal("45 guard upgrade changed native predecessors")
	}
	db := stdlib.OpenDBFromPool(pool)
	t.Cleanup(func() { _ = db.Close() })
	migrator, err := goose.NewProvider(
		goose.DialectPostgres,
		db,
		os.DirFS("../database/migrations"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = migrator.DownTo(ctx, 43); err != nil {
		t.Fatalf("empty45/44 rollback: %v", err)
	}
	if version, err := migrator.GetDBVersion(ctx); err != nil || version != 43 {
		t.Fatalf("empty rollback version %d %v", version, err)
	}
	if after := retainedRepositoryPredecessors(t, pool); !reflect.DeepEqual(before, after) {
		t.Fatal("empty45/44 rollback changed native predecessors")
	}
	if err = database.Migrate(ctx, pool, 44); err != nil {
		t.Fatal(err)
	}
	// The dangerous retained case has an uncertain external effect and no known ID.
	f.p.timeout = Name(f.event, team)
	f.queue()
	f.execute(team)
	retained := f.row(team)
	if !retained.Attempted || retained.GithubRepositoryID.Valid || retained.State != "ambiguous" {
		t.Fatalf("fixture not uncertain: %+v", retained)
	}
	history := retainedRepositoryPredecessors(t, pool)
	if err := database.Migrate(ctx, pool, 45); err != nil {
		t.Fatal(err)
	}
	if after := f.row(team); !reflect.DeepEqual(retained, after) {
		t.Fatal("44→45 guard upgrade changed uncertain identity/provenance")
	}
	if after := retainedRepositoryPredecessors(t, pool); !reflect.DeepEqual(history, after) {
		t.Fatal("44→45 guard upgrade changed native/delivery history")
	}
	for _, query := range []string{"DELETE FROM team_repositories WHERE event_id=$1", "TRUNCATE team_repositories", "UPDATE team_repositories SET attempted=false WHERE event_id=$1"} {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(query, "$1") {
			_, err = tx.Exec(ctx, query, f.event)
		} else {
			_, err = tx.Exec(ctx, query)
		}
		_ = tx.Rollback(ctx)
		if err == nil {
			t.Errorf("durable create barrier could be erased: %s", query)
		}
	}
	if _, err = migrator.DownTo(
		ctx,
		43,
	); err == nil ||
		!strings.Contains(err.Error(), "Repository provenance or provisioning history") {
		t.Fatalf("populated45 rollback was not protected: %v", err)
	}
	if version, err := migrator.GetDBVersion(ctx); err != nil || version != 45 {
		t.Fatalf("protected rollback changed schema version %d %v", version, err)
	}
	if after := f.row(team); !reflect.DeepEqual(retained, after) {
		t.Fatal("protected rollback changed uncertain identity/provenance")
	}
	if after := retainedRepositoryPredecessors(t, pool); !reflect.DeepEqual(history, after) {
		t.Fatal("protected rollback changed preceding native/delivery history")
	}
	// Repeated native queue/reconciliation cannot mint a replacement after failure.
	f.queue()
	if got := f.row(team); got.Provenance != retained.Provenance || !got.Attempted {
		t.Fatal("requeue replaced durable provenance/barrier")
	}
}
