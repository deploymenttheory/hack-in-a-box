package presentation

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
)

func timerRemaining(mode string, stored int32, ends pgtype.Timestamptz, now time.Time) int32 {
	if mode == "running" && ends.Valid {
		seconds := int64(math.Ceil(ends.Time.Sub(now).Seconds()))
		if seconds < 0 {
			return 0
		}
		if seconds > 86400 {
			return 86400
		}
		return int32(seconds)
	}
	return stored
}

func remaining(row dbgen.PresentationState, now time.Time) int32 {
	return timerRemaining(row.Mode, row.RemainingSeconds, row.TimerEndsAt, now)
}

func overrideRemaining(row dbgen.PresentationState, now time.Time) int32 {
	return timerRemaining(
		row.OverrideMode,
		row.OverrideRemainingSeconds,
		row.OverrideTimerEndsAt,
		now,
	)
}

func decodeOverride(row dbgen.PresentationState) (Cue, bool, error) {
	if len(row.OverrideCue) == 0 || string(row.OverrideCue) == "null" {
		return Cue{}, false, nil
	}
	var cue Cue
	if err := json.Unmarshal(row.OverrideCue, &cue); err != nil {
		return Cue{}, false, fmt.Errorf("decode urgent cue: %w", err)
	}
	return cue, true, nil
}

func endsAt(now time.Time, seconds int32) pgtype.Timestamptz {
	if seconds <= 0 {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: now.Add(time.Duration(seconds) * time.Second), Valid: true}
}

func activate(row *dbgen.PresentationState, published []Cue, index int, now time.Time) error {
	if index < 0 || index >= 200 || index >= len(published) {
		return access.ErrInvalid
	}
	row.CurrentIndex = int32(index)
	row.Mode = "running"
	row.RemainingSeconds = published[index].DurationSeconds
	row.TimerEndsAt = endsAt(now, row.RemainingSeconds)
	return nil
}

func changeTimer(
	row *dbgen.PresentationState,
	c Command,
	activeOverride bool,
	published []Cue,
	now time.Time,
) error {
	if !activeOverride && (row.CurrentIndex < 0 || int(row.CurrentIndex) >= len(published)) {
		return access.ErrInvalid
	}
	mode, seconds, end := &row.Mode, &row.RemainingSeconds, &row.TimerEndsAt
	if activeOverride {
		mode, seconds, end = &row.OverrideMode, &row.OverrideRemainingSeconds, &row.OverrideTimerEndsAt
	}
	switch c.Action {
	case "hold":
		*seconds = timerRemaining(*mode, *seconds, *end, now)
		*mode = "held"
		*end = pgtype.Timestamptz{}
	case "resume":
		if *mode != "held" {
			return access.ErrInvalid
		}
		*mode = "running"
		*end = endsAt(now, *seconds)
	case "timer":
		if c.Seconds == nil {
			return access.ErrInvalid
		}
		*seconds = *c.Seconds
		if *mode == "running" {
			*end = endsAt(now, *seconds)
		}
	}
	return nil
}

func change(row *dbgen.PresentationState, c Command, now time.Time) error {
	draft, published, err := decode(*row)
	if err != nil {
		return err
	}
	_, overridden, err := decodeOverride(*row)
	if err != nil {
		return err
	}
	if overridden &&
		slices.Contains([]string{"publish", "advance", "back", "activate", "reset"}, c.Action) {
		return &event.Blocked{
			Issues: []event.Issue{
				{
					Field:   "presentation",
					Code:    "override_active",
					Message: "Return to the planned schedule before activating another planned cue.",
				},
			},
		}
	}
	switch c.Action {
	case "save":
		row.Draft, err = json.Marshal(append([]Cue{}, c.Cues...))
		if err != nil {
			return fmt.Errorf("encode cue draft: %w", err)
		}
	case "publish":
		row.Published = append([]byte{}, row.Draft...)
		row.CurrentIndex = -1
		row.Mode = "held"
		row.RemainingSeconds = 0
		row.TimerEndsAt = pgtype.Timestamptz{}
	case "advance":
		if len(published) == 0 || int(row.CurrentIndex) >= len(published) {
			return access.ErrInvalid
		}
		if int(row.CurrentIndex)+1 < len(published) {
			return activate(row, published, int(row.CurrentIndex)+1, now)
		}
		row.CurrentIndex++
		row.Mode = "held"
		row.RemainingSeconds = 0
		row.TimerEndsAt = pgtype.Timestamptz{}
	case "back":
		return activate(row, published, int(row.CurrentIndex)-1, now)
	case "activate":
		if c.Index == nil {
			return access.ErrInvalid
		}
		if c.FromDraft {
			if *c.Index >= len(draft) {
				return access.ErrInvalid
			}
			row.Published = append([]byte{}, row.Draft...)
			return activate(row, draft, *c.Index, now)
		}
		return activate(row, published, *c.Index, now)
	case "hold", "resume", "timer":
		return changeTimer(row, c, overridden, published, now)
	case "reset":
		row.CurrentIndex = -1
		row.Mode = "held"
		row.RemainingSeconds = 0
		row.TimerEndsAt = pgtype.Timestamptz{}
	case "override":
		if c.Override == nil {
			return access.ErrInvalid
		}
		if !overridden {
			row.RemainingSeconds = remaining(*row, now)
			row.TimerEndsAt = pgtype.Timestamptz{}
		}
		row.OverrideCue, err = json.Marshal(c.Override)
		if err != nil {
			return fmt.Errorf("encode urgent cue: %w", err)
		}
		row.OverrideMode = "running"
		row.OverrideRemainingSeconds = c.Override.DurationSeconds
		row.OverrideTimerEndsAt = endsAt(now, c.Override.DurationSeconds)
	case "return_to_schedule":
		if !overridden {
			return access.ErrInvalid
		}
		row.OverrideCue = nil
		row.OverrideMode = "held"
		row.OverrideRemainingSeconds = 0
		row.OverrideTimerEndsAt = pgtype.Timestamptz{}
		if row.Mode == "running" {
			row.TimerEndsAt = endsAt(now, row.RemainingSeconds)
		}
	}
	return nil
}
