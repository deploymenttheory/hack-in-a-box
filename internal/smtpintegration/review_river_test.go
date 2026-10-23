package smtpintegration

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/deliveryhistory"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/maildelivery"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func (f *fixture) approvePastProduction() {
	f.t.Helper()
	f.now = time.Now().UTC().AddDate(0, 0, -40)
	d := f.campaign.Definition
	d.Timing.Days = -32
	result, err := f.comm.ChangeCampaign(
		f.t.Context(),
		f.owner.ID,
		f.event,
		d.ID,
		f.revision(),
		communications.CampaignCommand{
			Action:     "save",
			Version:    f.campaign.Version,
			Definition: &d,
			Reason:     "Synthetic past-due real worker test",
		},
	)
	if err != nil {
		f.t.Fatal(err)
	}
	f.campaign = result.Campaign.Campaign
	f.approveProduction()
}

func TestCancelOnlyQueuedSMTPResendRetainsAcceptedOrUncertainOriginalWithoutEffect(t *testing.T) {
	for _, prior := range []maildelivery.State{maildelivery.Accepted, maildelivery.Ambiguous} {
		t.Run(string(prior), func(t *testing.T) {
			f := setup(t)
			f.approvePastProduction()
			f.transport.state = prior
			d := f.queueProduction()
			if err := f.worker.Execute(t.Context(), d.ID); err != nil {
				t.Fatal(err)
			}
			q := dbgen.New(f.pool)
			op, err := q.Operation(t.Context(), d.ID)
			if err != nil {
				t.Fatal(err)
			}
			history := deliveryhistory.Service{
				Pool:      f.pool,
				Jobs:      f.worker,
				Providers: map[string]deliveryhistory.RecoveryProvider{"smtp": f.service},
			}
			grant := deliveryhistory.Command{
				RequestID: uuid.New(),
				Action:    "resend_acknowledged",
				Reason:    "Synthetic queued resend",
			}
			queued, err := history.Change(
				t.Context(),
				f.owner.ID,
				f.event,
				d.ID,
				op.Revision,
				grant,
			)
			if err != nil {
				t.Fatal(err)
			}
			cancel := deliveryhistory.Command{
				RequestID: uuid.New(),
				Action:    "cancel_resend",
				Reason:    "Synthetic cancel only unspent resend",
			}
			stopped, err := history.Change(
				t.Context(),
				f.owner.ID,
				f.event,
				d.ID,
				queued.Item.Revision,
				cancel,
			)
			if err != nil || !stopped.Item.Cancelled || len(stopped.Reviews) != 2 {
				t.Fatalf("cancel queued resend=%+v err%v", stopped, err)
			}
			if stopped.Item.State != map[maildelivery.State]string{maildelivery.Accepted: "provider_accepted", maildelivery.Ambiguous: "ambiguous"}[prior] {
				t.Fatal("cancellation rewrote original provider outcome")
			}
			if _, err = history.Change(
				t.Context(),
				f.owner.ID,
				f.event,
				d.ID,
				queued.Item.Revision,
				cancel,
			); err != nil {
				t.Fatal(err)
			}
			if err = f.worker.Execute(t.Context(), d.ID); err != nil {
				t.Fatal(err)
			}
			if err = f.worker.Reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			if f.transport.count() != 1 {
				t.Fatal("cancelled resend caused another external effect")
			}
			var consumed int
			if err = f.pool.QueryRow(t.Context(), "SELECT count(*) FROM communication_delivery_resend_uses WHERE review_id=$1", grant.RequestID).
				Scan(&consumed); err != nil ||
				consumed != 0 {
				t.Fatal("cancelled grant was spent", err)
			}
		})
	}
}

