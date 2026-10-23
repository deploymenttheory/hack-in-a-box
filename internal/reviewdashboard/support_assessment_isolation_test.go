package reviewdashboard

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/help"
	"github.com/deploymenttheory/hack-in-a-box/internal/observations"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestNativeHelpConcurrentOwnershipReopenAndOverdueNeverMutateAssessment(t *testing.T) {
	f := setup(t)
	board := dashboard(f)
	otherTeam, otherMember := f.journeyTeam("Isolated support team")
	for _, team := range []uuid.UUID{f.team, otherTeam} {
		if _, err := f.s.Change(
			t.Context(),
			f.primary.ID,
			f.event,
			team,
			f.revision(),
			f.command(f.checkpoint, "submit", false),
		); err != nil {
			t.Fatal(err)
		}
	}
	notes := observations.Service{Pool: f.pool, Now: func() time.Time { return f.now }}
	if _, err := notes.Change(
		t.Context(),
		f.primary.ID,
		f.event,
		f.team,
		f.revision(),
		observations.Command{
			Action: "observation",
			ID:     uuid.New(),
			Evidence: observations.Evidence{
				Category: "demonstration",
				Evidence: "PRIVATE-ASSESSMENT-UNCHANGED-BY-SUPPORT",
				Outcome:  "Unobserved categories remain pending",
			},
		},
	); err != nil {
		t.Fatal(err)
	}
	assessment := f.assessmentSnapshot()
	unchanged := func() {
		t.Helper()
		if current := f.assessmentSnapshot(); current != assessment {
			t.Fatal(
				"operational help changed private observations, independent cards, native runs or moderation proofs",
			)
		}
	}
	create := func(actor, team uuid.UUID, description string) (help.Page, error) {
		return board.Help.Create(
			t.Context(),
			actor,
			f.event,
			uuid.New(),
			help.Input{
				TeamID:      team,
				Category:    "competition_output",
				Description: description,
				Location:    "Synthetic support desk",
				CommandID:   uuid.New(),
			},
		)
	}
	p, err := create(f.member.ID, f.team, "PRIVATE-HELP-REQUEST-A")
	if err != nil {
		t.Fatal(err)
	}
	other, err := create(otherMember.ID, otherTeam, "PRIVATE-HELP-REQUEST-B")
	if err != nil {
		t.Fatal(err)
	}
	unchanged()
	result := make(chan error, 2)
	var wg sync.WaitGroup
	for _, actor := range []uuid.UUID{f.primary.ID, f.second.ID} {
		wg.Go(func() {
			_, claimErr := board.Help.Change(
				t.Context(),
				actor,
				f.event,
				p.Request.ID,
				p.Request.Revision,
				help.Command{Action: "claim", CommandID: uuid.New()},
			)
			result <- claimErr
		})
	}
	wg.Wait()
	close(result)
	won, lost := 0, 0
	for claimErr := range result {
		switch {
		case claimErr == nil:
			won++
		case errors.Is(claimErr, access.ErrConflict):
			lost++
		default:
			t.Fatal(claimErr)
		}
	}
	if won != 1 || lost != 1 {
		t.Fatal("concurrent current helper claims were not unique", won, lost)
	}
	current, err := board.Help.Read(t.Context(), f.owner.ID, f.event, p.Request.ID)
	if err != nil || current.Request.AssigneeID == nil {
		t.Fatal(current, err)
	}
	winner := *current.Request.AssigneeID
	replacement := f.primary.ID
	if winner == replacement {
		replacement = f.second.ID
	}
	current, err = board.Help.Change(
		t.Context(),
		winner,
		f.event,
		p.Request.ID,
		current.Request.Revision,
		help.Command{Action: "transfer", AssigneeID: &replacement, CommandID: uuid.New()},
	)
	if err != nil || current.Request.AssigneeID == nil ||
		*current.Request.AssigneeID != replacement {
		t.Fatal("native explicit transfer missing", current, err)
	}
	transferredRevision := current.Request.Revision
	f.exec(
		`UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='mentor'`,
		f.event,
		replacement,
	)
	maintain := func() {
		t.Helper()
		if maintainErr := database.Within(
			t.Context(),
			f.pool,
			func(tx pgx.Tx, _ *dbgen.Queries) error {
				return board.Help.MaintainInTransaction(t.Context(), tx, f.event)
			},
		); maintainErr != nil {
			t.Fatal(maintainErr)
		}
	}
	maintain()
	maintain()
	current, err = board.Help.Read(t.Context(), f.owner.ID, f.event, p.Request.ID)
	if err != nil || current.Request.Status != "waiting" || current.Request.AssigneeID != nil ||
		current.Request.Revision != transferredRevision+1 {
		t.Fatal("current helper loss not reconciled exactly once", current, err)
	}
	if _, err = board.Help.Change(
		t.Context(),
		replacement,
		f.event,
		p.Request.ID,
		current.Request.Revision,
		help.Command{Action: "claim", CommandID: uuid.New()},
	); !errors.Is(err, access.ErrForbidden) &&
		!errors.Is(err, access.ErrNotFound) {
		t.Fatal("revoked helper claimed support", err)
	}
	f.now = f.now.Add(21 * time.Minute)
	maintain()
	maintain()
	current, err = board.Help.Read(t.Context(), f.owner.ID, f.event, p.Request.ID)
	if err != nil {
		t.Fatal(err)
	}
	var followups int
	if err = f.pool.QueryRow(t.Context(), `SELECT count(*) FROM help_followups WHERE event_id=$1 AND request_id=$2`, f.event, p.Request.ID).
		Scan(&followups); err != nil ||
		followups != 1 {
		t.Fatal("same waiting cycle overdue was lost/duplicated", followups, err)
	}
	current, err = board.Help.Change(
		t.Context(),
		winner,
		f.event,
		p.Request.ID,
		current.Request.Revision,
		help.Command{Action: "claim", CommandID: uuid.New()},
	)
	if err != nil {
		t.Fatal(err)
	}
	current, err = board.Help.Change(
		t.Context(),
		winner,
		f.event,
		p.Request.ID,
		current.Request.Revision,
		help.Command{Action: "resolve", CommandID: uuid.New()},
	)
	if err != nil {
		t.Fatal(err)
	}
	priorCycle := current.Request.WaitingCycle
	current, err = board.Help.Change(
		t.Context(),
		f.member.ID,
		f.event,
		p.Request.ID,
		current.Request.Revision,
		help.Command{Action: "reopen", CommandID: uuid.New()},
	)
	if err != nil || current.Request.WaitingCycle != priorCycle+1 {
		t.Fatal("requester reopen did not create explicit new waiting cycle", current, err)
	}
	f.now = f.now.Add(21 * time.Minute)
	maintain()
	maintain()
	if err = f.pool.QueryRow(t.Context(), `SELECT count(*) FROM help_followups WHERE event_id=$1 AND request_id=$2`, f.event, p.Request.ID).
		Scan(&followups); err != nil ||
		followups != 2 {
		t.Fatal("new waiting cycle overdue missing/duplicated", followups, err)
	}
	for _, probe := range []struct{ actor, event, request uuid.UUID }{{otherMember.ID, f.event, p.Request.ID}, {f.member.ID, f.event, other.Request.ID}, {f.owner.ID, testdb.Event(t, f.pool, f.owner.ID), p.Request.ID}} {
		if _, err = board.Help.Read(
			t.Context(),
			probe.actor,
			probe.event,
			probe.request,
		); err == nil {
			t.Fatal("cross-team/event help disclosure", probe)
		}
	}
	if _, err = create(
		otherMember.ID,
		f.team,
		"Cannot impersonate another team",
	); !errors.Is(err, access.ErrForbidden) &&
		!errors.Is(err, access.ErrNotFound) {
		t.Fatal("other team created request for this team", err)
	}
	unchanged()
}
