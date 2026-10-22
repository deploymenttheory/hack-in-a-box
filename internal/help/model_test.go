package help

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
)

func TestOperationalSupportIsIndependentOfCompetitionReview(t *testing.T) {
	live := time.Date(2027, 6, 20, 9, 0, 0, 0, time.UTC)
	for _, category := range []string{"setup", "logistics"} {
		if err := outputReady(category, false, live, live.Add(-time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	for _, trial := range []struct {
		released bool
		now      time.Time
	}{{false, live}, {true, live.Add(-time.Nanosecond)}} {
		if err := outputReady(
			"competition_output",
			trial.released,
			live,
			trial.now,
		); !errors.Is(
			err,
			event.ErrBlocked,
		) {
			t.Fatalf("pre-live or unreleased review accepted: %v", err)
		}
	}
	if err := outputReady("competition_output", true, live, live); err != nil {
		t.Fatalf("exact live boundary: %v", err)
	}
}

func TestOwnershipTransitionsPreserveWaitingCyclesAndFirstResponse(t *testing.T) {
	now := time.Date(2027, 6, 20, 9, 0, 0, 0, time.UTC)
	helper, second, requester := uuid.New(), uuid.New(), uuid.New()
	r := Request{
		Status:       "waiting",
		Revision:     1,
		WaitingCycle: 1,
		RequestedAt:  now,
		WaitingSince: now,
	}
	command := func(action string) Command { return Command{Action: action, CommandID: uuid.New()} }
	if _, err := apply(
		r,
		requester,
		command("claim"),
		Authority{TeamMember: true},
		false,
		now,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("requester granted helper ownership")
	}
	claimed, err := apply(
		r,
		helper,
		command("claim"),
		Authority{Helper: true},
		false,
		now.Add(time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = apply(
		claimed,
		second,
		command("claim"),
		Authority{Helper: true},
		false,
		now,
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatal("claimed request accepted second winner")
	}
	transfer := command("transfer")
	transfer.AssigneeID = &second
	if _, err = apply(
		claimed,
		helper,
		transfer,
		Authority{Helper: true},
		false,
		now,
	); !errors.Is(
		err,
		access.ErrInvalid,
	) {
		t.Fatal("ineligible target accepted")
	}
	moved, err := apply(
		claimed,
		helper,
		transfer,
		Authority{Helper: true},
		true,
		now.Add(2*time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = apply(
		moved,
		helper,
		command("resolve"),
		Authority{Helper: true},
		false,
		now,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("former helper retained ownership")
	}
	resolved, err := apply(
		moved,
		second,
		command("resolve"),
		Authority{Helper: true},
		false,
		now.Add(3*time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := apply(
		resolved,
		requester,
		command("reopen"),
		Authority{TeamMember: true},
		false,
		now.Add(4*time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Status != "waiting" || reopened.WaitingCycle != 2 || reopened.AssigneeID != nil ||
		reopened.ResolvedAt != nil ||
		!reopened.FirstClaimedAt.Equal(*claimed.FirstClaimedAt) ||
		!reopened.WaitingSince.Equal(now.Add(4*time.Minute)) {
		t.Fatalf("reopened request lost operational history: %+v", reopened)
	}
}

func TestMeetingRoutesRequireExplicitEventApproval(t *testing.T) {
	settings := DefaultSettings()
	settings.MeetingHosts = []string{"meet.example.test"}
	in := Input{
		TeamID:      uuid.New(),
		CommandID:   uuid.New(),
		Category:    "logistics",
		Description: "Synthetic room setup question",
		MeetingURL:  "https://meet.example.test/event-room",
	}
	if err := validateInput(in, settings); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"https://other.example.test/room", "https://user:secret@meet.example.test/room", "javascript:alert(1)", "http://meet.example.test/room", "https://meet.example.test:443/room"} {
		in.MeetingURL = raw
		if err := validateInput(in, settings); !errors.Is(err, access.ErrInvalid) {
			t.Fatalf("unapproved meeting route %q: %v", raw, err)
		}
	}
	in.MeetingURL = ""
	in.Location = "Synthetic help desk"
	if err := validateInput(in, DefaultSettings()); err != nil {
		t.Fatalf("physical support wrongly requires meeting host configuration: %v", err)
	}
}
