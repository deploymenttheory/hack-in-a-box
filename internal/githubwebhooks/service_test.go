package githubwebhooks

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/githubapp"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/secretbox"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

const testSecret = "SYNTHETIC-WEBHOOK-SECRET-0000000000000000"

type provider struct{}

func (provider) ValidateKey(int64, []byte) bool { return true }

func (provider) Inspect(
	context.Context,
	githubapp.Configuration,
	[]byte,
	int64,
) githubapp.Diagnostics {
	return githubapp.Diagnostics{Code: "healthy"}
}

type fixture struct {
	t           *testing.T
	pool        *pgxpool.Pool
	s           *Service
	app         githubapp.Service
	owner       uuid.UUID
	event, team uuid.UUID
	publication time.Time
}

func setup(t *testing.T) *fixture {
	t.Helper()
	pool := testdb.New(t, true)
	owner := testdb.Person(t, pool)
	keys, err := secretbox.Parse(
		`{"active":"test","keys":{"test":"` + base64.StdEncoding.EncodeToString(
			bytes.Repeat([]byte{7}, 32),
		) + `"}}`,
	)
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{
		t:           t,
		pool:        pool,
		owner:       owner.ID,
		event:       testdb.Event(t, pool, owner.ID),
		team:        uuid.New(),
		publication: time.Now().Add(-time.Hour).UTC().Truncate(time.Second),
	}
	f.s = &Service{Pool: pool, Keys: keys}
	f.app = githubapp.Service{Pool: pool, Keys: keys, Provider: provider{}}
	f.configure(testSecret)
	f.exec("UPDATE github_app_integrations SET installation_id=7 WHERE event_id=$1", f.event)
	f.exec("UPDATE events SET published_at=$2 WHERE id=$1", f.event, f.publication)
	f.exec(
		"INSERT INTO hack_teams(event_id,id,name,state) VALUES($1,$2,'Synthetic team','published')",
		f.event,
		f.team,
	)
	if err := dbgen.New(pool).
		CreateCompetitionEntry(t.Context(), dbgen.CreateCompetitionEntryParams{EventID: f.event, TeamID: f.team}); err != nil {
		t.Fatal(err)
	}
	// The canonical provisioning owner is seeded with a synthetic proven immutable
	// provider identity; no parallel mapping or name-based resolution exists.
	f.exec(
		"INSERT INTO team_repositories(event_id,team_id,organization_id,organization,name,app_id,installation_id,template_repository,github_repository_id,state,requested_by) VALUES($1,$2,99,'synthetic-org','synthetic-repo',42,7,'synthetic-template',123,'ready',$3)",
		f.event,
		f.team,
		f.owner,
	)
	outbox, err := jobs.New(
		pool,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		map[string]jobs.Handler{HandlerKind: f.s},
	)
	if err != nil {
		t.Fatal(err)
	}
	f.s.Jobs = outbox
	return f
}

func (f *fixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.pool.Exec(f.t.Context(), sql, args...); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) configure(secret string) {
	f.t.Helper()
	event, err := dbgen.New(f.pool).Event(f.t.Context(), f.event)
	if err != nil {
		f.t.Fatal(err)
	}
	err = f.app.Configure(
		f.t.Context(),
		f.owner,
		f.event,
		event.Revision,
		githubapp.Command{
			Action: "configure",
			Configuration: githubapp.Configuration{
				AppID:              42,
				AppSlug:            "synthetic-app",
				OrganizationID:     99,
				Organization:       "synthetic-org",
				TemplateRepository: "synthetic-template",
			},
			PrivateKey:    "SYNTHETIC-PEM",
			WebhookSecret: secret,
		},
	)
	if err != nil {
		f.t.Fatal(err)
	}
}