func TestSMTPResendReviewerRevocationDeniesEffectWhileOriginalApproverRemainsCurrent(t *testing.T) {
	f := setup(t)
	reviewer := testdb.Person(t, f.pool)
	f.exec(
		"INSERT INTO event_memberships(event_id,person_id,role,active) VALUES($1,$2,'organiser',true)",
		f.event,
		reviewer.ID,
	)
	f.approvePastProduction()
	d := f.queueProduction()
	if err := f.worker.Execute(t.Context(), d.ID); err != nil {
		t.Fatal(err)
	}
	q := dbgen.New(f.pool)
	op, err := q.Operation(t.Context(), d.ID)
	if err != nil {
		t.Fatal(err)
	}
	history := deliveryhistory.Service{
		Pool:      f.pool,
		Jobs:      f.worker,
		Providers: map[string]deliveryhistory.RecoveryProvider{"smtp": f.service},
	}
	input := deliveryhistory.Command{
		RequestID: uuid.New(),
		Action:    "resend_acknowledged",
		Reason:    "Synthetic independently authorised reviewer",
	}
	if _, err = history.Change(
		t.Context(),
		reviewer.ID,
		f.event,
		d.ID,
		op.Revision,
		input,
	); err != nil {
		t.Fatal(err)
	}
	f.exec(
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2",
		f.event,
		reviewer.ID,
	)
	if err = f.worker.Execute(t.Context(), d.ID); err != nil {
		t.Fatal(err)
	}
	if f.transport.count() != 1 {
		t.Fatal("revoked resend reviewer bypassed current authority")
	}
	var consumed int
	if err = f.pool.QueryRow(t.Context(), "SELECT count(*) FROM communication_delivery_resend_uses WHERE review_id=$1", input.RequestID).
		Scan(&consumed); err != nil ||
		consumed != 0 {
		t.Fatal("revoked grant consumed", err)
	}
	var active bool
	if err = f.pool.QueryRow(t.Context(), "SELECT active FROM event_memberships WHERE event_id=$1 AND person_id=$2 AND role='organiser'", f.event, f.owner.ID).
		Scan(&active); err != nil ||
		!active {
		t.Fatal("test also revoked original approver", err)
	}
}

func TestDefiniteSMTPRetryNudgeRetainsOriginalApprovalAsExecutionAuthority(t *testing.T) {
	for _, revoke := range []string{"retry_operator", "original_approver"} {
		t.Run(revoke, func(t *testing.T) {
			f := setup(t)
			reviewer := testdb.Person(t, f.pool)
			f.exec(
				"INSERT INTO event_memberships(event_id,person_id,role,active) VALUES($1,$2,'organiser',true)",
				f.event,
				reviewer.ID,
			)
			f.approvePastProduction()
			f.transport.state = maildelivery.Retryable
			d := f.queueProduction()
			if err := f.worker.Execute(t.Context(), d.ID); !errors.Is(err, jobs.ErrRetry) {
				t.Fatalf("definite rejection=%v", err)
			}
			q := dbgen.New(f.pool)
			op, err := q.Operation(t.Context(), d.ID)
			if err != nil {
				t.Fatal(err)
			}
			history := deliveryhistory.Service{
				Pool:      f.pool,
				Jobs:      f.worker,
				Providers: map[string]deliveryhistory.RecoveryProvider{"smtp": f.service},
			}
			command := deliveryhistory.Command{
				RequestID: uuid.New(),
				Action:    "retry",
				Reason:    "Nudge an independently approved definite rejection",
			}
			if _, err = history.Change(
				t.Context(),
				reviewer.ID,
				f.event,
				d.ID,
				op.Revision,
				command,
			); err != nil {
				t.Fatal(err)
			}
			person := reviewer.ID
			if revoke == "original_approver" {
				person = f.owner.ID
			}
			f.exec(
				"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2",
				f.event,
				person,
			)
			f.transport.state = maildelivery.Accepted
			// Exercise the direct jobs path as well as reconciliation: neither may
			// treat the retry decision as replacement campaign authority.
			if err = f.worker.Execute(t.Context(), d.ID); err != nil {
				t.Fatal(err)
			}
			if err = f.worker.Reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			want := 2
			if revoke == "original_approver" {
				want = 1
			}
			if f.transport.count() != want {
				t.Fatalf("physical calls=%d want%d", f.transport.count(), want)
			}
			var uses int
			if err = f.pool.QueryRow(t.Context(), "SELECT count(*) FROM communication_delivery_resend_uses WHERE review_id=$1", command.RequestID).
				Scan(&uses); err != nil ||
				uses != 0 {
				t.Fatal("retry invented resend authority", err)
			}
		})
	}
}

