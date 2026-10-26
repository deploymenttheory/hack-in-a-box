package loadprofile

import (
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func syntheticWrite() WriteSpec {
	return WriteSpec{
		Kind:        MentorEffect,
		Event:       uuid.New(),
		Actor:       uuid.New(),
		Team:        uuid.New(),
		Resource:    uuid.New(),
		Version:     1,
		Fingerprint: strings.Repeat("a", 64),
	}
}

func TestEveryUnknownResponseRetainsBarrierEvenWhenDatabaseProvesItsEffect(t *testing.T) {
	for _, status := range []int{0, 200, 403, 409, 429, 500} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			var ledger AttemptLedger
			spec := syntheticWrite()
			id, err := ledger.Begin(spec)
			if err != nil {
				t.Fatal(err)
			}
			if err := ledger.Finish(id, status, false); err != nil {
				t.Fatal(err)
			}
			report := ledger.Reconcile(
				map[uuid.UUID]EffectFact{
					id: {Spec: spec, Present: true, Fingerprint: spec.Fingerprint, Effects: 1},
				},
			)
			if report.UnknownOriginalAcknowledgements != 1 ||
				report.CommittedWithUnknownResponse != 1 ||
				report.HTTPStatuses[status] != 1 ||
				report.Unreconciled != 0 {
				t.Fatal("unknown original response hidden", report)
			}
			if !ledger.ActorBlocked(spec.Actor) {
				t.Fatal("database effect inferred an original ACK")
			}
			next := spec
			next.Resource = uuid.New()
			next.Fingerprint = strings.Repeat("b", 64)
			if _, err := ledger.Begin(next); err == nil {
				t.Fatal("unknown actor retargeted another write")
			}
		})
	}
}

func TestKnownPrecommitConflictIsReconciledWithoutHidingLaterDifferentVersionPayload(t *testing.T) {
	var ledger AttemptLedger
	first := syntheticWrite()
	id, err := ledger.Begin(first)
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.Reject(id, 403); err == nil {
		t.Fatal("generic403 treated as proven precommit conflict")
	}
	if err := ledger.Reject(id, 409); err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Begin(first); err == nil {
		t.Fatal("known rejected write automatically repeated")
	}
	next := first
	next.Fingerprint = strings.Repeat("b", 64)
	nextID, err := ledger.Begin(next)
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.Finish(nextID, 200, true); err != nil {
		t.Fatal(err)
	}
	// The rejected proposed version was never committed; a later DIFFERENT
	// payload can legitimately occupy that previously uncommitted version.
	report := ledger.Reconcile(
		map[uuid.UUID]EffectFact{
			id:     {Spec: first, Present: true, Fingerprint: next.Fingerprint, Effects: 1},
			nextID: {Spec: next, Present: true, Fingerprint: next.Fingerprint, Effects: 1},
		},
	)
	if report.ExplicitConflicts != 1 || report.OriginalAcknowledgements != 1 ||
		report.CommittedDespiteConflict != 0 ||
		report.ChangedEffects != 0 ||
		report.UnknownOriginalAcknowledgements != 0 {
		t.Fatal(report)
	}
	bad := ledger.Reconcile(
		map[uuid.UUID]EffectFact{
			id:     {Spec: first, Present: true, Fingerprint: first.Fingerprint, Effects: 1},
			nextID: {Spec: next, Present: false},
		},
	)
	if bad.CommittedDespiteConflict != 1 || bad.MissingAcknowledgedEffects != 1 {
		t.Fatal("contradictory native effects hidden", bad)
	}
}
