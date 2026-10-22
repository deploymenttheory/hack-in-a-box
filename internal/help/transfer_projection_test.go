package help

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
)

func TestHelpCurrentMentorDeactivationCannotClaimOrKeepAssignment(t *testing.T) {
	f := newFixture(t)
	p := f.create("setup")
	p = f.change(f.helper.ID, p, "claim")
	f.exec(
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='mentor'",
		f.eventID,
		f.helper.ID,
	)
	current, err := f.s.Read(t.Context(), f.member.ID, f.eventID, p.Request.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Request.Status != "waiting" || current.Request.AssigneeID != nil {
		t.Fatal("deactivated mentor kept the help claim")
	}
	_, err = f.s.Change(
		t.Context(),
		f.helper.ID,
		f.eventID,
		current.Request.ID,
		current.Request.Revision,
		Command{Action: "claim", CommandID: uuid.New()},
	)
	if !errors.Is(err, access.ErrNotFound) {
		t.Fatalf("deactivated mentor claim: %v", err)
	}
}

func TestHelpTransferTargetsAreCurrentNativeHelpersAndNeverARequesterDirectory(t *testing.T) {
	f := newFixture(t)
	p := f.change(f.helper.ID, f.create("setup"), "claim")
	contains := func(p Page, id uuid.UUID) bool {
		for _, target := range p.TransferTargets {
			if target.PersonID == id {
				return true
			}
		}
		return false
	}
	owner, err := f.s.Read(t.Context(), f.helper.ID, f.eventID, p.Request.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !owner.CanTransfer || !contains(owner, f.second.ID) || !contains(owner, f.logistics.ID) ||
		contains(owner, f.member.ID) ||
		contains(owner, f.helper.ID) {
		t.Fatal("incorrect native transfer targets")
	}
	for _, target := range owner.TransferTargets {
		if target.Name == "" {
			t.Fatal("missing current helper display name")
		}
	}
	member, err := f.s.Read(t.Context(), f.member.ID, f.eventID, p.Request.ID)
	if err != nil || member.CanTransfer || len(member.TransferTargets) != 0 ||
		member.AssigneeName != f.helper.DisplayName {
		t.Fatal("requester enumerated helper directory or missing current owner")
	}
	unassigned, err := f.s.Read(t.Context(), f.second.ID, f.eventID, p.Request.ID)
	if err != nil || unassigned.CanTransfer || len(unassigned.TransferTargets) != 0 {
		t.Fatal("non-owner helper enumerated targets")
	}
	f.exec("UPDATE people SET active=false WHERE id=$1", f.second.ID)
	current, err := f.s.Read(t.Context(), f.helper.ID, f.eventID, p.Request.ID)
	if err != nil || contains(current, f.second.ID) {
		t.Fatal("inactive person remained a transfer target")
	}
	_, err = f.s.Change(
		t.Context(),
		f.helper.ID,
		f.eventID,
		p.Request.ID,
		p.Request.Revision,
		Command{Action: "transfer", AssigneeID: &f.second.ID, CommandID: uuid.New()},
	)
	if !errors.Is(err, access.ErrInvalid) {
		t.Fatalf("stale target accepted: %v", err)
	}
	f.exec(
		"INSERT INTO mentor_conflicts(event_id,mentor_id,competitor_id,kind)VALUES($1,$2,$3,'reporting')",
		f.eventID,
		f.helper.ID,
		f.member.ID,
	)
	_, err = f.s.Read(t.Context(), f.helper.ID, f.eventID, p.Request.ID)
	if !errors.Is(err, access.ErrNotFound) {
		t.Fatalf("conflicted actor retained private transfer projection: %v", err)
	}
	member, err = f.s.Read(t.Context(), f.member.ID, f.eventID, p.Request.ID)
	if err != nil || member.Request.AssigneeID != nil || member.Request.Status != "waiting" {
		t.Fatal("conflicted assignee not reconciled")
	}
}

func TestHelpRequestContextExposesOnlyCurrentConfirmedPublishedOwnTeam(t *testing.T) {
	f := newFixture(t)
	p, err := f.s.List(t.Context(), f.member.ID, f.eventID)
	if err != nil || !p.CanRequest || p.RequestTeam == nil || p.RequestTeam.ID != f.team {
		t.Fatal("own confirmed team missing")
	}
	other, err := f.s.List(t.Context(), f.other.ID, f.eventID)
	if err != nil || other.RequestTeam == nil || other.RequestTeam.ID != f.otherTeam {
		t.Fatal("request context crossed team")
	}
	helper, err := f.s.List(t.Context(), f.helper.ID, f.eventID)
	if err != nil || helper.CanRequest || helper.RequestTeam != nil {
		t.Fatal("helper received requester team directory")
	}
	f.exec(
		"UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2",
		f.eventID,
		f.member.ID,
	)
	p, err = f.s.List(t.Context(), f.member.ID, f.eventID)
	if (err != nil && !errors.Is(err, access.ErrForbidden) && !errors.Is(err, access.ErrNotFound)) ||
		p.CanRequest ||
		p.RequestTeam != nil {
		t.Fatal("withdrawn person retained request capability")
	}
}

func TestHelpInactivePersonCannotReadOrKeepCurrentClaim(t *testing.T) {
	f := newFixture(t)
	p := f.change(f.helper.ID, f.create("setup"), "claim")
	f.exec("UPDATE people SET active=false WHERE id=$1", f.helper.ID)
	_, err := f.s.Read(t.Context(), f.helper.ID, f.eventID, p.Request.ID)
	if !errors.Is(err, access.ErrNotFound) {
		t.Fatalf("inactive person read: %v", err)
	}
	current, err := f.s.Read(t.Context(), f.member.ID, f.eventID, p.Request.ID)
	if err != nil || current.Request.AssigneeID != nil || current.Request.Status != "waiting" {
		t.Fatal("inactive person's assignment was not released")
	}
}
