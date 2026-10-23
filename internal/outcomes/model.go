// Package outcomes owns submitted ideas, restricted follow-through and separately
// reviewed project publication. It never mutates competition submissions/scores.
package outcomes

import (
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/automation"
	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/objectives"
)

type Service struct {
	WorkspaceOrigin string
	Composer        *communications.Service
	Automation      *automation.Service

	Media MediaReferences
	Pool  *pgxpool.Pool
	Now   func() time.Time
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

// General pitches have no event/team association. Adoption is a deliberate
// organiser action; knowing an event UUID never grants its configuration access.
type Definition struct {
	Kind           string `json:"kind"`
	Problem        string `json:"problem"`
	Audience       string `json:"audience"`
	ProposedValue  string `json:"proposed_value"`
	Solution       string `json:"solution"`
	BacklogURL     string `json:"backlog_url"`
	ThemeSensitive bool   `json:"theme_sensitive"`
}
type Provenance struct {
	EventID *uuid.UUID `json:"event_id"`
	TeamID  *uuid.UUID `json:"team_id"`
}
type Version struct {
	ID              uuid.UUID                     `json:"id"`
	Version         int64                         `json:"version"`
	CommandID       uuid.UUID                     `json:"command_id"`
	AuthorID        uuid.UUID                     `json:"author_id"`
	ActorID         uuid.UUID                     `json:"actor_id"`
	At              time.Time                     `json:"at"`
	Reason          string                        `json:"reason"`
	Definition      Definition                    `json:"definition"`
	Provenance      Provenance                    `json:"provenance"`
	Status          string                        `json:"status"`
	OwnerID         *uuid.UUID                    `json:"owner_id"`
	ReviewAt        *time.Time                    `json:"review_at"`
	Measures        []objectives.MeasureReference `json:"measures"`
	EvidenceAddedID *uuid.UUID                    `json:"evidence_added_id"`
}

// Evidence is always restricted. A separately reviewed project is constructed
// from PublicFields, never by serialising or subtracting fields from this type.
type Evidence struct {
	ID          uuid.UUID                     `json:"id"`
	Basis       string                        `json:"basis"`
	Claim       string                        `json:"claim"`
	Value       *string                       `json:"value"`
	Denominator *string                       `json:"denominator"`
	Source      string                        `json:"source"`
	ObservedAt  *time.Time                    `json:"observed_at"`
	Measures    []objectives.MeasureReference `json:"measures"`
}
type Command struct {
	ID         uuid.UUID                     `json:"id"`
	Action     string                        `json:"action"`
	Reason     string                        `json:"reason"`
	Definition *Definition                   `json:"definition,omitempty"`
	Provenance *Provenance                   `json:"provenance,omitempty"`
	Status     string                        `json:"status"`
	OwnerID    *uuid.UUID                    `json:"owner_id"`
	ReviewAt   *time.Time                    `json:"review_at"`
	Measures   []objectives.MeasureReference `json:"measures"`
	Evidence   *Evidence                     `json:"evidence,omitempty"`
}
type Capability struct {
	CanRevise   bool `json:"can_revise"`
	CanModerate bool `json:"can_moderate"`
	CanFollowUp bool `json:"can_follow_up"`
	CanAdopt    bool `json:"can_adopt"`
	Archived    bool `json:"archived"`
}
type View struct {
	Version
	Capability     Capability `json:"capability"`
	CurrentVersion int64      `json:"current_version"`
}
type Page struct {
	Items      []View     `json:"items"`
	NextCursor *uuid.UUID `json:"next_cursor"`
}
type History struct {
	Items      []Version `json:"items"`
	NextCursor *int64    `json:"next_cursor"`
}

var decimal = regexp.MustCompile(`^-?(?:0|[1-9][0-9]{0,17})(?:\.[0-9]{1,12})?$`)

func text(value string, bound int) bool {
	return len(value) <= bound && utf8.ValidString(value) && !strings.ContainsRune(value, 0)
}

func required(value string, bound int) bool {
	return strings.TrimSpace(value) != "" && text(value, bound)
}

func referenceURL(value string) bool {
	if value == "" {
		return true
	}
	if !text(value, 2048) || strings.ContainsAny(value, "\r\n\t") {
		return false
	}
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme == "https" && parsed.Hostname() != "" &&
		parsed.User == nil &&
		parsed.Opaque == ""
}

func (d Definition) Validate() error {
	if !slices.Contains([]string{"pitch", "team_outcome"}, d.Kind) || !required(d.Problem, 10000) ||
		!required(d.Audience, 4000) ||
		!required(d.ProposedValue, 10000) ||
		!text(d.Solution, 10000) ||
		!referenceURL(d.BacklogURL) {
		return access.ErrInvalid
	}
	return nil
}

func validRefs(refs []objectives.MeasureReference) bool {
	if len(refs) > 100 {
		return false
	}
	seen := map[objectives.MeasureReference]bool{}
	for _, ref := range refs {
		if ref.Objective.ID == uuid.Nil || ref.Objective.Version < 1 || ref.MeasureID == uuid.Nil ||
			seen[ref] {
			return false
		}
		seen[ref] = true
	}
	return true
}

func (e Evidence) Validate() error {
	if e.Denominator != nil &&
		(!decimal.MatchString(*e.Denominator) || strings.HasPrefix(*e.Denominator, "-")) ||
		e.Value != nil && len(e.Measures) > 1 {
		return access.ErrInvalid
	}
	if e.ID == uuid.Nil ||
		!slices.Contains([]string{"estimate", "self_report", "demonstrated_adoption"}, e.Basis) ||
		!required(e.Claim, 10000) ||
		!text(e.Source, 4000) ||
		!validRefs(e.Measures) ||
		e.Value != nil && !decimal.MatchString(*e.Value) {
		return access.ErrInvalid
	}
	if e.Basis == "demonstrated_adoption" &&
		(!required(e.Source, 4000) || e.ObservedAt == nil || e.ObservedAt.IsZero()) {
		return access.ErrInvalid
	}
	return nil
}

func (p Provenance) Validate() error {
	if p.EventID != nil && *p.EventID == uuid.Nil ||
		p.TeamID != nil && (*p.TeamID == uuid.Nil || p.EventID == nil) {
		return access.ErrInvalid
	}
	return nil
}

func (c Command) validateFields() error {
	if c.Action != "propose" && c.Action != "revise" && c.Definition != nil ||
		c.Action != "propose" && c.Action != "adopt" && c.Provenance != nil ||
		c.Action != "moderate" && c.Action != "progress" && c.Status != "" ||
		c.Action != "assign" && (c.OwnerID != nil || c.ReviewAt != nil) ||
		c.Action != "moderate" && len(c.Measures) > 0 ||
		c.Action != "evidence" && c.Evidence != nil {
		return access.ErrInvalid
	}
	return nil
}

func (c Command) Validate() error {
	if err := c.validateFields(); err != nil {
		return err
	}
	if c.ID == uuid.Nil || !required(c.Reason, 2000) ||
		!slices.Contains(
			[]string{"propose", "revise", "adopt", "moderate", "progress", "assign", "evidence"},
			c.Action,
		) ||
		c.OwnerID != nil && *c.OwnerID == uuid.Nil ||
		!validRefs(c.Measures) {
		return access.ErrInvalid
	}
	switch c.Action {
	case "propose", "revise":
		if c.Definition == nil {
			return access.ErrInvalid
		}
		if err := c.Definition.Validate(); err != nil {
			return err
		}
	case "adopt":
		if c.Provenance == nil || c.Provenance.EventID == nil {
			return access.ErrInvalid
		}
	case "progress":
		if !slices.Contains([]string{"in_progress", "closed"}, c.Status) {
			return access.ErrInvalid
		}
	case "moderate":
		if !slices.Contains(
			[]string{"proposed", "reviewing", "accepted", "in_progress", "closed"},
			c.Status,
		) {
			return access.ErrInvalid
		}
	case "evidence":
		if c.Evidence == nil {
			return access.ErrInvalid
		}
		if err := c.Evidence.Validate(); err != nil {
			return err
		}
	}
	if c.Provenance != nil {
		return c.Provenance.Validate()
	}
	return nil
}

type OwnerCandidate struct {
	ID          uuid.UUID `json:"id"`
	DisplayName string    `json:"display_name"`
}
type OwnerPage struct {
	Items      []OwnerCandidate `json:"items"`
	NextCursor *uuid.UUID       `json:"next_cursor"`
}

type EvidenceRecord struct {
	Evidence
	OutcomeVersion int64     `json:"outcome_version"`
	ActorID        uuid.UUID `json:"actor_id"`
	At             time.Time `json:"at"`
}
type EvidencePage struct {
	Items      []EvidenceRecord `json:"items"`
	NextCursor *uuid.UUID       `json:"next_cursor"`
}
