package automation

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestGenericApprovedOutcomeReviewCancellationAndRescheduleRequireFreshApproval(t *testing.T) {
	f := newFixture(t)
	_, relay := installProviders(t, f)
	if err := dbgen.New(f.pool).
		ConfirmPersonEmail(t.Context(), dbgen.ConfirmPersonEmailParams{PersonID: f.person.ID, Mailbox: "outcome-owner@example.test"}); err != nil {
		t.Fatal(err)
	}
	// This is the existing generic approved outcome-review contract. It is not
	// evidence that the future117 production owner/publication journey exists.
	// Keep the whole controlled journey in the real past so the generic Jobs
	// due-time barrier is exercised without sleeps or moving time backwards.
	binding := f.retainedBindingAt(f.now)
	// Review a bounded window spanning this synthetic clock journey and the
	// worker's real-clock retained-work barrier; do not bypass that SQL policy.
	window := binding.Definition
	window.MaximumLatenessSeconds = 4 * 3600
	page, _ := f.change(
		Command{
			Action:     "save",
			Version:    binding.Version,
			Definition: &window,
			Reason:     "Review a bounded four-hour follow-up execution window",
		},
	)
	page, _ = f.change(
		Command{
			Action:    "approve",
			BindingID: binding.Definition.ID,
			Version:   page.Bindings[0].Version,
			Reason:    "Explicitly approve the bounded follow-up window",
		},
	)
	binding = page.Bindings[0]
	old := f.maintain().Occurrences[0]
	f.change(
		Command{
			Action:       "cancel_occurrence",
			OccurrenceID: old.ID,
			Reason:       "Explicitly cancel the old reviewed follow-up time",
		},
	)
	current, err := f.s.Composer.Campaign(t.Context(), f.owner.ID, f.id, f.campaignID)
	if err != nil {
		t.Fatal(err)
	}
	changed := rescheduleOutcomeReviewCampaign(t, f, current, f.now.Add(3*time.Minute))
	f.campaignVersion = changed.Campaign.Campaign.Version
	updatedBinding := binding.Definition
	updatedBinding.CampaignVersion = f.campaignVersion
	page, _ = f.change(
		Command{
			Action:     "save",
			Version:    binding.Version,
			Definition: &updatedBinding,
			Reason:     "Bind exact revised outcome-review campaign",
		},
	)
	version := page.Bindings[0].Version
	if _, _, err = f.s.Change(
		t.Context(),
		f.owner.ID,
		f.id,
		f.revision(),
		Command{
			Action:    "approve",
			BindingID: binding.Definition.ID,
			Version:   version,
			Reason:    "An automation approval cannot substitute for missing campaign approval",
		},
	); err == nil {
		t.Fatal("unapproved reschedule gained execution authority")
	}
	f.now = f.now.Add(time.Minute)
	f.execute(old.ID)
	if relay.calls.Load() != 0 {
		t.Fatal("cancelled original follow-up sent during reschedule")
	}
	for _, action := range []string{"preview", "approve"} {
		changed, err = f.s.Composer.ChangeCampaign(
			t.Context(),
			f.owner.ID,
			f.id,
			f.campaignID,
			f.revision(),
			communications.CampaignCommand{
				Action:  action,
				Version: f.campaignVersion,
				Reason:  "Explicitly review revised message audience and timing",
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		f.campaignVersion = changed.Campaign.Campaign.Version
	}
	// Campaign preview/approval changes version; pin the actual approved version.
	updatedBinding.CampaignVersion = f.campaignVersion
	page, _ = f.change(
		Command{
			Action:     "save",
			Version:    version,
			Definition: &updatedBinding,
			Reason:     "Select actual newly approved campaign version",
		},
	)
	version = page.Bindings[0].Version
	f.change(
		Command{
			Action:    "approve",
			BindingID: binding.Definition.ID,
			Version:   version,
			Reason:    "Approve the explicit replacement follow-up time",
		},
	)
	page = f.maintain()
	var replacement uuid.UUID
	for _, o := range page.Occurrences {
		if o.ID != old.ID && o.State != "cancelled" {
			replacement = o.ID
		}
	}
	if replacement == uuid.Nil {
		t.Fatal("approved reschedule did not create an independent current occurrence")
	}
	f.now = f.now.Add(2 * time.Minute)
	f.execute(old.ID)
	f.execute(replacement)
	f.execute(replacement)
	var op uuid.UUID
	if err = f.pool.QueryRow(t.Context(), "SELECT operation_id FROM smtp_campaign_messages WHERE occurrence_id=$1", replacement).
		Scan(&op); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err = f.s.Jobs.Execute(t.Context(), op); err != nil {
			t.Fatal(err)
		}
	}
	if relay.calls.Load() != 1 {
		t.Fatal("reschedule duplicated or omitted the one current approved follow-up")
	}
	var oldIntents int
	if err = f.pool.QueryRow(t.Context(), "SELECT count(*) FROM smtp_campaign_messages WHERE occurrence_id=$1", old.ID).
		Scan(&oldIntents); err != nil ||
		oldIntents != 0 {
		t.Fatal("cancelled follow-up acquired a provider intent", err)
	}
}

// rescheduleOutcomeReviewCampaign retains the original native save/approval seam.
func rescheduleOutcomeReviewCampaign(t *testing.T, f *fixture, current communications.CampaignDetail, target time.Time) communications.CampaignDetail {
	t.Helper()
	definition := current.Campaign.Campaign.Definition
	definition.Timing.WallTime, definition.Timing.DSTChoice = testdb.WallTimeForInstant(t, target, current.Timezone)
	changed, err := f.s.Composer.ChangeCampaign(
		t.Context(), f.owner.ID, f.id, f.campaignID, f.revision(),
		communications.CampaignCommand{Action: "save", Version: f.campaignVersion, Definition: &definition, Reason: "Propose a later follow-up without implicitly approving it"},
	)
	if err != nil {
		var blocked *event.Blocked
		if errors.As(err, &blocked) {
			t.Fatalf("native reschedule: %v; blockers=%+v", err, blocked.Issues)
		}
		t.Fatal(err)
	}
	return changed
}
