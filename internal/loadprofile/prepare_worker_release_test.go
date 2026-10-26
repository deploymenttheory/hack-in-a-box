package loadprofile

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/event"
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

func TestKnownNativePublicationRequiresExactTwoEventRevisions(t *testing.T) {
	plan, err := NewReleaseFactoryPlan(
		uuid.New(),
		time.Date(2026, time.October, 26, 12, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	planned := plan.Events[4]
	owner := uuid.New()
	before := event.View{
		ID:                 planned.Input.ID,
		Revision:           10,
		Stage:              "preparation",
		OwnerID:            &owner,
		ConfirmedAttendees: 180,
		Configuration:      planned.Input.Configuration,
	}
	source := PreparedEventSources{Event: before}
	if !releaseCurrentMatches(before, source, planned, owner, false) {
		t.Fatal("original unchanged source rejected")
	}
	publication := planned.Input.Configuration.LiveStart.AddDate(0, 0, -7)
	published := before
	published.PublishedAt = &publication
	published.Revision = 12
	if !releaseCurrentMatches(published, source, planned, owner, true) {
		t.Fatal("actual atomic rubric plus challenge publication rejected")
	}
	if releaseCurrentMatches(published, source, planned, owner, false) {
		t.Fatal("publication inferred without known acknowledged control")
	}
	for name, alter := range map[string]func(*event.View){
		"only-one-bump":        func(v *event.View) { v.Revision = 11 },
		"unrelated-third-bump": func(v *event.View) { v.Revision = 13 },
		"changed-date": func(v *event.View) {
			v.Configuration.SubmissionCutoff = v.Configuration.SubmissionCutoff.Add(time.Second)
		},
		"changed-owner":             func(v *event.View) { id := uuid.New(); v.OwnerID = &id },
		"later-stage":               func(v *event.View) { v.Stage = "live" },
		"wrong-publication-instant": func(v *event.View) { at := publication.Add(time.Second); v.PublishedAt = &at },
	} {
		t.Run(name, func(t *testing.T) {
			current := published
			alter(&current)
			if releaseCurrentMatches(current, source, planned, owner, true) {
				t.Fatal("changed native source retargeted continuation")
			}
		})
	}
}
