package mailcontact

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/maildelivery"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type recordingSender struct {
	mu       sync.Mutex
	messages []maildelivery.Message
	state    maildelivery.State
	code     string
}

func (*recordingSender) Configured() bool { return true }

func (s *recordingSender) Send(
	_ context.Context,
	message maildelivery.Message,
) maildelivery.Receipt {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, message)
	return maildelivery.Receipt{State: s.state, Code: s.code}
}
func (s *recordingSender) count() int { s.mu.Lock(); defer s.mu.Unlock(); return len(s.messages) }
func (s *recordingSender) latestCode(t *testing.T) string {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.messages) == 0 {
		t.Fatal("no neutral verification message")
	}
	matches := regexp.MustCompile(`code is ([0-9]{8})`).
		FindStringSubmatch(s.messages[len(s.messages)-1].PlainText)
	if len(matches) != 2 {
		t.Fatal("neutral confirmation message missing code")
	}
	return matches[1]
}

func TestMailboxConfirmationProvenanceAndPrivateCode(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	person := testdb.Person(t, pool)
	other := testdb.Person(t, pool)
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	sender := &recordingSender{state: maildelivery.Accepted, code: "relay_accepted"}
	service := Service{Pool: pool, Sender: sender, Now: func() time.Time { return now }}
	status, err := service.Request(ctx, person.ID, "chosen@example.test")
	if err != nil || !status.Pending || status.Verified || status.RequestState != "accepted" {
		t.Fatal("code request implied verification or failed")
	}
	code := sender.latestCode(t)
	var stored string
	if err = pool.QueryRow(ctx, "SELECT row_to_json(v)::text FROM email_verifications v WHERE person_id=$1", person.ID).
		Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, code) {
		t.Fatal("database retained plaintext code")
	}
	if _, err = service.Confirm(ctx, other.ID, code); !errors.Is(err, ErrCode) {
		t.Fatal("code crossed account boundary")
	}
	status, err = service.Confirm(ctx, person.ID, code)
	if err != nil || !status.Verified || status.Provenance != "code_confirmed" ||
		status.MaskedMailbox != "c***@example.test" ||
		status.Pending {
		t.Fatal("account confirmation failed")
	}
	encoded, _ := json.Marshal(status)
	if strings.Contains(string(encoded), "chosen@example.test") ||
		strings.Contains(string(encoded), code) {
		t.Fatal("status exposed private address/code")
	}
	if _, err = service.Confirm(ctx, person.ID, code); !errors.Is(err, ErrCode) {
		t.Fatal("confirmation code replayed")
	}
	syncClaim := func(address string, verified bool) {
		t.Helper()
		if err := database.Within(ctx, pool, func(_ pgx.Tx, q *dbgen.Queries) error {
			return SyncClaimInTransaction(ctx, q, person.ID, address, verified)
		}); err != nil {
			t.Fatal(err)
		}
	}
	syncClaim("changed-claim@example.test", true)
	syncClaim("unverified-hint@example.test", false)
	syncClaim("", false)
	contact, err := dbgen.New(pool).PersonEmailContact(ctx, person.ID)
	if err != nil || contact.Mailbox != "chosen@example.test" ||
		contact.Provenance != "code_confirmed" {
		t.Fatal("OIDC login erased or replaced chosen confirmed address")
	}
	for _, step := range []struct {
		address           string
		verified, present bool
	}{{"oidc@example.test", true, true}, {"", false, false}, {"again@example.test", true, true}, {"hint@example.test", false, false}} {
		if err = database.Within(ctx, pool, func(_ pgx.Tx, q *dbgen.Queries) error {
			return SyncClaimInTransaction(ctx, q, other.ID, step.address, step.verified)
		}); err != nil {
			t.Fatal(err)
		}
		contact, err := dbgen.New(pool).PersonEmailContact(ctx, other.ID)
		if step.present {
			if err != nil || contact.Provenance != "oidc_verified" ||
				contact.Mailbox != step.address {
				t.Fatal("verified OIDC claim not retained privately")
			}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			t.Fatal("stale claim-derived mailbox remained usable")
		}
	}
	if sender.count() != 1 {
		t.Fatal("status/confirmation/claim sync resent verification mail")
	}
}

