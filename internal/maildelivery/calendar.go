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
	EventID      uuid.UUID
	Sequence     int64
	Start, End   time.Time
	Timezone     string
	Summary      string
	WorkspaceURL string
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
	lines := []string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//Hack in a Box//Personal Event Calendar//EN",
		"CALSCALE:GREGORIAN",
		"METHOD:PUBLISH",
		"BEGIN:VEVENT",
		"UID:event-" + c.EventID.String() + "@" + settings.MessageDomain,
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
