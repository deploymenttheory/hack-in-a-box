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

// Exercise the HTTP adapter, not only the service: both UUID arguments have the
// same Go type, but swapping the event filter and cursor changes authority.
func TestOutcomesHTTPPaginationAndEventReviewScope(t *testing.T) {
	pool := testdb.New(t, true)
	f := newEmbargoFixture(t, pool, time.Now().UTC().AddDate(0, 0, 20))
	if err := database.Migrate(t.Context(), pool, 0); err != nil {
		t.Fatal(err)
	}
	native := &outcomes.Service{Pool: pool, Now: func() time.Time { return f.now }}
	f.router = NewRouter(f.cfg, slog.New(slog.NewJSONHandler(&f.logs, nil)), Dependencies{Pool: pool, Auth: f.auth, Outcomes: native, Events: event.Service{Pool: pool}})
	otherOwner := testdb.Person(t, pool)
	otherEvent := testdb.Event(t, pool, otherOwner.ID)
	ids := []uuid.UUID{
		uuid.MustParse("10000000-0000-4000-8000-000000000001"),
		uuid.MustParse("10000000-0000-4000-8000-000000000002"),
		uuid.MustParse("10000000-0000-4000-8000-000000000003"),
		uuid.MustParse("10000000-0000-4000-8000-000000000004"),
		uuid.MustParse("10000000-0000-4000-8000-000000000005"),
	}
	for i, id := range ids {
		c := outcomes.Command{ID: uuid.New(), Action: "propose", Reason: "Synthetic pagination regression", Definition: &outcomes.Definition{Kind: "pitch", Problem: "PRIVATE scoped outcome " + id.String(), Audience: "Synthetic people", ProposedValue: "Proposed benefit only"}}
		path := "/api/v1/outcomes/" + id.String()
		f.expect(f.request(f.stranger, "PUT", path, c, 0), 200)
		if i < 3 {
			eventID, reviewer := f.id, f.owner
			if i == 2 {
				eventID, reviewer = otherEvent, otherOwner
			}
			adopt := outcomes.Command{ID: uuid.New(), Action: "adopt", Reason: "Associate exact synthetic source", Provenance: &outcomes.Provenance{EventID: &eventID}}
			f.expect(f.request(reviewer, "PUT", path, adopt, 1), 200)
		}
	}
	assertPages := func(t *testing.T, actor dbgen.Person, query string, want []uuid.UUID) {
		t.Helper()
		var got []uuid.UUID
		cursor := ""
		for n := 0; n < 4; n++ {
			path := "/api/v1/outcomes?limit=2" + query
			if cursor != "" {
				path += "&cursor=" + cursor
			}
			response := f.request(actor, "GET", path, nil, 0)
			if response.Code != 200 {
				t.Fatalf("page %d: want 200 got %d: %s", n, response.Code, response.Body)
			}
			var page outcomes.Page
			if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
				t.Fatal(err)
			}
			for _, item := range page.Items {
				got = append(got, item.ID)
			}
			if page.NextCursor == nil {
				break
			}
			next := page.NextCursor.String()
			if next == cursor {
				t.Fatalf("cursor repeated on page %d: %s", n, next)
			}
			cursor = next
		}
		if len(got) != len(want) {
			t.Fatalf("want %v got %v", want, got)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("want ordered distinct %v got %v", want, got)
			}
		}
	}
	t.Run("own explicit and omitted mode paginate without repeats", func(t *testing.T) {
		assertPages(t, f.stranger, "&mode=own", ids)
		assertPages(t, f.stranger, "", ids)
	})
	t.Run("review filters exact event and preserves cursor", func(t *testing.T) {
		assertPages(t, f.owner, "&mode=review&event_id="+f.id.String(), []uuid.UUID{ids[0], ids[1], ids[3], ids[4]})
	})
	t.Run("wrong event and non organiser disclose no private outcomes", func(t *testing.T) {
		for _, actor := range []dbgen.Person{f.owner, f.mentor, f.stranger} {
			eventID := f.id
			if actor.ID == f.owner.ID {
				eventID = otherEvent
			}
			response := f.request(actor, "GET", "/api/v1/outcomes?mode=review&event_id="+eventID.String()+"&cursor="+ids[0].String(), nil, 0)
			if response.Code != 403 {
				t.Fatalf("want forbidden got %d: %s", response.Code, response.Body)
			}
			if strings.Contains(response.Body.String(), "PRIVATE scoped outcome") {
				t.Fatal("denied review disclosed private outcomes")
			}
		}
	})
	t.Run("review requires explicit event", func(t *testing.T) {
		response := f.request(f.owner, "GET", "/api/v1/outcomes?mode=review", nil, 0)
		if response.Code != 400 {
			t.Fatalf("want invalid missing event got %d: %s", response.Code, response.Body)
		}
	})
}
