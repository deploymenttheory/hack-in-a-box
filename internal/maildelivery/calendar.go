package maildelivery

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Calendar is an authorised, immutable event snapshot. Callers persist sequence
// and timestamps with the attempt; retries must reuse it rather than recalculate.
type Calendar struct {
	EventID      uuid.UUID  `json:"event_id"`
	SessionID    *uuid.UUID `json:"session_id,omitempty"`
	PersonID     *uuid.UUID `json:"person_id,omitempty"`
	Cancelled    bool       `json:"cancelled,omitempty"`
	Test         bool       `json:"test"`
	Sequence     int64      `json:"sequence"`
	Start        time.Time  `json:"start"`
	End          time.Time  `json:"end"`
	Timezone     string     `json:"timezone"`
	Summary      string     `json:"summary"`
	WorkspaceURL string     `json:"workspace_url"`
}

func escapeCalendar(value string) string {
	return strings.NewReplacer("\\", "\\\\", "\r\n", "\\n", "\r", "\\n", "\n", "\\n", ";", "\\;", ",", "\\,").
		Replace(value)
}

// foldCalendar counts UTF-8 octets and never splits a code point. Continuation
// whitespace counts toward RFC5545's75-octet content-line recommendation.
func foldCalendar(line string) string {
	var out strings.Builder
	width := 0
	for len(line) > 0 {
		r, n := utf8.DecodeRuneInString(line)
		if width+n > 75 {
			out.WriteString("\r\n ")
			width = 1
		}
		out.WriteRune(r)
		width += n
		line = line[n:]
	}
	out.WriteString("\r\n")
	return out.String()
}

func (c Calendar) render(settings Settings, stamp time.Time) ([]byte, error) {
	if c.SessionID != nil || c.PersonID != nil || c.Cancelled {
		return c.renderSession(settings, stamp)
	}
	if c.EventID == uuid.Nil || c.Sequence < 0 || c.Sequence > 2147483647 || c.Start.IsZero() ||
		!c.End.After(c.Start) ||
		!header(c.Summary, 200) ||
		strings.TrimSpace(c.Summary) == "" ||
		(c.Timezone == "" || !header(c.Timezone, 100)) {
		return nil, ErrMessage
	}
	if _, err := time.LoadLocation(c.Timezone); err != nil {
		return nil, ErrMessage
	}
	if !safeWorkspace(c.WorkspaceURL) {
		return nil, ErrMessage
	}
	uidPrefix := "event-"
	if c.Test {
		uidPrefix = "test-event-"
	}
	lines := []string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//Hack in a Box//Personal Event Calendar//EN",
		"CALSCALE:GREGORIAN",
		"METHOD:PUBLISH",
		"BEGIN:VEVENT",
		"UID:" + uidPrefix + c.EventID.String() + "@" + settings.MessageDomain,
		fmt.Sprintf("SEQUENCE:%d", c.Sequence),
		"DTSTAMP:" + stamp.UTC().Format("20060102T150405Z"),
		"DTSTART:" + c.Start.UTC().Format("20060102T150405Z"),
		"DTEND:" + c.End.UTC().Format("20060102T150405Z"),
		"ORGANIZER:mailto:" + settings.From,
		"SUMMARY:" + escapeCalendar(c.Summary),
		"DESCRIPTION:" + escapeCalendar("Event timezone: "+c.Timezone+"\n"+c.WorkspaceURL),
		"URL:" + c.WorkspaceURL,
		"STATUS:CONFIRMED",
		"END:VEVENT",
		"END:VCALENDAR",
	}
	var result strings.Builder
	for _, line := range lines {
		result.WriteString(foldCalendar(line))
	}
	return []byte(result.String()), nil
}

func (c Calendar) method() string {
	if c.Cancelled {
		return "CANCEL"
	}
	return "PUBLISH"
}

// A session calendar identity is per-person and stable across booking changes.
// Cancellation deliberately carries only that identity and its sequence, never
// old private session titles, times, meeting URLs or current event branding.
func (c Calendar) renderSession(settings Settings, stamp time.Time) ([]byte, error) {
	if c.EventID == uuid.Nil || c.SessionID == nil || *c.SessionID == uuid.Nil ||
		c.PersonID == nil ||
		*c.PersonID == uuid.Nil ||
		c.Sequence < 0 ||
		c.Sequence > 2147483647 {
		return nil, ErrMessage
	}
	prefix := "session-"
	if c.Test {
		prefix = "test-session-"
	}
	lines := []string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//Hack in a Box//Personal Session Calendar//EN",
		"CALSCALE:GREGORIAN",
		"METHOD:" + c.method(),
		"BEGIN:VEVENT",
		"UID:" + prefix + c.EventID.String() + "-" + c.SessionID.String() + "-" + c.PersonID.String() + "@" + settings.MessageDomain,
		fmt.Sprintf("SEQUENCE:%d", c.Sequence),
		"DTSTAMP:" + stamp.UTC().Format("20060102T150405Z"),
		"ORGANIZER:mailto:" + settings.From,
	}
	if c.Cancelled {
		if !c.Start.IsZero() || !c.End.IsZero() || c.Timezone != "" || c.Summary != "" ||
			c.WorkspaceURL != "" {
			return nil, ErrMessage
		}
		lines = append(lines, "STATUS:CANCELLED")
	} else {
		if c.Start.IsZero() || !c.End.After(c.Start) || !header(c.Summary, 200) ||
			strings.TrimSpace(c.Summary) == "" ||
			!header(c.Timezone, 100) ||
			!safeWorkspace(c.WorkspaceURL) {
			return nil, ErrMessage
		}
		if _, err := time.LoadLocation(c.Timezone); err != nil {
			return nil, ErrMessage
		}
		lines = append(
			lines,
			"DTSTART:"+c.Start.UTC().Format("20060102T150405Z"),
			"DTEND:"+c.End.UTC().Format("20060102T150405Z"),
			"SUMMARY:"+escapeCalendar(c.Summary),
			"DESCRIPTION:"+escapeCalendar("Event timezone: "+c.Timezone+"\n"+c.WorkspaceURL),
			"URL:"+c.WorkspaceURL,
			"STATUS:CONFIRMED",
		)
	}
	lines = append(lines, "END:VEVENT", "END:VCALENDAR")
	var result strings.Builder
	for _, line := range lines {
		result.WriteString(foldCalendar(line))
	}
	return []byte(result.String()), nil
}
