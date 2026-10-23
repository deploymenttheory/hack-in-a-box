package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/feedback"
	"github.com/deploymenttheory/hack-in-a-box/internal/moderation"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type moderationHTTPHarness struct {
	Pool                           *pgxpool.Pool
	EventID, TeamID                uuid.UUID
	Owner, Primary, Second, Member dbgen.Person
	Checkpoint                     scorecards.Checkpoint
	Dates                          event.Configuration
	Now                            func() time.Time
	Revision                       func() int64
	Request                        func(dbgen.Person, string, string, any, int64, bool) *httptest.ResponseRecorder
	Check                          func(*httptest.ResponseRecorder, int)
	Exec                           func(string, ...any)
	Install                        func(*moderation.Service)
}

func verifyHuddleHTTP(t *testing.T, f moderationHTTPHarness) []string {
	t.Helper()
	raw, err := os.ReadFile("../moderation/schema.provisional.sql")
	if err != nil {
		t.Fatal(err)
	}
	f.Exec(string(raw))
	availability, _ := json.Marshal(
		[]map[string]time.Time{{"start": f.Dates.LiveStart, "end": f.Dates.LiveEnd}},
	)
	for _, person := range []dbgen.Person{f.Primary, f.Second} {
		f.Exec(
			`INSERT INTO mentor_profiles(event_id,person_id,availability)VALUES($1,$2,$3)`,
			f.EventID,
			person.ID,
			availability,
		)
	}
	native := scorecards.Service{Pool: f.Pool, Now: f.Now}
	owner := &moderation.Service{
		Pool:    f.Pool,
		Clock:   f.Now,
		Cards:   &native,
		Actions: moderation.FeedbackActions{Service: &feedback.Service{Pool: f.Pool, Clock: f.Now}},
	}
	f.Install(owner)
	base := fmt.Sprintf("/api/v1/events/%s/teams/%s", f.EventID, f.TeamID)
	listPath := base + "/moderation-huddles"
	id := uuid.New()
	path := listPath + "/" + id.String()
	definition := moderation.Definition{
		ID:                id,
		CheckpointID:      f.Checkpoint.Definition.ID,
		CheckpointVersion: f.Checkpoint.Version,
		ModeratorID:       f.Owner.ID,
		SecondReviewerID:  f.Second.ID,
		ReviewDueAt:       f.Checkpoint.ClosesAt.Add(-time.Minute),
	}
	body := map[string]any{
		"action":     "configure",
		"version":    0,
		"definition": definition,
		"reason":     "Actual native reviewer context",
	}
	f.Check(
		f.Request(dbgen.Person{}, http.MethodGet, listPath, nil, f.Revision(), true),
		http.StatusUnauthorized,
	)
	f.Check(
		f.Request(f.Member, http.MethodPut, path, body, f.Revision(), true),
		http.StatusForbidden,
	)
	f.Check(
		f.Request(f.Owner, http.MethodPut, path, body, f.Revision(), false),
		http.StatusForbidden,
	)
	f.Check(
		f.Request(f.Owner, http.MethodPut, path, body, f.Revision()-1, true),
		http.StatusConflict,
	)
	w := f.Request(f.Owner, http.MethodPut, path, body, f.Revision(), true)
	f.Check(w, http.StatusOK)
	var h moderation.Huddle
	if err = json.Unmarshal(w.Body.Bytes(), &h); err != nil {
		t.Fatal(err)
	}
	panel := base + "/scorecards/panel?context_id=" + id.String()
	f.Check(
		f.Request(f.Owner, http.MethodGet, panel, nil, f.Revision(), true),
		http.StatusForbidden,
	)
	f.Check(
		f.Request(
			f.Owner,
			http.MethodPut,
			path,
			map[string]any{
				"action":  "open",
				"version": h.Version,
				"reason":  "No second independent review",
			},
			f.Revision(),
			true,
		),
		http.StatusConflict,
	)
	second := scorecards.CardCommand{
		Action:            "submit",
		CheckpointID:      f.Checkpoint.Definition.ID,
		CheckpointVersion: f.Checkpoint.Version,
		Scores:            []scorecards.CategoryScore{},
	}
	f.Check(
		f.Request(f.Second, http.MethodPost, base+"/scorecards", second, f.Revision(), true),
		http.StatusOK,
	)
	change := func(actor dbgen.Person, input map[string]any) {
		t.Helper()
		input["version"] = h.Version
		if input["reason"] == nil {
			input["reason"] = "Explicit current huddle decision"
		}
		w = f.Request(actor, http.MethodPut, path, input, f.Revision(), true)
		f.Check(w, http.StatusOK)
		if err = json.Unmarshal(w.Body.Bytes(), &h); err != nil {
			t.Fatal(err)
		}
	}
	change(f.Second, map[string]any{"action": "complete_second_review"})
	change(f.Owner, map[string]any{"action": "open"})
	w = f.Request(f.Second, http.MethodGet, panel, nil, f.Revision(), true)
	f.Check(w, http.StatusOK)
	var cards []scorecards.Card
	if err = json.Unmarshal(w.Body.Bytes(), &cards); err != nil || len(cards) != 2 {
		t.Fatal("actual panel projection missing", err)
	}
	var original scorecards.Card
	for _, card := range cards {
		if card.AuthorID == f.Primary.ID {
			original = card
		}
	}
	proposal := uuid.New()
	value := original.Criteria.Categories[0].MinScore
	change(
		f.Owner,
		map[string]any{
			"action":         "propose_change",
			"proposal_id":    proposal,
			"card":           original.Reference,
			"category_id":    original.Scores[0].CategoryID,
			"proposed_value": value,
			"evidence": []scorecards.EvidenceReference{
				{Kind: "note", Note: "HTTP-PRIVATE-HUDDLE-CANARY"},
			},
			"reason": "Explicit proposed change with original source evidence",
		},
	)
	f.Check(
		f.Request(
			f.Second,
			http.MethodPut,
			path,
			map[string]any{
				"action":      "accept_change",
				"version":     h.Version,
				"proposal_id": proposal,
				"reason":      "Second is not named moderator",
			},
			f.Revision(),
			true,
		),
		http.StatusForbidden,
	)
	change(f.Owner, map[string]any{"action": "accept_change", "proposal_id": proposal})
	for _, target := range []string{path, listPath + "?limit=1", path + "/history?limit=1"} {
		w = f.Request(f.Owner, http.MethodGet, target, nil, f.Revision(), true)
		f.Check(w, http.StatusOK)
		if !strings.Contains(w.Body.String(), "HTTP-PRIVATE-HUDDLE-CANARY") {
			t.Fatal("private huddle fixture unpopulated", target)
		}
		f.Check(
			f.Request(f.Member, http.MethodGet, target, nil, f.Revision(), true),
			http.StatusForbidden,
		)
	}
	outsider := testdb.Person(t, f.Pool)
	f.Exec(
		`INSERT INTO event_memberships(event_id,person_id,role)VALUES($1,$2,'organiser')`,
		f.EventID,
		outsider.ID,
	)
	f.Check(
		f.Request(outsider, http.MethodGet, path, nil, f.Revision(), true),
		http.StatusForbidden,
	)
	f.Check(
		f.Request(f.Owner, http.MethodGet, listPath+"?limit=101", nil, f.Revision(), true),
		http.StatusBadRequest,
	)
	f.Check(
		f.Request(f.Owner, http.MethodGet, path+"/history?cursor=0", nil, f.Revision(), true),
		http.StatusBadRequest,
	)
	copied := strings.Replace(
		path,
		f.EventID.String(),
		testdb.Event(t, f.Pool, outsider.ID).String(),
		1,
	)
	w = f.Request(outsider, http.MethodGet, copied, nil, f.Revision(), true)
	if w.Code != http.StatusNotFound && w.Code != http.StatusForbidden {
		t.Fatal("cross-event huddle did not deny", w.Code)
	}
	f.Check(
		f.Request(
			f.Owner,
			http.MethodPut,
			path,
			map[string]any{
				"action":  "sign_off",
				"version": h.Version,
				"reason":  "Provisional cards are never final",
			},
			f.Revision(),
			true,
		),
		http.StatusForbidden,
	)
	return []string{path, path + "/history", listPath, panel}
}