func TestMailboxCodeLockoutSupersessionExpiryAndSessionRevocation(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	person := testdb.Person(t, pool)
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	sender := &recordingSender{
		state: maildelivery.Ambiguous,
		code:  "SYNTHETIC-SECRET-PROVIDER-RESPONSE",
	}
	service := Service{Pool: pool, Sender: sender, Now: func() time.Time { return now }}
	status, err := service.Request(ctx, person.ID, "owned@example.test")
	if err != nil || status.RequestState != "ambiguous" {
		t.Fatal("uncertain acceptance incorrectly reported")
	}
	code := sender.latestCode(t)
	var receipt string
	if err = pool.QueryRow(ctx, "SELECT receipt_code FROM email_verifications WHERE person_id=$1", person.ID).
		Scan(&receipt); err != nil ||
		receipt != "unknown" {
		t.Fatal("unbounded provider error stored")
	}
	for range 5 {
		if _, err = service.Confirm(ctx, person.ID, "not-the-code"); !errors.Is(err, ErrCode) {
			t.Fatal("wrong confirmation accepted")
		}
	}
	if _, err = service.Confirm(ctx, person.ID, code); !errors.Is(err, ErrCode) {
		t.Fatal("locked code remained usable")
	}
	now = now.Add(61 * time.Second)
	if _, err = service.Request(ctx, person.ID, "owned@example.test"); err != nil {
		t.Fatal(err)
	}
	next := sender.latestCode(t)
	if _, err = service.Confirm(ctx, person.ID, code); !errors.Is(err, ErrCode) {
		t.Fatal("superseded code remained usable")
	}
	// An ambiguous send may really have arrived. Entering its exact code proves
	// ownership without pretending SMTP acceptance alone proves delivery.
	if status, err = service.Confirm(ctx, person.ID, next); err != nil || !status.Verified {
		t.Fatal("received ambiguous message could not be confirmed")
	}
	now = now.Add(61 * time.Second)
	if _, err = service.Request(ctx, person.ID, "new-owned@example.test"); err != nil {
		t.Fatal(err)
	}
	next = sender.latestCode(t)
	now = now.Add(10 * time.Minute)
	if _, err = service.Confirm(ctx, person.ID, next); !errors.Is(err, ErrCode) {
		t.Fatal("exact expiration boundary accepted")
	}
	now = now.Add(time.Hour)
	if _, err = service.Request(ctx, person.ID, "new-owned@example.test"); err != nil {
		t.Fatal(err)
	}
	next = sender.latestCode(t)
	if _, err = pool.Exec(
		ctx,
		"UPDATE people SET session_revision=session_revision+1 WHERE id=$1",
		person.ID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Confirm(ctx, person.ID, next); !errors.Is(err, ErrCode) {
		t.Fatal("revoked session's pending confirmation survived")
	}
	if _, err = pool.Exec(
		ctx,
		"UPDATE people SET active=false WHERE id=$1",
		person.ID,
	); err != nil {
		t.Fatal(err)
	}
	count := sender.count()
	if _, err = service.Request(
		ctx,
		person.ID,
		"other@example.test",
	); !errors.Is(
		err,
		access.ErrUnauthenticated,
	) {
		t.Fatal("inactive account requested email")
	}
	if _, err = service.Status(ctx, person.ID); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatal("inactive account read contact")
	}
	if sender.count() != count {
		t.Fatal("failed authority check sent email")
	}
}

func TestMailboxRequestQuotasAreAtomicAcrossAccounts(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	person := testdb.Person(t, pool)
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	sender := &recordingSender{state: maildelivery.Accepted, code: "relay_accepted"}
	service := Service{Pool: pool, Sender: sender, Now: func() time.Time { return now }}
	requests := func(people []uuid.UUID, address string) (int, int) {
		t.Helper()
		var wait sync.WaitGroup
		results := make(chan error, len(people))
		for _, id := range people {
			wait.Go(func() { _, err := service.Request(ctx, id, address); results <- err })
		}
		wait.Wait()
		close(results)
		accepted, limited := 0, 0
		for err := range results {
			if err == nil {
				accepted++
			} else if errors.Is(err, ErrRateLimited) {
				limited++
			} else {
				t.Fatalf("unexpected request result: %v", err)
			}
		}
		return accepted, limited
	}
	same := make([]uuid.UUID, 8)
	for i := range same {
		same[i] = person.ID
	}
	if accepted, limited := requests(same, "first@example.test"); accepted != 1 || limited != 7 {
		t.Fatal("concurrent requests bypassed account cooldown")
	}
	people := make([]uuid.UUID, 6)
	for i := range people {
		people[i] = testdb.Person(t, pool).ID
	}
	if accepted, limited := requests(
		people,
		"destination@example.test",
	); accepted != 5 ||
		limited != 1 {
		t.Fatal("concurrent accounts bypassed destination quota")
	}
	if sender.count() != 6 {
		t.Fatal("quota failures reached SMTP")
	}
	now = now.Add(61 * time.Second)
	if _, err := service.Request(ctx, person.ID, "second@example.test"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(61 * time.Second)
	if _, err := service.Request(ctx, person.ID, "third@example.test"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(61 * time.Second)
	if _, err := service.Request(
		ctx,
		person.ID,
		"fourth@example.test",
	); !errors.Is(
		err,
		ErrRateLimited,
	) {
		t.Fatal("hourly account quota not enforced")
	}
}
