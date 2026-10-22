package automation

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
)

func TestMissingOwningSourceAdapterCannotBeApproved(t *testing.T) {
	f := newFixture(t)
	f.campaign("reminder", "Safe literal reminder", true)
	for _, trigger := range []string{"help.updated", "clarification.published", "rubric.published", "results.published"} {
		t.Run(trigger, func(t *testing.T) {
			page, _ := f.change(
				Command{
					Action: "save",
					Definition: &Definition{
						ID:                     uuid.New(),
						CampaignID:             f.campaignID,
						CampaignVersion:        f.campaignVersion,
						Trigger:                trigger,
						MaximumLatenessSeconds: 60,
					},
					Reason: "Save incomplete lifecycle draft",
				},
			)
			var binding Binding
			for _, candidate := range page.Bindings {
				if candidate.Definition.Trigger == trigger {
					binding = candidate
				}
			}
			_, _, err := f.s.Change(
				t.Context(),
				f.owner.ID,
				f.id,
				f.revision(),
				Command{
					Action:    "approve",
					BindingID: binding.Definition.ID,
					Version:   binding.Version,
					Reason:    "Review source availability",
				},
			)
			if !errors.Is(err, communications.ErrUnresolved) {
				t.Fatalf("missing authoritative %s adapter allowed approval: %v", trigger, err)
			}
			current, err := f.s.Read(t.Context(), f.owner.ID, f.id)
			if err != nil {
				t.Fatal(err)
			}
			for _, saved := range current.Bindings {
				if saved.Definition.ID == binding.Definition.ID && saved.ApprovedAt != nil {
					t.Fatal("incomplete draft reported as approved")
				}
			}
		})
	}
}