func TestUnknownSMTPNoResendResolutionRetainsUncertaintyAndHasNoProviderEffect(t *testing.T) {
	f := setup(t)
	f.approvePastProduction()
	f.transport.state = maildelivery.Ambiguous
	d := f.queueProduction()
	if err := f.worker.Execute(t.Context(), d.ID); err != nil {
		t.Fatal(err)
	}
	op, err := dbgen.New(f.pool).Operation(t.Context(), d.ID)
	if err != nil {
		t.Fatal(err)
	}
	history := deliveryhistory.Service{
		Pool:      f.pool,
		Jobs:      f.worker,
		Providers: map[string]deliveryhistory.RecoveryProvider{"smtp": f.service},
	}
	input := deliveryhistory.Command{
		RequestID: uuid.New(),
		Action:    "resolve_no_resend",
		Reason:    "Synthetic operator chooses no additional effect",
	}
	detail, err := history.Change(t.Context(), f.owner.ID, f.event, d.ID, op.Revision, input)
	if err != nil || detail.Item.State != "ambiguous" || !detail.Item.Cancelled ||
		len(detail.Reviews) != 1 ||
		detail.Reviews[0].Action != "resolve_no_resend" {
		t.Fatalf("resolution %+v err%v", detail, err)
	}
	if _, err = history.Change(
		t.Context(),
		f.owner.ID,
		f.event,
		d.ID,
		op.Revision,
		input,
	); err != nil {
		t.Fatal(err)
	}
	if err = f.worker.Execute(t.Context(), d.ID); err != nil {
		t.Fatal(err)
	}
	if err = f.worker.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	if f.transport.count() != 1 {
		t.Fatal("no-resend resolution replayed")
	}
	var wire string
	if err = f.pool.QueryRow(t.Context(), "SELECT COALESCE(string_agg(row_to_json(a)::text,' '),'') FROM audit_events a WHERE event_id=$1", f.event).
		Scan(&wire); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(wire, f.transport.messages[0].Recipient) ||
		strings.Contains(wire, f.transport.messages[0].PlainText) {
		t.Fatal("provider address/body entered generic audit")
	}
}

func TestReviewedSMTPGrantCannotBypassChangedCurrentAuthorityMailboxProviderOrPause(t *testing.T) {
	for _, change := range []string{"role", "mailbox", "provider", "pause"} {
		t.Run(change, func(t *testing.T) {
			f := setup(t)
			f.approvePastProduction()
			d := f.queueProduction()
			if err := f.worker.Execute(t.Context(), d.ID); err != nil {
				t.Fatal(err)
			}
			q := dbgen.New(f.pool)
			op, err := q.Operation(t.Context(), d.ID)
			if err != nil {
				t.Fatal(err)
			}
			history := deliveryhistory.Service{
				Pool:      f.pool,
				Jobs:      f.worker,
				Providers: map[string]deliveryhistory.RecoveryProvider{"smtp": f.service},
			}
			input := deliveryhistory.Command{
				RequestID: uuid.New(),
				Action:    "resend_acknowledged",
				Reason:    "Synthetic explicitly reviewed resend",
			}
			if _, err = history.Change(
				t.Context(),
				f.owner.ID,
				f.event,
				d.ID,
				op.Revision,
				input,
			); err != nil {
				t.Fatal(err)
			}
			switch change {
			case "role":
				f.exec(
					"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2",
					f.event,
					f.owner.ID,
				)
			case "mailbox":
				f.exec(
					"UPDATE person_email_contacts SET mailbox='changed@example.test' WHERE person_id=$1",
					f.owner.ID,
				)
			case "provider":
				f.transport.revision = "changed-provider"
			case "pause":
				if err = q.SetAutomationPause(
					t.Context(),
					dbgen.SetAutomationPauseParams{
						EventID: f.event,
						Paused:  true,
						ActorID: f.owner.ID,
						Reason:  "Synthetic current pause",
					},
				); err != nil {
					t.Fatal(err)
				}
			}
			if err = f.worker.Execute(t.Context(), d.ID); err != nil {
				t.Fatal(err)
			}
			if f.transport.count() != 1 {
				t.Fatal("changed current native policy allowed resend")
			}
			var consumed int
			if err = f.pool.QueryRow(t.Context(), "SELECT count(*) FROM communication_delivery_resend_uses WHERE review_id=$1", input.RequestID).
				Scan(&consumed); err != nil ||
				consumed != 0 {
				t.Fatal("unexecuted grant consumed", err)
			}
			if change == "role" {
				if _, err = history.Detail(
					t.Context(),
					f.owner.ID,
					f.event,
					d.ID,
					0,
					nil,
				); !errors.Is(
					err,
					access.ErrForbidden,
				) {
					t.Fatal("revoked operator read history", err)
				}
			}
		})
	}
}

