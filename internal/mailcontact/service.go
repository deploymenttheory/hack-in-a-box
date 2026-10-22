// Package mailcontact owns private, account-confirmed delivery addresses. A
// mailbox is never an identity key and never grants an event or platform role.
package mailcontact

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/maildelivery"
)

var (
	ErrRateLimited = errors.New("email confirmation request limit reached")
	ErrUnavailable = errors.New("personal email is unavailable")
	ErrCode        = errors.New("email confirmation code is invalid or expired")
)

type (
	Sender interface {
		Configured() bool
		Send(context.Context, maildelivery.Message) maildelivery.Receipt
	}
	Service struct {
		Pool   *pgxpool.Pool
		Sender Sender
		Now    func() time.Time
	}
)

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
func (s Service) Configured() bool { return s.Sender != nil && s.Sender.Configured() }

type Status struct {
	Configured        bool       `json:"configured"`
	Verified          bool       `json:"verified"`
	MaskedMailbox     string     `json:"masked_mailbox"`
	Provenance        string     `json:"provenance"`
	Pending           bool       `json:"pending"`
	RequestState      string     `json:"request_state"`
	ExpiresAt         *time.Time `json:"expires_at"`
	RetryAfterSeconds int64      `json:"retry_after_seconds"`
}

func masked(address string) string {
	at := strings.LastIndexByte(address, '@')
	if at < 1 {
		return ""
	}
	return address[:1] + "***" + address[at:]
}

func active(ctx context.Context, q *dbgen.Queries, person uuid.UUID) (int64, error) {
	row, err := q.Person(ctx, person)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !row.Active) {
		return 0, access.ErrUnauthenticated
	}
	if err != nil {
		return 0, fmt.Errorf("check email account: %w", err)
	}
	return row.SessionRevision, nil
}

func lockActive(ctx context.Context, q *dbgen.Queries, person uuid.UUID) (int64, error) {
	row, err := q.LockMailPerson(ctx, person)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !row.Active) {
		return 0, access.ErrUnauthenticated
	}
	if err != nil {
		return 0, fmt.Errorf("check email account: %w", err)
	}
	return row.SessionRevision, nil
}

func (s Service) Status(ctx context.Context, person uuid.UUID) (Status, error) {
	q := dbgen.New(s.Pool)
	revision, err := active(ctx, q, person)
	if err != nil {
		return Status{}, err
	}
	result := Status{Configured: s.Configured()}
	contact, err := q.PersonEmailContact(ctx, person)
	if err == nil {
		result.Verified = true
		result.MaskedMailbox = masked(contact.Mailbox)
		result.Provenance = contact.Provenance
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Status{}, fmt.Errorf("read email contact: %w", err)
	}
	attempt, err := q.LatestEmailVerification(ctx, person)
	if err == nil {
		now := s.now()
		result.Pending = !attempt.ConsumedAt.Valid && attempt.ExpiresAt.After(now) &&
			attempt.FailedAttempts < 5 &&
			attempt.SessionRevision == revision
		if result.Pending {
			expiry := attempt.ExpiresAt
			result.ExpiresAt = &expiry
			result.RequestState = attempt.DeliveryState
		}
		if remaining := attempt.RequestedAt.Add(time.Minute).Sub(now); remaining > 0 {
			result.RetryAfterSeconds = int64((remaining + time.Second - 1) / time.Second)
			if result.RetryAfterSeconds < 1 {
				result.RetryAfterSeconds = 1
			}
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Status{}, fmt.Errorf("read email request: %w", err)
	}
	return result, nil
}

// SyncClaimInTransaction never replaces a mailbox the person confirmed with a
// code. Missing/unverified claims clear only stale claim-derived contact data.
func SyncClaimInTransaction(
	ctx context.Context,
	q *dbgen.Queries,
	person uuid.UUID,
	address string,
	verified bool,
) error {
	if verified && maildelivery.Mailbox(address) {
		if err := q.SyncVerifiedClaimContact(
			ctx,
			dbgen.SyncVerifiedClaimContactParams{PersonID: person, Mailbox: address},
		); err != nil {
			return fmt.Errorf("sync verified email claim: %w", err)
		}
	} else if err := q.ClearUnverifiedClaimContact(ctx, person); err != nil {
		return fmt.Errorf("clear stale email claim: %w", err)
	}
	return nil
}

// VerifiedMailboxInTransaction is a private integration boundary. No HTTP
// response, general audience preview or event member record includes this value.
func VerifiedMailboxInTransaction(
	ctx context.Context,
	q *dbgen.Queries,
	person uuid.UUID,
) (string, error) {
	if _, err := active(ctx, q, person); err != nil {
		return "", err
	}
	contact, err := q.PersonEmailContact(ctx, person)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrUnavailable
	}
	if err != nil {
		return "", fmt.Errorf("resolve verified mailbox: %w", err)
	}
	if !maildelivery.Mailbox(contact.Mailbox) {
		return "", ErrUnavailable
	}
	return contact.Mailbox, nil
}

