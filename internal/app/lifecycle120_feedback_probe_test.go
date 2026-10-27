package app

import (
	"net/http"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/teamreports"
	"github.com/deploymenttheory/hack-in-a-box/internal/teams"
	"github.com/google/uuid"
)

// An explicitly scoped diagnostic for an already-retained synthetic failure.
// It never replays the failed caller's command. After the original target has
// been classified as version zero, it probes a new denied source and a new
// private save intent. This diagnostic can create one synthetic private draft.
func TestLifecycleRetainedFeedbackSourceProbe(t *testing.T) {
	if os.Getenv("HACKBOX_LIFECYCLE_ISOLATED") != "synthetic-only" || os.Getenv("HACKBOX_LIFECYCLE_FEEDBACK_PROBE") != "classified-new-save-intent" {
		t.Skip("requires explicit retained isolated failure and new denied intent")
	}
	eventID, err := uuid.Parse(os.Getenv("HACKBOX_LIFECYCLE_PROBE_EVENT"))
	if err != nil {
		t.Fatal("explicit synthetic event required")
	}
	teamID, err := uuid.Parse(os.Getenv("HACKBOX_LIFECYCLE_PROBE_TEAM"))
	if err != nil {
		t.Fatal("explicit synthetic report target required")
	}
	foreignID, err := uuid.Parse(os.Getenv("HACKBOX_LIFECYCLE_PROBE_FOREIGN_TEAM"))
	if err != nil || foreignID == teamID {
		t.Fatal("distinct explicit synthetic source required")
	}
	owner, err := signInLifecycle(t.Context(), "http://admin:8080", "http://provider:8080", "owner")
	if err != nil {
		t.Fatal(err)
	}
	run := lifecycleJourney{owner: owner}
	base := "/api/v1/events/" + eventID.String() + "/teams/"
	path := base + teamID.String() + "/feedback-report"
	before := run.readTeamFeedback(t, path)
	foreign := run.readTeamFeedback(t, base+foreignID.String()+"/feedback-report")
	if before.CurrentSource.Binding == foreign.CurrentSource.Binding {
		t.Fatal("probe needs distinct current team sources")
	}
	t.Log("normal sign-in and both current source reads passed; probing a new wrong-source intent")
	run.denyTeamFeedbackSource(t, path, before, foreign.CurrentSource.Binding, "")
	if before.Version != 0 {
		t.Fatal("save probe requires a classified version-zero target")
	}
	id := uuid.New()
	scoreBinding := ""
	if before.CurrentSource.FinalScores != nil {
		scoreBinding = before.CurrentSource.FinalScores.Binding
	}
	command := lifecycleTeamFeedbackCommand(id, "save", before.CurrentSource.Binding, lifecycleTeamFeedbackBody(false), scoreBinding)
	var after teamreports.Page
	t.Logf("new diagnostic save intent=%s event=%s team=%s expected_version=0", id, eventID, teamID)
	if err := owner.call(t.Context(), http.MethodPost, path, 0, command, &after, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	if after.Version != 1 || after.Receipt == nil || after.Receipt.RequestID != id {
		t.Fatal("diagnostic save lost exact original acknowledgement")
	}
}

func TestLifecycleRetainedFeedbackContentionProbe(t *testing.T) {
	if os.Getenv("HACKBOX_LIFECYCLE_ISOLATED") != "synthetic-only" || os.Getenv("HACKBOX_LIFECYCLE_FEEDBACK_PROBE") != "new-contention-drafts" {
		t.Skip("explicit disposable retained diagnostic only")
	}
	eventID, err := uuid.Parse(os.Getenv("HACKBOX_LIFECYCLE_PROBE_EVENT"))
	if err != nil {
		t.Fatal("explicit synthetic event required")
	}
	count, err := strconv.Atoi(os.Getenv("HACKBOX_LIFECYCLE_PROBE_WRITERS"))
	if err != nil || count < 1 || count > 6 {
		t.Fatal("explicit one-to-six diagnostic writers required")
	}
	owner, err := signInLifecycle(t.Context(), "http://admin:8080", "http://provider:8080", "owner")
	if err != nil {
		t.Fatal(err)
	}
	run := lifecycleJourney{owner: owner}
	base := "/api/v1/events/" + eventID.String()
	var allocation teams.Page
	if err := owner.call(t.Context(), http.MethodGet, base+"/teams", -1, nil, &allocation, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	type attempt struct {
		path    string
		version int64
		command map[string]any
	}
	inputs := []attempt{}
	for _, team := range allocation.Teams {
		if team.State != "published" || len(inputs) == count {
			continue
		}
		path := base + "/teams/" + team.ID.String() + "/feedback-report"
		current := run.readTeamFeedback(t, path)
		id := uuid.New()
		body := lifecycleTeamFeedbackBody(false)
		body.Title = "Synthetic contention draft " + id.String()
		inputs = append(inputs, attempt{path: path, version: current.Version, command: lifecycleTeamFeedbackCommand(id, "save", current.CurrentSource.Binding, body, "")})
		t.Logf("new diagnostic save intent=%s team=%s expected_version=%d", id, team.ID, current.Version)
	}
	if len(inputs) != count {
		t.Fatal("insufficient distinct current report targets")
	}
	start := make(chan struct{})
	var wait sync.WaitGroup
	for _, input := range inputs {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			began := time.Now()
			var result teamreports.Page
			err := owner.call(t.Context(), http.MethodPost, input.path, input.version, input.command, &result, http.StatusOK)
			t.Logf("new intent=%s elapsed=%s acknowledged=%t", input.command["request_id"], time.Since(began), err == nil)
			if err != nil {
				t.Error(err)
				return
			}
			if result.Version != input.version+1 || result.Receipt == nil || result.Receipt.RequestID != input.command["request_id"] {
				t.Error("diagnostic intent lost its original acknowledgement")
			}
		}()
	}
	close(start)
	wait.Wait()
}
