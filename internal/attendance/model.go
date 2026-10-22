// Package attendance owns physical arrivals and operational headcounts, independently of admission.
package attendance

import (
	"time"

	"github.com/google/uuid"
)

type Entry struct {
	PersonID    uuid.UUID  `json:"person_id"`
	DisplayName string     `json:"display_name"`
	Active      bool       `json:"active"`
	Present     bool       `json:"present"`
	Revision    int64      `json:"revision"`
	UpdatedAt   *time.Time `json:"updated_at"`
}
type Counts struct {
	ConfirmedCompetitors int   `json:"confirmed_competitors"`
	MinimumCompetitors   int   `json:"minimum_competitors"`
	CheckedInCompetitors int   `json:"checked_in_competitors"`
	Staff                int   `json:"staff"`
	StaffAlsoCompeting   int   `json:"staff_also_competing"`
	CateringBuffer       int32 `json:"catering_buffer"`
	CateringTotal        int   `json:"catering_total"`
}
type Page struct {
	Revision         int64     `json:"revision"`
	Day              string    `json:"day"`
	Days             []string  `json:"days"`
	Timezone         string    `json:"timezone"`
	RosterAsOf       time.Time `json:"roster_as_of"`
	CanCheckIn       bool      `json:"can_check_in"`
	CanConfigure     bool      `json:"can_configure"`
	CanReadLogistics bool      `json:"can_read_logistics"`
	Counts           Counts    `json:"counts"`
	Items            []Entry   `json:"items"`
}
type Command struct {
	RequestID uuid.UUID `json:"request_id"`
	Present   bool      `json:"present"`
	Reason    string    `json:"reason"`
}
type Change struct {
	RequestID uuid.UUID `json:"request_id"`
	ActorID   uuid.UUID `json:"actor_id"`
	Present   bool      `json:"present"`
	Revision  int64     `json:"revision"`
	Reason    string    `json:"reason"`
	At        time.Time `json:"at"`
}
type OperationalEntry struct {
	PersonID         uuid.UUID `json:"person_id"`
	DisplayName      string    `json:"display_name"`
	Competing        bool      `json:"competing"`
	ContactEmail     *string   `json:"contact_email,omitempty"`
	ContactPhone     *string   `json:"contact_phone,omitempty"`
	Dietary          *string   `json:"dietary,omitempty"`
	Accessibility    *string   `json:"accessibility,omitempty"`
	TravelDetails    *string   `json:"travel_details,omitempty"`
	SocialAttendance *string   `json:"social_attendance,omitempty"`
}
type OperationalPage struct {
	RosterAsOf time.Time          `json:"roster_as_of"`
	Items      []OperationalEntry `json:"items"`
	NextCursor *uuid.UUID         `json:"next_cursor"`
}