func codeHash(person, attempt uuid.UUID, code string) []byte {
	sum := sha256.Sum256(
		[]byte(
			"hackbox:mailbox-confirmation:v1:" + person.String() + ":" + attempt.String() + ":" + code,
		),
	)
	return sum[:]
}

func (s Service) Request(ctx context.Context, person uuid.UUID, address string) (Status, error) {
	if !s.Configured() {
		return Status{}, ErrUnavailable
	}
	if !maildelivery.Mailbox(address) {
		return Status{}, access.ErrInvalid
	}
	number, err := rand.Int(rand.Reader, big.NewInt(100000000))
	if err != nil {
		return Status{}, ErrUnavailable
	}
	code := fmt.Sprintf("%08d", number.Int64())
	id := uuid.New()
	now := s.now()
	destination := sha256.Sum256([]byte(strings.ToLower(address)))
	err = database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		revision, err := lockActive(ctx, q, person)
		if err != nil {
			return err
		}
		// Serialise requests to the same destination across different accounts; only
		// the hash reaches this lock, and the address is absent from provider logs.
		if _, err = tx.Exec(
			ctx,
			"SELECT pg_advisory_xact_lock(hashtextextended($1,79))",
			fmt.Sprintf("%x", destination),
		); err != nil {
			return fmt.Errorf("lock email request quota: %w", err)
		}
		last, err := q.LatestEmailVerification(ctx, person)
		if err == nil && now.Before(last.RequestedAt.Add(time.Minute)) {
			return ErrRateLimited
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("read email cooldown: %w", err)
		}
		personCount, err := q.PersonEmailRequestCount(
			ctx,
			dbgen.PersonEmailRequestCountParams{PersonID: person, RequestedAt: now.Add(-time.Hour)},
		)
		if err != nil {
			return fmt.Errorf("check account email quota: %w", err)
		}
		destinationCount, err := q.DestinationEmailRequestCount(
			ctx,
			dbgen.DestinationEmailRequestCountParams{
				MailboxHash: destination[:],
				RequestedAt: now.Add(-time.Hour),
			},
		)
		if err != nil {
			return fmt.Errorf("check destination email quota: %w", err)
		}
		if personCount >= 3 || destinationCount >= 5 {
			return ErrRateLimited
		}
		if err = q.InvalidatePendingEmailVerifications(
			ctx,
			dbgen.InvalidatePendingEmailVerificationsParams{
				PersonID:   person,
				ConsumedAt: pgtype.Timestamptz{Time: now, Valid: true},
			},
		); err != nil {
			return fmt.Errorf("supersede email code: %w", err)
		}
		if err = q.InsertEmailVerification(
			ctx,
			dbgen.InsertEmailVerificationParams{
				ID:              id,
				PersonID:        person,
				SessionRevision: revision,
				Mailbox:         address,
				MailboxHash:     destination[:],
				CodeHash:        codeHash(person, id, code),
				RequestedAt:     now,
				ExpiresAt:       now.Add(10 * time.Minute),
			},
		); err != nil {
			return fmt.Errorf("prepare email confirmation: %w", err)
		}
		return nil
	})
	if err != nil {
		return Status{}, fmt.Errorf("prepare mailbox confirmation: %w", err)
	}
	receipt := s.Sender.Send(
		ctx,
		maildelivery.Message{
			ID:        id,
			Recipient: address,
			Subject:   "Confirm your Hack in a Box email address",
			PlainText: "Your confirmation code is " + code + ".\nIt expires in 10 minutes. Enter it only in your signed-in Hack in a Box account.\nIf you did not request this message, you can ignore it.",
			Brand: maildelivery.Brand{
				Name:       "Hack in a Box",
				Primary:    "#173a35",
				Background: "#ffffff",
				Foreground: "#111111",
			},
			SentAt: now,
		},
	)
	state := receipt.State
	switch state {
	case maildelivery.Accepted,
		maildelivery.Retryable,
		maildelivery.Permanent,
		maildelivery.Ambiguous:
	default:
		state = maildelivery.Ambiguous
	}
	// A bounded cleanup records uncertainty even if the browser disconnects. No
	// automatic resend exists: a new request is an explicit rate-limited action.
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err = dbgen.New(s.Pool).
		RecordEmailVerificationReceipt(cleanup, dbgen.RecordEmailVerificationReceiptParams{ID: id, DeliveryState: string(state), ReceiptCode: boundedCode(receipt.Code)}); err != nil {
		return Status{}, fmt.Errorf("record email confirmation outcome: %w", err)
	}
	return s.Status(cleanup, person)
}

