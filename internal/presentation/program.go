package presentation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
)

// Program is the existing allowlisted published room program, not the controller
// page, a personal agenda or a display session. It contains no drafts, pairings,
// bearer tokens, private content bodies or controller capability.
type Program struct {
	Sequence         int64       `json:"sequence"`
	ServerNow        time.Time   `json:"server_now"`
	Current          *DisplayCue `json:"current"`
	Next             *DisplayCue `json:"next"`
	Mode             string      `json:"mode"`
	RemainingSeconds int         `json:"remaining_seconds"`
	TimerEndsAt      *time.Time  `json:"timer_ends_at"`
	OverrideActive   bool        `json:"override_active"`
}

// ReadProgramIn requires the actual event reader's current authority and uses the
// same release projection as a room display. It neither creates a display state
// nor records a display heartbeat. The caller owns its read transaction.
func (s Service) ReadProgramIn(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
) (Program, error) {
	q := dbgen.New(tx)
	if err := (access.EventPolicy{Queries: q}).Authorize(
		ctx,
		actor,
		eventID,
		eventID,
		access.EventRead,
	); err != nil {
		return Program{}, fmt.Errorf("authorize room program reader: %w", err)
	}
	e, err := q.Event(ctx, eventID)
	if err != nil {
		return Program{}, fmt.Errorf("read program event: %w", err)
	}
	// An archived/cancelled event no longer has an active room projection,
	// matching the display owner's existing writes-allowed boundary. Historical
	// event-reader access does not revive operational presentation state.
	if !e.WritesAllowed {
		return Program{ServerNow: s.now(), Mode: "held"}, nil
	}
	row, err := q.PresentationState(ctx, eventID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Program{ServerNow: s.now(), Mode: "held"}, nil
	}
	if err != nil {
		return Program{}, fmt.Errorf("read room program: %w", err)
	}
	p, err := s.programSnapshot(ctx, q, e, row)
	if err != nil {
		return Program{}, err
	}
	return Program{
		Sequence:         p.Sequence,
		ServerNow:        p.ServerNow,
		Current:          p.Current,
		Next:             p.Next,
		Mode:             p.Mode,
		RemainingSeconds: p.RemainingSeconds,
		TimerEndsAt:      p.TimerEndsAt,
		OverrideActive:   p.OverrideActive,
	}, nil
}

// programSnapshot is shared with Snapshot so Today cannot weaken content release,
// current/next selection or override/timer semantics through a second projection.
func (s Service) programSnapshot(
	ctx context.Context,
	q *dbgen.Queries,
	e dbgen.Event,
	row dbgen.PresentationState,
) (Snapshot, error) {
	_, published, err := decode(row)
	if err != nil {
		return Snapshot{}, err
	}
	c := event.DefaultConfiguration()
	if err = json.Unmarshal(e.Configuration, &c); err != nil {
		return Snapshot{}, fmt.Errorf("decode display timezone: %w", err)
	}
	result := Snapshot{
		EventID:          e.ID,
		EventName:        e.Name,
		Timezone:         c.Timezone,
		Sequence:         row.Revision,
		ServerNow:        s.now(),
		Mode:             row.Mode,
		RemainingSeconds: int(remaining(row, s.now())),
		PrimaryColour:    "#173a35",
		BackgroundColour: "#f8f9f5",
		ForegroundColour: "#173a35",
	}
	if row.TimerEndsAt.Valid {
		result.TimerEndsAt = &row.TimerEndsAt.Time
	}
	if row.CurrentIndex >= 0 && int(row.CurrentIndex) < len(published) {
		cue, visible, projectErr := s.project(ctx, q, e, published[row.CurrentIndex])
		if projectErr != nil {
			return Snapshot{}, projectErr
		}
		if visible {
			result.Current = &cue
		}
	}
	if int(row.CurrentIndex)+1 < len(published) {
		cue, visible, projectErr := s.project(ctx, q, e, published[row.CurrentIndex+1])
		if projectErr != nil {
			return Snapshot{}, projectErr
		}
		if visible {
			result.Next = &cue
		}
	}
	override, overridden, overrideErr := decodeOverride(row)
	if overrideErr != nil {
		return Snapshot{}, overrideErr
	}
	if overridden {
		result.OverrideActive = true
		result.Mode = row.OverrideMode
		result.RemainingSeconds = int(overrideRemaining(row, s.now()))
		result.TimerEndsAt = nil
		if row.OverrideTimerEndsAt.Valid {
			result.TimerEndsAt = &row.OverrideTimerEndsAt.Time
		}
		if row.CurrentIndex >= 0 {
			result.Next = result.Current
		}
		result.Current = nil
		cue, visible, projectErr := s.project(ctx, q, e, override)
		if projectErr != nil {
			return Snapshot{}, projectErr
		}
		if visible {
			result.Current = &cue
		}
	}
	return result, nil
}
