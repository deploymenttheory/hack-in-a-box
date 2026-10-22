// Package observations owns private mentor evidence and separate operational
// status history. Neither record contributes to competition scores.
package observations

import (
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	Pool *pgxpool.Pool
	Now  func() time.Time
}

type Evidence struct {
	Category          string `json:"category"`
	Evidence          string `json:"evidence"`
	Outcome           string `json:"outcome"`
	FeedbackReference string `json:"feedback_reference"`
}

type Observation struct {
	ID         uuid.UUID `json:"id"`
	TeamID     uuid.UUID `json:"team_id"`
	AuthorID   uuid.UUID `json:"author_id"`
	Version    int64     `json:"version"`
	ObservedAt time.Time `json:"observed_at"`
	ChangedAt  time.Time `json:"changed_at"`
	Evidence   Evidence  `json:"evidence"`
}

type Status struct {
	Version    int64     `json:"version"`
	ActorID    uuid.UUID `json:"actor_id"`
	State      string    `json:"state"`
	Reason     string    `json:"reason"`
	NextAction string    `json:"next_action"`
	ChangedAt  time.Time `json:"changed_at"`
}

type Page struct {
	Revision        int64         `json:"revision"`
	CanRecord       bool          `json:"can_record"`
	CanChangeStatus bool          `json:"can_change_status"`
	Observations    []Observation `json:"observations"`
	Status          *Status       `json:"status"`
}

type Command struct {
	Action     string    `json:"action"`
	ID         uuid.UUID `json:"id"`
	Version    int64     `json:"version"`
	Evidence   Evidence  `json:"evidence"`
	State      string    `json:"state"`
	Reason     string    `json:"reason"`
	NextAction string    `json:"next_action"`
}

type History struct {
	Observations []Observation `json:"observations"`
	Statuses     []Status      `json:"statuses"`
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
