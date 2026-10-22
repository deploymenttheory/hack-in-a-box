// Package presentation owns approved public-to-room cues and scoped display
// sessions. A display never inherits the controller's account permissions.
package presentation

import (
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
)

type Cue struct {
	ID              uuid.UUID  `json:"id"`
	Title           string     `json:"title"`
	Text            string     `json:"text"`
	URL             string     `json:"url"`
	DurationSeconds int32      `json:"duration_seconds"`
	ContentID       *uuid.UUID `json:"content_id"`
}
type Pairing struct {
	LastSeenAt *time.Time `json:"last_seen_at"`
	Stale      bool       `json:"stale"`
	ID         uuid.UUID  `json:"id"`
	Label      string     `json:"label"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
}
type Page struct {
	CanControl               bool       `json:"can_control"`
	Override                 *Cue       `json:"override"`
	OverrideMode             string     `json:"override_mode"`
	OverrideRemainingSeconds int        `json:"override_remaining_seconds"`
	OverrideTimerEndsAt      *time.Time `json:"override_timer_ends_at"`
	Revision                 int64      `json:"revision"`
	ServerNow                time.Time  `json:"server_now"`
	Timezone                 string     `json:"timezone"`
	Draft                    []Cue      `json:"draft"`
	Published                []Cue      `json:"published"`
	CurrentIndex             int        `json:"current_index"`
	Mode                     string     `json:"mode"`
	RemainingSeconds         int        `json:"remaining_seconds"`
	TimerEndsAt              *time.Time `json:"timer_ends_at"`
	Pairings                 []Pairing  `json:"pairings"`
}
type Command struct {
	FromDraft bool   `json:"from_draft,omitempty"`
	Index     *int   `json:"index,omitempty"`
	Seconds   *int32 `json:"seconds,omitempty"`
	Override  *Cue   `json:"override,omitempty"`
	Action    string `json:"action"`
	Reason    string `json:"reason"`
	Cues      []Cue  `json:"cues,omitempty"`
}
type PairInput struct {
	Label   string `json:"label"`
	Minutes int    `json:"minutes"`
	Reason  string `json:"reason"`
}
type Paired struct {
	Revision int64   `json:"revision"`
	Pairing  Pairing `json:"pairing"`
	Token    string  `json:"token"`
}
type DisplayCue struct {
	ID           uuid.UUID `json:"id"`
	Title        string    `json:"title"`
	Text         string    `json:"text"`
	URL          string    `json:"url"`
	ContentTitle string    `json:"content_title"`
}
type DisplayAsset struct {
	ID        uuid.UUID `json:"id"`
	Kind      string    `json:"kind"`
	MediaType string    `json:"media_type"`
	Digest    string    `json:"digest"`
}
type Snapshot struct {
	Assets           []DisplayAsset `json:"assets"`
	OverrideActive   bool           `json:"override_active"`
	EventID          uuid.UUID      `json:"event_id"`
	EventName        string         `json:"event_name"`
	Timezone         string         `json:"timezone"`
	Sequence         int64          `json:"sequence"`
	ServerNow        time.Time      `json:"server_now"`
	Current          *DisplayCue    `json:"current"`
	Next             *DisplayCue    `json:"next"`
	Mode             string         `json:"mode"`
	RemainingSeconds int            `json:"remaining_seconds"`
	TimerEndsAt      *time.Time     `json:"timer_ends_at"`
	PrimaryColour    string         `json:"primary_colour"`
	BackgroundColour string         `json:"background_colour"`
	ForegroundColour string         `json:"foreground_colour"`
}

func validate(c Command) error {
	if !slices.Contains(
		[]string{
			"save",
			"publish",
			"advance",
			"back",
			"activate",
			"hold",
			"resume",
			"reset",
			"timer",
			"override",
			"return_to_schedule",
		},
		c.Action,
	) ||
		strings.TrimSpace(c.Reason) == "" ||
		len(c.Reason) > 2000 {
		return access.ErrInvalid
	}
	if c.FromDraft && c.Action != "activate" {
		return access.ErrInvalid
	}
	if c.Action != "save" && len(c.Cues) > 0 {
		return access.ErrInvalid
	}
	if (c.Action == "activate") != (c.Index != nil) ||
		(c.Index != nil && (*c.Index < 0 || *c.Index >= 200)) {
		return access.ErrInvalid
	}
	if (c.Action == "timer") != (c.Seconds != nil) ||
		(c.Seconds != nil && (*c.Seconds < 0 || *c.Seconds > 86400)) {
		return access.ErrInvalid
	}
	if (c.Action == "override") != (c.Override != nil) || len(c.Cues) > 200 {
		return access.ErrInvalid
	}
	seen := map[uuid.UUID]bool{}
	for _, cue := range c.Cues {
		if seen[cue.ID] {
			return access.ErrInvalid
		}
		seen[cue.ID] = true
		if err := validateCue(cue); err != nil {
			return err
		}
	}
	if c.Override != nil {
		return validateCue(*c.Override)
	}
	return nil
}

func validateCue(cue Cue) error {
	if cue.ID == uuid.Nil || strings.TrimSpace(cue.Title) == "" || len(cue.Title) > 200 ||
		len(cue.Text) > 4000 ||
		cue.DurationSeconds < 0 ||
		cue.DurationSeconds > 86400 ||
		(cue.ContentID != nil && *cue.ContentID == uuid.Nil) {
		return access.ErrInvalid
	}
	if cue.URL != "" {
		u, err := url.Parse(cue.URL)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil ||
			len(cue.URL) > 2048 {
			return access.ErrInvalid
		}
	}
	return nil
}
