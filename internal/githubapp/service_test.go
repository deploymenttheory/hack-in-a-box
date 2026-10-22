package githubapp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/secretbox"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type mockProvider struct {
	code  string
	calls atomic.Int64
}

func (p *mockProvider) ValidateKey(_ int64, key []byte) bool { return len(key) > 0 }
func (p *mockProvider) Inspect(_ context.Context, _ Configuration, _ []byte, _ int64) Diagnostics {
	p.calls.Add(1)
	return diagnostic(p.code)
}

type appFixture struct {
	t            *testing.T
	pool         *pgxpool.Pool
	s            Service
	provider     *mockProvider
	owner, other dbgen.Person
	event        uuid.UUID
	session      string
}

func appKeys(t *testing.T, active string, ids ...string) *secretbox.Keyring {
	t.Helper()
	keys := map[string]string{}
	for i, id := range ids {
		keys[id] = base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{byte(i + 1)}, 32))
	}
	data, _ := json.Marshal(map[string]any{"active": active, "keys": keys})
	key, err := secretbox.Parse(string(data))
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func setupApp(t *testing.T) *appFixture {
	pool := testdb.New(t, true)
	f := &appFixture{
		t:        t,
		pool:     pool,
		owner:    testdb.Person(t, pool),
		other:    testdb.Person(t, pool),
		session:  "SYNTHETIC-BROWSER-SESSION",
		provider: &mockProvider{code: "healthy"},
	}
	f.event = testdb.Event(t, pool, f.owner.ID)
	f.s = Service{Pool: pool, Keys: appKeys(t, "old", "old"), Provider: f.provider}
	if err := dbgen.New(pool).
		PutSession(t.Context(), dbgen.PutSessionParams{Token: sessionHash(f.session), Data: []byte("synthetic-session-data"), Expiry: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *appFixture) exec(query string, args ...any) {
	f.t.Helper()
	if _, err := f.pool.Exec(f.t.Context(), query, args...); err != nil {
		f.t.Fatal(err)
	}
}

func (f *appFixture) revision() int64 {
	f.t.Helper()
	row, err := dbgen.New(f.pool).Event(f.t.Context(), f.event)
	if err != nil {
		f.t.Fatal(err)
	}
	return row.Revision
}

func (f *appFixture) configuration() Command {
	return Command{
		Action: "configure",
		Configuration: Configuration{
			AppID:              42,
			AppSlug:            "synthetic-app",
			OrganizationID:     99,
			Organization:       "synthetic-org",
			TemplateRepository: "synthetic-template",
		},
		PrivateKey: "SYNTHETIC-PRIVATE-KEY",
	}
}

func (f *appFixture) configure() {
	f.t.Helper()
	if err := f.s.Configure(
		f.t.Context(),
		f.owner.ID,
		f.event,
		f.revision(),
		f.configuration(),
	); err != nil {
		f.t.Fatal(err)
	}
}

func (f *appFixture) begin() string {
	f.t.Helper()
	destination, err := f.s.Begin(f.t.Context(), f.owner.ID, f.event, f.session, f.revision())
	if err != nil {
		f.t.Fatal(err)
	}
	u, err := url.Parse(destination)
	if err != nil {
		f.t.Fatal(err)
	}
	if u.Host != "github.com" || u.Path != "/apps/synthetic-app/installations/new" {
		f.t.Fatal("unsafe installation destination")
	}
	return u.Query().Get("state")
}

func (f *appFixture) complete(state string) error {
	_, err := f.s.Complete(f.t.Context(), f.owner.ID, f.session, state, 7)
	return err
}

func TestAppConfigurationStateSessionAuthorityAndReplay(t *testing.T) {
	f := setupApp(t)
	ctx := t.Context()
	f.configure()
	row, err := dbgen.New(f.pool).GitHubAppIntegration(ctx, f.event)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(row.PrivateKeyEncrypted, []byte("SYNTHETIC-PRIVATE-KEY")) {
		t.Fatal("plaintext in PostgreSQL")
	}
	if _, err = f.s.Status(ctx, f.other.ID, f.event); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("outsider status: %v", err)
	}
	state := f.begin()
	if _, err = f.s.Complete(ctx, f.other.ID, f.session, state, 7); err == nil {
		t.Fatal("another actor consumed callback")
	}
	if _, err = f.s.Complete(ctx, f.owner.ID, "OTHER-BROWSER", state, 7); err == nil {
		t.Fatal("another browser consumed callback")
	}
	if err = f.complete(state); err != nil {
		t.Fatal(err)
	}
	if err = f.complete(state); err == nil {
		t.Fatal("state replay accepted")
	}
	status, err := f.s.Status(ctx, f.owner.ID, f.event)
	if err != nil || status.InstallationID != 7 || status.Diagnostics.Code != "healthy" {
		t.Fatalf("status: %+v %v", status, err)
	}
	data, _ := json.Marshal(status)
	if strings.Contains(string(data), "PRIVATE-KEY") ||
		strings.Contains(string(data), "encrypted") ||
		strings.Contains(string(data), "key_id") {
		t.Fatal("unsafe metadata")
	}
	state = f.begin()
	f.exec("UPDATE github_app_install_attempts SET expires_at=now()-interval '1 second'")
	if err = f.complete(state); err == nil {
		t.Fatal("expired state accepted")
	}
	state = f.begin()
	f.exec("UPDATE people SET session_revision=session_revision+1 WHERE id=$1", f.owner.ID)
	if err = f.complete(state); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatalf("stale identity session accepted: %v", err)
	}
	state = f.begin()
	f.exec(
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='organiser'",
		f.event,
		f.owner.ID,
	)
	if err = f.complete(state); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("revoked organiser accepted: %v", err)
	}
	f.exec(
		"UPDATE event_memberships SET active=true WHERE event_id=$1 AND person_id=$2 AND role='organiser'",
		f.event,
		f.owner.ID,
	)
	state = f.begin()
	f.exec("DELETE FROM sessions WHERE token=$1", sessionHash(f.session))
	if err = f.complete(state); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatalf("logged-out callback accepted: %v", err)
	}
}

