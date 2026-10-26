package app

import (
	"fmt"
	"testing"
	"time"
)

type lifecycleTimeline struct {
	Initial           time.Time `json:"initial"`
	MissedPublication time.Time `json:"missed_publication"`
	Publication       time.Time `json:"publication"`
	Live              time.Time `json:"live"`
	Cutoff            time.Time `json:"cutoff"`
	Completed         time.Time `json:"completed"`
	Day90             time.Time `json:"day_90"`
}

// Pick the latest genuine London DST transition whose complete 90-day follow-up
// is already in the past. River, authentication and leases keep real wall time;
// only native domain eligibility advances through this historical timeline.
// Expected T−7 is constructed independently of schedule.ChallengePublication.
func newLifecycleTimeline(wall time.Time) (lifecycleTimeline, error) {
	location, err := time.LoadLocation("Europe/London")
	if err != nil {
		return lifecycleTimeline{}, err
	}
	var selected lifecycleTimeline
	for year := wall.Year() - 2; year <= wall.Year(); year++ {
		for _, month := range []time.Month{time.March, time.October} {
			last := time.Date(year, month+1, 0, 9, 0, 0, 0, location)
			sunday := last.AddDate(0, 0, -int(last.Weekday()))
			live := sunday.AddDate(0, 0, 1)
			publication := live.AddDate(0, 0, -7)
			completed := live.Add(8 * time.Hour)
			day90 := completed.AddDate(0, 0, 90)
			if !day90.Before(wall.Add(-2*time.Hour)) || !live.After(selected.Live) {
				continue
			}
			selected = lifecycleTimeline{
				Initial: publication.AddDate(0, 0, -9), MissedPublication: publication.AddDate(0, 0, -7),
				Publication: publication, Live: live, Cutoff: live.Add(7 * time.Hour), Completed: completed, Day90: day90,
			}
		}
	}
	if selected.Live.IsZero() {
		return selected, fmt.Errorf("no complete historical DST lifecycle fits the wall-time anchor")
	}
	return selected, nil
}

func TestLifecycleCalendarFixtureActuallyCrossesDST(t *testing.T) {
	for _, test := range []struct {
		wall, publication, live string
		hours                   time.Duration
	}{
		{"2026-10-25T18:00:00Z", "2026-03-23T09:00:00Z", "2026-03-30T08:00:00Z", 167},
		{"2026-02-15T18:00:00Z", "2025-10-20T08:00:00Z", "2025-10-27T09:00:00Z", 169},
	} {
		t.Run(test.wall, func(t *testing.T) {
			wall, err := time.Parse(time.RFC3339, test.wall)
			if err != nil {
				t.Fatal(err)
			}
			value, err := newLifecycleTimeline(wall)
			if err != nil {
				t.Fatal(err)
			}
			if value.Publication.UTC().Format(time.RFC3339) != test.publication || value.Live.UTC().Format(time.RFC3339) != test.live ||
				value.Live.Sub(value.Publication) != test.hours*time.Hour || value.Live.Hour() != 9 || value.Publication.Hour() != 9 {
				t.Fatal("fixture did not preserve the independent seven-local-calendar-day DST oracle", value)
			}
			if !value.Day90.Before(wall) || !value.Initial.Before(value.MissedPublication) || !value.MissedPublication.Before(value.Publication) {
				t.Fatal("fixture clock cannot advance monotonically while leaving the native queue in wall time")
			}
		})
	}
}
