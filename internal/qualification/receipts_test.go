package qualification

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissionfreeze"
)

func receiptFixture(t *testing.T) (*fixture, Service, uuid.UUID) {
	t.Helper()
	f := setup(t)
	cp := f.configure("mentor_final")
	f.freeze()
	f.now = f.config.SubmissionCutoff
	return f, Service{
		Pool:       f.pool,
		Now:        func() time.Time { return f.now },
		Moderation: f.huddles(),
		Cards:      &f.s,
		Freeze:     &submissionfreeze.Service{Pool: f.pool, Now: func() time.Time { return f.now }},
	}, cp.Definition.RunID
}

func panelCommand(f *fixture) Command {
	return Command{
		RequestID: uuid.New(),
		Action:    "configure_panel",
		Reason:    "PRIVATE-PANEL-REVIEW-REASON",
		Panel: &Panel{
			MentorIDs:   []uuid.UUID{f.primary.ID, f.second.ID},
			OrganiserID: f.owner.ID,
		},
	}
}

func TestOriginalQualificationReceiptIsImmutablePrivateFreeAndNotAReadProjection(t *testing.T) {
	f, s, run := receiptFixture(t)
	cmd := panelCommand(f)
	expected := f.revision()
	first, err := s.Change(t.Context(), f.owner.ID, f.event, run, expected, cmd)
	if err != nil || first.Receipt == nil {
		t.Fatal(first, err)
	}
	next := Command{
		RequestID: uuid.New(),
		Version:   first.Round.Version,
		Action:    "propose",
		Reason:    "Readiness proposal does not confirm an earlier request",
	}
	current, err := s.Change(t.Context(), f.owner.ID, f.event, run, f.revision(), next)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := s.Change(t.Context(), f.owner.ID, f.event, run, expected, cmd)
	if err != nil || replay.Receipt == nil || *replay.Receipt != *first.Receipt ||
		replay.Round.Version != current.Round.Version {
		t.Fatal("original receipt was replaced with fresh state", replay, err)
	}
	fresh, err := s.Read(t.Context(), f.owner.ID, f.event, run)
	if err != nil || fresh.Receipt != nil {
		t.Fatal("fresh read claimed an original acknowledgement", fresh, err)
	}
	recovered, err := s.OriginalReceipt(t.Context(), f.owner.ID, f.event, run, cmd.RequestID)
	if err != nil || recovered.Receipt == nil || *recovered.Receipt != *first.Receipt {
		t.Fatal("immutable original lookup", recovered, err)
	}
	raw, _ := json.Marshal(recovered)
	for _, secret := range []string{"PRIVATE-PANEL", "scores", "evidence", "sha", "note"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("receipt contains private result payload", string(raw))
		}
	}
	changed := cmd
	changed.Reason = "A different reviewed intent"
	if _, err = s.Change(
		t.Context(),
		f.owner.ID,
		f.event,
		run,
		expected,
		changed,
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatal("same UUID accepted different intent", err)
	}
	if _, err = s.OriginalReceipt(
		t.Context(),
		f.primary.ID,
		f.event,
		run,
		cmd.RequestID,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("another panel actor read original receipt", err)
	}
	unknown, err := s.OriginalReceipt(t.Context(), f.owner.ID, f.event, run, uuid.New())
	if err != nil || unknown.Receipt != nil {
		t.Fatal("missing original was inferred from current state", unknown, err)
	}
	f.exec(
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2",
		f.event,
		f.owner.ID,
	)
	if _, err = s.OriginalReceipt(
		t.Context(),
		f.owner.ID,
		f.event,
		run,
		cmd.RequestID,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("receipt survived real role loss", err)
	}
}

func TestConcurrentQualificationCommandsRetainOneOriginalAndFenceStaleHeads(t *testing.T) {
	for _, sameRequest := range []bool{true, false} {
		t.Run(
			map[bool]string{true: "same-original", false: "competing-intents"}[sameRequest],
			func(t *testing.T) {
				f, s, run := receiptFixture(t)
				cmd := panelCommand(f)
				expected := f.revision()
				commands := []Command{cmd, cmd}
				if !sameRequest {
					commands[1].RequestID = uuid.New()
				}
				results := make([]Page, 2)
				errs := make([]error, 2)
				start := make(chan struct{})
				var wg sync.WaitGroup
				for index := range commands {
					wg.Go(func() {
						<-start
						results[index], errs[index] = s.Change(
							t.Context(),
							f.owner.ID,
							f.event,
							run,
							expected,
							commands[index],
						)
					})
				}
				close(start)
				wg.Wait()
				successes, conflicts := 0, 0
				for _, err := range errs {
					if err == nil {
						successes++
					} else if errors.Is(err, access.ErrConflict) {
						conflicts++
					} else {
						t.Fatal("unexpected concurrent command failure", err)
					}
				}
				if sameRequest &&
					(successes != 2 || results[0].Receipt == nil || results[1].Receipt == nil || *results[0].Receipt != *results[1].Receipt) {
					t.Fatal("same original was duplicated or lost", results, errs)
				}
				if !sameRequest && (successes != 1 || conflicts != 1) {
					t.Fatal("competing reviewed heads were both accepted", errs)
				}
				var rounds, receipts int
				if err := f.pool.QueryRow(t.Context(), "SELECT count(*) FROM mentor_qualification_rounds WHERE event_id=$1", f.event).
					Scan(&rounds); err != nil {
					t.Fatal(err)
				}
				if err := f.pool.QueryRow(t.Context(), "SELECT count(*) FROM mentor_qualification_commands WHERE event_id=$1", f.event).
					Scan(&receipts); err != nil {
					t.Fatal(err)
				}
				if rounds != 1 || receipts != 1 {
					t.Fatal(
						"concurrent commands created duplicate immutable effects",
						rounds,
						receipts,
					)
				}
			},
		)
	}
}
