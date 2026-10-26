package loadprofile

import (
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"
)

var errWriteAttempt = errors.New("invalid native write attempt evidence")

type EffectKind string

const (
	RegistrationEffect EffectKind = "registration"
	MentorEffect       EffectKind = "mentor-observation"
	HelpEffect         EffectKind = "help-command"
	CueEffect          EffectKind = "presentation-cue"
)

// WriteSpec contains only identity/version/fingerprint, never private evidence,
// form answers or a replay body. It is captured BEFORE sending one native write.
type WriteSpec struct {
	Kind                                  EffectKind
	Event, Actor, Team, Resource, Command uuid.UUID
	Version                               int64
	Fingerprint                           string
	Action                                string
}

func (s WriteSpec) scope() string {
	return fmt.Sprint(s.Kind, ":", s.Event, ":", s.Actor, ":", s.Resource)
}

func (s WriteSpec) identity() string {
	return fmt.Sprint(s.scope(), ":", s.Team, ":", s.Command, ":", s.Version)
}

type (
	writeAttempt struct {
		spec                            WriteSpec
		status                          int
		finished, originalACK, rejected bool
	}
	AttemptLedger struct {
		mu       sync.Mutex
		attempts map[uuid.UUID]writeAttempt
	}
)

// Begin refuses new writes to an unresolved original scope. Independent later
// database effects/current reads do not clear this barrier or imply an ACK.
func (l *AttemptLedger) Begin(spec WriteSpec) (uuid.UUID, error) {
	if spec.Event == uuid.Nil || spec.Actor == uuid.Nil || spec.Resource == uuid.Nil ||
		len(spec.Fingerprint) != 64 {
		return uuid.Nil, fmt.Errorf("%w: exact native intent identity required", errWriteAttempt)
	}
	switch spec.Kind {
	case RegistrationEffect:
	case MentorEffect:
		if spec.Team == uuid.Nil || spec.Version < 1 {
			return uuid.Nil, fmt.Errorf("%w: native mentor team/version required", errWriteAttempt)
		}
	case CueEffect:
		if spec.Resource != spec.Event || spec.Version < 1 || spec.Command == uuid.Nil ||
			spec.Action != "override" {
			return uuid.Nil, fmt.Errorf(
				"%w: exact cue revision and decision marker required",
				errWriteAttempt,
			)
		}
	case HelpEffect:
		if spec.Command == uuid.Nil || spec.Version < 1 ||
			(spec.Action != "create" && spec.Action != "claim" && spec.Action != "resolve") {
			return uuid.Nil, fmt.Errorf(
				"%w: original help command identity required",
				errWriteAttempt,
			)
		}
	default:
		return uuid.Nil, fmt.Errorf("%w: unknown native effect owner", errWriteAttempt)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, attempt := range l.attempts {
		if attempt.spec.Actor == spec.Actor &&
			(!attempt.finished || !attempt.originalACK && !attempt.rejected) {
			return uuid.Nil, fmt.Errorf(
				"%w: unresolved original actor cannot send or retarget another write",
				errWriteAttempt,
			)
		}
		if attempt.rejected && attempt.spec.identity() == spec.identity() &&
			attempt.spec.Fingerprint == spec.Fingerprint {
			return uuid.Nil, fmt.Errorf(
				"%w: a rejected write is not automatically repeated",
				errWriteAttempt,
			)
		}
	}
	if l.attempts == nil {
		l.attempts = make(map[uuid.UUID]writeAttempt)
	}
	id := uuid.New()
	l.attempts[id] = writeAttempt{spec: spec}
	return id, nil
}

// Finish records the original response, without assuming a 4xx proves rollback:
// some native owners commit and then recheck current read authority. Only the
// caller's exact original response/receipt parser may establish acknowledgement.
func (l *AttemptLedger) Finish(id uuid.UUID, status int, originalACK bool) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	attempt, exists := l.attempts[id]
	if !exists || attempt.finished || status < 0 || status > 599 ||
		originalACK && (status < 200 || status >= 300) {
		return fmt.Errorf("%w: contradictory original response", errWriteAttempt)
	}
	attempt.finished = true
	attempt.status = status
	attempt.originalACK = originalACK
	l.attempts[id] = attempt
	return nil
}

