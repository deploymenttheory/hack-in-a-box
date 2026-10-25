package smtpintegration

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/deliveryhistory"
	"github.com/deploymenttheory/hack-in-a-box/internal/inbox"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestHistoryUsesActualSelectionKeysetsOmissionsAndInboxPersistenceWithoutPrivatePayload(
	t *testing.T,
) {
	f := setup(t)
	unverified := testdb.Person(t, f.pool)
	f.exec(
		"INSERT INTO event_memberships(event_id,person_id,role,active) VALUES($1,$2,'organiser',true)",
		f.event,
		unverified.ID,
	)
	f.approveProduction()
	_ = f.queueProduction()
	history := deliveryhistory.Service{
		Pool:      f.pool,
		Jobs:      f.worker,
		Providers: map[string]deliveryhistory.RecoveryProvider{"smtp": f.service},
	}
	first, err := history.Read(t.Context(), f.owner.ID, f.event, deliveryhistory.Filter{Limit: 1})
	if err != nil || len(first.Items) != 1 || first.NextCursor == nil {
		t.Fatalf("first actual selection %+v err%v", first, err)
	}
	second, err := history.Read(
		t.Context(),
		f.owner.ID,
		f.event,
		deliveryhistory.Filter{Limit: 1, Before: first.NextCursor},
	)
	if err != nil || len(second.Items) != 1 || second.NextCursor != nil ||
		first.Items[0].ID == second.Items[0].ID {
		t.Fatalf("continued selections %+v err%v", second, err)
	}
	states := map[string]bool{first.Items[0].State: true, second.Items[0].State: true}
	if !states["never_attempted"] || !states["unavailable"] {
		t.Fatal("never-attempted intent and unverified omission were conflated")
	}
	for _, page := range []deliveryhistory.Page{first, second} {
		wire, _ := json.Marshal(page)
		if strings.Contains(string(wire), "organiser@example.test") ||
			strings.Contains(string(wire), "plain_text") ||
			strings.Contains(string(wire), "snapshot") {
			t.Fatal("history included private SMTP payload")
		}
		for _, item := range page.Items {
			if item.State == "unavailable" {
				detail, err := history.Detail(t.Context(), f.owner.ID, f.event, item.ID, 0, nil)
				if err != nil || len(detail.Actions) != 0 || len(detail.Attempts) != 0 ||
					detail.Item.Attempted {
					t.Fatal("omission invented an attempt/action", err)
				}
			}
		}
	}
	foreign := uuid.New()
	filtered, err := history.Read(
		t.Context(),
		f.owner.ID,
		f.event,
		deliveryhistory.Filter{Limit: 1, CampaignID: &foreign},
	)
	if err != nil || len(filtered.Items) != 0 || filtered.NextCursor != nil {
		t.Fatal("campaign filter happened after page bounds", err)
	}
	filtered, err = history.Read(
		t.Context(),
		f.owner.ID,
		f.event,
		deliveryhistory.Filter{Limit: 1, Channel: "teams"},
	)
	if err != nil || len(filtered.Items) != 0 || filtered.NextCursor != nil {
		t.Fatal("channel filter happened after page bounds", err)
	}
	local := inbox.Service{Pool: f.pool, Communications: f.comm}
	f.comm.Channels["in_app"] = local
	definition := f.campaign.Definition
	definition.ID = uuid.New()
	definition.Channels = []string{"in_app"}
	definition.IncludeCalendar = false
	wall, choice := testdb.WallTimeForInstant(t, f.now.Add(time.Minute), "Europe/London")
	definition.Timing = communications.CampaignTiming{
		Kind:      "fixed",
		WallTime:  wall,
		DSTChoice: choice,
	}
	var campaign communications.Campaign
	for _, action := range []string{"save", "preview", "approve"} {
		input := communications.CampaignCommand{
			Action:  action,
			Version: campaign.Version,
			Reason:  "Synthetic local inbox source",
		}
		if action == "save" {
			input.Definition = &definition
		}
		detail, err := f.comm.ChangeCampaign(
			t.Context(),
			f.owner.ID,
			f.event,
			definition.ID,
			f.revision(),
			input,
		)
		if err != nil {
			t.Fatal(err)
		}
		campaign = detail.Campaign.Campaign
	}
	f.now = campaign.Approval.DueAt
	err = database.Within(t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		_, err := local.QueueApprovedInTransaction(
			t.Context(),
			tx,
			f.event,
			definition.ID,
			campaign.Version,
		)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := history.Read(
		t.Context(),
		f.owner.ID,
		f.event,
		deliveryhistory.Filter{Limit: 10, Channel: "in_app", CampaignID: &definition.ID},
	)
	if err != nil || len(persisted.Items) != 2 {
		t.Fatalf("actual inbox selections %+v err%v", persisted, err)
	}
	for _, item := range persisted.Items {
		if item.State != "local_persisted" || item.Attempted || item.Attempts != 0 ||
			item.CampaignVersion != campaign.Version {
			t.Fatal("local inbox falsely claimed external acceptance", item)
		}
	}
	if f.transport.count() != 0 {
		t.Fatal("reading/persisting inbox caused SMTP provider effect")
	}
}
