package automation

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

// This verifies the orchestration transaction using a local recording adapter.
// Production SMTP mailbox/omission evidence is separately owned by its adapter.
func TestUnavailableReportsCommitEvidenceAndKeepPartialIntentsEligible(t *testing.T) {
	for _, partial := range []bool{false, true} {
		t.Run(
			map[bool]string{false: "all_unavailable", true: "partially_queued"}[partial],
			func(t *testing.T) {
				f := newFixture(t)
				other := testdb.Person(t, f.pool)
				f.exec("INSERT INTO competition_claims VALUES($1,$2,'confirmed')", f.id, other.ID)
				f.exec(
					"CREATE TABLE synthetic_provider_evidence(occurrence_id uuid,person_id uuid,kind text,PRIMARY KEY(occurrence_id,person_id))",
				)
				f.campaign("reminder", "Current audience reminder", true)
				f.bind()
				p := f.maintain()
				id := p.Occurrences[0].ID
				f.s.Providers["smtp"] = func(ctx context.Context, tx pgx.Tx, eventID, campaignID uuid.UUID, version int64, scope communications.DeliveryScope) (ChannelReport, error) {
					approved, err := f.s.Composer.ResolveApprovedScopedCampaignInTransaction(
						ctx,
						tx,
						eventID,
						campaignID,
						version,
						scope,
						f.s,
					)
					if err != nil {
						return ChannelReport{}, err
					}
					report := ChannelReport{}
					for _, recipient := range approved.Preview.Recipients {
						kind := "address_unverified"
						if partial && recipient.PersonID == f.person.ID {
							kind = "queued"
							report.Queued++
						} else {
							report.Unavailable++
						}
						if _, err = tx.Exec(
							ctx,
							"INSERT INTO synthetic_provider_evidence VALUES($1,$2,$3)",
							scope.OccurrenceID,
							recipient.PersonID,
							kind,
						); err != nil {
							return ChannelReport{}, err
						}
					}
					return report, nil
				}
				f.now = f.now.Add(time.Minute)
				f.execute(id)
				page, err := f.s.Read(t.Context(), f.owner.ID, f.id)
				if err != nil {
					t.Fatal(err)
				}
				occurrence := page.Occurrences[0]
				expected := "blocked"
				if partial {
					expected = "partially_queued"
				}
				if occurrence.State != expected {
					t.Fatalf("state=%s want %s", occurrence.State, expected)
				}
				report := occurrence.ChannelReports["smtp"]
				if report.Queued+report.Unavailable != 2 || report.LocalPersisted != 0 {
					t.Fatalf("untruthful report: %+v", report)
				}
				var evidence, queued int
				if err = f.pool.QueryRow(t.Context(), "SELECT count(*),count(*) FILTER(WHERE kind='queued') FROM synthetic_provider_evidence WHERE occurrence_id=$1", id).
					Scan(&evidence, &queued); err != nil {
					t.Fatal(err)
				}
				if evidence != 2 || queued != report.Queued {
					t.Fatalf(
						"evidence rolled back or invented intents: evidence=%d queued=%d",
						evidence,
						queued,
					)
				}
				if partial {
					err = database.Within(
						t.Context(),
						f.pool,
						func(tx pgx.Tx, _ *dbgen.Queries) error {
							_, resolveErr := f.s.ResolveInTransaction(
								t.Context(),
								tx,
								f.id,
								f.campaignID,
								f.campaignVersion,
								communications.DeliveryScope{OccurrenceID: id},
							)
							return resolveErr
						},
					)
					if err != nil {
						t.Fatalf("partial report stranded valid provider intent: %v", err)
					}
				}
			},
		)
	}
}
