package judging

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
)

func TestOrganiserReopenUsesCurrentMetadataWithoutPrivateCardAccess(t *testing.T) {
	f := setup(t)
	scores := f.scores(8)
	scores[0].Evidence[0].Note = "PRIVATE-JUDGE-NOTE-NOT-COORDINATION"
	if _, err := f.card(f.first.ID, 0, "submit", scores); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.Brief(
		t.Context(),
		f.owner.ID,
		f.event,
		f.run,
		f.team,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("organiser acquired private assessment brief", err)
	}
	for _, person := range []uuid.UUID{f.first.ID, f.member.ID} {
		if _, err := f.s.CardStates(
			t.Context(),
			person,
			f.event,
			f.run,
			f.team,
		); !errors.Is(
			err,
			access.ErrForbidden,
		) {
			t.Fatal("non-organiser read card coordination", err)
		}
	}
	page, err := f.s.CardStates(t.Context(), f.owner.ID, f.event, f.run, f.team)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(page)
	for _, forbidden := range []string{"scores", "evidence", "PRIVATE-JUDGE-NOTE", f.deliverable.String(), "sha", "source_id"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatal("metadata leaked private assessment/code", string(raw))
		}
	}
	var first CardState
	for _, item := range page.Items {
		if item.AuthorID == f.first.ID {
			first = item
		}
	}
	if first.Version != 1 || first.State != "submitted" || !first.CanReopen ||
		!first.SourceAvailable ||
		first.SourceChanged ||
		first.SubmissionRevision != 1 {
		t.Fatal("organiser lacks exact current reopen preconditions", first)
	}
	cmd := CardCommand{
		RequestID:            uuid.New(),
		Action:               "reopen",
		AuthorID:             first.AuthorID,
		SubmissionRevision:   first.SubmissionRevision,
		RepositoryDecisionID: first.RepositoryDecisionID,
		Reason:               "Reviewed correction of independently submitted assessment",
	}
	receipt, err := f.s.ChangeCard(
		t.Context(),
		f.owner.ID,
		f.event,
		first.RunID,
		first.TeamID,
		first.Version,
		cmd,
	)
	if err != nil || receipt.Revision != 2 || receipt.State != "draft" {
		t.Fatal("metadata could not drive reasoned exact-version reopen", receipt, err)
	}
	cmd.RequestID = uuid.New()
	if _, err = f.s.ChangeCard(
		t.Context(),
		f.owner.ID,
		f.event,
		first.RunID,
		first.TeamID,
		first.Version,
		cmd,
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatal("stale discovered metadata reopened newer card", err)
	}
	page, err = f.s.CardStates(t.Context(), f.owner.ID, f.event, f.run, f.team)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range page.Items {
		if item.AuthorID == f.first.ID &&
			(item.Version != 2 || item.State != "draft" || item.CanReopen) {
			t.Fatal("metadata not current after reopen", item)
		}
	}
	f.exec("UPDATE hack_teams SET state='disbanded' WHERE event_id=$1 AND id=$2", f.event, f.team)
	if _, err = f.s.CardStates(
		t.Context(),
		f.owner.ID,
		f.event,
		f.run,
		f.team,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("disbanded team retained reopen metadata", err)
	}
}
