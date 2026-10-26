package httpapi

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/attendance"
	"github.com/deploymenttheory/hack-in-a-box/internal/certificates"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestCertificatesHTTPReviewedIssuanceCurrentPrivatePrintAndRevocation(t *testing.T) {
	pool := testdb.New(t, true)
	f := newEmbargoFixture(t, pool, time.Date(2027, 7, 20, 8, 0, 0, 0, time.UTC))
	f.now = f.c.LiveStart.Add(2 * time.Hour)
	native := certificates.Service{Pool: pool, Now: func() time.Time { return f.now }}
	f.router = NewRouter(
		f.cfg,
		slog.New(slog.NewJSONHandler(&f.logs, nil)),
		Dependencies{Pool: pool, Auth: f.auth, Certificates: native},
	)
	person := f.members[0][0]
	var recipient dbgen.Person
	var err error
	recipient, err = f.q.Person(t.Context(), person.ID)
	if err != nil {
		t.Fatal(err)
	}
	template := uuid.New()
	settingsPath := "/api/v1/events/" + f.id.String() + "/certificate-settings"
	call := func(actor dbgen.Person, method, path string, body any, status int) *httptest.ResponseRecorder {
		t.Helper()
		w := f.request(actor, method, path, body, 0)
		f.expect(w, status)
		validateResponse(t, httptest.NewRequest(method, path, nil), w)
		if !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
			t.Fatal("private response cached")
		}
		return w
	}
	policy := certificates.SettingsCommand{
		RequestID: uuid.New(),
		Reviewed:  true,
		Attendance: &certificates.AttendancePolicy{
			Days:           []string{f.now.Format(time.DateOnly)},
			MinimumPresent: 1,
		},
		Templates: []certificates.Template{
			{
				ID:          template,
				Kind:        "attendance",
				Recognition: "Recorded attendance only",
				Issuer:      "Reviewed synthetic organiser",
				Accent:      "#007e67",
			},
		},
		Reason: "Explicit template review",
	}
	call(recipient, "PUT", settingsPath, policy, 403)
	raw, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	without := httptest.NewRequest("PUT", settingsPath, bytes.NewReader(raw))
	without.Header.Set("Content-Type", "application/json")
	without.Header.Set("Origin", f.cfg.PublicOrigin)
	without.AddCookie(&http.Cookie{Name: "hackbox_session", Value: f.tokens[f.owner.ID]})
	denied := httptest.NewRecorder()
	f.router.ServeHTTP(denied, without)
	f.expect(denied, 400)
	call(f.owner, "PUT", settingsPath, policy, 200)
	call(f.owner, "PUT", settingsPath, policy, 200)
	target := certificates.Target{PersonID: person.ID, TemplateID: template}
	previewPath := "/api/v1/events/" + f.id.String() + "/certificates/eligibility?person_id=" + person.ID.String() + "&template_id=" + template.String()
	var eligible certificates.Eligibility
	w := call(f.owner, "GET", previewPath, nil, 200)
	if err = json.Unmarshal(w.Body.Bytes(), &eligible); err != nil {
		t.Fatal(err)
	}
	if eligible.Eligible {
		t.Fatal("HTTP registration-only eligible")
	}
	_, err = (attendance.Service{Pool: pool, Now: func() time.Time { return f.now }}).Mark(
		t.Context(),
		f.owner.ID,
		f.id,
		person.ID,
		policy.Attendance.Days[0],
		0,
		attendance.Command{
			RequestID: uuid.New(),
			Present:   true,
			Reason:    "Verified physical check-in",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	w = call(f.owner, "GET", previewPath, nil, 200)
	if err = json.Unmarshal(w.Body.Bytes(), &eligible); err != nil {
		t.Fatal(err)
	}
	if !eligible.Eligible {
		t.Fatal("native check-in missing")
	}
	id := uuid.New()
	path := "/api/v1/events/" + f.id.String() + "/certificates/" + id.String()
	issue := certificates.Command{
		RequestID: uuid.New(),
		Action:    "issue",
		Target:    target,
		Binding:   eligible.Binding,
		Reason:    "PRIVATE organiser issuance note",
	}
	var issued certificates.View
	w = call(f.owner, "PUT", path, issue, 200)
	if err = json.Unmarshal(w.Body.Bytes(), &issued); err != nil {
		t.Fatal(err)
	}
	if issued.Certificate.Version != 1 || issued.Receipt == nil ||
		issued.Receipt.RequestID != issue.RequestID {
		t.Fatal("wrong original HTTP receipt")
	}
	w = call(recipient, "GET", path, nil, 200)
	if strings.Contains(w.Body.String(), "PRIVATE") ||
		strings.Contains(w.Body.String(), f.owner.ID.String()) {
		t.Fatal("private issuance provenance leaked to recipient")
	}
	call(f.members[0][1], "GET", path, nil, 404)
	call(dbgen.Person{}, "GET", path, nil, 401)
	call(f.owner, "GET", "/api/v1/events/"+uuid.NewString()+"/certificates/"+id.String(), nil, 404)
	w = call(recipient, "GET", path+"/print", nil, 200)
	if !strings.Contains(w.Body.String(), "<html lang=\"en\">") ||
		strings.Contains(w.Body.String(), "PRIVATE") ||
		!strings.Contains(w.Header().Get("Content-Security-Policy"), "default-src 'none'") ||
		!strings.Contains(w.Header().Get("Content-Disposition"), "attachment;") {
		t.Fatal("unsafe printable representation")
	}
	revoke := certificates.Command{
		RequestID: uuid.New(),
		Version:   1,
		Action:    "revoke",
		Target:    target,
		Reason:    "Explicit reviewed revocation",
	}
	call(f.owner, "PUT", path, revoke, 200)
	call(recipient, "GET", path+"/print", nil, 409)
}
