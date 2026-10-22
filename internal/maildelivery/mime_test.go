package maildelivery

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

func settings() Settings {
	return Settings{
		Host:          "localhost",
		Port:          587,
		Mode:          "starttls",
		Username:      "synthetic-user",
		Password:      "synthetic-password",
		From:          "sender@example.test",
		MessageDomain: "example.test",
		Timeout:       time.Second,
	}
}

func personal() Message {
	return Message{
		ID:        uuid.New(),
		Recipient: "one@example.test",
		Subject:   "Synthetic personal reminder",
		PlainText: "Hello <script>unsafe markup</script>\nYour event starts soon.",
		Brand: Brand{
			Name:       "Synthetic event",
			Primary:    "#173a35",
			Background: "#ffffff",
			Foreground: "#111111",
		},
		SentAt: time.Date(2027, 3, 20, 12, 0, 0, 0, time.UTC),
	}
}

func leaves(t *testing.T, body io.Reader, contentType string) map[string][]byte {
	t.Helper()
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(mediaType, "multipart/") {
		data, err := io.ReadAll(body)
		if err != nil {
			t.Fatal(err)
		}
		return map[string][]byte{mediaType: data}
	}
	reader := multipart.NewReader(body, params["boundary"])
	result := map[string][]byte{}
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		for kind, data := range leaves(t, part, part.Header.Get("Content-Type")) {
			result[kind] = data
		}
	}
	return result
}

func TestPersonalMIMECalendarAndStableUpdateIdentity(t *testing.T) {
	message := personal()
	zone, err := time.LoadLocation("Europe/London")
	if err != nil {
		t.Fatal(err)
	}
	eventID := uuid.New()
	message.Calendar = &Calendar{
		EventID:      eventID,
		Sequence:     7,
		Start:        time.Date(2027, 3, 28, 9, 0, 0, 0, zone),
		End:          time.Date(2027, 3, 28, 17, 0, 0, 0, zone),
		Timezone:     "Europe/London",
		Summary:      strings.Repeat("Hack 🦀,;\\", 12),
		WorkspaceURL: "https://hack.example.test/events/" + eventID.String(),
	}
	first, id, err := Build(settings(), message)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := mail.ReadMessage(bytes.NewReader(first))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Header.Get("To") != message.Recipient || parsed.Header.Get("Cc") != "" ||
		parsed.Header.Get("Bcc") != "" ||
		parsed.Header.Get("Message-ID") != id {
		t.Fatal("unsafe personal headers")
	}
	parts := leaves(t, parsed.Body, parsed.Header.Get("Content-Type"))
	if len(parts) != 3 ||
		!bytes.Equal(
			parts["text/plain"],
			[]byte(strings.ReplaceAll(message.PlainText, "\n", "\r\n")),
		) {
		t.Fatal("missing or changed plain alternative")
	}
	if strings.Contains(string(parts["text/html"]), "<script>") ||
		!strings.Contains(string(parts["text/html"]), "&lt;script&gt;") ||
		!strings.Contains(string(parts["text/html"]), message.Brand.Primary) {
		t.Fatal("HTML is not safely branded")
	}
	calendar := string(parts["text/calendar"])
	for _, line := range strings.Split(calendar, "\r\n") {
		if len(line) > 75 || !utf8.ValidString(line) {
			t.Fatal("calendar folding split UTF-8 or exceeded75octets")
		}
	}
	unfolded := strings.ReplaceAll(calendar, "\r\n ", "")
	for _, expected := range []string{"METHOD:PUBLISH", "UID:event-" + eventID.String() + "@example.test", "SEQUENCE:7", "DTSTART:20270328T080000Z", "DTEND:20270328T160000Z", "Event timezone: Europe/London", `\,\;\\`} {
		if !strings.Contains(unfolded, expected) {
			t.Fatalf("calendar missing %s", expected)
		}
	}
	retry, retryID, err := Build(settings(), message)
	if err != nil || retryID != id {
		t.Fatal("retry changed message identity")
	}
	if len(retry) == 0 {
		t.Fatal("retry omitted message")
	}
	message.ID = uuid.New()
	message.Calendar.Sequence++
	message.Calendar.Start = message.Calendar.Start.Add(time.Hour)
	updated, updateID, err := Build(settings(), message)
	if err != nil || updateID == id {
		t.Fatal("update reused logical send identity")
	}
	updateParsed, err := mail.ReadMessage(bytes.NewReader(updated))
	if err != nil {
		t.Fatal(err)
	}
	updateCalendar := string(
		leaves(t, updateParsed.Body, updateParsed.Header.Get("Content-Type"))["text/calendar"],
	)
	if !strings.Contains(updateCalendar, "UID:event-"+eventID.String()+"@example.test") ||
		!strings.Contains(updateCalendar, "SEQUENCE:8") ||
		!strings.Contains(updateCalendar, "DTSTART:20270328T090000Z") {
		t.Fatal("calendar update changed UID or lost new sequence/time")
	}
	details, err := DateDetails(*message.Calendar)
	if err != nil || !strings.Contains(details, "10:00 BST") ||
		!strings.Contains(details, "Europe/London") {
		t.Fatal("local calendar details lost DST zone")
	}
}

