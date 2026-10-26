// Package certificates owns optional explicitly issued attendance/award recognition.
// It never publishes a verification directory or imports private judging evidence.
package certificates

import (
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	Pool *pgxpool.Pool
	Now  func() time.Time
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

type AttendancePolicy struct {
	Days           []string `json:"days"`
	MinimumPresent int      `json:"minimum_present"`
}
type Template struct {
	ID          uuid.UUID `json:"id"`
	Kind        string    `json:"kind"`
	Recognition string    `json:"recognition"`
	Issuer      string    `json:"issuer"`
	Accent      string    `json:"accent"`
}
type Settings struct {
	Version    int64             `json:"version"`
	Attendance *AttendancePolicy `json:"attendance"`
	Templates  []Template        `json:"templates"`
}
type SettingsCommand struct {
	RequestID  uuid.UUID         `json:"request_id"`
	Version    int64             `json:"version"`
	Reviewed   bool              `json:"reviewed"`
	Attendance *AttendancePolicy `json:"attendance"`
	Templates  []Template        `json:"templates"`
	Reason     string            `json:"reason"`
}
type Target struct {
	PersonID      uuid.UUID `json:"person_id"`
	TemplateID    uuid.UUID `json:"template_id"`
	PublicationID uuid.UUID `json:"publication_id"`
	PrizeID       uuid.UUID `json:"prize_id"`
}

// Fields is the complete allowlist for printable personal/event data. No raw
// operational profile, registration answers, scoring or assessment prose exists here.
type Fields struct {
	Name        string `json:"name"`
	Event       string `json:"event"`
	Recognition string `json:"recognition"`
	Issuer      string `json:"issuer"`
	Accent      string `json:"accent"`
	Award       string `json:"award"`
}
type Eligibility struct {
	Target          Target   `json:"target"`
	SettingsVersion int64    `json:"settings_version"`
	Kind            string   `json:"kind"`
	Eligible        bool     `json:"eligible"`
	Blockers        []string `json:"blockers"`
	Binding         string   `json:"binding"`
	Fields          Fields   `json:"fields"`
}
type Command struct {
	RequestID uuid.UUID `json:"request_id"`
	Version   int64     `json:"version"`
	Action    string    `json:"action"`
	Target    Target    `json:"target"`
	Binding   string    `json:"binding"`
	Reason    string    `json:"reason"`
}
type Certificate struct {
	ID              uuid.UUID `json:"id"`
	EventID         uuid.UUID `json:"event_id"`
	PersonID        uuid.UUID `json:"person_id"`
	Version         int64     `json:"version"`
	State           string    `json:"state"`
	Kind            string    `json:"kind"`
	Target          Target    `json:"target"`
	SettingsVersion int64     `json:"settings_version"`
	Binding         string    `json:"binding"`
	Fields          Fields    `json:"fields"`
	IssuedAt        time.Time `json:"issued_at"`
	ChangedAt       time.Time `json:"changed_at"`
}
type History struct {
	Certificate Certificate `json:"certificate"`
	ActorID     uuid.UUID   `json:"actor_id"`
	Reason      string      `json:"reason"`
	RequestID   uuid.UUID   `json:"request_id"`
}
type Receipt struct {
	RequestID     uuid.UUID `json:"request_id"`
	CertificateID uuid.UUID `json:"certificate_id"`
	Version       int64     `json:"version"`
	Action        string    `json:"action"`
	Fingerprint   string    `json:"fingerprint"`
}

type View struct {
	Receipt     *Receipt    `json:"receipt"`
	Certificate Certificate `json:"certificate"`
	Current     bool        `json:"current"`
	History     []History   `json:"history"`
}
