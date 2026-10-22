// Package clarification owns private team questions and immutable common answers.
// Published answers are a separate projection and never disclose their originating
// question or attachments.
package clarification

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
)

// Source is derived from an actual common challenge release, not supplied as an
// alternative to the owning challenge/rubric approval and publication workflows.
type Source struct {
	ChallengeID      uuid.UUID        `json:"challenge_id"`
	ChallengeVersion int64            `json:"challenge_version"`
	Rubric           rubric.Reference `json:"rubric"`
}

type Question struct {
	ID          uuid.UUID   `json:"id"`
	TeamID      uuid.UUID   `json:"team_id"`
	AuthorID    uuid.UUID   `json:"author_id"`
	Revision    int64       `json:"revision"`
	Topic       string      `json:"topic"`
	Body        string      `json:"body"`
	Attachments []uuid.UUID `json:"attachments"`
	Source      Source      `json:"source"`
	Status      string      `json:"status"`
	CreatedAt   time.Time   `json:"created_at"`
}

type QuestionInput struct {
	TeamID      uuid.UUID   `json:"team_id"`
	CommandID   uuid.UUID   `json:"command_id"`
	Topic       string      `json:"topic"`
	Body        string      `json:"body"`
	Attachments []uuid.UUID `json:"attachments"`
}

// AnswerDocument has no private-question reference or attachment field. A material
// change announces a real owning release; it cannot carry scoring overrides.
type AnswerDocument struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Kind  string `json:"kind"`
}

type PublishedAnswer struct {
	ID            uuid.UUID      `json:"id"`
	Version       int64          `json:"version"`
	Document      AnswerDocument `json:"document"`
	Source        Source         `json:"source"`
	AuthorName    string         `json:"author_name"`
	AuthoredAt    time.Time      `json:"authored_at"`
	ApprovedAt    time.Time      `json:"approved_at"`
	PublishedAt   time.Time      `json:"published_at"`
	Current       bool           `json:"current"`
	SourceCurrent bool           `json:"source_current"`
}

type PublicPage struct {
	Answer         PublishedAnswer   `json:"answer"`
	History        []PublishedAnswer `json:"history"`
	HistoryHasMore bool              `json:"history_has_more"`
	Archived       bool              `json:"archived"`
}

type PublicList struct {
	Items      []PublishedAnswer `json:"items"`
	NextCursor *uuid.UUID        `json:"next_cursor"`
}

type PublicHistory struct {
	Items      []PublishedAnswer `json:"items"`
	NextCursor *int64            `json:"next_cursor"`
}

type QuestionList struct {
	Items      []Question `json:"items"`
	NextCursor *uuid.UUID `json:"next_cursor"`
}

type Transition struct {
	ID       uuid.UUID `json:"id"`
	Revision int64     `json:"revision"`
	Version  *int64    `json:"version"`
	Action   string    `json:"action"`
	ActorID  uuid.UUID `json:"actor_id"`
	At       time.Time `json:"at"`
}

type QuestionHistory struct {
	Items      []Transition `json:"items"`
	NextCursor *uuid.UUID   `json:"next_cursor"`
}

// AnswerConfiguration is an explicit staff-only editing projection. It must never
// be reused by the common answer reader or notification destination.
type AnswerConfiguration struct {
	ID         uuid.UUID      `json:"id"`
	QuestionID uuid.UUID      `json:"question_id"`
	Revision   int64          `json:"revision"`
	Version    int64          `json:"version"`
	Document   AnswerDocument `json:"document"`
	Source     Source         `json:"source"`
	Approved   bool           `json:"approved"`
	Published  bool           `json:"published"`
}

type AnswerCommand struct {
	Action  string `json:"action"`
	Version int64  `json:"version"`
}

func (c AnswerCommand) validate(expected int64) error {
	if expected < 1 || c.Version < 1 || (c.Action != "approve" && c.Action != "publish") {
		return access.ErrInvalid
	}
	return nil
}

func validText(value string, limit int) bool {
	return strings.TrimSpace(value) != "" && len(value) <= limit &&
		utf8.ValidString(value) && !strings.ContainsRune(value, 0)
}

func (in QuestionInput) validate() error {
	if in.TeamID == uuid.Nil || in.CommandID == uuid.Nil ||
		(in.Topic != "challenge" && in.Topic != "rubric") ||
		!validText(in.Body, 8000) || len(in.Attachments) > 10 {
		return access.ErrInvalid
	}
	seen := make(map[uuid.UUID]bool, len(in.Attachments))
	for _, id := range in.Attachments {
		if id == uuid.Nil || seen[id] {
			return access.ErrInvalid
		}
		seen[id] = true
	}
	return nil
}

func (d AnswerDocument) validate() error {
	if !validText(d.Title, 200) || !validText(d.Body, 16000) ||
		(d.Kind != "clarification" && d.Kind != "challenge_change" && d.Kind != "rubric_change") {
		return access.ErrInvalid
	}
	return nil
}
