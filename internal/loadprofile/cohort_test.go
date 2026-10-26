package loadprofile

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
)

func syntheticReleaseCohort() CohortPlan {
	plan := CohortPlan{Profile: "release"}
	for eventIndex := range 5 {
		event := uuid.New()
		for person := range 200 {
			plan.Members = append(plan.Members, CohortMember{
				Subject: fmt.Sprintf("synthetic-%d-%d", eventIndex, person), Event: event,
				Staff: person >= 180, Surface: PublicSurface,
			})
		}
	}
	return plan
}

func TestReleaseCohortCannotConcealStaffExtrasOrDuplicateIdentitySelection(t *testing.T) {
	if err := syntheticReleaseCohort().Validate(); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*CohortPlan){
		"extra-staff":         func(p *CohortPlan) { p.Members[0].Staff = true },
		"fewer-active-people": func(p *CohortPlan) { p.Members = p.Members[:999] },
		"duplicate-subject":   func(p *CohortPlan) { p.Members[1].Subject = p.Members[0].Subject },
		"hidden-sixth-event":  func(p *CohortPlan) { p.Members[0].Event = uuid.New() },
		"wrong-surface":       func(p *CohortPlan) { p.Members[0].Surface = WorkerSurface },
	} {
		t.Run(name, func(t *testing.T) {
			plan := syntheticReleaseCohort()
			change(&plan)
			if err := plan.Validate(); err == nil {
				t.Fatal("invalid release census accepted")
			}
		})
	}
}

// This synthetic HTTPS server only verifies driver isolation/accounting. It is
// explicitly not the application or a native OIDC/session acceptance fixture.
func TestCohortSignInCannotTurnCollapsedPeopleIntoDistinctActiveSessions(t *testing.T) {
	for _, collapse := range []bool{false, true} {
		t.Run(fmt.Sprint(collapse), func(t *testing.T) {
			var loginCalls atomic.Int32
			var origin string
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Cache-Control", "no-store")
				switch r.URL.Path {
				case "/auth/login":
					loginCalls.Add(1)
					http.Redirect(w, r, origin+"/authorize?state=synthetic", http.StatusFound)
				case "/authorize":
					http.Redirect(w, r, origin+"/auth/callback?subject="+url.QueryEscape(r.URL.Query().Get("actor")), http.StatusFound)
				case "/auth/callback":
					http.SetCookie(w, &http.Cookie{Name: "fixture-subject", Value: r.URL.Query().Get("subject"), Path: "/", Secure: true, HttpOnly: true})
					http.Redirect(w, r, origin+"/events", http.StatusSeeOther)
				case "/api/v1/session":
					cookie, err := r.Cookie("fixture-subject")
					if err != nil {
						t.Error("independent callback jar absent")
						w.WriteHeader(http.StatusUnauthorized)
						return
					}
					subject := cookie.Value
					if collapse {
						subject = "one-person"
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"person": map[string]any{"id": uuid.NewSHA1(uuid.NameSpaceOID, []byte(subject))}, "csrf_token": strings.Repeat("c", 32)})
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			origin = server.URL
			plan := CohortPlan{Profile: "smoke", Members: []CohortMember{
				{Subject: "synthetic-a", Event: uuid.New(), Surface: PublicSurface},
				{Subject: "synthetic-b", Event: uuid.New(), Surface: AdminSurface},
			}}
			actors, receipt, err := AuthenticateCohort(t.Context(), plan,
				map[Surface]string{PublicSurface: origin, AdminSurface: origin}, origin, server.Client().Transport, 2)
			for _, actor := range actors {
				defer actor.Session.Close()
			}
			if loginCalls.Load() != 2 || receipt.Attempted != 2 || receipt.Authenticated != 2 || receipt.SustainedProof {
				t.Fatal("authentication accounting/retry/active claim violated", receipt)
			}
			if collapse {
				if err == nil || receipt.DistinctPeople != 1 {
					t.Fatal("duplicate native person counted as distinct", receipt, err)
				}
			} else if err != nil || receipt.DistinctPeople != 2 {
				t.Fatal("independent cookie jars did not retain identities", receipt, err)
			}
		})
	}
}
