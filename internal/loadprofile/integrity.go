package loadprofile

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

var errLoadIntegrity = errors.New("invalid load integrity")

// AcceptedWrite is private reconciliation data, never serialized into the
// public capacity report. Identity names the native immutable effect/version,
// not a later current read used as a substitute for the original response.
type AcceptedWrite struct{ Identity, Fingerprint string }

type Commit struct {
	Identity, Fingerprint string
	Effects               int
}
type IntegrityReport struct {
	Accepted         uint64 `json:"accepted"`
	Committed        uint64 `json:"committed"`
	Missing          uint64 `json:"missing"`
	Changed          uint64 `json:"changed"`
	DuplicateEffects uint64 `json:"duplicate_effects"`
}

type WriteLedger struct {
	mu       sync.Mutex
	accepted map[string]string
}

func (l *WriteLedger) Accept(write AcceptedWrite) error {
	if write.Identity == "" || write.Fingerprint == "" {
		return fmt.Errorf(
			"%w: accepted write requires its original native effect identity and fingerprint",
			errLoadIntegrity,
		)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.accepted == nil {
		l.accepted = make(map[string]string)
	}
	if original, exists := l.accepted[write.Identity]; exists && original != write.Fingerprint {
		return fmt.Errorf(
			"%w: same accepted native effect has contradictory original responses",
			errLoadIntegrity,
		)
	}
	l.accepted[write.Identity] = write.Fingerprint
	return nil
}

// Reconcile consumes independently read native committed facts. Missing or
// conflicting facts remain failures; neither retries nor current UI state can
// silently repair the ledger. Exact response replay is one effect.
func (l *WriteLedger) Reconcile(commits []Commit) IntegrityReport {
	l.mu.Lock()
	defer l.mu.Unlock()
	report := IntegrityReport{Accepted: uint64(len(l.accepted))}
	indexed := make(map[string]Commit, len(commits))
	for _, commit := range commits {
		if previous, exists := indexed[commit.Identity]; exists {
			previous.Effects += commit.Effects
			if previous.Fingerprint != commit.Fingerprint {
				previous.Fingerprint = ""
			}
			indexed[commit.Identity] = previous
		} else {
			indexed[commit.Identity] = commit
		}
	}
	for identity, fingerprint := range l.accepted {
		commit, exists := indexed[identity]
		if !exists || commit.Effects < 1 {
			report.Missing++
			continue
		}
		report.Committed++
		if commit.Fingerprint != fingerprint {
			report.Changed++
		}
		if commit.Effects > 1 {
			report.DuplicateEffects += uint64(commit.Effects - 1)
		}
	}
	return report
}

// Activity accounts for authenticated successes in EVERY real minute of the
// sustained stage. Merely signing in/seeding a thousand accounts is insufficient.
// Actors are fixed at construction; unknown/replaced identities cannot fill a
// missing actor's activity. At most 60 minutes is retained in a uint64 bitmap.
type Activity struct {
	mu       sync.Mutex
	started  time.Time
	duration time.Duration
	actors   map[string]uint64
}

func NewActivity(started time.Time, duration time.Duration, actors []string) (*Activity, error) {
	if started.IsZero() || duration <= 0 || duration > time.Hour {
		return nil, fmt.Errorf(
			"%w: activity requires an actual start and duration in (0,1h]",
			errLoadIntegrity,
		)
	}
	a := &Activity{
		started:  started,
		duration: duration,
		actors:   make(map[string]uint64, len(actors)),
	}
	for _, actor := range actors {
		if actor == "" {
			return nil, fmt.Errorf("%w: empty actor", errLoadIntegrity)
		}
		if _, exists := a.actors[actor]; exists {
			return nil, fmt.Errorf(
				"%w: duplicate actor cannot represent two active sessions",
				errLoadIntegrity,
			)
		}
		a.actors[actor] = 0
	}
	return a, nil
}

func (a *Activity) Authenticated(actor string, at time.Time) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	value, exists := a.actors[actor]
	if !exists {
		return fmt.Errorf("%w: authenticated request belongs to an unknown actor", errLoadIntegrity)
	}
	elapsed := at.Sub(a.started)
	if elapsed < 0 || elapsed >= a.duration {
		return fmt.Errorf(
			"%w: request outside sustained stage cannot fill activity",
			errLoadIntegrity,
		)
	}
	a.actors[actor] = value | uint64(1)<<uint(elapsed/time.Minute)
	return nil
}

type ActivityReport struct {
	Sessions            int `json:"sessions"`
	ActiveEveryMinute   int `json:"active_every_minute"`
	MissingActorMinutes int `json:"missing_actor_minutes"`
	Windows             int `json:"windows"`
}

func (a *Activity) Report() ActivityReport {
	a.mu.Lock()
	defer a.mu.Unlock()
	windows := int((a.duration + time.Minute - 1) / time.Minute)
	report := ActivityReport{Sessions: len(a.actors), Windows: windows}
	for _, bits := range a.actors {
		complete := true
		for window := 0; window < windows; window++ {
			if bits&(uint64(1)<<uint(window)) == 0 {
				report.MissingActorMinutes++
				complete = false
			}
		}
		if complete {
			report.ActiveEveryMinute++
		}
	}
	return report
}