// Reject is used ONLY after an owner's explicit pre-commit conflict contract
// was validated. It is not a generic 4xx/429 escape hatch. Reconciliation still
// detects an identical immutable effect committed despite that rejection.
func (l *AttemptLedger) Reject(id uuid.UUID, status int) error {
	if status != 409 {
		return fmt.Errorf(
			"%w: only validated native pre-commit conflict can settle rejection",
			errWriteAttempt,
		)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	attempt, exists := l.attempts[id]
	if !exists || attempt.finished {
		return fmt.Errorf("%w: original conflict response missing", errWriteAttempt)
	}
	attempt.finished = true
	attempt.rejected = true
	attempt.status = status
	l.attempts[id] = attempt
	return nil
}

func (l *AttemptLedger) ActorBlocked(actor uuid.UUID) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, attempt := range l.attempts {
		if attempt.spec.Actor == actor &&
			(!attempt.finished || !attempt.originalACK && !attempt.rejected) {
			return true
		}
	}
	return false
}

func (l *AttemptLedger) Blocked(kind EffectKind, event, actor, resource uuid.UUID) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	scope := (WriteSpec{Kind: kind, Event: event, Actor: actor, Resource: resource}).scope()
	for _, attempt := range l.attempts {
		if attempt.spec.scope() == scope &&
			(!attempt.finished || !attempt.originalACK && !attempt.rejected) {
			return true
		}
	}
	return false
}

func (l *AttemptLedger) Specs() map[uuid.UUID]WriteSpec {
	l.mu.Lock()
	defer l.mu.Unlock()
	specs := make(map[uuid.UUID]WriteSpec, len(l.attempts))
	for id, attempt := range l.attempts {
		specs[id] = attempt.spec
	}
	return specs
}

type (
	EffectFact struct {
		Spec        WriteSpec
		Present     bool
		Fingerprint string
		Effects     int
	}
	AttemptReport struct {
		Sent                            int         `json:"sent"`
		OriginalAcknowledgements        int         `json:"original_acknowledgements"`
		ExplicitConflicts               int         `json:"explicit_conflicts"`
		CommittedDespiteConflict        int         `json:"committed_despite_conflict"`
		UnknownOriginalAcknowledgements int         `json:"unknown_original_acknowledgements"`
		CommittedWithUnknownResponse    int         `json:"committed_with_unknown_response"`
		Unreconciled                    int         `json:"unreconciled"`
		MissingAcknowledgedEffects      int         `json:"missing_acknowledged_effects"`
		ChangedEffects                  int         `json:"changed_effects"`
		DuplicateEffects                int         `json:"duplicate_effects"`
		HTTPStatuses                    map[int]int `json:"http_statuses"`
	}
)

// Reconcile accounts for EVERY sent intent, including 403/409/429/transport loss.
// It proves native effects without rewriting the original acknowledgement state.
func (l *AttemptLedger) Reconcile(facts map[uuid.UUID]EffectFact) AttemptReport {
	l.mu.Lock()
	defer l.mu.Unlock()
	report := AttemptReport{Sent: len(l.attempts), HTTPStatuses: make(map[int]int)}
	for id, attempt := range l.attempts {
		report.HTTPStatuses[attempt.status]++
		switch {
		case attempt.originalACK:
			report.OriginalAcknowledgements++
		case attempt.rejected:
			report.ExplicitConflicts++
		default:
			report.UnknownOriginalAcknowledgements++
		}
		fact, exists := facts[id]
		if !exists || fact.Spec.identity() != attempt.spec.identity() ||
			fact.Spec.Fingerprint != attempt.spec.Fingerprint {
			report.Unreconciled++
			continue
		}
		if attempt.rejected {
			if fact.Present && fact.Effects > 0 && fact.Fingerprint == attempt.spec.Fingerprint {
				report.CommittedDespiteConflict++
			}
			continue
		}
		if !fact.Present || fact.Effects < 1 {
			if attempt.originalACK {
				report.MissingAcknowledgedEffects++
			} else if attempt.status == 0 || attempt.status >= 500 || attempt.status >= 200 && attempt.status < 300 {
				report.Unreconciled++
			}
			continue
		}
		if !attempt.originalACK {
			report.CommittedWithUnknownResponse++
		}
		if fact.Fingerprint != attempt.spec.Fingerprint {
			report.ChangedEffects++
		}
		if fact.Effects > 1 {
			report.DuplicateEffects += fact.Effects - 1
		}
	}
	return report
}