func waitReviewOperation(t *testing.T, f *fixture, id uuid.UUID, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		op, err := dbgen.New(f.pool).Operation(t.Context(), id)
		if err != nil {
			t.Fatal(err)
		}
		if op.State == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("operation did not reach %s", want)
}

func TestReviewedSMTPCommandRunsActualRiverAfterCompletedOriginalWithoutExtraReplay(t *testing.T) {
	for _, prior := range []maildelivery.State{maildelivery.Accepted, maildelivery.Ambiguous} {
		t.Run(string(prior), func(t *testing.T) {
			f := setup(t)
			f.approvePastProduction()
			f.transport.state = prior
			d := f.queueProduction()
			history := deliveryhistory.Service{
				Pool:      f.pool,
				Jobs:      f.worker,
				Providers: map[string]deliveryhistory.RecoveryProvider{"smtp": f.service},
			}
			if err := f.worker.Client.Start(t.Context()); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = f.worker.Client.StopAndCancel(ctx)
			})
			want := "delivered"
			if prior == maildelivery.Ambiguous {
				want = "ambiguous"
			}
			waitReviewOperation(t, f, d.ID, want)
			if f.transport.count() != 1 {
				t.Fatal("original real worker did not make one effect")
			}
			q := dbgen.New(f.pool)
			original, err := q.Operation(t.Context(), d.ID)
			if err != nil {
				t.Fatal(err)
			}
			f.transport.mu.Lock()
			f.transport.state = maildelivery.Accepted
			f.transport.mu.Unlock()
			input := deliveryhistory.Command{
				RequestID: uuid.New(),
				Action:    "resend_acknowledged",
				Reason:    "Synthetic operator explicitly accepts possible duplicate",
			}
			result, err := history.Change(
				t.Context(),
				f.owner.ID,
				f.event,
				d.ID,
				original.Revision,
				input,
			)
			if err != nil {
				t.Fatal(err)
			}
			if result.Item.State == "never_attempted" || !result.Item.Attempted {
				t.Fatal("review hid the original effect")
			}
			waitReviewOperation(t, f, d.ID, "delivered")
			if f.transport.count() != 2 {
				t.Fatalf("actual reviewed River physical calls=%d want2", f.transport.count())
			}
			if _, err = history.Change(
				t.Context(),
				f.owner.ID,
				f.event,
				d.ID,
				original.Revision,
				input,
			); err != nil {
				t.Fatal("identical command replay", err)
			}
			if err = f.worker.Execute(t.Context(), d.ID); err != nil {
				t.Fatal(err)
			}
			if err = f.worker.Reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			if f.transport.count() != 2 {
				t.Fatal("duplicate command/old callback/reconcile sent again")
			}
			var jobs, reviewed int
			if err = f.pool.QueryRow(t.Context(), "SELECT count(*),count(*) FILTER(WHERE args->>'review_id'=$2) FROM river_job WHERE args->>'operation_id'=$1", d.ID.String(), input.RequestID.String()).
				Scan(&jobs, &reviewed); err != nil ||
				jobs != 2 ||
				reviewed != 1 {
				t.Fatalf("River durable identities=%d reviewed%d err%v", jobs, reviewed, err)
			}
			detail, err := history.Detail(t.Context(), f.owner.ID, f.event, d.ID, 0, nil)
			if err != nil || len(detail.Attempts) != 2 || len(detail.Reviews) != 1 ||
				detail.Item.State != "provider_accepted" {
				t.Fatalf("retained history=%+v err%v", detail, err)
			}
		})
	}
}
