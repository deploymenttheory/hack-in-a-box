// Package help owns operational team support independently of observations and scoring.
package help

import (
	"fmt"
	"net"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
)

type Request struct {
	ID             uuid.UUID  `json:"id"`
	EventID        uuid.UUID  `json:"event_id"`
	TeamID         uuid.UUID  `json:"team_id"`
	RequesterID    uuid.UUID  `json:"requester_id"`
	Revision       int64      `json:"revision"`
	Category       string     `json:"category"`
	Description    string     `json:"description"`
	Location       string     `json:"location"`
	MeetingURL     string     `json:"meeting_url"`
	Status         string     `json:"status"`
	AssigneeID     *uuid.UUID `json:"assignee_id"`
	WaitingCycle   int64      `json:"waiting_cycle"`
	RequestedAt    time.Time  `json:"requested_at"`
	WaitingSince   time.Time  `json:"waiting_since"`
	FirstClaimedAt *time.Time `json:"first_claimed_at"`
	ClaimedAt      *time.Time `json:"claimed_at"`
	ResolvedAt     *time.Time `json:"resolved_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type Settings struct {
	Revision                  int64    `json:"revision"`
	SetupThresholdSeconds     int64    `json:"setup_threshold_seconds"`
	LogisticsThresholdSeconds int64    `json:"logistics_threshold_seconds"`
	OutputThresholdSeconds    int64    `json:"output_threshold_seconds"`
	MeetingHosts              []string `json:"meeting_hosts"`
}

func DefaultSettings() Settings {
	return Settings{
		Revision:                  1,
		SetupThresholdSeconds:     900,
		LogisticsThresholdSeconds: 900,
		OutputThresholdSeconds:    1200,
		MeetingHosts:              []string{},
	}
}

type Input struct {
	TeamID      uuid.UUID `json:"team_id"`
	Category    string    `json:"category"`
	Description string    `json:"description"`
	Location    string    `json:"location"`
	MeetingURL  string    `json:"meeting_url"`
	CommandID   uuid.UUID `json:"command_id"`
}

type Command struct {
	Action     string     `json:"action"`
	AssigneeID *uuid.UUID `json:"assignee_id"`
	CommandID  uuid.UUID  `json:"command_id"`
}

type Authority struct {
	Organiser  bool
	TeamMember bool
	Helper     bool
}

type Transition struct {
	Revision   int64      `json:"revision"`
	Action     string     `json:"action"`
	Status     string     `json:"status"`
	ActorID    *uuid.UUID `json:"actor_id"`
	AssigneeID *uuid.UUID `json:"assignee_id"`
	At         time.Time  `json:"at"`
}

type Page struct {
	EventName       string         `json:"event_name"`
	Timezone        string         `json:"timezone"`
	Request         Request        `json:"request"`
	History         []Transition   `json:"history"`
	CanClaim        bool           `json:"can_claim"`
	CanRelease      bool           `json:"can_release"`
	CanTransfer     bool           `json:"can_transfer"`
	CanResolve      bool           `json:"can_resolve"`
	CanReopen       bool           `json:"can_reopen"`
	CanCancel       bool           `json:"can_cancel"`
	AssigneeName    string         `json:"assignee_name"`
	TransferTargets []HelperTarget `json:"transfer_targets"`
}

type HelperTarget struct {
	PersonID uuid.UUID `json:"person_id"`
	Name     string    `json:"name"`
}

type TeamReference struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

func threshold(settings Settings, category string) time.Duration {
	seconds := settings.OutputThresholdSeconds
	if category == "setup" {
		seconds = settings.SetupThresholdSeconds
	}
	if category == "logistics" {
		seconds = settings.LogisticsThresholdSeconds
	}
	return time.Duration(seconds) * time.Second
}

func validateSettings(s Settings) error {
	for _, seconds := range []int64{s.SetupThresholdSeconds, s.LogisticsThresholdSeconds, s.OutputThresholdSeconds} {
		if seconds < 60 || seconds > 86400 {
			return access.ErrInvalid
		}
	}
	if len(s.MeetingHosts) > 20 {
		return access.ErrInvalid
	}
	for _, host := range s.MeetingHosts {
		if host != strings.ToLower(strings.TrimSpace(host)) || host == "" || len(host) > 253 ||
			strings.ContainsAny(host, "/:@*% \\?\r\n\t") ||
			net.ParseIP(host) != nil ||
			!strings.Contains(host, ".") {
			return access.ErrInvalid
		}
	}
	return nil
}

func validateInput(in Input, settings Settings) error {
	if in.TeamID == uuid.Nil || in.CommandID == uuid.Nil ||
		!slices.Contains([]string{"setup", "logistics", "competition_output"}, in.Category) ||
		strings.TrimSpace(in.Description) == "" ||
		len(in.Description) > 8000 ||
		len(in.Location) > 300 ||
		len(in.MeetingURL) > 2048 ||
		strings.ContainsRune(in.Description, 0) ||
		strings.ContainsRune(in.Location, 0) {
		return access.ErrInvalid
	}
	if strings.TrimSpace(in.Location) == "" && in.MeetingURL == "" {
		return access.ErrInvalid
	}
	if in.MeetingURL != "" {
		u, err := url.Parse(in.MeetingURL)
		if err != nil || in.MeetingURL != strings.TrimSpace(in.MeetingURL) || u.Scheme != "https" ||
			u.User != nil ||
			u.Host != u.Hostname() ||
			!slices.Contains(settings.MeetingHosts, u.Hostname()) {
			return access.ErrInvalid
		}
	}
	return nil
}

func outputReady(category string, published bool, liveStart, now time.Time) error {
	if category == "competition_output" &&
		(!published || liveStart.IsZero() || now.Before(liveStart)) {
		return &event.Blocked{
			Issues: []event.Issue{
				{
					Field:   "help",
					Code:    "help_output_before_live",
					Message: "Competition-output help opens after actual common challenge release and the scheduled live start.",
				},
			},
		}
	}
	return nil
}

// apply changes operational ownership only. The caller derives Authority and
// target eligibility from current database state under the event lock.
func apply(
	r Request,
	actor uuid.UUID,
	command Command,
	authority Authority,
	targetEligible bool,
	now time.Time,
) (Request, error) {
	if !validCommand(command) {
		return r, access.ErrInvalid
	}
	assigned := r.AssigneeID != nil && *r.AssigneeID == actor && authority.Helper
	switch command.Action {
	case "claim":
		if r.Status != "waiting" {
			return r, access.ErrConflict
		}
		if !authority.Helper {
			return r, access.ErrForbidden
		}
		r.Status = "claimed"
		r.AssigneeID = &actor
		r.ClaimedAt = &now
		if r.FirstClaimedAt == nil {
			r.FirstClaimedAt = &now
		}
	case "release":
		if r.Status != "claimed" {
			return r, access.ErrConflict
		}
		if !authority.Organiser && !assigned {
			return r, access.ErrForbidden
		}
		r.Status = "waiting"
		r.AssigneeID = nil
		r.ClaimedAt = nil
		r.WaitingCycle++
		r.WaitingSince = now
	case "transfer":
		if r.Status != "claimed" {
			return r, access.ErrConflict
		}
		if !authority.Organiser && !assigned {
			return r, access.ErrForbidden
		}
		if !validTransferTarget(command.AssigneeID, r.AssigneeID, targetEligible) {
			return r, access.ErrInvalid
		}
		r.AssigneeID = command.AssigneeID
		r.ClaimedAt = &now
	case "resolve":
		if r.Status != "claimed" {
			return r, access.ErrConflict
		}
		if !authority.Organiser && !assigned {
			return r, access.ErrForbidden
		}
		r.Status = "resolved"
		r.ResolvedAt = &now
	case "reopen":
		if !slices.Contains([]string{"resolved", "cancelled"}, r.Status) {
			return r, access.ErrConflict
		}
		if !authority.Organiser && !authority.TeamMember {
			return r, access.ErrForbidden
		}
		r.Status = "waiting"
		r.AssigneeID = nil
		r.ClaimedAt = nil
		r.ResolvedAt = nil
		r.WaitingCycle++
		r.WaitingSince = now
	case "cancel":
		if !slices.Contains([]string{"waiting", "claimed"}, r.Status) {
			return r, access.ErrConflict
		}
		if !authority.Organiser && !authority.TeamMember {
			return r, access.ErrForbidden
		}
		r.Status = "cancelled"
		r.AssigneeID = nil
		r.ClaimedAt = nil
	default:
		return r, fmt.Errorf("unknown help transition: %w", access.ErrInvalid)
	}
	r.Revision++
	r.UpdatedAt = now
	return r, nil
}

func validTransferTarget(id, current *uuid.UUID, eligible bool) bool {
	return eligible && id != nil && *id != uuid.Nil && (current == nil || *id != *current)
}

func validCommand(c Command) bool {
	return c.CommandID != uuid.Nil && (c.Action == "transfer" || c.AssigneeID == nil)
}
