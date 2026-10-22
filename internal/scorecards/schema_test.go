package scorecards

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestScorecardMigrationPreservesPopulated32AndGuardedRollback(t *testing.T) {
	const predecessor, protectedVersion int64 = 32, 33
	pool := testdb.New(t, false)
	ctx := t.Context()
	if err := database.Migrate(ctx, pool, predecessor); err != nil {
		t.Fatal(err)
	}
	owner, mentor, member := testdb.Person(t, pool), testdb.Person(t, pool), testdb.Person(t, pool)
	eventID, teamID, entryID, helpID, noteID, rubricID := testdb.Event(
		t,
		pool,
		owner.ID,
	), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC().Truncate(time.Second)
	config := event.DefaultConfiguration()
	config.LiveStart = now.Add(-time.Hour)
	config.LiveEnd = now.Add(3 * time.Hour)
	config.SubmissionCutoff = now.Add(2 * time.Hour)
	raw, _ := json.Marshal(config)
	exec(
		"UPDATE events SET stage='live',configuration=$2,published_at=$3 WHERE id=$1",
		eventID,
		raw,
		now.AddDate(0, 0, -7),
	)
	exec(
		"INSERT INTO hack_teams(event_id,id,name,state,frozen) VALUES($1,$2,'Retained pre33 team','published',true)",
		eventID,
		teamID,
	)
	exec(
		"INSERT INTO competition_entries(id,event_id,team_id) VALUES($1,$2,$3)",
		entryID,
		eventID,
		teamID,
	)
	exec(
		"INSERT INTO competition_claims(event_id,person_id,state) VALUES($1,$2,'confirmed')",
		eventID,
		member.ID,
	)
	exec(
		"INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot) VALUES($1,$2,$3,'published')",
		eventID,
		teamID,
		member.ID,
	)
	exec(
		"INSERT INTO event_memberships(event_id,person_id,role) VALUES($1,$2,'mentor')",
		eventID,
		mentor.ID,
	)
	exec(
		"INSERT INTO mentor_assignments(event_id,team_id,person_id,slot,snapshot,starts_at,ends_at) VALUES($1,$2,$3,'primary','published',$4,$5)",
		eventID,
		teamID,
		mentor.ID,
		config.LiveStart,
		config.LiveEnd,
	)
	exec(
		"INSERT INTO policy_resources(event_id,id,kind) VALUES($1,$2,'team'),($1,$3,'assessment'),($1,$4,'help')",
		eventID,
		teamID,
		entryID,
		helpID,
	)
	// Explicit prior32 columns retain real native private histories; migration
	// must not manufacture a card, rubric publication or moderated result.
	exec(
		`INSERT INTO help_requests(event_id,id,team_id,requester_id,revision,category,description,location,meeting_url,status,waiting_cycle,requested_at,waiting_since,updated_at) VALUES($1,$2,$3,$4,1,'logistics','Retained private help canary','Retained private location','','waiting',1,$5,$5,$5)`,
		eventID,
		helpID,
		teamID,
		member.ID,
		now,
	)
	exec(
		`INSERT INTO help_transitions(event_id,request_id,revision,action,status,actor_id,occurred_at) VALUES($1,$2,1,'create','waiting',$3,$4)`,
		eventID,
		helpID,
		member.ID,
		now,
	)
	exec(
		`INSERT INTO mentor_observation_versions(event_id,team_id,id,version,author_id,observed_at,changed_at,category,evidence,outcome) VALUES($1,$2,$3,1,$4,$5,$5,'technical','Retained private observation canary','Retained outcome')`,
		eventID,
		teamID,
		noteID,
		mentor.ID,
		now,
	)
	definition := rubric.Default(rubric.AllTeams, eventID)
	full, _ := json.Marshal(definition)
	public, _ := json.Marshal(definition.PublicDefinition)
	exec(
		"INSERT INTO event_rubrics(event_id,id,revision,latest_version) VALUES($1,$2,2,1)",
		eventID,
		rubricID,
	)
	exec(
		"INSERT INTO rubric_versions(event_id,rubric_id,version,document,public_document,public_digest,actor_id,created_at) VALUES($1,$2,1,$3,$4,$5,$6,$7)",
		eventID,
		rubricID,
		full,
		public,
		definition.PublicDigest(),
		owner.ID,
		now,
	)
	exec(
		"INSERT INTO rubric_approvals(event_id,rubric_id,version,actor_id,reason,approved_at) VALUES($1,$2,1,$3,'Retained approved definition',$4)",
		eventID,
		rubricID,
		owner.ID,
		now,
	)
	exec("UPDATE event_rubrics SET approved_version=1 WHERE event_id=$1", eventID)
	if err := audit.Append(
		ctx,
		dbgen.New(pool),
		audit.Record{
			ActorID:  &owner.ID,
			EventID:  &eventID,
			TargetID: eventID,
			Action:   audit.EventCreated,
			Reason:   audit.UserRequest,
		},
	); err != nil {
		t.Fatal(err)
	}
	tables := []string{
		"events",
		"hack_teams",
		"competition_entries",
		"competition_claims",
		"hack_team_members",
		"event_memberships",
		"mentor_assignments",
		"policy_resources",
		"help_requests",
		"help_transitions",
		"mentor_observation_versions",
		"event_rubrics",
		"rubric_versions",
		"rubric_approvals",
		"audit_events",
	}
	snapshot := func() map[string]string {
		t.Helper()
		result := map[string]string{}
		for _, table := range tables {
			column := "event_id"
			if table == "events" {
				column = "id"
			}
			query := fmt.Sprintf(
				"SELECT COALESCE(jsonb_agg(to_jsonb(t) ORDER BY to_jsonb(t)::text),'[]')::text FROM %s t WHERE %s=$1",
				table,
				column,
			)
			var value string
			if err := pool.QueryRow(ctx, query, eventID).Scan(&value); err != nil {
				t.Fatal(err)
			}
			result[table] = value
		}
		return result
	}
	before := snapshot()
	if err := database.Migrate(ctx, pool, protectedVersion); err != nil {
		t.Fatal(err)
	}
	after := snapshot()
	for table, value := range before {
		if value != after[table] {
			t.Fatal("normal32→33 upgrade changed retained source", table)
		}
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM mentor_scorecard_versions").
		Scan(&count); err != nil ||
		count != 0 {
		t.Fatal("upgrade invented independent scores", count, err)
	}
	db := stdlib.OpenDBFromPool(pool)
	defer func() { _ = db.Close() }()
	provider, err := goose.NewProvider(
		goose.DialectPostgres,
		db,
		os.DirFS("../database/migrations"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = provider.DownTo(ctx, predecessor); err != nil {
		t.Fatal("empty new schema could not roll back", err)
	}
	if err = database.Migrate(ctx, pool, protectedVersion); err != nil {
		t.Fatal(err)
	}
	rs := rubric.Service{Pool: pool, Clock: func() time.Time { return now }}
	configured, err := rs.Configuration(ctx, owner.ID, eventID, 0)
	if err != nil {
		t.Fatal(err)
	}
	configured, err = rs.Change(
		ctx,
		owner.ID,
		eventID,
		configured.Revision,
		rubric.Command{
			Action:  "create_run",
			Version: 1,
			Purpose: "checkpoint",
			Reason:  "Explicit retained new checkpoint",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	zone, _ := time.LoadLocation(config.Timezone)
	service := Service{Pool: pool, Now: func() time.Time { return now }}
	row, err := dbgen.New(pool).Event(ctx, eventID)
	if err != nil {
		t.Fatal(err)
	}
	cp, err := service.Configure(
		ctx,
		owner.ID,
		eventID,
		row.Revision,
		CheckpointCommand{
			Definition: CheckpointDefinition{
				ID:             uuid.New(),
				Name:           "Retained configured checkpoint",
				RunID:          configured.Runs[0].ID,
				OpensWallTime:  config.LiveStart.In(zone).Format("2006-01-02T15:04:05"),
				ClosesWallTime: config.SubmissionCutoff.In(zone).Format("2006-01-02T15:04:05"),
			},
			Reason: "Normal explicit new configuration",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	retained := snapshot()
	if _, err = provider.DownTo(ctx, predecessor); err == nil {
		t.Fatal("populated rollback discarded immutable configuration")
	}
	version, err := provider.GetDBVersion(ctx)
	if err != nil || version != protectedVersion {
		t.Fatal("failed rollback damaged protected schema", version, err)
	}
	afterRejected := snapshot()
	for table, value := range retained {
		if afterRejected[table] != value {
			t.Fatal("failed rollback changed retained source", table)
		}
	}
	stored, err := checkpoint(ctx, dbgen.New(pool), eventID, cp.Definition.ID)
	if err != nil || stored.Version != cp.Version {
		t.Fatal("failed rollback lost configured checkpoint", err)
	}
	if err = database.Migrate(ctx, pool, 0); err != nil {
		t.Fatal(err)
	}
	if err = database.CheckSchema(ctx, pool); err != nil {
		t.Fatal(err)
	}
}
