package maildelivery

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/smtp"
	"net/textproto"
	"slices"
	"strconv"
	"strings"
	"time"
)

type SMTP struct {
	settings Settings
	roots    *x509.CertPool // Tests can trust a synthetic local CA; production uses system roots.
}

func NewSMTP(settings Settings) (*SMTP, error) {
	if err := settings.Validate(); err != nil {
		return nil, err
	}
	return &SMTP{settings: settings}, nil
}
func (s *SMTP) Configured() bool { return s != nil && s.settings.Configured() }

func failure(err error, uncertain bool, messageID string) Receipt {
	var response *textproto.Error
	if errors.As(err, &response) {
		if response.Code >= 400 && response.Code < 500 {
			return Receipt{
				State:     Retryable,
				Code:      "relay_temporary_rejection",
				MessageID: messageID,
			}
		}
		return Receipt{State: Permanent, Code: "relay_rejection", MessageID: messageID}
	}
	if uncertain {
		return Receipt{State: Ambiguous, Code: "acceptance_unknown", MessageID: messageID}
	}
	return Receipt{State: Retryable, Code: "relay_unavailable", MessageID: messageID}
}

func tlsFailure(err error, messageID string) Receipt {
	var network net.Error
	if errors.Is(err, io.EOF) || errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, context.Canceled) ||
		(errors.As(err, &network) && network.Timeout()) {
		return failure(err, false, messageID)
	}
	return Receipt{State: Permanent, Code: "tls_verification_failed", MessageID: messageID}
}

// Send never retries. The caller records the bounded receipt and must reconcile
// Ambiguous explicitly; SMTP has no reliable generic receipt lookup mechanism.
func (s *SMTP) Send(ctx context.Context, message Message) Receipt {
	if !s.Configured() {
		return Receipt{State: Permanent, Code: "smtp_not_configured"}
	}
	data, messageID, err := Build(s.settings, message)
	if err != nil {
		return Receipt{State: Permanent, Code: "invalid_message"}
	}
	ctx, cancel := context.WithTimeout(ctx, s.settings.Timeout)
	defer cancel()
	dialer := net.Dialer{Timeout: s.settings.Timeout}
	connection, err := dialer.DialContext(
		ctx,
		"tcp",
		net.JoinHostPort(s.settings.Host, strconv.Itoa(s.settings.Port)),
	)
	if err != nil {
		return failure(err, false, messageID)
	}
	defer func() { _ = connection.Close() }()
	rawConnection := connection
	stop := context.AfterFunc(ctx, func() { _ = rawConnection.Close() })
	defer stop()
	deadline, _ := ctx.Deadline()
	if err = connection.SetDeadline(deadline); err != nil {
		return failure(err, false, messageID)
	}
	tlsConfig := &tls.Config{
		ServerName: s.settings.Host,
		MinVersion: tls.VersionTLS12,
		RootCAs:    s.roots,
	}
	if s.settings.Mode == "tls" {
		protected := tls.Client(connection, tlsConfig)
		if err = protected.HandshakeContext(ctx); err != nil {
			return tlsFailure(err, messageID)
		}
		connection = protected
	}
	client, err := smtp.NewClient(connection, s.settings.Host)
	if err != nil {
		return failure(err, false, messageID)
	}
	defer func() { _ = client.Close() }()
	if s.settings.Mode == "starttls" {
		available, _ := client.Extension("STARTTLS")
		if !available {
			return Receipt{State: Permanent, Code: "tls_required", MessageID: messageID}
		}
		if err = client.StartTLS(tlsConfig); err != nil {
			return tlsFailure(err, messageID)
		}
	}
	if supported, mechanisms := client.Extension(
		"AUTH",
	); !supported ||
		!slices.Contains(strings.Fields(mechanisms), "PLAIN") {
		return Receipt{State: Permanent, Code: "authentication_unavailable", MessageID: messageID}
	}
	if err = client.Auth(
		smtp.PlainAuth("", s.settings.Username, s.settings.Password, s.settings.Host),
	); err != nil {
		return failure(err, false, messageID)
	}
	if err = client.Mail(s.settings.From); err != nil {
		return failure(err, false, messageID)
	}
	// Exactly one RCPT command: personal deliveries never combine recipients.
	if err = client.Rcpt(message.Recipient); err != nil {
		return failure(err, false, messageID)
	}
	writer, err := client.Data()
	if err != nil {
		return failure(err, false, messageID)
	}
	if _, err = writer.Write(data); err != nil {
		return failure(err, true, messageID)
	}
	if err = writer.Close(); err != nil {
		return failure(err, true, messageID)
	}
	// DATA's final250 is the acceptance boundary. A later QUIT failure cannot
	// undo it and must never turn an accepted message into an automatic retry.
	_ = connection.SetDeadline(time.Now().Add(time.Second))
	_ = client.Quit()
	return Receipt{State: Accepted, Code: "relay_accepted", MessageID: messageID}
}
