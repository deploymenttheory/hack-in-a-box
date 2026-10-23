package automation

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/credentials"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/secretbox"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestApprovedMultiChannelAutomationCannotProjectNativePrivateCardsCredentialsOrOtherMailboxes(
	t *testing.T,
) {
	f := newFixture(t)
	f.now = f.now.AddDate(0, 0, -30)
	smtp, _ := installProviders(t, f)
	relay := &capturingAcceptedRelay{}
	smtp.Transport = relay
	installTeamsProvider(t, f) // Queue/resolve only; the separate TLS fault proof sends locally.
	rubrics, source, challenge, team := prepareNativeCommonChallenge(t, f)
	f.now = f.now.Add(time.Minute)
	if err := source.Dispatch(t.Context(), f.id, challenge.ID, challenge.Revision); err != nil {
		t.Fatal(err)
	}
	seedPriorScorecard(t, f, rubrics, team) // Native configured live run and private card evidence.
	keys, err := secretbox.Parse(
		`{"active":"synthetic","keys":{"synthetic":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="}}`,
	)
	if err != nil {
		t.Fatal(err)
	}
	resource := credentials.Service{
		Pool: f.pool,
		Keys: keys,
		Now:  func() time.Time { return f.now },
	}
	if _, err = resource.Change(
		t.Context(),
		f.owner.ID,
		f.id,
		f.revision(),
		credentials.Command{
			Action:  "create",
			Label:   "Synthetic private team service",
			Value:   "PRIVATE-CREDENTIAL-VALUE-CANARY",
			TeamIDs: []uuid.UUID{team},
		},
	); err != nil {
		t.Fatal(err)
	}
	q := dbgen.New(f.pool)
	if err = q.ConfirmPersonEmail(
		t.Context(),
		dbgen.ConfirmPersonEmailParams{
			PersonID: f.person.ID,
			Mailbox:  "intended-only@example.test",
		},
	); err != nil {
		t.Fatal(err)
	}
	outsider := testdb.Person(t, f.pool)
	if err = q.ConfirmPersonEmail(
		t.Context(),
		dbgen.ConfirmPersonEmailParams{
			PersonID: outsider.ID,
			Mailbox:  "excluded-private@example.test",
		},
	); err != nil {
		t.Fatal(err)
	}
	var cards, encrypted int
	if err = f.pool.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM mentor_scorecard_versions WHERE event_id=$1),(SELECT count(*) FROM team_credentials WHERE event_id=$1)`, f.id).
		Scan(&cards, &encrypted); err != nil ||
		cards < 1 ||
		encrypted != 1 {
		t.Fatal("native confidential source fixture missing", cards, encrypted, err)
	}
	f.campaign("reminder", "Approved common operational reminder.", true, "smtp", "in_app", "teams")
	f.bind()
	id := f.maintain().Occurrences[0].ID
	f.now = f.now.Add(time.Minute)
	f.execute(id)
	f.execute(id)
	var broadcast communications.BroadcastMessage
	err = database.Within(t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		var e error
		broadcast, e = f.s.Composer.ResolveApprovedScopedBroadcastInTransaction(
			t.Context(),
			tx,
			f.id,
			f.campaignID,
			f.campaignVersion,
			communications.DeliveryScope{OccurrenceID: id},
			f.s,
		)
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	var op uuid.UUID
	if err = f.pool.QueryRow(t.Context(), "SELECT operation_id FROM smtp_campaign_messages WHERE occurrence_id=$1", id).
		Scan(&op); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err = f.s.Jobs.Execute(t.Context(), op); err != nil {
			t.Fatal(err)
		}
	}
	messages := relay.snapshot()
	if len(messages) != 1 || messages[0].Recipient != "intended-only@example.test" {
		t.Fatal("automation sent extra or cross-person SMTP envelope")
	}
	var local []string
	rows, err := f.pool.Query(
		t.Context(),
		"SELECT subject||body FROM inbox_notifications WHERE source_kind='campaign'",
	)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var text string
		if err = rows.Scan(&text); err != nil {
			t.Fatal(err)
		}
		local = append(local, text)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		t.Fatal(err)
	}
	if len(local) != 2 {
		t.Fatal("current confirmed local audience differs from native source")
	}
	public, _ := json.Marshal(struct {
		Broadcast     communications.BroadcastMessage
		Local         []string
		Subject, Body string
	}{broadcast, local, messages[0].Subject, messages[0].PlainText})
	for _, private := range []string{"PRIVATE-CARD-MIGRATION-CANARY", "PRIVATE-JUDGE-GUIDANCE-CANARY", "PRIVATE-CREDENTIAL-VALUE-CANARY", "excluded-private@example.test", "intended-only@example.test"} {
		if strings.Contains(string(public), private) {
			t.Fatal("private native state entered common communication projection")
		}
	}
	var broadcasts, receipts int
	if err = f.pool.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM teams_workflow_deliveries WHERE occurrence_id=$1),(SELECT count(*) FROM teams_workflow_receipts)`, id).
		Scan(&broadcasts, &receipts); err != nil ||
		broadcasts != 1 ||
		receipts != 0 {
		t.Fatal("queue/resolve invented shared-channel network evidence", broadcasts, receipts, err)
	}
}