func TestAppCallbackConcurrentSingleUseAndStaleConfiguration(t *testing.T) {
	f := setupApp(t)
	ctx := t.Context()
	f.configure()
	state := f.begin()
	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		go func() { <-start; results <- f.complete(state) }()
	}
	close(start)
	successes := 0
	for range 2 {
		if err := <-results; err == nil {
			successes++
		}
	}
	if successes != 1 || f.provider.calls.Load() != 1 {
		t.Fatalf(
			"callback consumed more than once: %d calls %d",
			successes,
			f.provider.calls.Load(),
		)
	}
	var installed int
	if err := f.pool.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE event_id=$1 AND action='github.integration_installed'", f.event).
		Scan(&installed); err != nil ||
		installed != 1 {
		t.Fatalf("installation audit: %d %v", installed, err)
	}
	state = f.begin()
	f.exec("UPDATE events SET revision=revision+1 WHERE id=$1", f.event)
	if err := f.complete(state); !errors.Is(err, access.ErrConflict) {
		t.Fatalf("stale config callback accepted: %v", err)
	}
	if err := f.complete(state); err == nil {
		t.Fatal("failed callback did not consume state")
	}
	state = f.begin()
	f.provider.code = "organization_mismatch"
	if err := f.complete(state); err == nil {
		t.Fatal("wrong organization accepted")
	}
	f.provider.code = "healthy"
	if err := f.complete(state); err == nil {
		t.Fatal("wrong-org callback can be replayed")
	}
}

func TestAppKeyFaultRotationRecoveryAndDisconnect(t *testing.T) {
	f := setupApp(t)
	ctx := t.Context()
	f.configure()
	if err := f.complete(f.begin()); err != nil {
		t.Fatal(err)
	}
	f.s.Keys = nil
	status, err := f.s.Status(ctx, f.owner.ID, f.event)
	if err != nil || status.Diagnostics.Code != "credentials_unavailable" {
		t.Fatalf("missing key: %+v %v", status, err)
	}
	before := f.revision()
	if err = f.s.Configure(
		ctx,
		f.owner.ID,
		f.event,
		before,
		Command{Action: "rotate"},
	); err == nil ||
		f.revision() != before {
		t.Fatal("failed rotation changed state")
	}
	f.s.Keys = appKeys(t, "new", "old", "new")
	if err = f.s.Configure(
		ctx,
		f.owner.ID,
		f.event,
		f.revision(),
		Command{Action: "rotate"},
	); err != nil {
		t.Fatal(err)
	}
	row, err := dbgen.New(f.pool).GitHubAppIntegration(ctx, f.event)
	if err != nil || row.CredentialVersion != 2 || row.KeyID != "new" || row.InstallationID != 7 {
		t.Fatalf("rotation metadata: %+v %v", row, err)
	}
	f.exec(
		"UPDATE github_app_integrations SET private_key_encrypted='\\x00' WHERE event_id=$1",
		f.event,
	)
	status, err = f.s.Status(ctx, f.owner.ID, f.event)
	if err != nil || status.Diagnostics.Code != "credentials_unavailable" {
		t.Fatalf("tamper: %+v %v", status, err)
	}
	f.configure()
	status, err = f.s.Status(ctx, f.owner.ID, f.event)
	if err != nil || status.InstallationID != 0 ||
		status.Diagnostics.Code != "installation_required" {
		t.Fatalf("replacement recovery: %+v %v", status, err)
	}
	if err = f.s.Configure(
		ctx,
		f.owner.ID,
		f.event,
		f.revision(),
		Command{Action: "disconnect"},
	); err != nil {
		t.Fatal(err)
	}
	status, err = f.s.Status(ctx, f.owner.ID, f.event)
	if err != nil || status.Configured || status.Diagnostics.Code != "not_configured" {
		t.Fatalf("disconnect: %+v %v", status, err)
	}
}
