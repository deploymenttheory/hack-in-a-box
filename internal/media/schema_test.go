package media_test

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
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

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/files"
	"github.com/deploymenttheory/hack-in-a-box/internal/forms"
	"github.com/deploymenttheory/hack-in-a-box/internal/githubapp"
	"github.com/deploymenttheory/hack-in-a-box/internal/media"
	"github.com/deploymenttheory/hack-in-a-box/internal/objectives"
	"github.com/deploymenttheory/hack-in-a-box/internal/privacy"
	"github.com/deploymenttheory/hack-in-a-box/internal/secretbox"
	"github.com/deploymenttheory/hack-in-a-box/internal/surveys"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func mediaMigrator(t *testing.T, pool *pgxpool.Pool) *goose.Provider {
	t.Helper()
	db := stdlib.OpenDBFromPool(pool)
	t.Cleanup(func() { _ = db.Close() })
	m, err := goose.NewProvider(goose.DialectPostgres, db, os.DirFS("../database/migrations"))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func retained41(t *testing.T, pool *pgxpool.Pool, eventID uuid.UUID) map[string]string {
	t.Helper()
	out := map[string]string{}
	// These are fixed owning tables, never caller-controlled SQL identifiers.
	for _, table := range []string{"survey_heads", "survey_versions", "survey_approvals", "survey_publications", "survey_invitations", "survey_response_heads", "survey_responses", "survey_commands", "survey_report_snapshots", "survey_report_reviews", "survey_report_approvals", "survey_report_publications", "objective_versions", "rubric_versions", "rubric_approvals", "files", "github_app_integrations", "github_app_install_attempts"} {
		var rows string
		query := fmt.Sprintf(
			`SELECT COALESCE(jsonb_agg(to_jsonb(t) ORDER BY to_jsonb(t)::text),'[]'::jsonb)::text FROM %s t WHERE event_id=$1`,
			table,
		) // #nosec G201 -- Fixed owning table names above, not request input.
		if err := pool.QueryRow(t.Context(), query, eventID).Scan(&rows); err != nil {
			t.Fatal(table, err)
		}
		out[table] = rows
	}
	var profile string
	if err := pool.QueryRow(t.Context(), `SELECT jsonb_build_object('id',id,'person',person_id,'email',contact_email,'dietary',dietary,'accessibility',accessibility,'photo',photo_consent,'revision',revision)::text FROM operational_profiles WHERE event_id=$1`, eventID).
		Scan(&profile); err != nil {
		t.Fatal(err)
	}
	out["operational_profile_legacy_fields"] = profile
	return out
}

func TestMedia42Populated41UpgradePreservesNativeSurveyHistoryAndPrivacyErasure(t *testing.T) {
	pool := testdb.New(t, false)
	ctx := t.Context()
	if err := database.Migrate(ctx, pool, 41); err != nil {
		t.Fatal(err)
	}
	owner, subject := testdb.Person(t, pool), testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, owner.ID)
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Date(2027, 6, 20, 20, 0, 0, 0, time.UTC)
	dates := event.DefaultConfiguration()
	dates.LiveStart = now.Add(-12 * time.Hour)
	dates.LiveEnd = now.Add(-4 * time.Hour)
	dates.SubmissionCutoff = now.Add(-5 * time.Hour)
	dates.TeamFreeze = now.AddDate(0, 0, -8)
	dates.AdmissionCutoff = dates.TeamFreeze.Add(-time.Hour)
	raw, _ := json.Marshal(dates)
	exec(
		`UPDATE events SET stage='completed',configuration=$2,completed_at=$3 WHERE id=$1`,
		eventID,
		raw,
		now.Add(-3*time.Hour),
	)
	exec(
		`INSERT INTO competition_claims(event_id,person_id,state)VALUES($1,$2,'confirmed')`,
		eventID,
		subject.ID,
	)
	// Historical fixture deliberately uses the actual41 columns; latest generated
	// privacy queries include42's new column and cannot seed an older schema.
	exec(
		`INSERT INTO operational_profiles(event_id,person_id,contact_email,dietary,accessibility,photo_consent)VALUES($1,$2,'legacy-private@example.test','LEGACY-DIETARY','LEGACY-ACCESS','yes')`,
		eventID,
		subject.ID,
	)
	keyMaterial := make([]byte, 32)
	if _, err := rand.Read(keyMaterial); err != nil {
		t.Fatal(err)
	}
	keys, err := secretbox.Parse(
		fmt.Sprintf(
			`{"active":"fixture","keys":{"fixture":"%s"}}`,
			base64.StdEncoding.EncodeToString(keyMaterial),
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	// This fixture intentionally runs schema41. Seed its exact historical columns
	// and authenticated encryption purpose instead of invoking the current App
	// service, whose configuration lifecycle also owns schema46 webhook secrets.
	appPEM := pem.EncodeToMemory(&pem.Block{
		Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(rsaKey),
	})
	defer clear(appPEM)
	legacyAppBinding := []byte(fmt.Sprintf(
		"hackbox:github-app-private-key:v1:%s:%d:%d", eventID, 123, 1,
	))
	appKeyID, appEncrypted, err := keys.Seal(appPEM, legacyAppBinding)
	if err != nil {
		t.Fatal("encrypt actual predecessor41 App state", err)
	}
	exec(
		`INSERT INTO github_app_integrations(event_id,app_id,app_slug,organization_id,organization,template_repository,credential_version,key_id,private_key_encrypted,installation_id)
	 VALUES($1,123,'synthetic-app',456,'synthetic-org','synthetic-template',1,$2,$3,0)`,
		eventID,
		appKeyID,
		appEncrypted,
	)
	objective, err := (objectives.Service{Pool: pool}).Change(
		ctx,
		owner.ID,
		eventID,
		uuid.New(),
		0,
		objectives.Command{
			Action: "save",
			Reason: "Retain actual39 objective evidence",
			Definition: &objectives.Definition{
				Description: "Native39 learning objective",
				Measures: []objectives.Measure{
					{
						ID:          uuid.New(),
						Description: "Learning evidence",
						Method:      "Survey",
						Unit:        "person",
					},
				},
			},
		},
	)
	if err != nil || objective.Reference.Version != 1 {
		t.Fatal(err)
	}
	testdb.ApprovedRubric(t, pool, owner.ID, eventID)

	storage, err := files.OpenLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storage.Close() })
	nativeFiles := files.Service{
		Pool:       pool,
		Storage:    storage,
		Policy:     access.EventPolicy{Queries: dbgen.New(pool)},
		MaxBytes:   1024,
		EventQuota: 8192,
	}
	fileEvent, _ := dbgen.New(pool).Event(ctx, eventID)
	oldFile, err := nativeFiles.Upload(
		ctx,
		files.Upload{
			Actor:     owner.ID,
			Event:     eventID,
			Key:       uuid.New(),
			Revision:  fileEvent.Revision,
			Filename:  "predecessor.txt",
			MediaType: "text/plain",
			Content:   []byte("ACTUAL41-NATIVE-FILE"),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	surveyID, questionID := uuid.New(), uuid.New()
	survey := surveys.Service{Pool: pool, Now: func() time.Time { return now }}
	definition := surveys.Definition{
		Form: forms.Definition{
			ID:       surveyID,
			Name:     "Actual41 retained survey",
			Audience: "attendee",
			Fields: []forms.Field{
				{ID: questionID, Label: "Private response", Type: "text", Required: true},
			},
		},
		OpensAt:       now.Add(-time.Hour),
		ClosesAt:      now.Add(time.Hour),
		MinimumCohort: 5,
		Measures:      []surveys.QuestionMeasure{},
	}
	config, err := survey.Change(
		ctx,
		owner.ID,
		eventID,
		surveyID,
		0,
		surveys.Command{Action: "save", Definition: &definition},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"approve", "publish"} {
		config, err = survey.Change(
			ctx,
			owner.ID,
			eventID,
			surveyID,
			config.Version,
			surveys.Command{Action: action},
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	response := surveys.ResponseCommand{
		CommandID:   uuid.New(),
		FormVersion: config.Version,
		Answers: forms.Answers{
			questionID.String(): json.RawMessage(`"ACTUAL41-PRIVATE-ANSWER"`),
		},
	}
	if _, err = survey.Respond(ctx, subject.ID, eventID, surveyID, 0, response); err != nil {
		t.Fatal(err)
	}
	now = definition.ClosesAt
	review, err := survey.ChangeReport(
		ctx,
		owner.ID,
		eventID,
		surveyID,
		0,
		surveys.ReportCommand{Action: "review_text", ReviewID: uuid.New(), ReviewedText: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"approve", "publish"} {
		review, err = survey.ChangeReport(
			ctx,
			owner.ID,
			eventID,
			surveyID,
			review.Version,
			surveys.ReportCommand{Action: action, ReviewID: review.ReviewID},
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	releasedReport, err := survey.PublishedReport(ctx, owner.ID, eventID, surveyID)
	if err != nil {
		t.Fatal(err)
	}
	before := retained41(t, pool, eventID)
	for _, table := range []string{"survey_responses", "survey_commands", "survey_report_publications", "objective_versions", "rubric_versions", "files"} {
		if before[table] == "[]" {
			t.Fatal("preservation fixture was empty", table)
		}
	}
	if err = database.Migrate(ctx, pool, 42); err != nil {
		t.Fatal(err)
	}
	if after := retained41(t, pool, eventID); !reflect.DeepEqual(before, after) {
		t.Fatal("normal41→42 changed owning survey/prior39 history", before, after)
	}

	if _, data, e := nativeFiles.Download(
		ctx,
		owner.ID,
		eventID,
		oldFile.ID,
	); e != nil ||
		string(data) != "ACTUAL41-NATIVE-FILE" {
		t.Fatal("media42 changed predecessor ordinary-file scope/bytes", e)
	}
	own, err := survey.ReadOwn(ctx, subject.ID, eventID, surveyID)
	if err != nil || own.Response == nil ||
		string(own.Response.Answers[questionID.String()]) != `"ACTUAL41-PRIVATE-ANSWER"` {
		t.Fatal("upgraded native reader lost private authored response", own, err)
	}
	// Empty media rollback is allowed even though genuine41/39 evidence exists.
	migrator := mediaMigrator(t, pool)
	if _, err = migrator.DownTo(ctx, 41); err != nil {
		t.Fatal(err)
	}
	if after := retained41(t, pool, eventID); !reflect.DeepEqual(before, after) {
		t.Fatal("media rollback changed retained predecessor data")
	}
	if err = database.Migrate(ctx, pool, 42); err != nil {
		t.Fatal(err)
	}
	native := media.Service{Pool: pool, Clock: func() time.Time { return now }}
	row, _ := dbgen.New(pool).Event(ctx, eventID)
	if _, err = native.Change(
		ctx,
		owner.ID,
		eventID,
		uuid.New(),
		row.Revision,
		media.Command{
			Action:    "save",
			Version:   0,
			CommandID: uuid.New(),
			Reason:    "Guard immutable media history",
			Metadata: &media.Metadata{
				Kind:         "recording",
				Title:        "Native42 recording",
				RecordingURL: "https://recordings.example.test/review",
			},
		},
	); err != nil {
		t.Fatal(err)
	}
	if _, err = migrator.DownTo(
		ctx,
		41,
	); err == nil ||
		!strings.Contains(err.Error(), "Reviewed media") {
		t.Fatal("populated media rollback discarded retained history", err)
	}
	if version, e := migrator.GetDBVersion(ctx); e != nil || version != 42 {
		t.Fatal("guard did not preserve schema42", version, e)
	}
	// Preserve the explicit concrete42 rollback assertion above, then restore
	// all current migrations before later native services and compatibility checks.
	if err = database.Migrate(ctx, pool, 0); err != nil {
		t.Fatal(err)
	}
	// The historical ciphertext must remain byte-for-byte intact, decrypt with
	// its actual41 AAD, and support the current App key-rotation lifecycle after
	// all migrations. This catches fixtures that merely seed arbitrary bytes.
	appRow, err := dbgen.New(pool).GitHubAppIntegration(ctx, eventID)
	if err != nil || !bytes.Equal(appRow.PrivateKeyEncrypted, appEncrypted) ||
		appRow.CredentialVersion != 1 || appRow.KeyID != appKeyID {
		t.Fatal("upgrade changed predecessor41 encrypted App state", err)
	}
	appPlain, err := keys.Open(appRow.KeyID, appRow.PrivateKeyEncrypted, legacyAppBinding)
	if err != nil || !bytes.Equal(appPlain, appPEM) {
		t.Fatal("predecessor41 App ciphertext no longer decrypts", err)
	}
	clear(appPlain)
	currentApp := githubapp.Service{Pool: pool, Keys: keys, Provider: githubapp.NewProvider()}
	currentEvent, err := dbgen.New(pool).Event(ctx, eventID)
	if err != nil {
		t.Fatal(err)
	}
	if err = currentApp.Configure(
		ctx,
		owner.ID,
		eventID,
		currentEvent.Revision,
		githubapp.Command{Action: "rotate"},
	); err != nil {
		t.Fatal("current App owner cannot rotate predecessor41 ciphertext", err)
	}
	rotatedApp, err := dbgen.New(pool).GitHubAppIntegration(ctx, eventID)
	if err != nil || rotatedApp.CredentialVersion != 2 ||
		bytes.Equal(rotatedApp.PrivateKeyEncrypted, appEncrypted) {
		t.Fatal("current rotation did not replace predecessor41 ciphertext", err)
	}
	rotatedBinding := []byte(fmt.Sprintf(
		"hackbox:github-app-private-key:v1:%s:%d:%d", eventID, 123, 2,
	))
	appPlain, err = keys.Open(rotatedApp.KeyID, rotatedApp.PrivateKeyEncrypted, rotatedBinding)
	if err != nil || !bytes.Equal(appPlain, appPEM) {
		t.Fatal("current rotation changed predecessor41 App key", err)
	}
	clear(appPlain)
	// The concrete115 native privacy callback must still erase invitations,
	// private authored revisions and command receipts, preserving the frozen report.
	privacyService := privacy.Service{Pool: pool, Clock: func() time.Time { return now }}
	if err = privacyService.Delete(ctx, subject.ID, eventID, subject.ID); err != nil {
		t.Fatal(err)
	}
	var privateRows int
	if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM survey_responses WHERE event_id=$1 AND person_id=$2)+(SELECT count(*) FROM survey_commands WHERE event_id=$1 AND person_id=$2)+(SELECT count(*) FROM survey_invitations WHERE event_id=$1 AND person_id=$2)`, eventID, subject.ID).
		Scan(&privateRows); err != nil ||
		privateRows != 0 {
		t.Fatal("media composition lost native survey privacy erasure", privateRows, err)
	}
	if _, err = survey.ReadOwn(
		ctx,
		subject.ID,
		eventID,
		surveyID,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("erased private response remained readable", err)
	}
	published, err := survey.PublishedReport(ctx, owner.ID, eventID, surveyID)
	if err != nil || !reflect.DeepEqual(published, releasedReport) {
		t.Fatal("native privacy removal lost retained suppressed report", published, err)
	}
	if err = database.CheckSchema(ctx, pool); err != nil {
		t.Fatal(err)
	}
}

func TestMedia42EmptyRollbackAndPhotoConsentOnlyHistoryGuard(t *testing.T) {
	pool := testdb.New(t, false)
	ctx := t.Context()
	if err := database.Migrate(ctx, pool, 41); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, pool, 42); err != nil {
		t.Fatal(err)
	}
	migrator := mediaMigrator(t, pool)
	if _, err := migrator.DownTo(ctx, 41); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, pool, 42); err != nil {
		t.Fatal(err)
	}
	owner := testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, owner.ID)
	service := privacy.Service{Pool: pool}
	choice, err := service.SetOwnPhotoConsent(ctx, owner.ID, eventID, 0, "unknown")
	if err != nil || choice.Revision != 1 {
		t.Fatal(choice, err)
	}
	if _, err = migrator.DownTo(
		ctx,
		41,
	); err == nil ||
		!strings.Contains(err.Error(), "Photo-consent") {
		t.Fatal("photo-only audit provenance lost on rollback", err)
	}
	if version, e := migrator.GetDBVersion(ctx); e != nil || version != 42 {
		t.Fatal("photo-only guard lost schema42", version, e)
	}
	if err = database.Migrate(ctx, pool, 0); err != nil {
		t.Fatal(err)
	}
}
