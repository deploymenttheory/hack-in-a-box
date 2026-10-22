// Package maildelivery builds single-recipient messages and submits them to an
// authenticated TLS SMTP relay. Callers own authorization and durable attempts.
package maildelivery

import (
	"errors"
	"net"
	"net/mail"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

type State string

const (
	Accepted  State = "accepted"
	Retryable State = "retryable"
	Permanent State = "permanent"
	Ambiguous State = "ambiguous"
)

// Receipt never includes provider-supplied text, addresses or message content.
// Accepted means only the relay accepted DATA, not inbox delivery or reading.
type Receipt struct {
	State     State
	Code      string
	MessageID string
}

var (
	ErrConfiguration = errors.New("invalid SMTP configuration")
	ErrMessage       = errors.New("invalid personal email")
	domain           = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9.-]{0,251}[a-zA-Z0-9])?$`)
	colour           = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
)

type Settings struct {
	Host          string
	Port          int
	Mode          string
	Username      string
	Password      string
	From          string
	MessageDomain string
	Timeout       time.Duration
}

func (s Settings) Configured() bool { return s.Host != "" }
func (s Settings) Validate() error {
	if !s.Configured() {
		if s.Port != 0 || s.Mode != "" || s.Username != "" || s.Password != "" || s.From != "" ||
			s.MessageDomain != "" ||
			s.Timeout != 0 {
			return ErrConfiguration
		}
		return nil
	}
	if strings.TrimSpace(s.Host) != s.Host ||
		(net.ParseIP(s.Host) == nil && !domain.MatchString(s.Host)) ||
		s.Port < 1 ||
		s.Port > 65535 ||
		(s.Mode != "tls" && s.Mode != "starttls") {
		return ErrConfiguration
	}
	if s.Username == "" || s.Password == "" || strings.ContainsRune(s.Username, 0) ||
		strings.ContainsRune(s.Password, 0) {
		return ErrConfiguration
	}
	if !Mailbox(s.From) || !domain.MatchString(s.MessageDomain) ||
		strings.Contains(s.MessageDomain, "..") ||
		s.Timeout <= 0 ||
		s.Timeout > 20*time.Second {
		return ErrConfiguration
	}
	if _, _, err := net.SplitHostPort(net.JoinHostPort(s.Host, strconv.Itoa(s.Port))); err != nil {
		return ErrConfiguration
	}
	return nil
}

// Mailbox accepts exactly one ASCII SMTP mailbox, without display-name syntax.
// OIDC address capture uses this same boundary; SMTPUTF8 is not assumed.
func Mailbox(value string) bool {
	if len(value) == 0 || len(value) > 254 {
		return false
	}
	for _, r := range value {
		if r > 127 || unicode.IsControl(r) || unicode.IsSpace(r) {
			return false
		}
	}
	parsed, err := mail.ParseAddress(value)
	return err == nil && parsed.Name == "" && parsed.Address == value &&
		strings.Count(value, "@") == 1
}

func header(value string, limit int) bool {
	if !utf8.ValidString(value) || len(value) > limit {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

type (
	Brand struct {
		Name       string `json:"name"`
		Primary    string `json:"primary"`
		Background string `json:"background"`
		Foreground string `json:"foreground"`
	}
	Message struct {
		ID        uuid.UUID `json:"id"`
		Recipient string    `json:"recipient"`
		Subject   string    `json:"subject"`
		PlainText string    `json:"plain_text"`
		Brand     Brand     `json:"brand"`
		SentAt    time.Time `json:"sent_at"`
		Calendar  *Calendar `json:"calendar"`
	}
)

func (m Message) validate() error {
	if m.ID == uuid.Nil || !Mailbox(m.Recipient) || strings.TrimSpace(m.Subject) == "" ||
		!header(m.Subject, 256) ||
		!header(m.Brand.Name, 160) ||
		len(m.PlainText) > 65536 ||
		!utf8.ValidString(m.PlainText) ||
		strings.ContainsRune(m.PlainText, 0) ||
		m.SentAt.IsZero() {
		return ErrMessage
	}
	for _, c := range []string{m.Brand.Primary, m.Brand.Background, m.Brand.Foreground} {
		if !colour.MatchString(c) {
			return ErrMessage
		}
	}
	return nil
}

// SafeReceipt bounds third-party/mock adapter outcomes before persistence.
func SafeReceipt(receipt Receipt) Receipt {
	switch receipt.State {
	case Accepted, Retryable, Permanent, Ambiguous:
	default:
		receipt.State = Ambiguous
	}
	switch receipt.Code {
	case "relay_accepted",
		"relay_temporary_rejection",
		"relay_rejection",
		"acceptance_unknown",
		"relay_unavailable",
		"tls_verification_failed",
		"tls_required",
		"authentication_unavailable",
		"invalid_message",
		"smtp_not_configured":
	default:
		receipt.Code = "unknown"
	}
	return receipt
}
