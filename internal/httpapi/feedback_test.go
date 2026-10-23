package httpapi

import (
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/feedback"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestFeedbackHTTPReviewedPublicationOwnTeamActionsPrivacyAndCurrentSession(t *testing.T) {
	pool := testdb.New(t, true)
	f := newEmbargoFixture(t, pool, time.Date(2027, 6, 20, 9, 0, 0, 0, time.UTC))
	if err := database.Migrate(t.Context(), pool, 0); err != nil {
		t.Fatal(err)
	}
	native := feedback.Service{Pool: pool, Clock: func() time.Time { return f.now }}
	f.router = NewRouter(
		f.cfg,
		slog.New(slog.NewJSONHandler(&f.logs, nil)),
		Dependencies{Pool: pool, Auth: f.auth, Feedback: native},
	)
	id := uuid.New()
	path := f.base() + "/feedback/" + id.String()
	member, peer, other := f.members[0][0], f.members[0][1], f.members[1][0]
	input := feedback.Command{
		Action: "save",
		Input: &feedback.Input{
			TeamID:   f.teams[0],
			Category: "setup",
			Title:    "Reviewed team support",
			Body:     "SELECTED SHARED TEXT <script>literal()</script>",
			Sources:  []feedback.Source{},
		},
	}
	f.expect(f.request(dbgen.Person{}, "GET", path, nil, 0), 401)
	f.expect(f.request(member, "PUT", path, input, 0), 403)
	saved := f.request(f.owner, "PUT", path, input, 0)
	f.expect(saved, 200)
	if !strings.Contains(saved.Header().Get("Cache-Control"), "no-store") {
		t.Fatal("private support response cached")
	}
	f.denied(f.request(member, "GET", path, nil, 0), 404)
	f.expect(f.request(f.owner, "PUT", path, feedback.Command{Action: "publish"}, 1), 409)
	f.expect(f.request(f.owner, "PUT", path, feedback.Command{Action: "review"}, 1), 200)
	f.expect(f.request(f.owner, "PUT", path, feedback.Command{Action: "publish"}, 2), 200)
	read := f.request(member, "GET", path, nil, 0)
	f.expect(read, 200)
	var page feedback.Page
	if err := json.Unmarshal(read.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Draft != nil || page.Published == nil || page.Published.Version != 3 ||
		!page.CanAcknowledge {
		t.Fatal("team released projection", read.Body.String())
	}
	f.denied(f.request(other, "GET", path, nil, 0), 404)
	f.expect(f.request(member, "POST", path+"/acknowledgements", nil, 3), 200)
	targets := f.request(f.owner, "GET", path+"/owners", nil, 0)
	f.expect(targets, 200)
	if strings.Contains(targets.Body.String(), other.ID.String()) {
		t.Fatal("owner target directory crossed teams")
	}
	f.expect(f.request(member, "GET", path+"/owners", nil, 0), 403)
	taskID := uuid.New()
	taskPath := path + "/actions/" + taskID.String()
	task := feedback.ActionCommand{
		Action: "details",
		Input: &feedback.ActionInput{
			Reviewed: true,
			Title:    "Reviewed task",
			Body:     "Check the replacement kit",
			OwnerID:  member.ID,
			DueAt:    f.now.Add(time.Hour),
		},
	}
	f.expect(f.request(f.owner, "PUT", taskPath, task, 0), 200)
	f.expect(
		f.request(
			peer,
			"PUT",
			taskPath,
			feedback.ActionCommand{Action: "status", Status: "done"},
			1,
		),
		403,
	)
	f.expect(
		f.request(
			member,
			"PUT",
			taskPath,
			feedback.ActionCommand{Action: "status", Status: "done"},
			1,
		),
		200,
	)
	f.expect(f.request(member, "GET", taskPath+"/history", nil, 0), 200)
	task.Input.Reviewed = false
	f.expect(f.request(f.owner, "PUT", taskPath, task, 2), 400)
	f.exec(
		"UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2",
		f.id,
		member.ID,
	)
	f.denied(f.request(member, "GET", path, nil, 0), 403)
	f.denied(f.request(member, "GET", path+"/actions", nil, 0), 403)
	f.denied(f.request(member, "POST", path+"/acknowledgements", nil, 3), 403)
	if strings.Contains(f.logs.String(), "SELECTED SHARED TEXT") ||
		strings.Contains(f.logs.String(), "replacement kit") {
		t.Fatal("support prose entered HTTP logs")
	}
}
