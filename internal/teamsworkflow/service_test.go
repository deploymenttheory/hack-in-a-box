package teamsworkflow

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/secretbox"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type fixture struct {
	t            *testing.T
	pool         *pgxpool.Pool
	owner        dbgen.Person
	id           uuid.UUID
	now          time.Time
	s            *Service
	calls        atomic.Int32
	status       atomic.Int32
	retrySeconds atomic.Int32
	payloads     chan string
}

func keyring(t *testing.T, value byte) *secretbox.Keyring {
	t.Helper()
	raw, _ := json.Marshal(
		map[string]any{
			"active": "synthetic",
			"keys": map[string]string{
				"synthetic": base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{value}, 32)),
			},
		},
	)
	keys, err := secretbox.Parse(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	return keys
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	pool := testdb.New(t, true)
	owner := testdb.Person(t, pool)
	id := testdb.Event(t, pool, owner.ID)
	f := &fixture{
		t:        t,
		pool:     pool,
		owner:    owner,
		id:       id,
		now:      time.Now().UTC().Add(-2 * time.Hour),
		payloads: make(chan string, 20),
	}
	config, err := json.Marshal(event.DefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	f.exec("UPDATE events SET configuration=$2 WHERE id=$1", f.id, config)
	f.status.Store(202)
	f.retrySeconds.Store(12)
	network, _ := tlsFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		f.payloads <- string(body)
		if f.status.Load() == 429 {
			w.Header().Set("Retry-After", strconv.Itoa(int(f.retrySeconds.Load())))
		}
		w.WriteHeader(int(f.status.Load()))
	}))
	f.s = &Service{
		Pool:    pool,
		Keys:    keyring(t, 31),
		Now:     func() time.Time { return f.now },
		network: network,
	}
	composer := communications.Service{
		Pool:     pool,
		Now:      func() time.Time { return f.now },
		Channels: map[string]communications.ChannelProvider{"teams": f.s},
	}
	service, err := jobs.New(
		pool,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		map[string]jobs.Handler{HandlerKind(): f.s},
	)
	if err != nil {
		t.Fatal(err)
	}
	f.s.Jobs = service
	composer.Jobs = service
	f.s.Composer = composer
	f.change(
		Command{
			Action:   "save",
			Label:    "Synthetic event channel",
			Owner:    "Synthetic owner",
			CoOwner:  "Synthetic co-owner",
			Endpoint: endpoint,
		},
	)
	return f
}

func (f *fixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.pool.Exec(f.t.Context(), sql, args...); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) revision() int64 {
	f.t.Helper()
	e, err := dbgen.New(f.pool).Event(f.t.Context(), f.id)
	if err != nil {
		f.t.Fatal(err)
	}
	return e.Revision
}

func (f *fixture) change(input Command) Page {
	f.t.Helper()
	p, err := f.s.Change(f.t.Context(), f.owner.ID, f.id, f.revision(), input)
	if err != nil {
		f.t.Fatal(err)
	}
	return p
}

func (f *fixture) testOperation() uuid.UUID {
	f.t.Helper()
	p, err := f.s.QueueTest(
		f.t.Context(),
		f.owner.ID,
		f.id,
		f.revision(),
		TestInput{RequestID: uuid.New()},
	)
	if err != nil {
		f.t.Fatal(err)
	}
	row, err := dbgen.New(f.pool).
		TeamsWorkflowDelivery(f.t.Context(), dbgen.TeamsWorkflowDeliveryParams{EventID: f.id, ID: p.History[0].ID})
	if err != nil {
		f.t.Fatal(err)
	}
	return row.OperationID
}

func (f *fixture) execute(id uuid.UUID) {
	f.t.Helper()
	if err := f.s.Jobs.Execute(f.t.Context(), id); err != nil && !errors.Is(err, jobs.ErrRetry) {
		f.t.Fatal(err)
	}
}

func (f *fixture) page() Page {
	f.t.Helper()
	p, err := f.s.Read(f.t.Context(), f.owner.ID, f.id)
	if err != nil {
		f.t.Fatal(err)
	}
	return p
}

