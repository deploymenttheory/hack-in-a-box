package submissionfreeze

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestDelayedCapturePreservesOriginalNativeSettlement(t *testing.T) {
	pool := testdb.New(t, true)
	t.Run("organiser replacement before first worker", func(t *testing.T) {
		f := newFixture(t, pool)
		f.submit("repository")
		before := f.read(f.owner)
		originalRevision := *before.CurrentSubmissionRevision
		f.now = f.cfg.SubmissionCutoff.Add(time.Hour)
		s := native(*f.service)
		p, err := s.Read(t.Context(), f.owner, f.event, f.team)
		if err != nil {
			t.Fatal(err)
		}
		_, err = s.Submit(
			t.Context(),
			f.owner,
			f.event,
			f.team,
			p.Revision,
			submissions.Command{
				Reason: "Audited native late correction",
				Evidence: []submissions.Evidence{
					{
						DeliverableID: f.deliverable,
						Kind:          "repository",
						Label:         "Later native final",
						URL:           "https://github.com/synthetic-org/freeze-" + f.team.String() + "/tree/main",
					},
				},
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		f.capture()
		got := f.read(f.owner)
		if got.Source.SubmissionRevision == nil ||
			*got.Source.SubmissionRevision != originalRevision ||
			got.CurrentSubmissionRevision == nil ||
			*got.CurrentSubmissionRevision == originalRevision ||
			got.BindingCurrent ||
			got.CanReconcile ||
			!got.CanAmend ||
			f.provider.calls != 0 {
			t.Fatalf("late final silently inherited cutoff %+v", got)
		}
		if got.Observations[0].Status != "authority_or_configuration_lost" {
			t.Fatal(got)
		}
		err = database.Within(t.Context(), pool, func(tx pgx.Tx, q *dbgen.Queries) error {
			if _, err := q.LockEvent(t.Context(), f.event); err != nil {
				return err
			}
			e, err := f.service.EvidenceForFinalInTransaction(
				t.Context(),
				tx,
				f.event,
				f.team,
				got.Source.EntryID,
				*got.CurrentSubmissionRevision,
			)
			if err != nil {
				return err
			}
			if e.Status != "native_revision_mismatch" || e.SHA != "" {
				t.Fatal(e)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		amendment := f.decision(got, "amend")
		original := *got.Source.SubmissionRevision
		ack, err := f.service.Change(t.Context(), f.owner, f.event, f.team, amendment)
		if err != nil {
			t.Fatal(err)
		}
		fixed := f.read(f.owner)
		if !fixed.BindingCurrent ||
			fixed.Current.SubmissionRevision != *got.CurrentSubmissionRevision ||
			*fixed.Source.SubmissionRevision != original ||
			fixed.Current.ID != ack.Decision.ID {
			t.Fatal("explicit repair changed original freeze", fixed)
		}
	})
	t.Run("configuration loss after outage leaves retained source unresolved", func(t *testing.T) {
		f := newFixture(t, pool)
		f.submit("repository")
		f.now = f.cfg.SubmissionCutoff
		f.provider.err = context.DeadlineExceeded
		id := f.operation()
		if err := f.jobs.Execute(t.Context(), id); !errors.Is(err, jobs.ErrRetry) {
			t.Fatal(err)
		}
		source := f.read(f.owner).Source
		f.exec(
			"UPDATE github_app_integrations SET credential_version=credential_version+1 WHERE event_id=$1",
			f.event,
		)
		f.provider.err = nil
		f.now = f.now.Add(time.Hour)
		if err := f.jobs.Execute(t.Context(), id); err != nil {
			t.Fatal(err)
		}
		p := f.read(f.owner)
		if p.Source.ID != source.ID || p.Status != "unresolved" || len(p.Observations) != 2 ||
			p.Observations[1].Status != "authority_or_configuration_lost" ||
			f.provider.calls != 1 {
			t.Fatal(p)
		}
	})
	t.Run(
		"current GET is not original acknowledgment and current role loss denies recovery",
		func(t *testing.T) {
			f := newFixture(t, pool)
			c := Command{
				RequestID:        uuid.New(),
				Action:           "configure",
				Branch:           "main",
				ExpectedRevision: 0,
			}
			ack, err := f.service.Change(t.Context(), f.member, f.event, f.team, c)
			if err != nil {
				t.Fatal(err)
			}
			unknown, err := f.service.OriginalReceipt(
				t.Context(),
				f.member,
				f.event,
				f.team,
				uuid.New(),
			)
			if err != nil || unknown.Receipt != nil {
				t.Fatal(unknown, err)
			}
			c.Branch = "other"
			if _, err = f.service.Change(
				t.Context(),
				f.member,
				f.event,
				f.team,
				c,
			); !errors.Is(
				err,
				access.ErrConflict,
			) {
				t.Fatal(err)
			}
			another, err := f.service.OriginalReceipt(
				t.Context(),
				f.owner,
				f.event,
				f.team,
				ack.RequestID,
			)
			if err != nil || another.Receipt != nil {
				t.Fatal("other actor saw original command", another, err)
			}
			f.exec(
				"UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2",
				f.event,
				f.member,
			)
			if _, err = f.service.OriginalReceipt(
				t.Context(),
				f.member,
				f.event,
				f.team,
				ack.RequestID,
			); !errors.Is(
				err,
				access.ErrForbidden,
			) {
				t.Fatal("lost membership retained private receipt", err)
			}
		},
	)
	t.Run("pre-live formal submission and review remain native gates", func(t *testing.T) {
		f := newFixture(t, pool)
		f.now = f.cfg.LiveStart.Add(-time.Hour)
		p := f.read(f.member)
		if p.Source != nil || p.CanReconcile || p.CanAmend {
			t.Fatal(p)
		}
		s := native(*f.service)
		n, err := s.Read(t.Context(), f.member, f.event, f.team)
		if err != nil {
			t.Fatal(err)
		}
		_, err = s.Submit(
			t.Context(),
			f.member,
			f.event,
			f.team,
			n.Revision,
			submissions.Command{
				Evidence: []submissions.Evidence{
					{
						DeliverableID: f.deliverable,
						Kind:          "repository",
						Label:         "Early code",
						URL:           "https://example.test/pre-live",
					},
				},
			},
		)
		if err == nil {
			t.Fatal("accepted formal pre-live submission")
		}
		c := Command{
			RequestID:        uuid.New(),
			Action:           "reconcile",
			ExpectedRevision: p.Revision,
			SourceID:         uuid.New(),
			SHA:              strings.Repeat("a", 40),
			Reason:           "Premature",
			ExternalEvidence: "No accepted output",
		}
		if _, err = f.service.Change(
			t.Context(),
			f.owner,
			f.event,
			f.team,
			c,
		); !errors.Is(
			err,
			access.ErrForbidden,
		) {
			t.Fatal(err)
		}
	})
}

func TestReviewedAmendmentRepairsLaterNativeFinalWithoutErasingReceipt(t *testing.T) {
	pool := testdb.New(t, true)
	f := newFixture(t, pool)
	f.submit("repository")
	f.now = f.cfg.SubmissionCutoff
	f.capture()
	p := f.read(f.owner)
	initial, err := f.service.Change(
		t.Context(),
		f.owner,
		f.event,
		f.team,
		f.decision(p, "reconcile"),
	)
	if err != nil {
		t.Fatal(err)
	}
	old := f.read(f.owner)
	stale := f.decision(old, "amend")
	native := native(*f.service)
	current, err := native.Read(t.Context(), f.owner, f.event, f.team)
	if err != nil {
		t.Fatal(err)
	}
	_, err = native.Submit(
		t.Context(),
		f.owner,
		f.event,
		f.team,
		current.Revision,
		submissions.Command{
			Reason: "Legitimate native final correction after completed receipt",
			Evidence: []submissions.Evidence{
				{
					DeliverableID: f.deliverable,
					Kind:          "repository",
					Label:         "Corrected final",
					URL:           "https://github.com/synthetic-org/freeze-" + f.team.String() + "/tree/main",
				},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	mismatch := f.read(f.owner)
	if mismatch.BindingCurrent || mismatch.Status != "native_revision_mismatch" ||
		!mismatch.CanAmend ||
		mismatch.Current.ID != initial.Decision.ID {
		t.Fatal(mismatch)
	}
	if _, err = f.service.Change(
		t.Context(),
		f.owner,
		f.event,
		f.team,
		stale,
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatal("stale native final target accepted", err)
	}
	correction := f.decision(mismatch, "amend")
	correction.Reason = "Explicit review of corrected native final; retain original cutoff receipt"
	amended, err := f.service.Change(t.Context(), f.owner, f.event, f.team, correction)
	if err != nil {
		t.Fatal(err)
	}
	final := f.read(f.owner)
	if !final.BindingCurrent || final.Status != "completed" || len(final.Decisions) != 2 ||
		final.Decisions[0].ID != initial.Decision.ID ||
		final.Current.ID != amended.Decision.ID ||
		*final.Source.SubmissionRevision != *old.Source.SubmissionRevision ||
		final.Current.SubmissionRevision == *final.Source.SubmissionRevision {
		t.Fatal("repair silently rewrote original cutoff", final)
	}
	if _, err = f.service.Change(
		t.Context(),
		f.owner,
		f.event,
		f.team,
		f.decision(mismatch, "amend"),
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatal("racing receipt target overwritten", err)
	}
	recovery, err := f.service.OriginalReceipt(
		t.Context(),
		f.owner,
		f.event,
		f.team,
		initial.RequestID,
	)
	if err != nil || recovery.Receipt == nil ||
		recovery.Receipt.Decision.ID != initial.Decision.ID {
		t.Fatal("old ACK rewritten by amendment", recovery, err)
	}
}
