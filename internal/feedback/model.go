// Package feedback owns deliberately reviewed team-visible support, never scores.
package feedback

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
)

type (
	Source struct {
		Kind    string    `json:"kind"`
		ID      uuid.UUID `json:"id"`
		Version int64     `json:"version"`
	}
	// SourcePolicy must use the native owner's current exact-resource authority.
	// It returns no original question, observation, request prose or attachments.
	SourcePolicy interface {
		AuthorizeInTransaction(
			context.Context,
			pgx.Tx,
			uuid.UUID,
			uuid.UUID,
			uuid.UUID,
			Source,
		) error
	}
	Document struct {
		ID             uuid.UUID  `json:"id"`
		TeamID         uuid.UUID  `json:"team_id"`
		Version        int64      `json:"version"`
		Category       string     `json:"category"`
		Title          string     `json:"title"`
		Body           string     `json:"body"`
		State          string     `json:"state"`
		Sources        []Source   `json:"sources"`
		HelpID         *uuid.UUID `json:"help_request_id"`
		AuthorID       uuid.UUID  `json:"author_id"`
		ReviewerID     *uuid.UUID `json:"reviewer_id"`
		ReviewedDigest string     `json:"reviewed_digest"`
		PublishedAt    *time.Time `json:"published_at"`
		ChangedAt      time.Time  `json:"changed_at"`
	}
)

type (
	Input struct {
		TeamID   uuid.UUID  `json:"team_id"`
		Category string     `json:"category"`
		Title    string     `json:"title"`
		Body     string     `json:"body"`
		Sources  []Source   `json:"sources"`
		HelpID   *uuid.UUID `json:"help_request_id"`
	}
	Command struct {
		Action string `json:"action"`
		Input  *Input `json:"input"`
	}
	Action struct {
		ID         uuid.UUID `json:"id"`
		FeedbackID uuid.UUID `json:"feedback_id"`
		Version    int64     `json:"version"`
		Title      string    `json:"title"`
		Body       string    `json:"body"`
		OwnerID    uuid.UUID `json:"owner_id"`
		DueAt      time.Time `json:"due_at"`
		Status     string    `json:"status"`
		Cycle      int64     `json:"cycle"`
		ChangedBy  uuid.UUID `json:"changed_by"`
		ChangedAs  string    `json:"changed_as"`
		Change     string    `json:"change"`
		ChangedAt  time.Time `json:"changed_at"`
	}
)

type (
	ActionInput struct {
		Reviewed bool      `json:"reviewed"`
		Title    string    `json:"title"`
		Body     string    `json:"body"`
		OwnerID  uuid.UUID `json:"owner_id"`
		DueAt    time.Time `json:"due_at"`
	}
	ActionCommand struct {
		Action string       `json:"action"`
		Input  *ActionInput `json:"input"`
		Status string       `json:"status"`
	}
	ActionView struct {
		writeAs         string
		CommandVersion  *int64 `json:"command_version"`
		Action          Action `json:"action"`
		CanEdit         bool   `json:"can_edit"`
		CanUpdateStatus bool   `json:"can_update_status"`
		OwnerAvailable  bool   `json:"owner_available"`
		Overdue         bool   `json:"overdue"`
	}
	Acknowledgement struct {
		Version  int64     `json:"version"`
		PersonID uuid.UUID `json:"person_id"`
		At       time.Time `json:"at"`
	}
	Escalation struct {
		ID               uuid.UUID `json:"id"`
		ActionID         uuid.UUID `json:"action_id"`
		Cycle            int64     `json:"cycle"`
		SettingsVersion  int64     `json:"settings_version"`
		RecipientID      uuid.UUID `json:"recipient_id"`
		ThresholdSeconds int64     `json:"threshold_seconds"`
		At               time.Time `json:"at"`
	}
	Candidate struct {
		PersonID   uuid.UUID `json:"person_id"`
		Name       string    `json:"name"`
		TeamMember bool      `json:"team_member"`
	}
	Page struct {
		CommandVersion   *int64            `json:"command_version"`
		CurrentVersion   int64             `json:"current_version"`
		Published        *Document         `json:"published"`
		Draft            *Document         `json:"draft"`
		CanManage        bool              `json:"can_manage"`
		CanAcknowledge   bool              `json:"can_acknowledge"`
		Acknowledgements []Acknowledgement `json:"acknowledgements"`
		Escalations      []Escalation      `json:"escalations"`
	}
	List struct {
		Items      []Page     `json:"items"`
		NextCursor *uuid.UUID `json:"next_cursor"`
	}
	Actions struct {
		Items      []ActionView `json:"items"`
		NextCursor *uuid.UUID   `json:"next_cursor"`
	}
	History struct {
		Items      []Document `json:"items"`
		NextCursor *int64     `json:"next_cursor"`
	}
	ActionHistory struct {
		Items      []Action `json:"items"`
		NextCursor *int64   `json:"next_cursor"`
	}
	Settings struct {
		Version          int64 `json:"version"`
		SetupSeconds     int64 `json:"setup_seconds"`
		LogisticsSeconds int64 `json:"logistics_seconds"`
		OutputSeconds    int64 `json:"output_seconds"`
	}
)

func validCategory(c string) bool {
	return slices.Contains([]string{"setup", "logistics", "competition_output"}, c)
}

func (in Input) validate() error {
	if in.TeamID == uuid.Nil || !validCategory(in.Category) || strings.TrimSpace(in.Title) == "" ||
		len(in.Title) > 200 ||
		strings.TrimSpace(in.Body) == "" ||
		len(in.Body) > 12000 ||
		strings.ContainsRune(in.Title+in.Body, 0) ||
		len(in.Sources) > 10 {
		return access.ErrInvalid
	}
	for _, s := range in.Sources {
		if !slices.Contains([]string{"observation", "question", "help"}, s.Kind) ||
			s.ID == uuid.Nil ||
			s.Version < 1 {
			return access.ErrInvalid
		}
		if in.Category != "competition_output" && s.Kind != "help" {
			return access.ErrInvalid
		}
	}
	if in.HelpID != nil && *in.HelpID == uuid.Nil {
		return access.ErrInvalid
	}
	return nil
}

func (in ActionInput) validate() error {
	if !in.Reviewed || strings.TrimSpace(in.Title) == "" || len(in.Title) > 200 ||
		strings.TrimSpace(in.Body) == "" ||
		len(in.Body) > 4000 ||
		strings.ContainsRune(in.Title+in.Body, 0) ||
		in.OwnerID == uuid.Nil ||
		in.DueAt.IsZero() {
		return access.ErrInvalid
	}
	return nil
}

func (s Settings) validate() error {
	for _, v := range []int64{s.SetupSeconds, s.LogisticsSeconds, s.OutputSeconds} {
		if v < 60 || v > 86400 {
			return access.ErrInvalid
		}
	}
	return nil
}

func (s Settings) threshold(category string) int64 {
	if category == "setup" {
		return s.SetupSeconds
	}
	if category == "logistics" {
		return s.LogisticsSeconds
	}
	return s.OutputSeconds
}
