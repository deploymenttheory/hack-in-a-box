package outcomes

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestEvidenceBasisRequiresDatedSourceForDemonstratedAdoption(t *testing.T) {
	zero := "0"
	evidence := Evidence{
		ID:    uuid.New(),
		Basis: "estimate",
		Claim: "An estimated adoption count",
		Value: &zero,
	}
	if err := evidence.Validate(); err != nil {
		t.Fatal(err)
	}
	evidence.Basis = "self_report"
	if err := evidence.Validate(); err != nil {
		t.Fatal(err)
	}
	evidence.Basis = "demonstrated_adoption"
	if evidence.Validate() == nil {
		t.Fatal("demonstration without dated source accepted")
	}
	at := time.Date(2027, 10, 23, 12, 0, 0, 0, time.UTC)
	evidence.ObservedAt = &at
	evidence.Source = "A dated adoption review with independently retained source"
	if err := evidence.Validate(); err != nil {
		t.Fatal(err)
	}
	evidence.Value = nil
	if err := evidence.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestReferencesAreLinksNotAutomaticExternalRequests(t *testing.T) {
	for _, link := range []string{"https://example.test/backlog/123", ""} {
		if !referenceURL(link) {
			t.Errorf("valid reference rejected %q", link)
		}
	}
	for _, link := range []string{"javascript:alert(1)", "https://user:secret@example.test/", "http://example.test", "//example.test", "https://example.test/\nprivate"} {
		if referenceURL(link) {
			t.Errorf("unsafe reference accepted %q", link)
		}
	}
}

func TestGeneralPitchDoesNotNeedAnEventOrTeam(t *testing.T) {
	command := Command{
		ID:     uuid.New(),
		Action: "propose",
		Reason: "Organisation member proposes a future idea",
		Definition: &Definition{
			Kind:          "pitch",
			Problem:       "A problem",
			Audience:      "People using this process",
			ProposedValue: "An estimate, not an achieved result",
		},
	}
	if err := command.Validate(); err != nil {
		t.Fatal(err)
	}
	command.Action = "adopt"
	if command.Validate() == nil {
		t.Fatal("adoption without authorised event provenance accepted")
	}
}

func TestReviewScheduleKeepsCalendarTimeExplicitAndNeverBroadcasts(t *testing.T) {
	schedule := ReviewSchedule{
		Days:     30,
		WallTime: "2027-10-31T01:30:00",
		Timezone: "Europe/London",
		Channels: []string{"in_app"},
	}
	if schedule.Validate() == nil {
		t.Fatal("ambiguous clock accepted without a choice")
	}
	schedule.DSTChoice = "earlier"
	if err := schedule.Validate(); err != nil {
		t.Fatal(err)
	}
	schedule.Channels = []string{"teams"}
	if schedule.Validate() == nil {
		t.Fatal("private owner review can broadcast")
	}
	schedule.Channels = []string{"in_app"}
	schedule.Days = 45
	if schedule.Validate() == nil {
		t.Fatal("invented follow-up cadence accepted")
	}
	schedule.Days = 60
	schedule.Timezone = "Invalid/Timezone"
	if schedule.Validate() == nil {
		t.Fatal("invalid timezone accepted")
	}
}

func TestPublicationRequiresExplicitSafeLinksAndExactOwnedMediaReferences(t *testing.T) {
	fields := PublicationFields{
		Problem:  "Reviewed public problem",
		Solution: "Reviewed public solution",
		Audience: "organisation",
		Links: []ProjectLink{
			{ID: uuid.New(), Label: "Demonstration", URL: "https://example.test/demo"},
		},
	}
	if err := fields.Validate(); err != nil {
		t.Fatal(err)
	}
	fields.Media = []MediaReference{{ID: uuid.New(), EventID: uuid.New(), Version: 0}}
	if fields.Validate() == nil {
		t.Fatal("unversioned native media accepted")
	}
	fields.Media[0].Version = 1
	if err := fields.Validate(); err != nil {
		t.Fatal(err)
	}
	fields.Links[0].URL = "javascript:alert(1)"
	if fields.Validate() == nil {
		t.Fatal("unsafe public reference accepted")
	}
}

func TestReviewMessageRejectsSMTPHeaderInjectionAndOverlongSubject(t *testing.T) {
	for _, subject := range []string{"Subject\r\nBcc: hidden@example.test", "Subject\nAnother header", strings.Repeat("x", 257)} {
		if (ReviewMessage{Subject: subject, Body: "Reviewed body"}).valid() {
			t.Fatal("invalid personal-message header approved", subject)
		}
	}
	if !(ReviewMessage{Subject: "Reviewed owner reminder", Body: "Hello {{recipient.name}}; {{outcome.url}} at {{review.date}}"}).valid() {
		t.Fatal("valid owner-only message denied")
	}
}
