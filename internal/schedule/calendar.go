// Package schedule resolves event-local calendar rules to explicit instants.
package schedule

import (
	"errors"
	"fmt"
	"slices"
	"time"
)

// ChallengeDispatchTolerance is the agreed worker scheduling allowance after T−7.
// Later initial publication requires postponement or cancellation.
const ChallengeDispatchTolerance = 60 * time.Second

var (
	ErrLocalTime = errors.New("local time does not exist in this timezone")
	ErrAmbiguous = errors.New("local time occurs twice; choose earlier or later explicitly")
	ErrCalendar  = errors.New("invalid business calendar or relative offset")
)

type Calendar struct {
	WorkingDays   []int    `json:"working_days"`
	ExcludedDates []string `json:"excluded_dates"`
}

func (c Calendar) Validate() error {
	if len(c.WorkingDays) < 1 || len(c.WorkingDays) > 7 || len(c.ExcludedDates) > 3660 {
		return ErrCalendar
	}
	seen := map[int]bool{}
	for _, day := range c.WorkingDays {
		if day < 0 || day > 6 || seen[day] {
			return ErrCalendar
		}
		seen[day] = true
	}
	for _, date := range c.ExcludedDates {
		if _, err := time.Parse(time.DateOnly, date); err != nil {
			return ErrCalendar
		}
	}
	return nil
}

// ResolveLocal refuses Go's implicit choice at DST gaps/folds. The caller must
// select a fold explicitly; a nonexistent wall time always needs a new time.
func ResolveLocal(wall, zone, fold string) (time.Time, error) {
	local, err := time.Parse("2006-01-02T15:04:05", wall)
	if err != nil {
		return time.Time{}, ErrLocalTime
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		return time.Time{}, ErrLocalTime
	}
	offsets := map[int]bool{}
	for hour := -48; hour <= 48; hour++ {
		_, offset := local.Add(time.Duration(hour) * time.Hour).In(location).Zone()
		offsets[offset] = true
	}
	matches := make([]time.Time, 0, 2)
	for offset := range offsets {
		candidate := local.Add(-time.Duration(offset) * time.Second)
		if candidate.In(location).Format("2006-01-02T15:04:05") == wall {
			matches = append(matches, candidate)
		}
	}
	slices.SortFunc(matches, func(a, b time.Time) int { return a.Compare(b) })
	if len(matches) == 0 {
		return time.Time{}, ErrLocalTime
	}
	if len(matches) > 1 {
		switch fold {
		case "earlier":
			return matches[0], nil
		case "later":
			return matches[len(matches)-1], nil
		default:
			return time.Time{}, ErrAmbiguous
		}
	}
	return matches[0], nil
}

func CalendarOffset(anchor time.Time, zone string, days int, fold string) (time.Time, error) {
	if days < -3660 || days > 3660 {
		return time.Time{}, ErrCalendar
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		return time.Time{}, ErrCalendar
	}
	local := anchor.In(location)
	// Do date arithmetic in UTC to avoid silently resolving a DST gap/fold.
	date := time.Date(local.Year(), local.Month(), local.Day(), local.Hour(), local.Minute(), local.Second(), 0, time.UTC).
		AddDate(0, 0, days)
	return ResolveLocal(date.Format("2006-01-02T15:04:05"), zone, fold)
}

func ChallengePublication(live time.Time, zone, fold string) (time.Time, error) {
	instant, err := CalendarOffset(live, zone, -7, fold)
	if err != nil {
		return time.Time{}, fmt.Errorf("challenge publication: %w", err)
	}
	return instant, nil
}

func (c Calendar) Offset(anchor time.Time, zone string, days int, fold string) (time.Time, error) {
	if err := c.Validate(); err != nil {
		return time.Time{}, err
	}
	if days < -366 || days > 366 {
		return time.Time{}, ErrCalendar
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		return time.Time{}, ErrCalendar
	}
	local := anchor.In(location)
	date := time.Date(
		local.Year(),
		local.Month(),
		local.Day(),
		local.Hour(),
		local.Minute(),
		local.Second(),
		0,
		time.UTC,
	)
	direction := 1
	if days < 0 {
		direction = -1
		days = -days
	}
	for traversed := 0; days > 0; traversed++ {
		if traversed > 10000 {
			return time.Time{}, ErrCalendar
		}
		date = date.AddDate(0, 0, direction)
		if slices.Contains(c.WorkingDays, int(date.Weekday())) &&
			!slices.Contains(c.ExcludedDates, date.Format(time.DateOnly)) {
			days--
		}
	}
	return ResolveLocal(date.Format("2006-01-02T15:04:05"), zone, fold)
}
