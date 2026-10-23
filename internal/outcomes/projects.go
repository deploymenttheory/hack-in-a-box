package outcomes

import (
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
)

// PublicationFields is a positive projection chosen for separate organiser
// review. Restricted Evidence, author identity, owner and private history cannot
// enter it by accidental whole-resource serialisation.
type PublicationFields struct {
	Problem        string        `json:"problem"`
	Solution       string        `json:"solution"`
	Links          []ProjectLink `json:"links"`
	Audience       string        `json:"audience"`
	ShareEventName bool          `json:"share_event_name"`
	ShareTeamName  bool          `json:"share_team_name"`
}
type ProjectLink struct {
	ID             uuid.UUID `json:"id"`
	Label          string    `json:"label"`
	URL            string    `json:"url"`
	Media          bool      `json:"media"`
	ContainsPeople bool      `json:"contains_people"`
	// Consent metadata belongs only to the private review record. PublicLink has
	// no person IDs. Current consent is checked again on each projection read.
	People []uuid.UUID `json:"people"`
}
type PublicLink struct {
	Label string `json:"label"`
	URL   string `json:"url"`
	Media bool   `json:"media"`
}
type Project struct {
	ID             uuid.UUID    `json:"id"`
	Version        int64        `json:"version"`
	Problem        string       `json:"problem"`
	Solution       string       `json:"solution"`
	Links          []PublicLink `json:"links"`
	Source         Provenance   `json:"source"`
	EventName      *string      `json:"event_name"`
	TeamName       *string      `json:"team_name"`
	FollowUpStatus string       `json:"follow_up_status"`
	PublishedAt    time.Time    `json:"published_at"`
}
type ProjectPage struct {
	Items      []Project  `json:"items"`
	NextCursor *uuid.UUID `json:"next_cursor"`
}
type PublicationVersion struct {
	OutcomeID     uuid.UUID         `json:"outcome_id"`
	Version       int64             `json:"version"`
	SourceVersion int64             `json:"source_version"`
	CommandID     uuid.UUID         `json:"command_id"`
	Fields        PublicationFields `json:"fields"`
	State         string            `json:"state"`
	Reason        string            `json:"reason"`
	ActorID       uuid.UUID         `json:"actor_id"`
	At            time.Time         `json:"at"`
}
type PublicationCommand struct {
	ID            uuid.UUID          `json:"id"`
	Action        string             `json:"action"`
	SourceVersion int64              `json:"source_version"`
	Fields        *PublicationFields `json:"fields"`
	Reason        string             `json:"reason"`
}

func (p PublicationFields) Validate() error {
	if !required(p.Problem, 10000) || !required(p.Solution, 10000) ||
		!slices.Contains([]string{"organisation", "event", "team"}, p.Audience) ||
		len(p.Links) > 30 {
		return access.ErrInvalid
	}
	seen := map[uuid.UUID]bool{}
	for _, link := range p.Links {
		if link.ID == uuid.Nil || seen[link.ID] || !required(link.Label, 240) ||
			strings.TrimSpace(link.URL) == "" ||
			!referenceURL(link.URL) ||
			len(link.People) > 100 ||
			!link.Media && len(link.People) > 0 {
			return access.ErrInvalid
		}
		seen[link.ID] = true
		people := map[uuid.UUID]bool{}
		for _, person := range link.People {
			if person == uuid.Nil || people[person] {
				return access.ErrInvalid
			}
			people[person] = true
		}
	}
	return nil
}

func (c PublicationCommand) Validate() error {
	if c.ID == uuid.Nil || c.SourceVersion < 1 || !required(c.Reason, 2000) ||
		!slices.Contains([]string{"save", "publish", "withdraw"}, c.Action) {
		return access.ErrInvalid
	}
	if c.Action == "save" {
		if c.Fields == nil {
			return access.ErrInvalid
		}
		return c.Fields.Validate()
	}
	if c.Fields != nil {
		return access.ErrInvalid
	}
	return nil
}
