package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/outcomes"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestOutcomesHTTPOrdinaryMemberPrivateHistoryAndReviewedPublication(t *testing.T) {
	pool := testdb.New(t, true)
	f := newEmbargoFixture(t, pool, time.Now().UTC().AddDate(0, 0, 20))
	if err := database.Migrate(t.Context(), pool, 0); err != nil {
		t.Fatal(err)
	}
	native := &outcomes.Service{Pool: pool, Now: func() time.Time { return f.now }}
	f.router = NewRouter(
		f.cfg,
		slog.New(slog.NewJSONHandler(&f.logs, nil)),
		Dependencies{Pool: pool, Auth: f.auth, Outcomes: native, Events: event.Service{Pool: pool}},
	)
	ordinary := f.stranger
	id := uuid.New()
	path := "/api/v1/outcomes/" + id.String()
	create := outcomes.Command{
		ID:     uuid.New(),
		Action: "propose",
		Reason: "Ordinary organisation member proposes an idea",
		Definition: &outcomes.Definition{
			Kind:          "pitch",
			Problem:       "PRIVATE PITCH <script>literal</script>",
			Audience:      "People who experience the problem",
			ProposedValue: "Estimated benefit",
		},
	}
	f.expect(f.request(dbgen.Person{}, "PUT", path, create, 0), 401)

	f.expect(f.request(ordinary, "PUT", path, create, 0), 200)
	f.expect(f.request(ordinary, "GET", f.base(), nil, 0), 403)
	f.expect(f.request(ordinary, "GET", path+"/history", nil, 0), 200)
	receipt := f.request(ordinary, "GET", path+"?command_id="+create.ID.String(), nil, 0)
	f.expect(receipt, 200)
	validateResponse(
		t,
		httptest.NewRequest("GET", path+"?command_id="+create.ID.String(), nil),
		receipt,
	)
	f.expect(f.request(ordinary, "GET", path+"?command_id="+uuid.NewString(), nil, 0), 404)
	f.expect(f.request(ordinary, "GET", path+"/owners", nil, 0), 403)
	f.expect(f.request(f.members[0][0], "GET", path, nil, 0), 404)
	own := f.request(ordinary, "GET", "/api/v1/outcomes?mode=own", nil, 0)
	f.expect(own, 200)
	if strings.Contains(own.Body.String(), "Embargo verification") ||
		strings.Contains(own.Body.String(), "live_start") {
		t.Fatal("ordinary pitch endpoint leaked draft-event context", own.Body.String())
	}
	invalid := map[string]any{
		"id":     uuid.New(),
		"action": "evidence",
		"reason": "Attempt fabricated achieved financial claim",
		"evidence": map[string]any{
			"id":           uuid.New(),
			"basis":        "estimate",
			"claim":        "Bad field",
			"achieved_roi": "900%",
		},
	}
	f.expect(f.request(ordinary, "PUT", path, invalid, 1), 400)
	adopt := outcomes.Command{
		ID:         uuid.New(),
		Action:     "adopt",
		Reason:     "Deliberate authorised event adoption",
		Provenance: &outcomes.Provenance{EventID: &f.id},
	}
	f.expect(f.request(ordinary, "PUT", path, adopt, 1), 403)
	f.expect(f.request(f.owner, "PUT", path, adopt, 1), 200)
	f.expect(f.request(ordinary, "GET", f.base(), nil, 0), 403)
	f.expect(f.request(ordinary, "GET", f.base()+"/outcomes/observations", nil, 0), 403)
	f.expect(f.request(f.owner, "GET", f.base()+"/outcomes/observations", nil, 0), 200)
	fields := outcomes.PublicationFields{
		Problem:  "Reviewed public problem",
		Solution: "Reviewed public solution",
		Audience: "organisation",
		Links:    []outcomes.ProjectLink{},
		Media:    []outcomes.MediaReference{},
	}
	save := outcomes.PublicationCommand{
		ID:            uuid.New(),
		Action:        "save",
		SourceVersion: 2,
		Fields:        &fields,
		Reason:        "Review public projection independently",
	}
	f.expect(f.request(ordinary, "PUT", path+"/publication", save, 0), 403)
	f.expect(f.request(f.owner, "PUT", path+"/publication", save, 0), 200)
	f.expect(f.request(ordinary, "GET", "/api/v1/projects/"+id.String(), nil, 0), 404)
	publish := outcomes.PublicationCommand{
		ID:            uuid.New(),
		Action:        "publish",
		SourceVersion: 2,
		Reason:        "Explicit reviewed publication",
	}
	f.expect(f.request(f.owner, "PUT", path+"/publication", publish, 1), 200)
	public := f.request(f.members[0][0], "GET", "/api/v1/projects?search=public&limit=1", nil, 0)
	f.expect(public, 200)
	validateResponse(
		t,
		httptest.NewRequest("GET", "/api/v1/projects?search=public&limit=1", nil),
		public,
	)
	if strings.Contains(public.Body.String(), "PRIVATE PITCH") ||
		strings.Contains(public.Body.String(), "Embargo verification") ||
		strings.Contains(public.Body.String(), "owner_id") {
		t.Fatal("private content leaked in searchable project", public.Body.String())
	}
	var page outcomes.ProjectPage
	if err := json.Unmarshal(public.Body.Bytes(), &page); err != nil || len(page.Items) != 1 {
		t.Fatal(page, err)
	}
	f.expect(
		f.request(
			f.owner,
			"PUT",
			path+"/publication",
			outcomes.PublicationCommand{
				ID:            uuid.New(),
				Action:        "withdraw",
				SourceVersion: 2,
				Reason:        "Withdraw separately",
			},
			2,
		),
		200,
	)
	f.expect(f.request(f.owner, "PUT", path+"/publication", publish, 1), 200)
	f.expect(f.request(ordinary, "GET", "/api/v1/projects/"+id.String(), nil, 0), 404)
	f.exec("UPDATE people SET active=false WHERE id=$1", ordinary.ID)
	f.expect(f.request(ordinary, "GET", path, nil, 0), 401)
	f.expect(f.request(ordinary, "PUT", path, create, 0), 401)
	if strings.Contains(f.logs.String(), "PRIVATE PITCH") ||
		strings.Contains(f.logs.String(), "Estimated benefit") {
		t.Fatal("private outcome body in logs")
	}
}