func boundedCode(code string) string {
	switch code {
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
		return code
	default:
		return "unknown"
	}
}

func (s Service) Confirm(ctx context.Context, person uuid.UUID, code string) (Status, error) {
	outcome := ErrCode
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		revision, err := lockActive(ctx, q, person)
		if err != nil {
			return err
		}
		attempt, err := q.LockEmailVerification(ctx, person)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("lock email confirmation: %w", err)
		}
		if attempt.ConsumedAt.Valid || attempt.FailedAttempts >= 5 ||
			!s.now().Before(attempt.ExpiresAt) ||
			attempt.SessionRevision != revision {
			return nil
		}
		if subtle.ConstantTimeCompare(codeHash(person, attempt.ID, code), attempt.CodeHash) != 1 {
			if err = q.FailEmailConfirmation(ctx, attempt.ID); err != nil {
				return fmt.Errorf("record failed confirmation: %w", err)
			}
			return nil
		}
		if err = q.ConfirmPersonEmail(
			ctx,
			dbgen.ConfirmPersonEmailParams{PersonID: person, Mailbox: attempt.Mailbox},
		); err != nil {
			return fmt.Errorf("confirm account email: %w", err)
		}
		if err = q.ConsumeEmailVerification(
			ctx,
			dbgen.ConsumeEmailVerificationParams{
				ID:         attempt.ID,
				ConsumedAt: pgtype.Timestamptz{Time: s.now(), Valid: true},
			},
		); err != nil {
			return fmt.Errorf("consume email code: %w", err)
		}
		outcome = nil
		return nil
	})
	if err != nil {
		return Status{}, fmt.Errorf("confirm mailbox ownership: %w", err)
	}
	if outcome != nil {
		return Status{}, outcome
	}
	return s.Status(ctx, person)
}

// Maintain retains quota/confirmation evidence for24hours, longer than all code
// lifetimes and request windows, then removes the private pending address/hash.
func (s Service) Maintain(ctx context.Context) error {
	if err := dbgen.New(s.Pool).
		RetainEmailVerificationAttempts(ctx, s.now().Add(-24*time.Hour)); err != nil {
		return fmt.Errorf("retain email confirmation attempts: %w", err)
	}
	return nil
}
