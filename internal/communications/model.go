// Package communications owns versioned message composition and current-audience
// resolution. Delivery providers and campaign execution records belong to callers.
package communications

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
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

type Template struct {
	ID      uuid.UUID  `json:"id"`
	Version int64      `json:"version"`
	Name    string     `json:"name"`
	Purpose string     `json:"purpose"`
	Format  string     `json:"format"`
	Subject string     `json:"subject"`
	Body    string     `json:"body"`
	FactID  *uuid.UUID `json:"fact_id"`
}
type Audience struct {
	ID                 uuid.UUID   `json:"id"`
	Version            int64       `json:"version"`
	Name               string      `json:"name"`
	Roles              []string    `json:"roles"`
	RegistrationStates []string    `json:"registration_states"`
	Teams              []uuid.UUID `json:"teams"`
	Outstanding        string      `json:"outstanding"`
	FactID             *uuid.UUID  `json:"fact_id"`
}
type Recipient struct {
	PersonID   uuid.UUID `json:"person_id"`
	Name       string    `json:"name"`
	Subject    string    `json:"subject"`
	Body       string    `json:"body"`
	Unresolved []string  `json:"unresolved"`
}
type Preview struct {
	ID              uuid.UUID   `json:"id"`
	TemplateID      uuid.UUID   `json:"template_id"`
	TemplateVersion int64       `json:"template_version"`
	AudienceID      uuid.UUID   `json:"audience_id"`
	AudienceVersion int64       `json:"audience_version"`
	Recipients      []Recipient `json:"recipients"`
	Blockers        []string    `json:"blockers"`
	Fingerprint     string      `json:"fingerprint"`
	Approved        bool        `json:"approved"`
}
type Page struct {
	Revision  int64      `json:"revision"`
	Templates []Template `json:"templates"`
	Audiences []Audience `json:"audiences"`
	Variables []string   `json:"variables"`
	Preview   *Preview   `json:"preview"`
}
type Command struct {
	Action     string    `json:"action"`
	Template   *Template `json:"template"`
	Audience   *Audience `json:"audience"`
	TemplateID uuid.UUID `json:"template_id"`
	AudienceID uuid.UUID `json:"audience_id"`
	PreviewID  uuid.UUID `json:"preview_id"`
	Reason     string    `json:"reason"`
}

var (
	purposes = []string{
		"facilitator_selection",
		"registration",
		"offer",
		"preparation",
		"brief_release",
		"challenge_release",
		"reminder",
		"feedback",
		"results",
		"postponement",
		"help",
		"clarification",
		"rubric",
		"outcome_review",
	}
	variables = []string{
		"event.name",
		"event.workspace",
		"event.final_admission_cutoff",
		"event.team_freeze",
		"event.challenge_publication",
		"event.live_start",
		"event.live_end",
		"event.submission_cutoff",
		"event.timezone",
		"person.name",
		"team.name",
		"challenge.title",
		"brief.title",
		"help.status",
		"help.revision",
		"clarification.revision",
		"rubric.revision",
		"outcome_review.status",
		"outcome_review.revision", "results.revision",
	}
	placeholder   = regexp.MustCompile(`\{\{\s*([a-z][a-z0-9_.]*)\s*\}\}`)
	ErrUnresolved = errors.New("message has unresolved or changed approved facts")
)

func blocked(code string) error {
	return &event.Blocked{
		Issues: []event.Issue{
			{
				Field:   "communications",
				Code:    code,
				Message: "Review the current message, audience and unresolved variables before approval.",
			},
		},
	}
}

func tokens(value string) ([]string, error) {
	out := []string{}
	for _, match := range placeholder.FindAllStringSubmatch(value, -1) {
		if !slices.Contains(variables, match[1]) {
			return nil, blocked("unknown_placeholder")
		}
		out = append(out, match[1])
	}
	rest := placeholder.ReplaceAllString(value, "")
	if strings.Contains(rest, "{{") || strings.Contains(rest, "}}") {
		return nil, blocked("invalid_placeholder")
	}
	return out, nil
}

func validateTemplate(t Template) error {
	if t.ID == uuid.Nil || t.Version < 0 || strings.TrimSpace(t.Name) == "" || len(t.Name) > 160 ||
		!slices.Contains(purposes, t.Purpose) ||
		!slices.Contains([]string{"text", "html", "markdown"}, t.Format) ||
		strings.TrimSpace(t.Subject) == "" ||
		len(t.Subject) > 200 ||
		len(t.Body) > 10000 ||
		strings.ContainsAny(t.Subject, "\r\n\x00") ||
		strings.ContainsRune(t.Body, 0) ||
		(t.FactID != nil && *t.FactID == uuid.Nil) {
		return access.ErrInvalid
	}
	_, err := tokens(t.Subject + "\n" + t.Body)
	return err
}

func validateAudience(a Audience) error {
	if a.ID == uuid.Nil || a.Version < 0 || strings.TrimSpace(a.Name) == "" || len(a.Name) > 160 ||
		len(a.Roles) > 8 ||
		len(a.RegistrationStates) > 8 ||
		len(a.Teams) > 100 ||
		(len(a.Roles)+len(a.RegistrationStates)+len(a.Teams) == 0) ||
		(a.FactID != nil && *a.FactID == uuid.Nil) {
		return access.ErrInvalid
	}
	for _, role := range a.Roles {
		if !slices.Contains(
			[]string{"organiser", "mentor", "judge", "mc", "logistics", "photographer", "attendee"},
			role,
		) {
			return access.ErrInvalid
		}
	}
	for _, state := range a.RegistrationStates {
		if !slices.Contains(
			[]string{"received", "confirmed", "waitlisted", "offered", "selected", "backup"},
			state,
		) {
			return access.ErrInvalid
		}
	}
	for _, id := range a.Teams {
		if id == uuid.Nil {
			return access.ErrInvalid
		}
	}
	if !slices.Contains(
		[]string{"", "preparation", "offer", "help", "outcome_review", "clarification", "rubric"},
		a.Outstanding,
	) {
		return access.ErrInvalid
	}
	return nil
}

func contextError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("communications %s: %w", operation, err)
}
