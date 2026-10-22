package automation

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
)

func (f *fixture) retainedBinding() Binding {
	f.t.Helper()
	f.now = time.Now().UTC().Add(-70 * time.Second).Truncate(time.Second)
	f.exec("INSERT INTO event_gate_facts(event_id) VALUES($1) ON CONFLICT DO NOTHING", f.id)
	f.exec(
		"UPDATE events SET stage='completed',completed_at=$2 WHERE id=$1",
		f.id,
		f.now.Add(-time.Hour),
	)
	fact := uuid.New()
	f.factID = &fact
	f.exec(
		"INSERT INTO communication_facts(event_id,id,kind,revision,owner_id,status) VALUES($1,$2,'outcome_review',1,$3,'open')",
		f.id,
		fact,
		f.person.ID,
	)
	f.campaign("outcome_review", "Your follow-up is {{outcome_review.status}}", true)
	until := f.now.Add(24 * time.Hour)
	definition := Definition{
		ID:                     uuid.New(),
		CampaignID:             f.campaignID,
		CampaignVersion:        f.campaignVersion,
		Trigger:                "outcome.follow_up",
		SourceID:               &fact,
		SourceVersion:          1,
		MaximumLatenessSeconds: 60,
		RetainAfterArchive:     true,
		RetainUntil:            &until,
	}
	page, _ := f.change(
		Command{
			Action:     "save",
			Definition: &definition,
			Reason:     "Configure retained outcome follow-up",
		},
	)
	page, _ = f.change(
		Command{
			Action:    "approve",
			BindingID: definition.ID,
			Version:   page.Bindings[0].Version,
			Reason:    "Explicitly approve retention-bound follow-up",
		},
	)
	return page.Bindings[0]
}

func TestArchiveCancelsOperationalWorkButOnlyValidatedRetainedFollowupSurvives(t *testing.T) {
	for _, closed := range []bool{false, true} {
		t.Run(
			map[bool]string{false: "retained", true: "source_resolved"}[closed],
			func(t *testing.T) {
				f := newFixture(t)
				f.retainedBinding()
				f.now = f.now.Add(70 * time.Second)
				page := f.maintain()
				if len(page.Occurrences) != 1 {
					t.Fatal("retained occurrence missing")
				}
				id := page.Occurrences[0].ID
				var ordinary dbgen.Operation
				if err := database.Within(
					t.Context(),
					f.pool,
					func(tx pgx.Tx, _ *dbgen.Queries) error {
						var e error
						ordinary, e = f.s.Jobs.Enqueue(
							t.Context(),
							tx,
							dbgen.InsertOperationParams{
								EventID:      f.id,
								OperationKey: "synthetic-ordinary",
								Kind:         "ordinary",
								TargetID:     uuid.New(),
								DueAt:        time.Now().Add(-time.Second),
								Approved:     true,
							},
						)
						return e
					},
				); err != nil {
					t.Fatal(err)
				}
				if _, err := (event.Service{Pool: f.pool}).Transition(
					t.Context(),
					f.owner.ID,
					f.id,
					f.revision(),
					"archived",
					"Archive operational work",
					false,
				); err != nil {
					t.Fatal(err)
				}
				old, err := dbgen.New(f.pool).Operation(t.Context(), ordinary.ID)
				if err != nil || !old.Cancelled {
					t.Fatal("ordinary operation survived archive", err)
				}
				if closed {
					f.exec(
						"UPDATE communication_facts SET status='resolved',revision=revision+1 WHERE event_id=$1 AND id=$2",
						f.id,
						*f.factID,
					)
				}
				f.execute(id)
				expected := 1
				if closed {
					expected = 0
				}
				if f.count() != expected {
					t.Fatalf(
						"retained current source guard: effects%d, want%d",
						f.count(),
						expected,
					)
				}
				// Ordinary communications APIs retain their existing read-only denial.
				if err := database.Within(
					t.Context(),
					f.pool,
					func(tx pgx.Tx, _ *dbgen.Queries) error {
						_, e := f.s.Composer.ResolveApprovedCampaignInTransaction(
							t.Context(),
							tx,
							f.id,
							f.campaignID,
						)
						return e
					},
				); err == nil {
					t.Fatal("ordinary resolver bypassed archive")
				}
			},
		)
	}
}

func TestRetainedDeadlineCannotExceedEventPrivacyPolicy(t *testing.T) {
	f := newFixture(t)
	binding := f.retainedBinding()
	definition := binding.Definition
	tooLate := f.now.Add(200 * 24 * time.Hour)
	definition.ID = uuid.New()
	definition.RetainUntil = &tooLate
	page, _ := f.change(
		Command{
			Action:     "save",
			Definition: &definition,
			Reason:     "Review an excessive retention request",
		},
	)
	var version int64
	for _, b := range page.Bindings {
		if b.Definition.ID == definition.ID {
			version = b.Version
		}
	}
	if _, _, err := f.s.Change(
		t.Context(),
		f.owner.ID,
		f.id,
		f.revision(),
		Command{
			Action:    "approve",
			BindingID: definition.ID,
			Version:   version,
			Reason:    "Excessive retention must be rejected",
		},
	); err == nil {
		t.Fatal("approved follow-up beyond event retention deadline")
	}
}
