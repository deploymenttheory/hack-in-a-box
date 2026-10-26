package loadprofile

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/submissionfreeze"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
)

func TestURLCutoffRejectsSubstitutedAcceptedFormalAndInventedCommit(t *testing.T) {
	eventID, teamID, entryID := uuid.New(), uuid.New(), uuid.New()
	cutoff := time.Date(2026, time.September, 25, 16, 0, 0, 0, time.UTC)
	revision := int64(1)
	original := submissions.Page{
		EntryID: entryID,
		TeamID:  teamID,
		Final: &submissions.Revision{
			Revision: 1,
			ActorID:  uuid.New(),
			Document: submissions.Document{
				Evidence: []submissions.Evidence{{Kind: "url", URL: "https://example.test/demo"}},
			},
		},
	}
	native := original
	native.Outcome = "frozen"
	native.FrozenAt = &cutoff
	capture := func() submissionfreeze.Page {
		return submissionfreeze.Page{
			Status:         "not_applicable",
			BindingCurrent: true,
			Source: &submissionfreeze.Source{
				ID:                 uuid.New(),
				EventID:            eventID,
				TeamID:             teamID,
				EntryID:            entryID,
				NativeOutcome:      "frozen",
				Cutoff:             cutoff,
				Status:             "not_applicable",
				SubmissionRevision: &revision,
			},
		}
	}
	if err := validateURLCutoff(
		eventID,
		teamID,
		cutoff,
		native,
		capture(),
		original,
		true,
	); err != nil {
		t.Fatal(err)
	}
	for name, alter := range map[string]func(*submissions.Page, *submissionfreeze.Page){
		"later-native-revision": func(n *submissions.Page, _ *submissionfreeze.Page) {
			copy := *n.Final
			copy.Revision++
			n.Final = &copy
		},
		"different-native-evidence": func(n *submissions.Page, _ *submissionfreeze.Page) {
			copy := *n.Final
			copy.Evidence = []submissions.Evidence{{Kind: "url", URL: "https://example.test/replacement"}}
			n.Final = &copy
		},
		"wrong-cutoff":        func(_ *submissions.Page, c *submissionfreeze.Page) { c.Source.Cutoff = cutoff.Add(time.Second) },
		"wrong-entry":         func(_ *submissions.Page, c *submissionfreeze.Page) { c.Source.EntryID = uuid.New() },
		"not-current":         func(_ *submissions.Page, c *submissionfreeze.Page) { c.BindingCurrent = false },
		"invented-repository": func(_ *submissions.Page, c *submissionfreeze.Page) { c.Source.RepositoryID = 101 },
		"invented-observation": func(_ *submissions.Page, c *submissionfreeze.Page) {
			c.Observations = []submissionfreeze.Observation{{SHA: "abcdef"}}
		},
		"invented-commit": func(_ *submissions.Page, c *submissionfreeze.Page) {
			c.Current = &submissionfreeze.Decision{SHA: "abcdef"}
		},
		"missing-accepted-formal": func(n *submissions.Page, c *submissionfreeze.Page) {
			n.Outcome = "not_submitted"
			n.Final = nil
			c.Source.NativeOutcome = "not_submitted"
			c.Source.Status = "not_submitted"
			c.Source.SubmissionRevision = nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			n, c := native, capture()
			alter(&n, &c)
			if err := validateURLCutoff(eventID, teamID, cutoff, n, c, original, true); err == nil {
				t.Fatal("substituted accepted URL source passed")
			}
		})
	}
	missing := submissions.Page{
		EntryID:  uuid.New(),
		TeamID:   uuid.New(),
		Outcome:  "not_submitted",
		FrozenAt: &cutoff,
	}
	missingCapture := submissionfreeze.Page{
		Source: &submissionfreeze.Source{
			ID:            uuid.New(),
			EventID:       eventID,
			TeamID:        missing.TeamID,
			EntryID:       missing.EntryID,
			NativeOutcome: "not_submitted",
			Status:        "not_submitted",
			Cutoff:        cutoff,
		},
	}
	if err := validateURLCutoff(
		eventID,
		missing.TeamID,
		cutoff,
		missing,
		missingCapture,
		submissions.Page{},
		false,
	); err != nil {
		t.Fatal(err)
	}
	missingCapture.Source.SubmissionRevision = &revision
	if err := validateURLCutoff(
		eventID,
		missing.TeamID,
		cutoff,
		missing,
		missingCapture,
		submissions.Page{},
		false,
	); err == nil {
		t.Fatal("missing entry invented a formal revision")
	}
}
