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
