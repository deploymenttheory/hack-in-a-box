package maildelivery

import (
	"bytes"
	"fmt"
	"html/template"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"net/url"
	"strings"
	"time"
)

func safeWorkspace(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil &&
		parsed.RawQuery == "" &&
		parsed.Fragment == "" &&
		header(value, 2048)
}

var layout = template.Must(
	template.New("personal-email").
		Parse(`<!doctype html><html><body style="margin:0;background:{{.Brand.Background}};color:{{.Brand.Foreground}};font-family:Arial,sans-serif"><main style="max-width:640px;margin:auto;padding:24px"><h1 style="color:{{.Brand.Primary}}">{{.Brand.Name}}</h1><div style="white-space:pre-wrap">{{.PlainText}}</div><p>Sent individually by Hack in a Box.</p></main></body></html>`),
)

func writePart(writer *multipart.Writer, kind string, body []byte, attachment bool) error {
	headers := textproto.MIMEHeader{}
	headers.Set("Content-Type", kind)
	headers.Set("Content-Transfer-Encoding", "quoted-printable")
	if attachment {
		headers.Set("Content-Disposition", `attachment; filename="event.ics"`)
	}
	part, err := writer.CreatePart(headers)
	if err != nil {
		return ErrMessage
	}
	encoded := quotedprintable.NewWriter(part)
	if _, err = encoded.Write(body); err != nil {
		return ErrMessage
	}
	if err = encoded.Close(); err != nil {
		return ErrMessage
	}
	return nil
}

// Build emits one To header and no CC/BCC. The body is literal approved text;
// HTML escaping belongs here, not to an arbitrary sender-supplied HTML fragment.
func Build(settings Settings, message Message) ([]byte, string, error) {
	if err := settings.Validate(); err != nil || !settings.Configured() {
		return nil, "", ErrConfiguration
	}
	if err := message.validate(); err != nil {
		return nil, "", err
	}
	var htmlBody bytes.Buffer
	if err := layout.Execute(&htmlBody, message); err != nil {
		return nil, "", ErrMessage
	}
	var alternative bytes.Buffer
	parts := multipart.NewWriter(&alternative)
	if err := writePart(
		parts,
		"text/plain; charset=UTF-8",
		[]byte(message.PlainText),
		false,
	); err != nil {
		return nil, "", ErrMessage
	}
	if err := writePart(parts, "text/html; charset=UTF-8", htmlBody.Bytes(), false); err != nil {
		return nil, "", ErrMessage
	}
	if err := parts.Close(); err != nil {
		return nil, "", ErrMessage
	}
	contentType := mime.FormatMediaType(
		"multipart/alternative",
		map[string]string{"boundary": parts.Boundary()},
	)
	body := alternative.Bytes()
	if message.Calendar != nil {
		calendar, err := message.Calendar.render(settings, message.SentAt)
		if err != nil {
			return nil, "", err
		}
		var mixed bytes.Buffer
		outer := multipart.NewWriter(&mixed)
		nested, err := outer.CreatePart(textproto.MIMEHeader{"Content-Type": []string{contentType}})
		if err != nil {
			return nil, "", ErrMessage
		}
		if _, err = nested.Write(body); err != nil {
			return nil, "", ErrMessage
		}
		if err = writePart(
			outer,
			`text/calendar; charset=UTF-8; method=PUBLISH`,
			calendar,
			true,
		); err != nil {
			return nil, "", ErrMessage
		}
		if err = outer.Close(); err != nil {
			return nil, "", ErrMessage
		}
		contentType = mime.FormatMediaType(
			"multipart/mixed",
			map[string]string{"boundary": outer.Boundary()},
		)
		body = mixed.Bytes()
	}
	messageID := "<hbox." + message.ID.String() + "@" + settings.MessageDomain + ">"
	from := (&mail.Address{Name: message.Brand.Name, Address: settings.From}).String()
	var result bytes.Buffer
	fmt.Fprintf(
		&result,
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMessage-ID: %s\r\nMIME-Version: 1.0\r\nContent-Type: %s\r\n\r\n",
		from,
		message.Recipient,
		mime.QEncoding.Encode("UTF-8", message.Subject),
		message.SentAt.UTC().Format("Mon, 02 Jan 2006 15:04:05 -0700"),
		messageID,
		contentType,
	)
	result.Write(body)
	return result.Bytes(), messageID, nil
}

func DateDetails(c Calendar) (string, error) {
	zone, err := time.LoadLocation(c.Timezone)
	if err != nil {
		return "", ErrMessage
	}
	return strings.Join(
		[]string{
			c.Start.In(zone).Format("Mon 2 Jan 2006 15:04 MST"),
			c.End.In(zone).Format("Mon 2 Jan 2006 15:04 MST"),
			c.Timezone,
		},
		" — ",
	), nil
}
