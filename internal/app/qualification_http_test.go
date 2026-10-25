package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/judging"
	"github.com/deploymenttheory/hack-in-a-box/internal/qualification"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
)

// Actual native runtime, common publication, lifecycle, sessions and strict
// generated router; no fixture endpoint, provider operation or fabricated policy.
func qualificationHTTPFixture(
	t *testing.T,
	beforeCutoff ...func(*liveRehearsal, rehearsalSurface, uuid.UUID),
) (*liveRehearsal, rehearsalSurface, rehearsalSurface, uuid.UUID) {
	t.Helper()
	r := newLiveRehearsal(t, true)
	admin, public := r.surface(t, "admin"), r.surface(t, "public")
	criteria, err := admin.deps.Rubrics.Configuration(t.Context(), r.owner.ID, r.eventID, 0)
	if err != nil {
		t.Fatal(err)
	}
	definition := rubric.Default(rubric.MentorQualification, r.eventID)
	criteria, err = admin.deps.Rubrics.Change(
		t.Context(),
		r.owner.ID,
		r.eventID,
		criteria.Revision,
		rubric.Command{
			Action:     "save",
			Definition: &definition,
			Reason:     "Actual configured mentor qualification criteria",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	criteria, err = admin.deps.Rubrics.Change(
		t.Context(),
		r.owner.ID,
		r.eventID,
		criteria.Revision,
		rubric.Command{
			Action:  "approve",
			Version: criteria.Reference.Version,
			Reason:  "Explicit current common criteria approval",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := admin.deps.Content.Save(
		t.Context(),
		r.owner.ID,
		r.eventID,
		r.challengeID,
		r.challengeRevision,
		content.Input{
			Kind: "challenge",
			Document: content.Document{
				Title:    "Synthetic qualification challenge",
				Markdown: "Common synthetic challenge with reviewed qualification criteria",
				Audience: access.ContentAudience{Confirmed: true},
				Rubric:   &criteria.Reference,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"review", "approve"} {
		challenge, err = admin.deps.Content.Action(
			t.Context(),
			r.owner.ID,
			r.eventID,
			challenge.ID,
			challenge.Revision,
			action,
			"Explicit synthetic common challenge review",
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	page, err := admin.deps.Events.Read(t.Context(), r.owner.ID, r.eventID)
	if err != nil {
		t.Fatal(err)
	}
	if page.ChallengePublication == nil {
		t.Fatal("native common publication time absent")
	}
	r.clock.set(*page.ChallengePublication)
	if err = admin.deps.Content.Dispatch(
		t.Context(),
		r.eventID,
		challenge.ID,
		challenge.Revision,
	); err != nil {
		t.Fatal(err)
	}
	criteria, err = admin.deps.Rubrics.Configuration(t.Context(), r.owner.ID, r.eventID, 0)
	if err != nil {
		t.Fatal(err)
	}
	criteria, err = admin.deps.Rubrics.Change(
		t.Context(),
		r.owner.ID,
		r.eventID,
		criteria.Revision,
		rubric.Command{
			Action:  "create_run",
			Purpose: "mentor_final",
			Version: criteria.Reference.Version,
			Reason:  "Actual current mentor final run",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	var runID uuid.UUID
	for _, run := range criteria.Runs {
		if run.Purpose == "mentor_final" {
			runID = run.ID
		}
	}
	if runID == uuid.Nil {
		t.Fatal("no actual mentor-final run")
	}
	page, err = admin.deps.Events.Read(t.Context(), r.owner.ID, r.eventID)
	if err != nil {
		t.Fatal(err)
	}
	r.clock.set(page.Configuration.LiveStart)
	if _, err = admin.deps.Events.Transition(
		t.Context(),
		r.owner.ID,
		r.eventID,
		page.Revision,
		"live",
		"Actual organiser live transition",
		false,
	); err != nil {
		t.Fatal(err)
	}
	for _, prepare := range beforeCutoff {
		prepare(r, admin, runID)
	}
	r.clock.set(page.Configuration.SubmissionCutoff)
	return r, admin, public, runID
}

func TestQualificationHTTPStrictNativeRuntimeOriginalReceiptsAndRoleLoss(t *testing.T) {
	r, admin, public, run := qualificationHTTPFixture(t)
	for _, surface := range []rehearsalSurface{admin, public} {
		judgingPolicy, ok := surface.deps.Judging.Finalists.(*qualification.Service)
		if !ok {
			t.Fatal("judging lacks real qualification owner")
		}
		orderOwner, nativeOrder := surface.deps.Presentation.OrderAuthority.(judging.Service)
		if !nativeOrder || orderOwner.Finalists != judgingPolicy {
			t.Fatal("presentation order did not preserve the actual configured judging graph")
		}
		rubricPolicy, ok := surface.deps.Rubrics.Finalists.(*qualification.Service)
		if !ok || rubricPolicy != judgingPolicy || judgingPolicy.Pool != r.pool ||
			judgingPolicy.Moderation != surface.deps.Moderation ||
			judgingPolicy.Cards.Moderation != surface.deps.Moderation {
			t.Fatal("runtime qualification policy is detached from actual native owners")
		}
	}
	base := fmt.Sprintf("/api/v1/events/%s/mentor-qualification/%s", r.eventID, run)
	request := qualificationHTTPRequest(t)
	collection := fmt.Sprintf("/api/v1/events/%s/mentor-qualification", r.eventID)
	status, raw := request(admin, r.owner, "GET", collection, nil, 0, true)
	if status != 200 {
		t.Fatal("actual organiser run discovery", status, string(raw))
	}
	var discovered qualification.Rounds
	if err := json.Unmarshal(
		raw,
		&discovered,
	); err != nil || len(discovered.Items) != 1 || discovered.Items[0].RunID != run ||
		!discovered.Items[0].CanConfigurePanel {
		t.Fatal("native discovery source metadata", discovered, err)
	}
	status, raw = request(public, r.mentor, "GET", collection, nil, 0, true)
	if status != 200 {
		t.Fatal("private pre-panel discovery", status, string(raw))
	}
	if err := json.Unmarshal(raw, &discovered); err != nil || len(discovered.Items) != 0 {
		t.Fatal("unrecorded mentor saw private run context", discovered, err)
	}
	status, raw = request(admin, r.owner, "GET", base, nil, 0, true)
	if status != 200 {
		t.Fatal("current native qualification read", status, string(raw))
	}
	var initial qualification.Page
	if err := json.Unmarshal(raw, &initial); err != nil {
		t.Fatal(err)
	}
	if initial.Receipt != nil || initial.Round.Version != 0 {
		t.Fatal("new native read invented an original ACK", initial)
	}
	command := map[string]any{
		"request_id": uuid.New(),
		"version":    0,
		"action":     "configure_panel",
		"reason":     "Explicit intended result panel",
		"panel": map[string]any{
			"mentor_ids":   []uuid.UUID{r.mentor.ID},
			"organiser_id": r.owner.ID,
		},
	}
	if status, raw = request(
		admin,
		r.owner,
		"POST",
		base,
		command,
		initial.Revision,
		false,
	); status != 400 {
		t.Fatal("missing CSRF wrote qualification", status, string(raw))
	}
	invalid := map[string]any{}
	for key, value := range command {
		invalid[key] = value
	}
	invalid["unexpected_private_payload"] = "must be rejected"
	if status, raw = request(
		admin,
		r.owner,
		"POST",
		base,
		invalid,
		initial.Revision,
		true,
	); status != 400 {
		t.Fatal("strict contract accepted unknown input", status, string(raw))
	}
	if status, raw = request(
		admin,
		r.owner,
		"POST",
		base,
		command,
		initial.Revision,
		true,
	); status != 200 {
		t.Fatal("native reviewed panel write", status, string(raw))
	}
	var accepted qualification.Page
	if err := json.Unmarshal(raw, &accepted); err != nil {
		t.Fatal(err)
	}
	if accepted.Receipt == nil || accepted.Receipt.RequestID != command["request_id"] ||
		accepted.Receipt.ExpectedRevision != initial.Revision ||
		accepted.Receipt.ExpectedVersion != 0 ||
		accepted.Receipt.Version != 1 {
		t.Fatal("HTTP lost exact original request metadata", accepted)
	}
	if len(accepted.Teams) != 1 || accepted.Teams[0].Outcome != "not_submitted" ||
		accepted.Teams[0].Eligible {
		t.Fatal("native missing outcome became a competing final", accepted.Teams)
	}
	if status, raw = request(public, r.mentor, "GET", base, nil, 0, true); status != 200 {
		t.Fatal("independent portal current intended panel read", status, string(raw))
	}
	var current qualification.Page
	if err := json.Unmarshal(raw, &current); err != nil || current.Receipt != nil {
		t.Fatal("fresh public portal read became an ACK", err)
	}
	if !current.Capabilities.CanReadPanel || current.Capabilities.CanConfigurePanel ||
		current.Capabilities.CanPropose ||
		current.Capabilities.CanSignOff {
		t.Fatal("HTTP action projection inferred organiser rights", current.Capabilities)
	}
	status, raw = request(public, r.mentor, "GET", collection, nil, 0, true)
	if status != 200 {
		t.Fatal("intended mentor run discovery", status, string(raw))
	}
	if err := json.Unmarshal(
		raw,
		&discovered,
	); err != nil || len(discovered.Items) != 1 || discovered.Items[0].RunID != run ||
		discovered.Items[0].CanConfigurePanel {
		t.Fatal("intended native discovery capabilities", discovered, err)
	}
	receiptPath := base + "/commands/" + accepted.Receipt.RequestID.String()
	if status, raw = request(admin, r.owner, "GET", receiptPath, nil, 0, true); status != 200 {
		t.Fatal("native original receipt lookup", status, string(raw))
	}
	var recovered qualification.Recovery
	if err := json.Unmarshal(
		raw,
		&recovered,
	); err != nil || recovered.Receipt == nil ||
		*recovered.Receipt != *accepted.Receipt {
		t.Fatal("HTTP original receipt identity changed", recovered, err)
	}
	for _, probe := range []struct {
		person dbgen.Person
		path   string
		status int
	}{{r.competitor, base, 403}, {r.mentor, receiptPath, 403}, {r.owner, fmt.Sprintf("/api/v1/events/%s/mentor-qualification/%s", r.otherEventID, run), 403}, {r.owner, fmt.Sprintf("/api/v1/events/%s/mentor-qualification/%s", r.eventID, uuid.New()), 404}} {
		if status, raw = request(
			public,
			probe.person,
			"GET",
			probe.path,
			nil,
			0,
			true,
		); status != probe.status {
			t.Fatal("current scoped API boundary", status, string(raw), probe.status)
		}
	}
	if status, raw = request(
		admin,
		r.owner,
		"GET",
		base+"/history?limit=1",
		nil,
		0,
		true,
	); status != 200 {
		t.Fatal("strict native history", status, string(raw))
	}
	if _, err := r.pool.Exec(
		t.Context(),
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2",
		r.eventID,
		r.owner.ID,
	); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{base, receiptPath, base + "/history"} {
		if status, raw = request(admin, r.owner, "GET", path, nil, 0, true); status != 403 {
			t.Fatal("native role loss retained a private API projection", status, string(raw))
		}
	}
	var effects int
	if err := r.pool.QueryRow(t.Context(), "SELECT count(*) FROM mentor_qualification_commands WHERE event_id=$1", r.eventID).
		Scan(&effects); err != nil ||
		effects != 1 {
		t.Fatal("denied HTTP probes mutated qualification", effects, err)
	}
}

func qualificationHTTPRequest(
	t *testing.T,
) func(rehearsalSurface, dbgen.Person, string, string, any, int64, bool) (int, []byte) {
	return func(surface rehearsalSurface, person dbgen.Person, method, path string, body any, revision int64, csrf bool) (int, []byte) {
		t.Helper()
		var reader io.Reader
		if body != nil {
			raw, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			reader = bytes.NewReader(raw)
		}
		req, err := http.NewRequestWithContext(t.Context(), method, surface.server.URL+path, reader)
		if err != nil {
			t.Fatal(err)
		}
		req.AddCookie(surface.cookie(t, person))
		req.Header.Set("Origin", surface.server.URL)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("If-Match", fmt.Sprintf("\"%d\"", revision))
		if csrf {
			req.Header.Set("X-CSRF-Token", strings.Repeat("c", 32))
		}
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = response.Body.Close() }()
		raw, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(response.Header.Get("Cache-Control"), "no-store") {
			t.Fatal("private qualification response was cacheable")
		}
		return response.StatusCode, raw
	}
}
