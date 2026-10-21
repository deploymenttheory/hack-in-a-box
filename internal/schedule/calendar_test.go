package schedule_test

import (
	"errors"
	"testing"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/schedule"
)

func instant(t *testing.T, value string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestPublicationUsesCalendarDaysAcrossDST(t *testing.T) {
	for _, tc := range []struct {
		start, want string
		hours       time.Duration
	}{{"2026-03-31T10:00:00+01:00", "2026-03-24T10:00:00Z", 167}, {"2026-10-28T10:00:00Z", "2026-10-21T10:00:00+01:00", 169}} {
		live := instant(t, tc.start)
		publication, err := schedule.ChallengePublication(live, "Europe/London", "")
		if err != nil || !publication.Equal(instant(t, tc.want)) ||
			live.Sub(publication) != tc.hours*time.Hour {
			t.Fatalf("calendar publication: %v %v", publication, err)
		}
	}
}

func TestLocalTimeRequiresExplicitFoldAndRejectsGaps(t *testing.T) {
	if _, err := schedule.ResolveLocal(
		"2026-03-29T01:30:00",
		"Europe/London",
		"",
	); !errors.Is(
		err,
		schedule.ErrLocalTime,
	) {
		t.Fatalf("nonexistent time: %v", err)
	}
	if _, err := schedule.ResolveLocal(
		"2026-10-25T01:30:00",
		"Europe/London",
		"",
	); !errors.Is(
		err,
		schedule.ErrAmbiguous,
	) {
		t.Fatalf("ambiguous time: %v", err)
	}
	earlier, err := schedule.ResolveLocal("2026-10-25T01:30:00", "Europe/London", "earlier")
	if err != nil {
		t.Fatal(err)
	}
	later, err := schedule.ResolveLocal("2026-10-25T01:30:00", "Europe/London", "later")
	if err != nil || later.Sub(earlier) != time.Hour {
		t.Fatalf("explicit fold: %v %v", later, err)
	}
}

func TestBusinessCalendarSkipsWeekendsAndExcludedDates(t *testing.T) {
	c := schedule.Calendar{WorkingDays: []int{1, 2, 3, 4, 5}, ExcludedDates: []string{"2026-10-23"}}
	actual, err := c.Offset(instant(t, "2026-10-26T10:00:00Z"), "Europe/London", -1, "")
	if err != nil || !actual.Equal(instant(t, "2026-10-22T10:00:00+01:00")) {
		t.Fatalf("business cutoff: %v %v", actual, err)
	}
	c.WorkingDays = nil
	if _, err := c.Offset(time.Now(), "UTC", 1, ""); !errors.Is(err, schedule.ErrCalendar) {
		t.Fatal("empty business week accepted")
	}
}