func TestConnectionEncryptionAuthorityAndExplicitChannelTest(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	row, err := dbgen.New(f.pool).TeamsWorkflowConnection(ctx, f.id)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(row.Ciphertext, []byte("SYNTHETIC_SIGNATURE")) {
		t.Fatal("endpoint persisted in plaintext")
	}
	if _, err = f.s.Keys.Open(
		row.KeyID,
		row.Ciphertext,
		binding(uuid.New(), row.ID, row.Version),
	); err == nil {
		t.Fatal("ciphertext not event-bound")
	}
	if _, err = f.s.Keys.Open(
		row.KeyID,
		row.Ciphertext,
		binding(f.id, row.ID, row.Version+1),
	); err == nil {
		t.Fatal("ciphertext not version-bound")
	}
	outsider := testdb.Person(t, f.pool)
	if _, err = f.s.Read(ctx, outsider.ID, f.id); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("outsider settings: %v", err)
	}
	if _, err = f.s.QueueTest(
		ctx,
		outsider.ID,
		f.id,
		f.revision(),
		TestInput{RequestID: uuid.New()},
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatalf("outsider channel test: %v", err)
	}
	request := TestInput{RequestID: uuid.New()}
	rev := f.revision()
	first, err := f.s.QueueTest(ctx, f.owner.ID, f.id, rev, request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.QueueTest(ctx, f.owner.ID, f.id, rev, request); err != nil {
		t.Fatal("safe test retry lost original operation")
	}
	delivery, err := dbgen.New(f.pool).
		TeamsWorkflowDelivery(ctx, dbgen.TeamsWorkflowDeliveryParams{EventID: f.id, ID: first.History[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	f.execute(delivery.OperationID)
	f.execute(delivery.OperationID)
	if f.calls.Load() != 1 {
		t.Fatal("channel test replayed")
	}
	payload := <-f.payloads
	if !strings.Contains(payload, "TEST — Hack-in-a-Box") ||
		!strings.Contains(payload, "broadcast to the configured event channel") ||
		strings.Contains(payload, "SYNTHETIC_SIGNATURE") {
		t.Fatal("unsafe test payload")
	}
	page := f.page()
	if page.History[0].Status != "provider_accepted" {
		t.Fatalf("provider receipt: %#v", page.History[0])
	}
	raw, _ := json.Marshal(page)
	if bytes.Contains(raw, []byte("SYNTHETIC_SIGNATURE")) ||
		bytes.Contains(raw, []byte("ciphertext")) ||
		bytes.Contains(raw, []byte(testHost)) {
		t.Fatal("private endpoint escaped domain projection")
	}
	var auditJSON string
	if err = f.pool.QueryRow(ctx, "SELECT coalesce(jsonb_agg(metadata)::text,'[]') FROM audit_events WHERE event_id=$1", f.id).
		Scan(&auditJSON); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(auditJSON, "SYNTHETIC_SIGNATURE") {
		t.Fatal("endpoint entered audit metadata")
	}
}

func TestQueuedTestRechecksRoleConnectionKeyAndEvent(t *testing.T) {
	f := newFixture(t)
	op := f.testOperation()
	other := testdb.Person(t, f.pool)
	f.exec(
		"INSERT INTO event_memberships(event_id,person_id,role) VALUES($1,$2,'organiser')",
		f.id,
		other.ID,
	)
	f.exec(
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='organiser'",
		f.id,
		f.owner.ID,
	)
	f.execute(op)
	if f.calls.Load() != 0 {
		t.Fatal("revoked organiser sent test")
	}
	f.exec(
		"UPDATE event_memberships SET active=true WHERE event_id=$1 AND person_id=$2 AND role='organiser'",
		f.id,
		f.owner.ID,
	)
	original := f.s.Keys
	f.s.Keys = keyring(t, 29)
	f.execute(op)
	if f.calls.Load() != 0 {
		t.Fatal("wrong key sent test")
	}
	f.s.Keys = original
	// A changed connection revision never receives a queued test for the old URL.
	f.change(Command{Action: "rotate"})
	f.execute(op)
	if f.calls.Load() != 0 {
		t.Fatal("old intent followed a replaced connection")
	}
	fresh := f.testOperation()
	f.exec("UPDATE events SET writes_allowed=false WHERE id=$1", f.id)
	f.execute(fresh)
	if f.calls.Load() != 0 {
		t.Fatal("paused event sent test")
	}
}

func TestRateLimitHasBoundedRetryAndAmbiguousSendNeverReplays(t *testing.T) {
	f := newFixture(t)
	f.status.Store(429)
	op := f.testOperation()
	f.execute(op)
	page := f.page()
	if page.History[0].Status != "retryable" || page.History[0].RetryAt == nil {
		t.Fatal("429 retry receipt missing")
	}
	f.execute(op)
	if f.calls.Load() != 1 {
		t.Fatal("Retry-After not honored")
	}
	f.now = f.now.Add(13 * time.Second)
	f.status.Store(500)
	f.execute(op)
	if f.calls.Load() != 2 || f.page().History[0].Status != "ambiguous" {
		t.Fatal("uncertain outcome not retained")
	}
	f.now = f.now.Add(time.Hour)
	f.execute(op)
	if f.calls.Load() != 2 {
		t.Fatal("uncertain send replayed")
	}
	var attempts int
	if err := f.pool.QueryRow(t.Context(), "SELECT count(*) FROM teams_workflow_attempts WHERE event_id=$1", f.id).
		Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatal("actual attempt history lost")
	}
	if _, err := f.pool.Exec(
		t.Context(),
		"UPDATE teams_workflow_receipts SET reason='rewrite' WHERE event_id=$1",
		f.id,
	); err == nil {
		t.Fatal("immutable provider receipt changed")
	}
	if _, err := f.pool.Exec(
		t.Context(),
		"DELETE FROM teams_workflow_attempts WHERE event_id=$1",
		f.id,
	); err == nil {
		t.Fatal("immutable attempt removed")
	}
}

func (f *fixture) campaign(purpose, body string) uuid.UUID {
	f.t.Helper()
	ctx := f.t.Context()
	person := testdb.Person(f.t, f.pool)
	f.exec(
		"INSERT INTO competition_claims(event_id,person_id,state) VALUES($1,$2,'confirmed')",
		f.id,
		person.ID,
	)
	template := communications.Template{
		ID:      uuid.New(),
		Name:    "Synthetic public notice",
		Purpose: purpose,
		Format:  "markdown",
		Subject: "Event reminder",
		Body:    body,
	}
	audience := communications.Audience{
		ID:                 uuid.New(),
		Name:               "Current confirmed competitors",
		Roles:              []string{"attendee"},
		RegistrationStates: []string{},
		Teams:              []uuid.UUID{},
	}
	for _, input := range []communications.Command{{Action: "save_template", Template: &template, Reason: "Synthetic public template"}, {Action: "save_audience", Audience: &audience, Reason: "Synthetic common audience"}} {
		if _, err := f.s.Composer.Change(ctx, f.owner.ID, f.id, f.revision(), input); err != nil {
			f.t.Fatal(err)
		}
	}
	wall, choice := testdb.WallTimeForInstant(f.t, f.now.Add(time.Minute), "Europe/London")
	definition := communications.CampaignDefinition{
		ID:         uuid.New(),
		Name:       "Synthetic event broadcast",
		TemplateID: template.ID,
		AudienceID: audience.ID,
		Channels:   []string{"teams"},
		Timing: communications.CampaignTiming{
			Kind:      "fixed",
			WallTime:  wall,
			DSTChoice: choice,
		},
	}
	var version int64
	for _, action := range []string{"save", "preview", "approve"} {
		input := communications.CampaignCommand{
			Action:  action,
			Version: version,
			Reason:  "Synthetic explicit broadcast review",
		}
		if action == "save" {
			input.Definition = &definition
		}
		detail, err := f.s.Composer.ChangeCampaign(
			ctx,
			f.owner.ID,
			f.id,
			definition.ID,
			f.revision(),
			input,
		)
		if err != nil {
			var blocked *event.Blocked
			if errors.As(err, &blocked) {
				f.t.Fatalf("%v: %+v", err, blocked.Issues)
			}
			f.t.Fatal(err)
		}
		version = detail.Campaign.Campaign.Version
	}
	f.now = f.now.Add(2 * time.Minute)
	return definition.ID
}

func TestCampaignFixturePreservesBothRepeatedHourInstants(t *testing.T) {
	for _, tc := range []struct {
		name, instant, choice string
	}{
		{"earlier", "2026-10-25T00:30:00Z", "earlier"},
		{"later", "2026-10-25T01:30:00Z", "later"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			target, err := time.Parse(time.RFC3339, tc.instant)
			if err != nil {
				t.Fatal(err)
			}
			f.now = target.Add(-time.Minute)
			id := f.campaign("reminder", "Synthetic repeated-hour reminder")
			current, err := f.s.Composer.Campaign(t.Context(), f.owner.ID, f.id, id)
			if err != nil {
				t.Fatal(err)
			}
			campaign := current.Campaign.Campaign
			if current.Campaign.DueAt == nil || !current.Campaign.DueAt.Equal(target) ||
				campaign.Approval == nil || !campaign.Approval.DueAt.Equal(target) ||
				campaign.Definition.Timing.DSTChoice != tc.choice {
				t.Fatalf("campaign changed the intended repeated-hour instant: %+v", current.Campaign)
			}
		})
	}
}

func TestOnlyCurrentApprovedPublicCampaignCanReachChannel(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	id := f.campaign("reminder", "Public **literal** message for {{event.name}}")
	campaign, err := f.s.Composer.Campaign(ctx, f.owner.ID, f.id, id)
	if err != nil {
		t.Fatal(err)
	}
	version := campaign.Campaign.Campaign.Version
	p, err := f.s.QueueBroadcast(ctx, f.owner.ID, f.id, id, version)
	if err != nil {
		t.Fatal(err)
	}
	delivery, err := dbgen.New(f.pool).
		TeamsWorkflowDelivery(ctx, dbgen.TeamsWorkflowDeliveryParams{EventID: f.id, ID: p.History[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.QueueBroadcast(ctx, f.owner.ID, f.id, id, version); err != nil {
		t.Fatal(err)
	}
	f.execute(delivery.OperationID)
	if f.calls.Load() != 1 {
		t.Fatal("approved common campaign did not send exactly once")
	}
	payload := <-f.payloads
	if !strings.Contains(payload, "Public") || strings.Contains(payload, "person.name") ||
		strings.Contains(payload, "SYNTHETIC_SIGNATURE") {
		t.Fatal("unsafe public projection")
	}
	// Approval is bound to the exact endpoint revision, including a same-time edit.
	f.change(Command{Action: "rotate"})
	if _, err = f.s.QueueBroadcast(ctx, f.owner.ID, f.id, id, version); err == nil {
		t.Fatal("connection change retained campaign approval")
	}
}

// This starts the actual River client. Direct Execute calls cannot detect a job
// incorrectly terminalized while its durable business operation remains pending.
func TestRiverWorkersResumeAfter429AndSharedConnectionWindow(t *testing.T) {
	f := newFixture(t)
	f.s.Now = nil
	f.status.Store(429)
	f.retrySeconds.Store(2)
	first := f.testOperation()
	if err := f.s.Jobs.Client.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := f.s.Jobs.Client.StopAndCancel(ctx); err != nil {
			t.Error(err)
		}
	})
	wait := func(check func() bool) {
		t.Helper()
		deadline := time.Now().Add(95 * time.Second)
		for time.Now().Before(deadline) {
			if check() {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatal("River did not finish the retained Teams operation")
	}
	wait(
		func() bool { p := f.page(); return len(p.History) > 0 && p.History[0].Status == "retryable" },
	)
	if f.calls.Load() != 1 {
		t.Fatal("unexpected first attempt count")
	}
	retryAt := *f.page().History[0].RetryAt
	f.status.Store(202)
	second := f.testOperation()
	wait(func() bool {
		var lost int
		err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM operations o JOIN river_job j ON (j.args->>'operation_id')::uuid=o.id WHERE o.id=ANY($1::uuid[]) AND o.state='pending' AND j.state='completed'`, []uuid.UUID{first, second}).
			Scan(&lost)
		if err != nil {
			t.Fatal(err)
		}
		if lost > 0 {
			t.Fatal("River completed a temporarily ineligible Teams operation")
		}
		if time.Now().Before(retryAt) && f.calls.Load() != 1 {
			t.Fatal("provider retried before Retry-After")
		}
		var done int
		if err = f.pool.QueryRow(t.Context(), "SELECT count(*) FROM operations WHERE id=ANY($1::uuid[]) AND state='delivered'", []uuid.UUID{first, second}).
			Scan(&done); err != nil {
			t.Fatal(err)
		}
		return done == 2
	})
	if f.calls.Load() != 3 {
		t.Fatal("rate-window recovery lost or duplicated an external request")
	}
}

func TestQueuedBroadcastRechecksCurrentRecipientsAndCancellation(t *testing.T) {
	for _, change := range []string{"withdrawal", "cancelled", "private_source", "stale_version"} {
		t.Run(change, func(t *testing.T) {
			f := newFixture(t)
			ctx := t.Context()
			id := f.campaign("reminder", "Public event reminder")
			detail, err := f.s.Composer.Campaign(ctx, f.owner.ID, f.id, id)
			if err != nil {
				t.Fatal(err)
			}
			version := detail.Campaign.Campaign.Version
			if change == "stale_version" {
				if _, err := f.s.QueueBroadcast(ctx, f.owner.ID, f.id, id, version-1); err == nil {
					t.Fatal("stale version queued")
				}
				return
			}
			page, err := f.s.QueueBroadcast(ctx, f.owner.ID, f.id, id, version)
			if err != nil {
				t.Fatal(err)
			}
			intent, err := dbgen.New(f.pool).
				TeamsWorkflowDelivery(ctx, dbgen.TeamsWorkflowDeliveryParams{EventID: f.id, ID: page.History[0].ID})
			if err != nil {
				t.Fatal(err)
			}
			switch change {
			case "withdrawal":
				f.exec("UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1", f.id)
			case "cancelled":
				_, err = f.s.Composer.ChangeCampaign(
					ctx,
					f.owner.ID,
					f.id,
					id,
					f.revision(),
					communications.CampaignCommand{
						Action:  "cancel",
						Version: version,
						Reason:  "Cancel synthetic broadcast",
					},
				)
				if err != nil {
					t.Fatal(err)
				}
			case "private_source":
				source, err := f.s.Composer.Read(ctx, f.owner.ID, f.id)
				if err != nil {
					t.Fatal(err)
				}
				template := source.Templates[0]
				template.Body = "PRIVATE canary for {{person.name}}"
				if _, err := f.s.Composer.Change(
					ctx,
					f.owner.ID,
					f.id,
					f.revision(),
					communications.Command{
						Action:   "save_template",
						Template: &template,
						Reason:   "Personal content cannot be broadcast",
					},
				); err != nil {
					t.Fatal(err)
				}
			}
			f.execute(intent.OperationID)
			if f.calls.Load() != 0 {
				t.Fatal("obsolete/private broadcast reached the channel")
			}
		})
	}
}

func TestProviderAttemptsAreBoundedAndCrashRecoveryNeverReplays(t *testing.T) {
	t.Run("retry limit", func(t *testing.T) {
		f := newFixture(t)
		f.status.Store(429)
		op := f.testOperation()
		for i := 0; i < 7; i++ {
			f.execute(op)
			f.now = f.now.Add(20 * time.Second)
		}
		if f.calls.Load() != 5 || f.page().History[0].Status != "permanent" {
			t.Fatalf(
				"unbounded provider retries: calls%d receipt%+v",
				f.calls.Load(),
				f.page().History[0],
			)
		}
	})
	for _, accepted := range []bool{false, true} {
		t.Run(fmt.Sprintf("accepted_%t", accepted), func(t *testing.T) {
			f := newFixture(t)
			op := f.testOperation()
			ctx := t.Context()
			page := f.page()
			intent, err := dbgen.New(f.pool).
				TeamsWorkflowDelivery(ctx, dbgen.TeamsWorkflowDeliveryParams{EventID: f.id, ID: page.History[0].ID})
			if err != nil {
				t.Fatal(err)
			}
			attempt := uuid.New()
			f.exec(
				"INSERT INTO teams_workflow_attempts(event_id,id,delivery_id,attempt_number,connection_version,payload_fingerprint,started_at) VALUES($1,$2,$3,1,1,'synthetic',$4)",
				f.id,
				attempt,
				intent.ID,
				f.now,
			)
			if accepted {
				f.exec(
					"INSERT INTO teams_workflow_receipts(event_id,attempt_id,outcome,http_status,reason,recorded_at) VALUES($1,$2,'provider_accepted',202,'provider_accepted',$3)",
					f.id,
					attempt,
					f.now,
				)
			}
			delivery := jobs.Delivery{ID: op, EventID: f.id, TargetID: intent.ID}
			expected := jobs.Ambiguous
			if accepted {
				expected = jobs.Delivered
			}
			if got := f.s.Reconcile(ctx, delivery); got != expected {
				t.Fatalf("recovery %s, want %s", got, expected)
			}
			f.execute(op)
			if f.calls.Load() != 0 {
				t.Fatal("recovered request was replayed")
			}
			if !accepted && f.page().History[0].Status != "ambiguous" {
				t.Fatal("missing receipt was not recorded as uncertain")
			}
		})
	}
}