func TestMIMERejectsHeaderAndCalendarInjection(t *testing.T) {
	for _, change := range []func(*Message){func(m *Message) { m.Recipient = "one@example.test,two@example.test" }, func(m *Message) { m.Recipient = "One <one@example.test>" }, func(m *Message) { m.Recipient = "one@example.test\r\nBcc: other@example.test" }, func(m *Message) { m.Subject = "Reminder\r\nBcc: other@example.test" }, func(m *Message) { m.Brand.Name = "Event\nBcc: other@example.test" }, func(m *Message) { m.Brand.Primary = "red;background:url(https://unsafe.example.test)" }} {
		message := personal()
		change(&message)
		if _, _, err := Build(settings(), message); err == nil {
			t.Fatal("accepted header or style injection")
		}
	}
	for _, address := range []string{"", "a@example.test,b@example.test", "same@example.test\n", "naïve@example.test", "name <one@example.test>", "user@example.test (comment)"} {
		if Mailbox(address) {
			t.Fatal("accepted unsupported mailbox syntax")
		}
	}
	for _, zone := range []string{"", "not/a-zone", "Europe/London\r\nATTENDEE:mailto:other@example.test"} {
		message := personal()
		message.Calendar = &Calendar{
			EventID:      uuid.New(),
			Sequence:     1,
			Start:        message.SentAt,
			End:          message.SentAt.Add(time.Hour),
			Timezone:     zone,
			Summary:      "Synthetic event",
			WorkspaceURL: "https://hack.example.test/events",
		}
		if _, _, err := Build(settings(), message); err == nil {
			t.Fatal("accepted invalid calendar timezone")
		}
	}
}

func TestCalendarUTCDisambiguatesRepeatedLocalHourAndTestUID(t *testing.T) {
	message := personal()
	start := time.Date(2027, 10, 31, 0, 30, 0, 0, time.UTC)
	message.Calendar = &Calendar{
		EventID:      uuid.New(),
		Test:         true,
		Sequence:     3,
		Start:        start,
		End:          start.Add(time.Hour),
		Timezone:     "Europe/London",
		Summary:      "[TEST] Synthetic event",
		WorkspaceURL: "https://hack.example.test/events",
	}
	data, err := message.Calendar.render(settings(), message.SentAt)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("DTSTART:20271031T003000Z")) ||
		!bytes.Contains(data, []byte("DTEND:20271031T013000Z")) ||
		!bytes.Contains(data, []byte("UID:test-event-")) {
		t.Fatal("calendar conflated repeated local hour or test/live UID")
	}
	details, err := DateDetails(*message.Calendar)
	if err != nil || !strings.Contains(details, "01:30 BST") ||
		!strings.Contains(details, "01:30 GMT") {
		t.Fatal("ambiguous local event details lost their offsets")
	}
	message.Calendar.Test = false
	live, err := message.Calendar.render(settings(), message.SentAt)
	if err != nil || bytes.Contains(live, []byte("UID:test-event-")) ||
		!bytes.Contains(live, []byte("UID:event-")) {
		t.Fatal("test calendar could replace the live calendar")
	}
}

func TestSMTPRevisionIsStableOpaqueAndSettingsBound(t *testing.T) {
	cfg := settings()
	first, err := NewSMTP(cfg)
	if err != nil {
		t.Fatal(err)
	}
	same, err := NewSMTP(cfg)
	if err != nil {
		t.Fatal(err)
	}
	revision := first.Revision()
	if len(revision) != 64 || revision != same.Revision() ||
		strings.Contains(revision, cfg.Password) ||
		strings.Contains(revision, cfg.Host) {
		t.Fatal("revision is unstable or exposed settings")
	}
	for _, change := range []func(*Settings){func(s *Settings) { s.Password = "different-synthetic-password" }, func(s *Settings) { s.Host = "other.example.test" }, func(s *Settings) { s.From = "other@example.test" }, func(s *Settings) { s.Mode = "tls" }, func(s *Settings) { s.MessageDomain = "other.example.test" }} {
		changed := cfg
		change(&changed)
		sender, err := NewSMTP(changed)
		if err != nil || sender.Revision() == revision {
			t.Fatal("changed SMTP configuration retained approval binding")
		}
	}
	absent, err := NewSMTP(Settings{})
	if err != nil || absent.Revision() != "" {
		t.Fatal("unconfigured SMTP exposed a revision")
	}
}
