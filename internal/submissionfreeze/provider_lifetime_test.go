package submissionfreeze

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestHeldProviderDoesNotBlockOtherNativeTeams(t *testing.T) {
	pool := testdb.New(t, true)
	f := newFixture(t, pool)
	f.submit("repository")
	other := uuid.New()
	f.exec(
		"INSERT INTO hack_teams(id,event_id,name,state,frozen) VALUES($1,$2,'Another native team','published',true)",
		other,
		f.event,
	)
	f.exec("INSERT INTO competition_entries(event_id,team_id) VALUES($1,$2)", f.event, other)
	f.now = f.cfg.SubmissionCutoff
	f.provider.entered = make(chan struct{})
	f.provider.release = make(chan struct{})
	id := f.operation()
	operation, err := f.q.Operation(t.Context(), id)
	if err != nil || operation.TargetID != f.team {
		t.Fatal("held-provider fixture selected another team", operation.TargetID, err)
	}
	done := make(chan error, 1)
	go func() { done <- f.jobs.Execute(t.Context(), id) }()
	select {
	case <-f.provider.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("capture never reached provider")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 250*time.Millisecond)
	defer cancel()
	native := submissions.Service{Pool: pool, Clock: func() time.Time { return f.now }}
	p, err := native.Read(ctx, f.owner, f.event, other)
	close(f.provider.release)
	workerErr := <-done
	if err != nil {
		t.Fatalf("unrelated team blocked by external observation: %v", err)
	}
	if p.Outcome != "not_submitted" || workerErr != nil {
		t.Fatal(p, workerErr)
	}
}

func TestHeldObservationCannotPublishAfterAuthorityOrNativeBindingLoss(t *testing.T) {
	pool := testdb.New(t, true)
	for _, loss := range []string{"credential", "organiser", "final", "archive"} {
		t.Run(loss, func(t *testing.T) {
			f := newFixture(t, pool)
			f.submit("repository")
			f.now = f.cfg.SubmissionCutoff
			f.provider.entered = make(chan struct{})
			f.provider.release = make(chan struct{})
			id := f.operation()
			done := make(chan error, 1)
			go func() { done <- f.jobs.Execute(t.Context(), id) }()
			select {
			case <-f.provider.entered:
			case <-time.After(5 * time.Second):
				t.Fatal("provider was never reached")
			}
			switch loss {
			case "credential":
				f.exec(
					"UPDATE github_app_integrations SET credential_version=credential_version+1 WHERE event_id=$1",
					f.event,
				)
			case "organiser":
				f.exec(
					"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2",
					f.event,
					f.owner,
				)
			case "archive":
				f.exec(
					"UPDATE events SET stage='archived',writes_allowed=false WHERE id=$1",
					f.event,
				)
			case "final":
				s := native(*f.service)
				p, err := s.Read(t.Context(), f.owner, f.event, f.team)
				if err != nil {
					t.Fatal(err)
				}
				_, err = s.Submit(
					t.Context(),
					f.owner,
					f.event,
					f.team,
					p.Revision,
					submissions.Command{
						Reason: "Current native final replacement during lookup",
						Evidence: []submissions.Evidence{
							{
								DeliverableID: f.deliverable,
								Kind:          "repository",
								Label:         "Replacement",
								URL:           "https://example.test/replacement",
							},
						},
					},
				)
				if err != nil {
					t.Fatal(err)
				}
			}
			close(f.provider.release)
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			rows, err := pool.Query(
				t.Context(),
				"SELECT o.definition FROM submission_freeze_observations o JOIN submission_freeze_sources s ON s.id=o.source_id WHERE s.event_id=$1",
				f.event,
			)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			count := 0
			for rows.Next() {
				var encoded []byte
				if err = rows.Scan(&encoded); err != nil {
					t.Fatal(err)
				}
				var o Observation
				if err = json.Unmarshal(encoded, &o); err != nil {
					t.Fatal(err)
				}
				if o.SHA != "" || o.Status != "authority_or_configuration_lost" {
					t.Fatal("invalidated private response published", o)
				}
				count++
			}
			if err = rows.Err(); err != nil {
				t.Fatal(err)
			}
			if loss == "archive" && count != 0 {
				t.Fatal("archived competition mutated")
			}
			if loss != "archive" && count != 1 {
				t.Fatal("lost explicit denied attempt", count)
			}
		})
	}
}
