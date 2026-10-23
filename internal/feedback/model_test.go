package feedback

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
)

func TestReviewedSupportDoesNotInventAssessmentOrPrivateSourceAuthority(t *testing.T) {
	in := Input{
		TeamID:   uuid.New(),
		Category: "setup",
		Title:    "Reviewed operational guidance",
		Body:     "Meet the event logistics desk",
		Sources:  []Source{},
	}
	if err := in.validate(); err != nil {
		t.Fatal(err)
	}
	in.Sources = []Source{{Kind: "observation", ID: uuid.New(), Version: 1}}
	if !errors.Is(in.validate(), access.ErrInvalid) {
		t.Fatal("private review source accepted as pre-live logistics")
	}
	in.Category = "competition_output"
	if err := in.validate(); err != nil {
		t.Fatal(err)
	}
	in.Sources[0].Kind = "scorecard"
	if !errors.Is(in.validate(), access.ErrInvalid) {
		t.Fatal("scorecards cannot be support provenance")
	}
	in.Sources = nil
	in.Body = strings.Repeat("😀", 3001)
	if !errors.Is(in.validate(), access.ErrInvalid) {
		t.Fatal("native UTF-8 body limit not applied")
	}
	task := ActionInput{
		Title:   "Check integration",
		Body:    "Run the reviewed test",
		OwnerID: uuid.New(),
		DueAt:   time.Now(),
	}
	if !errors.Is(task.validate(), access.ErrInvalid) {
		t.Fatal("unreviewed action details became shareable")
	}
	task.Reviewed = true
	if err := task.validate(); err != nil {
		t.Fatal(err)
	}
}

func TestConfiguredThresholdIsIndependentOfActionAndScore(t *testing.T) {
	now := time.Date(2027, 6, 20, 9, 0, 0, 0, time.UTC)
	s := Service{Clock: func() time.Time { return now }}
	cfg := Settings{Version: 2, SetupSeconds: 60, LogisticsSeconds: 120, OutputSeconds: 180}
	if err := cfg.validate(); err != nil {
		t.Fatal(err)
	}
	action := Action{Status: "open", DueAt: now.Add(-60 * time.Second)}
	if !s.overdue(action, "setup", cfg) || s.overdue(action, "competition_output", cfg) {
		t.Fatal("category threshold/instant mismatch")
	}
	action.Status = "done"
	if s.overdue(action, "setup", cfg) {
		t.Fatal("completed operational action escalates")
	}
	if action.Version != 0 || action.Cycle != 0 {
		t.Fatal("overdue display mutated source")
	}
}
