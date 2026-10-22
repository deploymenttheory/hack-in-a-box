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
	ID        uuid.UUID  `json:"id"`
	Label     string     `json:"label"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt time.Time  `json:"expires_at"`
	RevokedAt *time.Time `json:"revoked_at"`
}
type Page struct {
	Revision         int64      `json:"revision"`
	ServerNow        time.Time  `json:"server_now"`
	Timezone         string     `json:"timezone"`
	Draft            []Cue      `json:"draft"`
	Published        []Cue      `json:"published"`
	CurrentIndex     int        `json:"current_index"`
	Mode             string     `json:"mode"`
	RemainingSeconds int        `json:"remaining_seconds"`
	TimerEndsAt      *time.Time `json:"timer_ends_at"`
	Pairings         []Pairing  `json:"pairings"`
}
type Command struct {
	Action string `json:"action"`
	Reason string `json:"reason"`
	Cues   []Cue  `json:"cues,omitempty"`
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
type Snapshot struct {
	EventID          uuid.UUID   `json:"event_id"`
	EventName        string      `json:"event_name"`
	Timezone         string      `json:"timezone"`
	Sequence         int64       `json:"sequence"`
	ServerNow        time.Time   `json:"server_now"`
	Current          *DisplayCue `json:"current"`
	Next             *DisplayCue `json:"next"`
	Mode             string      `json:"mode"`
	RemainingSeconds int         `json:"remaining_seconds"`
	TimerEndsAt      *time.Time  `json:"timer_ends_at"`
	PrimaryColour    string      `json:"primary_colour"`
	BackgroundColour string      `json:"background_colour"`
	ForegroundColour string      `json:"foreground_colour"`
}

func validate(c Command) error {
	if !slices.Contains(
		[]string{"save", "publish", "advance", "hold", "resume", "reset"},
		c.Action,
	) ||
		strings.TrimSpace(c.Reason) == "" ||
		len(c.Reason) > 2000 {
		return access.ErrInvalid
	}
	if c.Action != "save" && len(c.Cues) > 0 {
		return access.ErrInvalid
	}
	if len(c.Cues) > 200 {
		return access.ErrInvalid
	}
	seen := map[uuid.UUID]bool{}
	for _, cue := range c.Cues {
		if cue.ID == uuid.Nil || seen[cue.ID] || strings.TrimSpace(cue.Title) == "" ||
			len(cue.Title) > 200 ||
			len(cue.Text) > 4000 ||
			cue.DurationSeconds < 0 ||
			cue.DurationSeconds > 86400 ||
			(cue.ContentID != nil && *cue.ContentID == uuid.Nil) {
			return access.ErrInvalid
		}
		seen[cue.ID] = true
		if cue.URL != "" {
			u, err := url.Parse(cue.URL)
			if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil ||
				len(cue.URL) > 2048 {
				return access.ErrInvalid
			}
		}
	}
	return nil
}
