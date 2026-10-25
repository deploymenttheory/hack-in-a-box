// Package sessionbooking owns optional session places independently of competition admission.
package sessionbooking

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/event"
)

type Service struct {
	Pool *pgxpool.Pool
	Now  func() time.Time
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// LocalTime uses the event timezone and the same explicit DST choice as native scheduling.
type LocalTime struct {
	Wall      string `json:"wall"`
	DSTChoice string `json:"dst_choice"`
}

type Definition struct {
	DurationMinutes int         `json:"duration_minutes"`
	Capacity        int         `json:"capacity"`
	Room            string      `json:"room"`
	MeetingURL      string      `json:"meeting_url"`
	FacilitatorIDs  []uuid.UUID `json:"facilitator_ids"`
	BookingOpens    LocalTime   `json:"booking_opens"`
	BookingCloses   LocalTime   `json:"booking_closes"`
}

type Booking struct {
	PersonID         uuid.UUID `json:"person_id"`
	State            string    `json:"state"`
	Revision         int64     `json:"revision"`
	WaitlistPosition *int64    `json:"waitlist_position,omitempty"`
}

type Page struct {
	Definition          *Definition `json:"definition,omitempty"`
	NeedsReview         bool        `json:"needs_review"`
	ID                  uuid.UUID   `json:"id"`
	Revision            int64       `json:"revision"`
	CalendarSequence    int64       `json:"calendar_sequence"`
	Title               string      `json:"title"`
	Timezone            string      `json:"timezone"`
	Start               time.Time   `json:"start"`
	End                 time.Time   `json:"end"`
	BookingOpens        time.Time   `json:"booking_opens"`
	BookingCloses       time.Time   `json:"booking_closes"`
	Capacity            int         `json:"capacity"`
	Booked              int         `json:"booked"`
	Available           int         `json:"available"`
	CapacityShortfall   int         `json:"capacity_shortfall"`
	Cancelled           bool        `json:"cancelled"`
	Room                string      `json:"room"`
	MeetingURL          *string     `json:"meeting_url,omitempty"`
	OwnBooking          *Booking    `json:"own_booking,omitempty"`
	Conflicts           []uuid.UUID `json:"conflicts"`
	CanReserve          bool        `json:"can_reserve"`
	CanManage           bool        `json:"can_manage"`
	CanRecordAttendance bool        `json:"can_record_attendance"`
}

type Receipt struct {
	RequestID uuid.UUID `json:"request_id"`
	SessionID uuid.UUID `json:"session_id"`
	PersonID  uuid.UUID `json:"person_id"`
	Action    string    `json:"action"`
	State     string    `json:"state"`
	Revision  int64     `json:"revision"`
}

type AttendanceEntry struct {
	PersonID    uuid.UUID `json:"person_id"`
	DisplayName string    `json:"display_name"`
	Present     bool      `json:"present"`
	Revision    int64     `json:"revision"`
}

func contextError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("session %s: %w", operation, err)
}

type sourceSnapshot struct {
	Item     event.AgendaItem `json:"item"`
	Timezone string           `json:"timezone"`
	Start    time.Time        `json:"start"`
	End      time.Time        `json:"end"`
	Opens    time.Time        `json:"opens"`
	Closes   time.Time        `json:"closes"`
}
