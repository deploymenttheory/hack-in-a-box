package automation

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/help"
)

func TestNativeHelpSourceUsesActualTransitionsAndCurrentAudience(t *testing.T) {
	f := newFixture(t)
	team := uuid.New()
	f.exec(
		`INSERT INTO hack_teams(event_id,id,name,state) VALUES($1,$2,'Synthetic team','published')`,
		f.id,
		team,
	)
	f.exec(
		`INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot) VALUES($1,$2,$3,'published')`,
		f.id,
		team,
		f.person.ID,
	)
	native := help.Service{Pool: f.pool, Clock: func() time.Time { return f.now }}
	request, err := native.Create(
		t.Context(),
		f.person.ID,
		f.id,
		uuid.New(),
		help.Input{
			TeamID:      team,
			Category:    "setup",
			Description: "PRIVATE-HELP-PROSE-CANARY",
			Location:    "PRIVATE-LOCATION-CANARY",
			CommandID:   uuid.New(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	adapter := HelpSource{Help: native}
	var source Source
	err = database.Within(t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		items, e := adapter.DiscoverInTransaction(
			t.Context(),
			tx,
			f.id,
			Definition{SourceID: &request.Request.ID},
		)
		if e != nil {
			return e
		}
		if len(items) != 1 || items[0].ID != request.Request.ID ||
			items[0].Revision != request.Request.Revision {
			return fmt.Errorf("not the actual native transition: %+v", items)
		}
		source = items[0]
		audience, e := adapter.ResolveInTransaction(t.Context(), tx, f.id, source)
		if e != nil {
			return e
		}
		if audience.Broadcast || !slices.Contains(audience.RecipientIDs, f.person.ID) {
			return fmt.Errorf("native private audience missing: %+v", audience)
		}
		raw, _ := json.Marshal(struct {
			Source   Source
			Audience SourceAudience
		}{source, audience})
		if strings.Contains(string(raw), "CANARY") {
			return fmt.Errorf("private source text entered routing")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(16 * time.Minute)
	f.s.SourceMaintenance = native.MaintainInTransaction
	if err = f.s.reconcileEvent(t.Context(), f.id); err != nil {
		t.Fatal(err)
	}
	err = database.Within(t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		if _, e := adapter.ResolveInTransaction(t.Context(), tx, f.id, source); e == nil {
			return fmt.Errorf("old native revision retained dispatch authority")
		}
		items, e := adapter.DiscoverInTransaction(
			t.Context(),
			tx,
			f.id,
			Definition{SourceID: &request.Request.ID},
		)
		if e != nil {
			return e
		}
		if len(items) != 1 || items[0].Revision != source.Revision+1 {
			return fmt.Errorf("actual overdue transition not discovered: %+v", items)
		}
		source = items[0]
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	f.exec(
		`UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2`,
		f.id,
		f.person.ID,
	)
	err = database.Within(t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		audience, e := adapter.ResolveInTransaction(t.Context(), tx, f.id, source)
		if e == nil && slices.Contains(audience.RecipientIDs, f.person.ID) {
			return fmt.Errorf("withdrawn requester retained native routing authority")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestNativeHelpOccurrenceQueuesApprovedLocalMessageOnce(t *testing.T) {
	f := newFixture(t)
	_, relay := installProviders(t, f)
	native := help.Service{Pool: f.pool, Clock: func() time.Time { return f.now }}
	f.s.Sources = map[string]SourcePolicy{"help.updated": HelpSource{Help: native}}
	f.s.SourceMaintenance = native.MaintainInTransaction
	team, requestID := uuid.New(), uuid.New()
	f.exec(
		`INSERT INTO hack_teams(event_id,id,name,state) VALUES($1,$2,'Synthetic team','published')`,
		f.id,
		team,
	)
	f.exec(
		`INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot) VALUES($1,$2,$3,'published')`,
		f.id,
		team,
		f.person.ID,
	)
	f.campaign("reminder", "Safe reviewed help update", true, "in_app")
	page, _ := f.change(
		Command{
			Action: "save",
			Definition: &Definition{
				ID:                     uuid.New(),
				CampaignID:             f.campaignID,
				CampaignVersion:        f.campaignVersion,
				Trigger:                "help.updated",
				SourceID:               &requestID,
				MaximumLatenessSeconds: 3600,
			},
			Reason: "Review native help journey",
		},
	)
	binding := page.Bindings[0]
	f.change(
		Command{
			Action:    "approve",
			BindingID: binding.Definition.ID,
			Version:   binding.Version,
			Reason:    "Approve exact current message and native source",
		},
	)
	f.now = f.now.Add(time.Minute)
	if _, err := native.Create(
		t.Context(),
		f.person.ID,
		f.id,
		requestID,
		help.Input{
			TeamID:      team,
			Category:    "setup",
			Description: "PRIVATE-QUESTION-NOT-A-MESSAGE",
			Location:    "PRIVATE-LOCATION",
			CommandID:   uuid.New(),
		},
	); err != nil {
		t.Fatal(err)
	}
	page = f.maintain()
	if len(page.Occurrences) != 1 || page.Occurrences[0].SourceID != requestID {
		t.Fatalf("actual help transition not retained: %+v", page)
	}
	f.execute(page.Occurrences[0].ID)
	f.maintain()
	f.execute(page.Occurrences[0].ID)
	var count int
	var subject, body string
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*),min(subject),min(body) FROM inbox_notifications WHERE event_id=$1 AND source_kind='campaign'`, f.id).
		Scan(&count, &subject, &body); err != nil {
		t.Fatal(err)
	}
	if count != 1 || strings.Contains(subject+body, "PRIVATE") || relay.calls.Load() != 0 {
		t.Fatalf(
			"native approved update duplicated/leaked/sent externally: count%d %q %q calls%d",
			count,
			subject,
			body,
			relay.calls.Load(),
		)
	}
}

func TestNativeHelpUnknownSweepPassesKnownPagesWithoutLifetimeCap(t *testing.T) {
	f := newFixture(t)
	native := help.Service{Pool: f.pool, Clock: func() time.Time { return f.now }}
	adapter := HelpSource{Help: native}
	f.s.Sources = map[string]SourcePolicy{"help.updated": adapter}
	team := uuid.New()
	f.exec(
		`INSERT INTO hack_teams(event_id,id,name,state)VALUES($1,$2,'Synthetic team','published')`,
		f.id,
		team,
	)
	f.exec(
		`INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot)VALUES($1,$2,$3,'published')`,
		f.id,
		team,
		f.person.ID,
	)
	f.campaign("reminder", "Reviewed help notification", true)
	page, _ := f.change(
		Command{
			Action: "save",
			Definition: &Definition{
				ID:                     uuid.New(),
				CampaignID:             f.campaignID,
				CampaignVersion:        f.campaignVersion,
				Trigger:                "help.updated",
				MaximumLatenessSeconds: 3600,
			},
			Reason: "Review native help subscription",
		},
	)
	page, _ = f.change(
		Command{
			Action:    "approve",
			BindingID: page.Bindings[0].Definition.ID,
			Version:   page.Bindings[0].Version,
			Reason:    "Approve native help subscription",
		},
	)
	binding := page.Bindings[0]
	f.exec(
		`INSERT INTO policy_resources(event_id,id,kind)SELECT $1,gen_random_uuid(),'help' FROM generate_series(1,1005)`,
		f.id,
	)
	f.exec(
		`INSERT INTO help_requests(event_id,id,team_id,requester_id,revision,category,description,location,meeting_url,status,waiting_cycle,requested_at,waiting_since,updated_at)SELECT event_id,id,$2,$3,1,'setup','PRIVATE-QUESTION','','','waiting',1,$4,$4,$4 FROM policy_resources WHERE event_id=$1 AND kind='help'`,
		f.id,
		team,
		f.person.ID,
		f.now,
	)
	f.exec(
		`INSERT INTO help_transitions(event_id,request_id,revision,action,status,occurred_at)SELECT event_id,id,1,'create','waiting',requested_at FROM help_requests WHERE event_id=$1`,
		f.id,
	)
	seen := map[uuid.UUID]bool{}
	for pass := 0; pass < 7; pass++ {
		err := database.Within(t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
			items, e := adapter.DiscoverUnknownInTransaction(t.Context(), tx, f.id, binding)
			if e != nil {
				return e
			}
			want := min(200, 1005-len(seen))
			if len(items) != want {
				return fmt.Errorf("pass%d saw%d unknowns want%d", pass, len(items), want)
			}
			for _, item := range items {
				if seen[item.ID] {
					return fmt.Errorf("repeated native source %s", item.ID)
				}
				seen[item.ID] = true
				_, e = tx.Exec(
					t.Context(),
					`INSERT INTO communication_occurrences(event_id,id,binding_id,binding_version,source_kind,source_id,source_revision,source_key,due_at,deadline,scope_binding,state)VALUES($1,$2,$3,$4,'help.updated',$5,$6,$7,$8,$8,'known','cancelled')`,
					f.id,
					uuid.New(),
					binding.Definition.ID,
					binding.Version,
					item.ID,
					item.Revision,
					sourceKey(item),
					f.now,
				)
				if e != nil {
					return e
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(seen) != 1005 {
		t.Fatalf("native discovery lost sources: %d", len(seen))
	}
}
