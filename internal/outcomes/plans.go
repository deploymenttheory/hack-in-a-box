package outcomes

import (
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/schedule"
)

// Review schedules are optional. Owner progress updates do not approve sends.
// A plan approval binds exact source/owner/composition/timing/provider versions;
// a replacement always retires its predecessor instead of transferring consent.
type ReviewSchedule struct {
	Days      int      `json:"days"`
	WallTime  string   `json:"wall_time"`
	Timezone  string   `json:"timezone"`
	DSTChoice string   `json:"dst_choice"`
	Channels  []string `json:"channels"`
}
type ReviewPlan struct {
	Message       ReviewMessage  `json:"message"`
	ID            uuid.UUID      `json:"id"`
	OutcomeID     uuid.UUID      `json:"outcome_id"`
	Version       int64          `json:"version"`
	CommandID     uuid.UUID      `json:"command_id"`
	SourceVersion int64          `json:"source_version"`
	OwnerID       uuid.UUID      `json:"owner_id"`
	Schedule      ReviewSchedule `json:"schedule"`
	DueAt         time.Time      `json:"due_at"`
	State         string         `json:"state"`
	Reason        string         `json:"reason"`
	ActorID       uuid.UUID      `json:"actor_id"`
	At            time.Time      `json:"at"`
}
type PlanCommand struct {
	Message       *ReviewMessage  `json:"message"`
	ID            uuid.UUID       `json:"id"`
	Action        string          `json:"action"`
	Reason        string          `json:"reason"`
	SourceVersion int64           `json:"source_version"`
	Schedule      *ReviewSchedule `json:"schedule"`
}

func (s ReviewSchedule) Validate() error {
	if !slices.Contains([]int{30, 60, 90}, s.Days) ||
		!slices.Contains([]string{"", "earlier", "later"}, s.DSTChoice) ||
		len(s.Channels) < 1 ||
		len(s.Channels) > 2 ||
		!required(s.WallTime, 30) ||
		!required(s.Timezone, 100) {
		return access.ErrInvalid
	}
	seen := map[string]bool{}
	for _, channel := range s.Channels {
		if seen[channel] || !slices.Contains([]string{"smtp", "in_app"}, channel) {
			return access.ErrInvalid
		}
		seen[channel] = true
	}
	if _, err := schedule.ResolveLocal(s.WallTime, s.Timezone, s.DSTChoice); err != nil {
		return access.ErrInvalid
	}
	return nil
}

func (c PlanCommand) Validate() error {
	if c.ID == uuid.Nil || c.SourceVersion < 1 || !required(c.Reason, 2000) ||
		!slices.Contains([]string{"save", "approve", "cancel"}, c.Action) {
		return access.ErrInvalid
	}
	if c.Action == "save" {
		if c.Schedule == nil || c.Message == nil || !c.Message.valid() {
			return access.ErrInvalid
		}
		return c.Schedule.Validate()
	}
	if c.Schedule != nil || c.Message != nil {
		return access.ErrInvalid
	}
	return nil
}