func sign(raw []byte, secret string) string {
	m := hmac.New(sha256.New, []byte(secret))
	_, _ = m.Write(raw)
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

func (f *fixture) push(at time.Time) []byte {
	f.t.Helper()
	raw, err := json.Marshal(
		map[string]any{
			"installation": map[string]any{"id": 7},
			"repository": map[string]any{
				"id":          123,
				"owner":       map[string]any{"id": 99},
				"pushed_at":   at.Unix(),
				"description": "PRIVATE-REPOSITORY-PROSE-CANARY",
			},
			"ref":     "refs/heads/main",
			"after":   strings.Repeat("a", 40),
			"commits": []any{map[string]any{"message": "PRIVATE-COMMIT-PROSE-CANARY"}},
		},
	)
	if err != nil {
		f.t.Fatal(err)
	}
	return raw
}

func (f *fixture) ingest(id uuid.UUID, raw []byte) error {
	return f.s.Ingest(
		f.t.Context(),
		f.event,
		Headers{DeliveryID: id.String(), Event: "push", Signature: sign(raw, testSecret)},
		raw,
	)
}

func (f *fixture) delivery(id uuid.UUID) dbgen.GithubWebhookDelivery {
	f.t.Helper()
	d, err := dbgen.New(f.pool).GitHubWebhookDelivery(f.t.Context(), id)
	if err != nil {
		f.t.Fatal(err)
	}
	return d
}

func (f *fixture) execute(id uuid.UUID) {
	f.t.Helper()
	d := f.delivery(id)
	if d.OperationID == nil {
		f.t.Fatal("missing atomic operation")
	}
	if err := f.s.Jobs.Execute(f.t.Context(), *d.OperationID); err != nil {
		f.t.Fatal(err)
	}
}

func TestSignatureExactRawBytesOfficialVector(t *testing.T) {
	raw := []byte("Hello, World!")
	signature := "sha256=757107ea0eb2509fc211221cce984b8a37570b6d7586c22c46f4379c8b043e17"
	if !validSignature(raw, []byte("It's a Secret to Everybody"), signature) {
		t.Fatal("GitHub published SHA256 vector rejected")
	}
	for _, bad := range []string{"", "sha1=" + strings.Repeat("a", 40), "sha256=" + strings.Repeat("z", 64), signature + "a"} {
		if validSignature(raw, []byte("It's a Secret to Everybody"), bad) {
			t.Fatalf("accepted malformed signature %q", bad)
		}
	}
	if validSignature(append(raw, '\n'), []byte("It's a Secret to Everybody"), signature) {
		t.Fatal("normalized raw body")
	}
}

func TestWebhookReplayOutOfOrderAndAtomicInformationalActivity(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	newer := time.Now().Add(-time.Minute).UTC().Truncate(time.Second)
	older := newer.Add(-time.Minute)
	id := uuid.New()
	raw := f.push(newer)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Go(func() { errs <- f.ingest(id, raw) })
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	f.execute(id)
	delayed := uuid.New()
	if err := f.ingest(delayed, f.push(older)); err != nil {
		t.Fatal(err)
	}
	f.execute(delayed)
	d := f.delivery(id)
	if got := f.s.Reconcile(
		ctx,
		jobs.Delivery{ID: *d.OperationID, EventID: f.event, TargetID: id},
	); got != jobs.Delivered {
		t.Fatal(got)
	}
	status, err := f.s.Status(ctx, f.owner, f.event)
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Activity) != 1 || status.Activity[0].Pushes != 2 ||
		status.Activity[0].LastProviderAt == nil ||
		!status.Activity[0].LastProviderAt.Equal(newer) {
		t.Fatalf("regressed/double counted activity: %+v", status.Activity)
	}
	if len(status.Deliveries) != 2 || status.Deliveries[0].Disposition != "recorded" {
		t.Fatalf("missing durable status: %+v", status)
	}
	data, _ := json.Marshal(status)
	if strings.Contains(string(data), "CANARY") || strings.Contains(string(data), testSecret) ||
		strings.Contains(string(data), "payload_hash") {
		t.Fatal("private metadata escaped")
	}
	var submissions, scores int
	if err := f.pool.QueryRow(ctx, "SELECT count(*) FROM submission_states WHERE event_id=$1", f.event).
		Scan(&submissions); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, "SELECT count(*) FROM mentor_scorecard_versions WHERE event_id=$1", f.event).
		Scan(&scores); err != nil {
		t.Fatal(err)
	}
	if submissions != 0 || scores != 0 {
		t.Fatal("ordinary pre-live activity became assessment input")
	}
	if err := f.ingest(id, f.push(older)); !errors.Is(err, access.ErrConflict) {
		t.Fatalf("GUID content substitution: %v", err)
	}
}

