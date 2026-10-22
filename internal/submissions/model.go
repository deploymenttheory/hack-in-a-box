// Package submissions owns private team drafts, immutable formal evidence and
// clock-driven final outcomes. Repository URLs are references, not SHA receipts.
package submissions

import (
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/templates"
)

type Definition struct {
	templates.Deliverable
	DueAt time.Time `json:"due_at"`
}
type Evidence struct {
	DeliverableID uuid.UUID  `json:"deliverable_id"`
	Kind          string     `json:"kind"`
	Label         string     `json:"label"`
	URL           string     `json:"url"`
	FileID        *uuid.UUID `json:"file_id"`
}
type Document struct {
	Definitions []Definition `json:"definitions"`
	Evidence    []Evidence   `json:"evidence"`
}
type Revision struct {
	Document
	Revision     int64     `json:"revision"`
	ActorID      uuid.UUID `json:"actor_id"`
	SubmittedAt  time.Time `json:"submitted_at"`
	Late         bool      `json:"late"`
	Reason       string    `json:"reason"`
	ReviewStatus string    `json:"review_status"`
}
type Draft struct {
	Revision  int64      `json:"revision"`
	Evidence  []Evidence `json:"evidence"`
	UpdatedAt *time.Time `json:"updated_at"`
}
type Page struct {
	EventRevision int64        `json:"event_revision"`
	Revision      int64        `json:"revision"`
	EntryID       uuid.UUID    `json:"entry_id"`
	TeamID        uuid.UUID    `json:"team_id"`
	Outcome       string       `json:"outcome"`
	FrozenAt      *time.Time   `json:"frozen_at"`
	CanDraft      bool         `json:"can_draft"`
	CanSubmit     bool         `json:"can_submit"`
	CanReview     bool         `json:"can_review"`
	LiveStart     time.Time    `json:"live_start"`
	Cutoff        time.Time    `json:"cutoff"`
	Definitions   []Definition `json:"definitions"`
	Final         *Revision    `json:"final"`
}
type Change struct {
	Revision       int64      `json:"revision"`
	ActorID        *uuid.UUID `json:"actor_id"`
	Action         string     `json:"action"`
	ActiveRevision *int64     `json:"active_revision"`
	ReviewStatus   string     `json:"review_status"`
	Reason         string     `json:"reason"`
	At             time.Time  `json:"at"`
	Submission     *Revision  `json:"submission"`
}
type History struct {
	Items      []Change `json:"items"`
	NextCursor *int64   `json:"next_cursor"`
}
type Command struct {
	Evidence []Evidence `json:"evidence"`
	Reason   string     `json:"reason"`
}

func blocked(field, code, message string) error {
	return &event.Blocked{Issues: []event.Issue{{Field: field, Code: code, Message: message}}}
}

func audience(d Definition) []string {
	if len(d.Audience) == 0 {
		return []string{"organiser", "mentor", "judge"}
	}
	return d.Audience
}

func kinds(d Definition) []string {
	if len(d.EvidenceKinds) == 0 {
		return []string{"file", "url", "repository"}
	}
	return d.EvidenceKinds
}

func validateEvidence(defs []Definition, items []Evidence, complete bool) error {
	if len(items) > 100 {
		return blocked("evidence", "too_many", "Use at most 100 evidence items.")
	}
	byID := map[uuid.UUID]Definition{}
	seen := map[uuid.UUID]bool{}
	for _, d := range defs {
		byID[d.ID] = d
	}
	for _, e := range items {
		d, ok := byID[e.DeliverableID]
		if !ok || seen[e.DeliverableID] || !slices.Contains(kinds(d), e.Kind) ||
			strings.TrimSpace(e.Label) == "" ||
			len(e.Label) > 300 {
			return blocked(
				"evidence",
				"invalid_evidence",
				"Choose one permitted evidence item for each defined deliverable.",
			)
		}
		seen[e.DeliverableID] = true
		if e.Kind == "file" {
			if e.FileID == nil || *e.FileID == uuid.Nil || e.URL != "" {
				return blocked(
					"evidence",
					"invalid_file",
					"Choose a protected uploaded file without a URL.",
				)
			}
		} else {
			u, err := url.Parse(e.URL)
			if e.FileID != nil || err != nil || u.Host == "" || u.User != nil ||
				!slices.Contains([]string{"https", "http"}, u.Scheme) ||
				len(e.URL) > 2048 {
				return blocked(
					"evidence",
					"invalid_url",
					"Use an HTTP or HTTPS reference without embedded credentials.",
				)
			}
		}
	}
	if complete {
		if len(defs) == 0 {
			return blocked(
				"deliverables",
				"not_configured",
				"Configure competition deliverables before accepting final submissions.",
			)
		}
		for _, d := range defs {
			if d.Required && !seen[d.ID] {
				return blocked(
					"evidence",
					"required_evidence",
					"Supply every required deliverable before submitting.",
				)
			}
		}
	}
	return nil
}
