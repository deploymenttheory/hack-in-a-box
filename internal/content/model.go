// Package content owns immutable native pages and guarded audience publication.
package content

import (
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/permissions"
)

type Document struct {
	Title       string                 `json:"title"`
	Markdown    string                 `json:"markdown"`
	Audience    access.ContentAudience `json:"audience"`
	Attachments []uuid.UUID            `json:"attachments"`
	ScheduledAt *time.Time             `json:"scheduled_at"`
}
type Input struct {
	Kind     string   `json:"kind"`
	Document Document `json:"document"`
}
type Page struct {
	AttachmentDetails []Attachment `json:"attachment_details"`
	ID                uuid.UUID    `json:"id"`
	Kind              string       `json:"kind"`
	Revision          int64        `json:"revision"`
	Version           int64        `json:"version"`
	State             string       `json:"state"`
	Document          Document     `json:"document"`
	ReleasedVersion   *int64       `json:"released_version"`
	ReleasedAt        *time.Time   `json:"released_at"`
	CanManage         bool         `json:"can_manage"`
	Message           string       `json:"message"`
}
type List struct {
	Items      []Page     `json:"items"`
	NextCursor *uuid.UUID `json:"next_cursor"`
}
type Change struct {
	Revision    int64      `json:"revision"`
	Version     int64      `json:"version"`
	Action      string     `json:"action"`
	ActorID     *uuid.UUID `json:"actor_id"`
	Reason      string     `json:"reason"`
	ScheduledAt *time.Time `json:"scheduled_at"`
	At          time.Time  `json:"at"`
}
type History struct {
	Items      []Change `json:"items"`
	NextCursor *int64   `json:"next_cursor"`
}
type Notice struct {
	ID               uuid.UUID `json:"id"`
	Revision         int64     `json:"revision"`
	LiveStart        time.Time `json:"live_start"`
	LiveEnd          time.Time `json:"live_end"`
	SubmissionCutoff time.Time `json:"submission_cutoff"`
}

func blocked(code, message string) error {
	return &event.Blocked{Issues: []event.Issue{{Field: "content", Code: code, Message: message}}}
}

func validate(input Input) error {
	d := input.Document
	if !slices.Contains([]string{"page", "brief", "challenge"}, input.Kind) ||
		strings.TrimSpace(d.Title) == "" ||
		len(d.Title) > 200 ||
		len(d.Markdown) > 100000 ||
		len(d.Attachments) > 30 ||
		len(d.Audience.Teams) > 100 ||
		len(d.Audience.Roles) > len(permissions.Roles) {
		return access.ErrInvalid
	}
	if !d.Audience.Confirmed && len(d.Audience.Roles) == 0 && len(d.Audience.Teams) == 0 {
		return blocked("audience_required", "Choose at least one content audience.")
	}
	for _, r := range d.Audience.Roles {
		if !slices.Contains(permissions.Roles, r) {
			return access.ErrInvalid
		}
	}
	if input.Kind == "challenge" &&
		(!d.Audience.Confirmed || len(d.Audience.Roles) > 0 || len(d.Audience.Teams) > 0) {
		return blocked(
			"common_challenge",
			"The challenge is published to all confirmed attendees together.",
		)
	}
	seen := map[uuid.UUID]bool{}
	for _, id := range append(append([]uuid.UUID{}, d.Attachments...), d.Audience.Teams...) {
		if id == uuid.Nil || seen[id] {
			return access.ErrInvalid
		}
		seen[id] = true
	}
	return nil
}