func TestWebhookAuthenticationProvenanceAndPublicationRejectBeforeMutation(t *testing.T) {
	f := setup(t)
	raw := f.push(time.Now().Add(-time.Minute))
	id := uuid.New()
	if err := f.s.Ingest(
		t.Context(),
		f.event,
		Headers{DeliveryID: id.String(), Event: "push", Signature: sign(raw, "wrong")},
		raw,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatalf("bad signature: %v", err)
	}
	for _, change := range []struct{ old, new string }{{`"id":123`, `"id":124`}, {`"id":99`, `"id":98`}, {`"id":7`, `"id":8`}} {
		modified := bytes.Replace(raw, []byte(change.old), []byte(change.new), 1)
		if err := f.ingest(uuid.New(), modified); !errors.Is(err, access.ErrForbidden) {
			t.Fatalf("unrelated source accepted: %v", err)
		}
	}
	f.exec("UPDATE github_app_integrations SET app_id=43 WHERE event_id=$1", f.event)
	if err := f.ingest(uuid.New(), raw); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("unrelated App: %v", err)
	}
	f.exec("UPDATE github_app_integrations SET app_id=42 WHERE event_id=$1", f.event)
	f.exec("UPDATE events SET published_at=NULL WHERE id=$1", f.event)
	if err := f.ingest(uuid.New(), raw); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("unpublished challenge: %v", err)
	}
	var count int
	if err := f.pool.QueryRow(t.Context(), "SELECT count(*) FROM github_webhook_deliveries").
		Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("rejected source mutated domain")
	}
}

func TestWebhookWorkerRechecksSourceDisbandApprovalAndReconcilesCrash(t *testing.T) {
	f := setup(t)
	raw := f.push(time.Now().Add(-time.Minute))
	id := uuid.New()
	if err := f.ingest(id, raw); err != nil {
		t.Fatal(err)
	}
	f.exec("UPDATE hack_teams SET state='disbanded' WHERE event_id=$1 AND id=$2", f.event, f.team)
	f.execute(id)
	if f.delivery(id).Disposition != "discarded" {
		t.Fatal("disbanded work was applied")
	}
	if err := f.ingest(uuid.New(), raw); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("disbanded repository accepted")
	}
	var active bool
	if err := f.pool.QueryRow(t.Context(), "SELECT active FROM competition_entries WHERE event_id=$1 AND team_id=$2", f.event, f.team).
		Scan(&active); err != nil {
		t.Fatal(err)
	}
	// Native disband commands deactivate entries; the webhook must preserve that.
	f.exec(
		"UPDATE competition_entries SET active=false WHERE event_id=$1 AND team_id=$2",
		f.event,
		f.team,
	)
	f.exec("UPDATE hack_teams SET state='published' WHERE event_id=$1 AND id=$2", f.event, f.team)
	id = uuid.New()
	if err := f.ingest(id, raw); err != nil {
		t.Fatal(err)
	}
	d := f.delivery(id)
	f.exec("UPDATE operations SET approved=false WHERE id=$1", *d.OperationID)
	f.execute(id)
	if f.delivery(id).Disposition != "pending" {
		t.Fatal("unapproved work executed")
	}
	f.exec("UPDATE operations SET approved=true WHERE id=$1", *d.OperationID)
	// Simulate application transaction committed then process died before finish.
	outcome := f.s.Deliver(
		t.Context(),
		jobs.Delivery{ID: *d.OperationID, EventID: f.event, TargetID: id},
	)
	if outcome != jobs.Delivered {
		t.Fatal(outcome)
	}
	f.exec(
		"UPDATE operations SET state='running',lease_token=$2,lease_until=now()-interval '1 minute' WHERE id=$1",
		*d.OperationID,
		uuid.New(),
	)
	if err := f.s.Jobs.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	status, err := f.s.Status(t.Context(), f.owner, f.event)
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Activity) != 1 || status.Activity[0].Pushes != 1 {
		t.Fatalf("crash recovery duplicated: %+v", status)
	}
	var current bool
	if err := f.pool.QueryRow(t.Context(), "SELECT active FROM competition_entries WHERE event_id=$1 AND team_id=$2", f.event, f.team).
		Scan(&current); err != nil {
		t.Fatal(err)
	}
	if current {
		t.Fatal("webhook reactivated competition entry")
	}
}

