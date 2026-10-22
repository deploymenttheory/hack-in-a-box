// Package mentors owns mentor coverage independently of frozen competitor teams.
package mentors

import (
	"time"

	"github.com/google/uuid"
)

type Window struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}
type Profile struct {
	PersonID     uuid.UUID `json:"person_id"`
	Name         string    `json:"name"`
	Active       bool      `json:"active"`
	Skills       []string  `json:"skills"`
	Availability []Window  `json:"availability"`
	MaximumTeams *int      `json:"maximum_teams"`
}
type Team struct {
	ID        uuid.UUID   `json:"id"`
	Name      string      `json:"name"`
	Published bool        `json:"published"`
	Skills    []string    `json:"skills"`
	Members   []uuid.UUID `json:"-"`
}
type Conflict struct {
	MentorID     uuid.UUID `json:"mentor_id"`
	CompetitorID uuid.UUID `json:"competitor_id"`
	Kind         string    `json:"kind"`
}
type Assignment struct {
	TeamID   uuid.UUID `json:"team_id"`
	PersonID uuid.UUID `json:"person_id"`
	Slot     string    `json:"slot"`
	Window   Window    `json:"window"`
}
type Gap struct {
	Code     string     `json:"code"`
	TeamID   *uuid.UUID `json:"team_id"`
	PersonID *uuid.UUID `json:"person_id"`
	Hard     bool       `json:"hard"`
	Message  string     `json:"message"`
}
type Snapshot struct {
	DefaultCapacity int
	Live            Window
	Profiles        []Profile
	Teams           []Team
	Conflicts       []Conflict
}
type Proposal struct {
	Revision    int64        `json:"revision"`
	Assignments []Assignment `json:"assignments"`
	Gaps        []Gap        `json:"gaps"`
}

type Page struct {
	PublishedGaps   []Gap        `json:"published_gaps"`
	Revision        int64        `json:"revision"`
	CanManage       bool         `json:"can_manage"`
	DefaultCapacity int          `json:"default_capacity"`
	Live            Window       `json:"live"`
	Profiles        []Profile    `json:"profiles"`
	Teams           []Team       `json:"teams"`
	Conflicts       []Conflict   `json:"conflicts"`
	Draft           []Assignment `json:"draft"`
	Published       []Assignment `json:"published"`
	Gaps            []Gap        `json:"gaps"`
}
