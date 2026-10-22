package presentation

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

func TestPresentationCommandsKeepTimersAndPublicFieldsBounded(t *testing.T) {
	cue := Cue{
		ID:              uuid.New(),
		Title:           "Synthetic room",
		URL:             "https://example.invalid",
		DurationSeconds: 60,
	}
	for _, url := range []string{"javascript:alert(1)", "http://example.invalid", "https://secret:password@example.invalid"} {
		bad := cue
		bad.URL = url
		if err := validate(
			Command{Action: "save", Reason: "Reviewed cue", Cues: []Cue{bad}},
		); !errors.Is(
			err,
			access.ErrInvalid,
		) {
			t.Fatalf("unsafe room URL accepted: %q", url)
		}
	}
	row := dbgen.PresentationState{
		Draft:        []byte("[]"),
		Published:    []byte("[]"),
		CurrentIndex: -1,
		Mode:         "held",
	}
	if err := change(
		&row,
		Command{Action: "advance"},
		time.Now(),
	); !errors.Is(
		err,
		access.ErrInvalid,
	) {
		t.Fatal("advanced an empty program")
	}
	row.Published = []byte(
		`[{"id":"00000000-0000-4000-8000-000000000001","title":"Room","duration_seconds":60}]`,
	)
	if err := change(&row, Command{Action: "advance"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := change(
		&row,
		Command{Action: "resume"},
		time.Now(),
	); !errors.Is(
		err,
		access.ErrInvalid,
	) {
		t.Fatal("resume silently reset a running timer")
	}
}
