package maildelivery

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestPersonalSessionCalendarCancellationKeepsIdentityAndScrubsPayload(t *testing.T) {
	event, session, person := uuid.New(), uuid.New(), uuid.New()
	stamp := time.Date(2027, 10, 31, 12, 0, 0, 0, time.UTC)
	c := Calendar{
		EventID:      event,
		SessionID:    &session,
		PersonID:     &person,
		Sequence:     1,
		Start:        stamp,
		End:          stamp.Add(time.Hour),
		Timezone:     "Europe/London",
		Summary:      "PRIVATE session",
		WorkspaceURL: "https://example.test/PRIVATE-meeting",
	}
	first, err := c.render(settings(), stamp)
	if err != nil {
		t.Fatal(err)
	}
	c.Sequence = 2
	c.Start = c.Start.Add(time.Hour)
	c.End = c.End.Add(time.Hour)
	changed, err := c.render(settings(), stamp)
	if err != nil {
		t.Fatal(err)
	}
	uid := "UID:session-" + event.String() + "-" + session.String() + "-" + person.String() + "@example.test"
	if !strings.Contains(strings.ReplaceAll(string(first), "\r\n ", ""), uid) ||
		!strings.Contains(strings.ReplaceAll(string(changed), "\r\n ", ""), uid) ||
		!strings.Contains(string(changed), "SEQUENCE:2") {
		t.Fatal("session calendar identity or sequence lost")
	}
	other := c
	other.EventID = uuid.New()
	separate, err := other.render(settings(), stamp)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ReplaceAll(string(separate), "\r\n ", ""), uid) {
		t.Fatal("session calendar UID collided across different events")
	}
	c.Cancelled = true
	if _, err = c.render(settings(), stamp); err == nil {
		t.Fatal("cancellation accepted retained private payload")
	}
	c.Start = time.Time{}
	c.End = time.Time{}
	c.Timezone = ""
	c.Summary = ""
	c.WorkspaceURL = ""
	c.Sequence = 3
	cancelled, err := c.render(settings(), stamp)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{uid, "METHOD:CANCEL", "SEQUENCE:3", "STATUS:CANCELLED"} {
		if !strings.Contains(strings.ReplaceAll(string(cancelled), "\r\n ", ""), required) {
			t.Fatal("cancellation missing identity", required)
		}
	}
	for _, forbidden := range []string{"PRIVATE", "DTSTART", "DTEND", "SUMMARY", "DESCRIPTION", "URL:"} {
		if strings.Contains(string(cancelled), forbidden) {
			t.Fatal("cancellation leaked private metadata", forbidden)
		}
	}
}
