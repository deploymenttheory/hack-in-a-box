package automation

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/schedule"
)

func TestOutcomeReviewFixtureReschedulePreservesExactFoldInstant(t *testing.T) {
	for _, clock := range []time.Time{
		time.Date(2026, 10, 24, 23, 58, 30, 0, time.UTC), // unique initial target, ambiguous replacement
		time.Date(2026, 10, 25, 0, 58, 30, 0, time.UTC),  // earlier initial target, later replacement
	} {
		t.Run(clock.Format(time.RFC3339), func(t *testing.T) {
			f := newFixture(t)
			f.retainedBindingAt(clock)
			current, err := f.s.Composer.Campaign(t.Context(), f.owner.ID, f.id, f.campaignID)
			if err != nil {
				t.Fatal(err)
			}
			target := clock.Add(3 * time.Minute)
			changed := rescheduleOutcomeReviewCampaign(t, f, current, target)
			if changed.Campaign.DueAt == nil || !changed.Campaign.DueAt.Equal(target) || changed.Campaign.Status != "draft" {
				t.Fatalf("reschedule did not retain exact unapproved target %s: %+v", target, changed.Campaign)
			}
		})
	}
}

func TestCommonChallengeFixturePreservesNativeFoldPublication(t *testing.T) {
	for _, clock := range []time.Time{
		time.Date(2026, 10, 25, 0, 29, 0, 0, time.UTC),  // earlier repeated 01:30
		time.Date(2026, 10, 25, 1, 29, 0, 0, time.UTC),  // later repeated 01:30
		time.Date(2026, 10, 25, 0, 59, 30, 0, time.UTC), // one-minute target crosses the clock rollback
	} {
		t.Run(clock.Format(time.RFC3339), func(t *testing.T) {
			f := newFixture(t)
			f.now = clock
			_, service, challenge, _ := prepareNativeCommonChallenge(t, f)
			target := clock.Add(time.Minute)
			if challenge.Document.ScheduledAt == nil || !challenge.Document.ScheduledAt.Equal(target) {
				t.Fatalf("native common schedule changed exact target %s: %+v", target, challenge.Document.ScheduledAt)
			}
			row, err := dbgen.New(f.pool).Event(t.Context(), f.id)
			if err != nil {
				t.Fatal(err)
			}
			var configuration event.Configuration
			if err = json.Unmarshal(row.Configuration, &configuration); err != nil {
				t.Fatal(err)
			}
			if _, err = schedule.ChallengePublication(configuration.LiveStart, configuration.Timezone, ""); !errors.Is(err, schedule.ErrAmbiguous) {
				t.Fatalf("native repeated-hour validation changed: %v", err)
			}
			if err = service.Dispatch(t.Context(), f.id, challenge.ID, challenge.Revision); err != nil {
				t.Fatal(err)
			}
			row, err = dbgen.New(f.pool).Event(t.Context(), f.id)
			if err != nil || row.PublishedAt.Valid {
				t.Fatalf("challenge published before the exact common instant: %v %+v", err, row.PublishedAt)
			}
			f.now = target
			if err = service.Dispatch(t.Context(), f.id, challenge.ID, challenge.Revision); err != nil {
				t.Fatal(err)
			}
			row, err = dbgen.New(f.pool).Event(t.Context(), f.id)
			if err != nil || !row.PublishedAt.Valid || !row.PublishedAt.Time.Equal(target) {
				t.Fatalf("native guarded publication lost exact target %s: %v %+v", target, err, row.PublishedAt)
			}
		})
	}
}