func TestWebhookSecretReplacementReadOnlyScopeAndMemberMetadata(t *testing.T) {
	f := setup(t)
	raw := f.push(time.Now().Add(-time.Minute))
	id := uuid.New()
	if err := f.ingest(id, raw); err != nil {
		t.Fatal(err)
	}
	if err := f.app.Configure(
		t.Context(),
		f.owner,
		f.event,
		func() int64 { e, _ := dbgen.New(f.pool).Event(t.Context(), f.event); return e.Revision }(),
		githubapp.Command{Action: "rotate"},
	); err != nil {
		t.Fatal(err)
	}
	f.execute(id)
	if f.delivery(id).Disposition != "discarded" {
		t.Fatal("secret-version source changed but pending activity applied")
	}
	member := []byte(
		`{"action":"added","member":{"id":77,"login":"PRIVATE-LOGIN-CANARY"},"installation":{"id":7},"repository":{"id":123,"owner":{"id":99}}}`,
	)
	memberID := uuid.New()
	if err := f.s.Ingest(
		t.Context(),
		f.event,
		Headers{
			DeliveryID: memberID.String(),
			Event:      "member",
			Signature:  sign(member, testSecret),
		},
		member,
	); err != nil {
		t.Fatal(err)
	}
	f.execute(memberID)
	d := f.delivery(memberID)
	if d.ProviderAt.Valid || d.MemberID != 77 || d.Action != "added" {
		t.Fatalf("invented provider time/lost selected invitation metadata: %+v", d)
	}
	status, err := f.s.Status(t.Context(), f.owner, f.event)
	if err != nil || len(status.Activity) != 1 || status.Activity[0].AcceptedInvitations != 1 {
		t.Fatalf("member activity: %+v %v", status, err)
	}
	outsider := testdb.Person(t, f.pool)
	if _, err = f.s.Status(
		t.Context(),
		outsider.ID,
		f.event,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("outsider read operational evidence")
	}
	f.exec("UPDATE events SET writes_allowed=false WHERE id=$1", f.event)
	if _, err = f.s.Status(t.Context(), f.owner, f.event); err != nil {
		t.Fatal("retained operational evidence became unreadable")
	}
	if err = f.ingest(uuid.New(), raw); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("read-only event accepted activity")
	}
	f.exec("UPDATE events SET writes_allowed=true WHERE id=$1", f.event)
	pendingID := uuid.New()
	if err := f.ingest(pendingID, raw); err != nil {
		t.Fatal(err)
	}
	event, _ := dbgen.New(f.pool).Event(t.Context(), f.event)
	if err := f.app.Configure(
		t.Context(),
		f.owner,
		f.event,
		event.Revision,
		githubapp.Command{Action: "disconnect"},
	); err != nil {
		t.Fatal(err)
	}
	f.configure(testSecret)
	f.exec("UPDATE github_app_integrations SET installation_id=7 WHERE event_id=$1", f.event)
	f.execute(pendingID)
	if f.delivery(pendingID).Disposition != "discarded" {
		t.Fatal("disconnect/reconnect resurrected an old secret generation")
	}
	f.configure("SYNTHETIC-REPLACEMENT-WEBHOOK-SECRET-0000")
	f.exec("UPDATE github_app_integrations SET installation_id=7 WHERE event_id=$1", f.event)
	if err = f.ingest(uuid.New(), raw); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("old shared secret remained valid after replacement")
	}
}

func TestWebhookCapacityAndTransactionRollback(t *testing.T) {
	f := setup(t)
	raw := f.push(time.Now().Add(-time.Minute))
	id := uuid.New()
	outbox := f.s.Jobs
	f.s.Jobs = nil
	if err := f.ingest(id, raw); err == nil {
		t.Fatal("missing outbox accepted work")
	}
	var count int
	if err := f.pool.QueryRow(t.Context(), "SELECT count(*) FROM github_webhook_deliveries").
		Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("partial domain write without job")
	}
	f.s.Jobs = outbox
	f.exec(
		`INSERT INTO github_webhook_deliveries(id,event_id,team_id,repository_id,app_id,installation_id,provenance,secret_version,payload_hash,kind)
 SELECT gen_random_uuid(),$1,$2,123,42,7,provenance,1,decode(repeat('00',32),'hex'),'push' FROM team_repositories CROSS JOIN generate_series(1,$3) WHERE event_id=$1 AND team_id=$2`,
		f.event,
		f.team,
		MaxPending,
	)
	if err := f.ingest(id, raw); !errors.Is(err, ErrCapacity) {
		t.Fatalf("unbounded pending work: %v", err)
	}
}
