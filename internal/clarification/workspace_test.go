package clarification_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/clarification"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestWorkspaceDerivesOwnTeamAndStaffOnlyAudienceFromCurrentReleasedAuthority(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	for _, actor := range []uuid.UUID{f.owner, f.first} {
		if value, err := f.service.Workspace(
			ctx,
			actor,
			f.eventID,
		); !errors.Is(err, access.ErrNotFound) ||
			value.Source.ChallengeID != uuid.Nil {
			t.Fatal("unreleased workspace disclosed a source or capability", value, err)
		}
	}
	f.release()
	question, err := f.service.Ask(ctx, f.first, f.eventID, clarification.QuestionInput{
		TeamID: f.team, CommandID: uuid.New(), Topic: "challenge", Body: "Private source question",
	})
	if err != nil {
		t.Fatal(err)
	}
	review, err := f.service.Review(ctx, f.owner, f.eventID, question.ID)
	if err != nil || review.Draft != nil || !review.Workspace.CanReview ||
		review.Workspace.ReviewAudience == nil {
		t.Fatal("authorised empty draft was confused with denied review", review, err)
	}
	if _, err = f.service.Review(
		ctx,
		f.first,
		f.eventID,
		question.ID,
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatal("question author obtained staff draft review", err)
	}
	if _, err = f.service.Review(
		ctx,
		f.owner,
		f.eventID,
		uuid.New(),
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatal("review preview fabricated a missing question", err)
	}
	draft, err := f.service.SaveAnswer(
		ctx,
		f.owner,
		f.eventID,
		question.ID,
		0,
		clarification.AnswerDocument{
			Title: "Reviewable official draft",
			Body:  "Actual saved answer draft",
			Kind:  "clarification",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	review, err = f.service.Review(ctx, f.owner, f.eventID, question.ID)
	if err != nil || review.Draft == nil || review.Draft.ID != draft.ID ||
		review.Draft.Version != draft.Version ||
		review.Draft.Source != review.Workspace.Source {
		t.Fatal("staff preview did not retain the exact draft/current-source pair", review, err)
	}
	staff, err := f.service.Workspace(ctx, f.owner, f.eventID)
	if err != nil || !staff.CanReview || staff.CanAsk || staff.TeamID != nil ||
		staff.ReviewAudience == nil ||
		staff.ReviewAudience.Count != 3 ||
		staff.StaffAttachmentsOpen {
		t.Fatal("staff preview did not reflect current native publication audience", staff, err)
	}
	for _, member := range []uuid.UUID{f.first, f.peer, f.other} {
		own, ownErr := f.service.Workspace(ctx, member, f.eventID)
		if ownErr != nil || !own.CanAsk || own.CanReview || own.TeamID == nil ||
			own.ReviewAudience != nil ||
			own.Source != staff.Source {
			t.Fatal(
				"participant workspace disclosed staff counts or lost common source",
				own,
				ownErr,
			)
		}
		want := f.team
		if member == f.other {
			want = f.otherTeam
		}
		if *own.TeamID != want {
			t.Fatal("workspace substituted another team's question capability")
		}
	}
	mentor := testdb.Person(t, f.pool)
	f.exec(
		"INSERT INTO event_memberships(event_id,person_id,role) VALUES($1,$2,'mentor')",
		f.eventID,
		mentor.ID,
	)
	if _, err = f.service.Workspace(
		ctx,
		mentor.ID,
		f.eventID,
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatal("generic staff role obtained organiser question review or common early access", err)
	}
	f.exec(
		"UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2",
		f.eventID,
		f.other,
	)
	if _, err = f.service.Workspace(ctx, f.other, f.eventID); !errors.Is(err, access.ErrNotFound) {
		t.Fatal("withdrawn attendee retained workspace links or source", err)
	}
	staff, err = f.service.Workspace(ctx, f.owner, f.eventID)
	if err != nil || staff.ReviewAudience == nil || staff.ReviewAudience.Count != 2 {
		t.Fatal("review preview reused an obsolete confirmed audience", staff, err)
	}
	var raw []byte
	if err = f.pool.QueryRow(ctx, "SELECT configuration FROM events WHERE id=$1", f.eventID).
		Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var dates event.Configuration
	if err = json.Unmarshal(raw, &dates); err != nil {
		t.Fatal(err)
	}
	f.now = dates.LiveStart.Add(-time.Microsecond)
	staff, err = f.service.Workspace(ctx, f.owner, f.eventID)
	if err != nil || staff.StaffAttachmentsOpen {
		t.Fatal("staff attachment gate opened before actual live start", err)
	}
	f.now = dates.LiveStart
	staff, err = f.service.Workspace(ctx, f.owner, f.eventID)
	if err != nil || !staff.StaffAttachmentsOpen {
		t.Fatal("staff attachment capability did not open at live start", err)
	}
	f.exec("UPDATE events SET writes_allowed=false WHERE id=$1", f.eventID)
	member, err := f.service.Workspace(ctx, f.first, f.eventID)
	if err != nil || !member.Archived || member.CanAsk || member.TeamID == nil {
		t.Fatal("archive lost own historical reader or retained question writes", member, err)
	}
	f.exec("UPDATE people SET active=false WHERE id=$1", f.owner)
	if value, readErr := f.service.Review(
		ctx,
		f.owner,
		f.eventID,
		question.ID,
	); readErr == nil || value.Draft != nil ||
		value.Workspace.ReviewAudience != nil {
		t.Fatal("revoked staff retained saved draft or preview", readErr)
	}
	if value, readErr := f.service.Workspace(
		ctx,
		f.owner,
		f.eventID,
	); (!errors.Is(readErr, access.ErrNotFound) && !errors.Is(readErr, access.ErrForbidden)) ||
		value.ReviewAudience != nil {
		t.Fatal("revoked staff retained review preview", value, readErr)
	}
}
