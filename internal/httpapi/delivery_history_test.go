package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/deliveryhistory"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/identity"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/maildelivery"
	"github.com/deploymenttheory/hack-in-a-box/internal/smtpintegration"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type reviewRecordingTransport struct {
	mu       sync.Mutex
	state    maildelivery.State
	messages []maildelivery.Message
	entered  chan struct{}
	release  chan struct{}
}

func (*reviewRecordingTransport) Configured() bool { return true }
func (*reviewRecordingTransport) Revision() string { return "synthetic-http-relay-v1" }
func (*reviewRecordingTransport) MessageID(id uuid.UUID) string {
	return "<" + id.String() + "@example.test>"
}

func (p *reviewRecordingTransport) Send(
	ctx context.Context,
	m maildelivery.Message,
) maildelivery.Receipt {
	p.mu.Lock()
	p.messages = append(p.messages, m)
	state, entered, release := p.state, p.entered, p.release
	p.mu.Unlock()
	if entered != nil {
		entered <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
			return maildelivery.Receipt{State: maildelivery.Ambiguous, Code: "acceptance_unknown"}
		}
	}
	code := "relay_accepted"
	if state == maildelivery.Ambiguous {
		code = "acceptance_unknown"
	}
	return maildelivery.Receipt{State: state, Code: code}
}

func (p *reviewRecordingTransport) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.messages)
}

func TestDeliveryHistoryHTTPToActualReviewedRiverPreservesEvidenceAndFiltersCurrentAuthority(
	t *testing.T,
) {
	for _, prior := range []maildelivery.State{maildelivery.Accepted, maildelivery.Ambiguous} {
		t.Run(string(prior), func(t *testing.T) {
			ctx := t.Context()
			pool := testdb.New(t, true)
			q := dbgen.New(pool)
			owner := testdb.Person(t, pool)
			other := testdb.Person(t, pool)
			admin := testdb.Person(t, pool)
			id := testdb.Event(t, pool, owner.ID)
			if _, err := pool.Exec(
				ctx,
				"UPDATE people SET platform_admin=true WHERE id=$1",
				admin.ID,
			); err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC().Add(-time.Hour)
			dates := event.DefaultConfiguration()
			dates.LiveStart = time.Now().UTC().AddDate(0, 1, 0)
			dates.LiveEnd = dates.LiveStart.Add(8 * time.Hour)
			dates.SubmissionCutoff = dates.LiveEnd.Add(-time.Hour)
			dates.TeamFreeze = dates.LiveStart.AddDate(0, 0, -8)
			dates.AdmissionCutoff = dates.TeamFreeze.Add(-time.Hour)
			raw, _ := json.Marshal(dates)
			if _, err := pool.Exec(
				ctx,
				"UPDATE events SET configuration=$2,stage='registration' WHERE id=$1",
				id,
				raw,
			); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(
				ctx,
				"INSERT INTO event_gate_facts(event_id) VALUES($1)",
				id,
			); err != nil {
				t.Fatal(err)
			}
			if err := q.ConfirmPersonEmail(
				ctx,
				dbgen.ConfirmPersonEmailParams{
					PersonID: owner.ID,
					Mailbox:  "PRIVATE-DESTINATION@example.test",
				},
			); err != nil {
				t.Fatal(err)
			}
			transport := &reviewRecordingTransport{state: prior}
			composer := &communications.Service{Pool: pool, Now: func() time.Time { return now }}
			smtp := &smtpintegration.Service{
				Pool:      pool,
				Transport: transport,
				Origin:    "https://hack.example.test",
				Campaigns: composer,
				Now:       func() time.Time { return now },
			}
			composer.Channels = map[string]communications.ChannelProvider{"smtp": smtp}
			worker, err := jobs.New(
				pool,
				slog.New(slog.NewTextHandler(io.Discard, nil)),
				map[string]jobs.Handler{
					"communication_campaign_smtp": smtpintegration.CampaignHandler{Service: smtp},
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			composer.Jobs = worker
			rev := func() int64 {
				row, err := q.Event(ctx, id)
				if err != nil {
					t.Fatal(err)
				}
				return row.Revision
			}
			template := communications.Template{
				ID:      uuid.New(),
				Name:    "Synthetic reminder",
				Purpose: "reminder",
				Format:  "text",
				Subject: "PRIVATE-SUBJECT-CANARY",
				Body:    "PRIVATE-MESSAGE-BODY-CANARY for {{person.name}}",
			}
			audience := communications.Audience{
				ID:                 uuid.New(),
				Name:               "Current organisers",
				Roles:              []string{"organiser"},
				RegistrationStates: []string{},
				Teams:              []uuid.UUID{},
			}
			for _, input := range []communications.Command{{Action: "save_template", Template: &template, Reason: "Synthetic native template"}, {Action: "save_audience", Audience: &audience, Reason: "Synthetic native selector"}} {
				if _, err = composer.Change(ctx, owner.ID, id, rev(), input); err != nil {
					t.Fatal(err)
				}
			}
			wall, choice := testdb.WallTimeForInstant(
				t,
				time.Now().Add(-5*time.Minute),
				dates.Timezone,
			)
			definition := communications.CampaignDefinition{
				ID:         uuid.New(),
				Name:       "Synthetic retained campaign",
				TemplateID: template.ID,
				AudienceID: audience.ID,
				Channels:   []string{"smtp"},
				Timing: communications.CampaignTiming{
					Kind:      "fixed",
					WallTime:  wall,
					DSTChoice: choice,
				},
			}
			var campaign communications.Campaign
			for _, action := range []string{"save", "preview", "approve"} {
				input := communications.CampaignCommand{
					Action:  action,
					Version: campaign.Version,
					Reason:  "Synthetic reviewed intent",
				}
				if action == "save" {
					input.Definition = &definition
				}
				detail, err := composer.ChangeCampaign(
					ctx,
					owner.ID,
					id,
					definition.ID,
					rev(),
					input,
				)
				if err != nil {
					t.Fatal(err)
				}
				campaign = detail.Campaign.Campaign
			}
			now = campaign.Approval.DueAt
			if err = database.Within(ctx, pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
				_, err := smtp.QueueApprovedCampaignInTransaction(
					ctx,
					tx,
					id,
					definition.ID,
					campaign.Version,
				)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			var operationID uuid.UUID
			if err = pool.QueryRow(ctx, "SELECT operation_id FROM smtp_campaign_messages WHERE event_id=$1", id).
				Scan(&operationID); err != nil {
				t.Fatal(err)
			}
			cfg, err := config.Load(func(key string) string {
				if key == "HACKBOX_ENV" {
					return "test"
				}
				return ""
			})
			if err != nil {
				t.Fatal(err)
			}
			auth, err := identity.New(ctx, pool, identity.Settings{Origin: cfg.PublicOrigin})
			if err != nil {
				t.Fatal(err)
			}
			var logs bytes.Buffer
			history := &deliveryhistory.Service{
				Pool:      pool,
				Jobs:      worker,
				Providers: map[string]deliveryhistory.RecoveryProvider{"smtp": smtp},
			}
			router := NewRouter(
				cfg,
				slog.New(slog.NewTextHandler(&logs, nil)),
				Dependencies{
					Pool:           pool,
					Auth:           auth,
					Deliveries:     history,
					SMTP:           smtp,
					Communications: *composer,
				},
			)
			tokens := map[uuid.UUID]string{}
			csrf := strings.Repeat("a", 32)
			request := func(person dbgen.Person, method, path string, body any, revision int64, goodCSRF bool, want int) *httptest.ResponseRecorder {
				t.Helper()
				if person.ID != uuid.Nil && tokens[person.ID] == "" {
					session, err := auth.Sessions.Load(ctx, "")
					if err != nil {
						t.Fatal(err)
					}
					auth.Sessions.Put(session, "person_id", person.ID.String())
					auth.Sessions.Put(session, "session_revision", person.SessionRevision)
					auth.Sessions.Put(session, "csrf", csrf)
					token, _, err := auth.Sessions.Commit(session)
					if err != nil {
						t.Fatal(err)
					}
					tokens[person.ID] = token
				}
				var reader io.Reader
				if body != nil {
					encoded, _ := json.Marshal(body)
					reader = bytes.NewReader(encoded)
				}
				r := httptest.NewRequest(method, path, reader)
				if body != nil {
					r.Header.Set("Content-Type", "application/json")
				}
				r.Header.Set("Origin", cfg.PublicOrigin)
				r.Header.Set("If-Match", fmt.Sprintf("\"%d\"", revision))
				token := csrf
				if !goodCSRF {
					token = strings.Repeat("b", 32)
				}
				r.Header.Set("X-CSRF-Token", token)
				if person.ID != uuid.Nil {
					r.AddCookie(&http.Cookie{Name: "hackbox_session", Value: tokens[person.ID]})
				}
				w := httptest.NewRecorder()
				router.ServeHTTP(w, r)
				validateResponse(t, r, w)
				if w.Code != want {
					t.Fatalf("%s %s status%d want%d %s", method, path, w.Code, want, w.Body)
				}
				if w.Header().Get("Cache-Control") != "no-store" {
					t.Fatal("private history cacheable")
				}
				for _, canary := range []string{"PRIVATE-DESTINATION@example.test", "PRIVATE-SUBJECT-CANARY", "PRIVATE-MESSAGE-BODY-CANARY"} {
					if strings.Contains(w.Body.String(), canary) {
						t.Fatal("private content leaked to history/error")
					}
				}
				return w
			}
			base := "/api/v1/events/" + id.String() + "/communications/deliveries"
			detailPath := base + "/" + operationID.String()
			request(owner, "GET", base, nil, 1, true, 200)
			request(other, "GET", base, nil, 1, true, 403)
			request(admin, "GET", detailPath, nil, 1, true, 403)
			request(dbgen.Person{}, "GET", base, nil, 1, true, 401)
			if err = worker.Client.Start(ctx); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				stop, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = worker.Client.StopAndCancel(stop)
			})
			wantState := "delivered"
			if prior == maildelivery.Ambiguous {
				wantState = "ambiguous"
			}
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				op, err := q.Operation(ctx, operationID)
				if err != nil {
					t.Fatal(err)
				}
				if op.State == wantState {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			original, err := q.Operation(ctx, operationID)
			if err != nil || original.State != wantState || transport.count() != 1 {
				t.Fatalf("original actual worker=%+v %v", original, err)
			}
			var oldToken uuid.UUID
			if err = pool.QueryRow(ctx, "SELECT lease_token FROM delivery_attempts WHERE operation_id=$1 ORDER BY started_at LIMIT 1", operationID).
				Scan(&oldToken); err != nil {
				t.Fatal(err)
			}
			input := deliveryhistory.Command{
				RequestID: uuid.New(),
				Action:    "resend_acknowledged",
				Reason:    "Synthetic explicit duplicate risk review",
			}
			request(owner, "POST", detailPath, input, original.Revision, false, 403)
			bad := input
			bad.Action = "retry"
			request(owner, "POST", detailPath, bad, original.Revision, true, 409)
			transport.mu.Lock()
			transport.state = maildelivery.Accepted
			transport.entered = make(chan struct{}, 1)
			transport.release = make(chan struct{})
			entered, release := transport.entered, transport.release
			transport.mu.Unlock()
			request(owner, "POST", detailPath, input, original.Revision, true, 200)
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("reviewed River identity did not reach provider")
			}
			changed, err := q.FinishOperation(
				ctx,
				dbgen.FinishOperationParams{
					ID:         operationID,
					LeaseToken: &oldToken,
					State:      "permanent",
				},
			)
			if err != nil || changed != 0 {
				t.Fatal("old callback overwrote reviewed generation", err)
			}
			current, err := q.Operation(ctx, operationID)
			if err != nil || current.State != "running" || current.LeaseToken == nil ||
				*current.LeaseToken == oldToken {
				t.Fatal("new reviewed lease lost", err)
			}
			request(owner, "POST", detailPath, input, original.Revision, true, 200)
			changedInput := input
			changedInput.Reason = "Different decision"
			request(owner, "POST", detailPath, changedInput, original.Revision, true, 409)
			if transport.count() != 2 {
				t.Fatal("duplicate decision spent another grant")
			}
			close(release)
			deadline = time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				op, err := q.Operation(ctx, operationID)
				if err != nil {
					t.Fatal(err)
				}
				if op.State == "delivered" {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			w := request(owner, "GET", detailPath, nil, 1, true, 200)
			var detail deliveryhistory.Detail
			if err = json.Unmarshal(
				w.Body.Bytes(),
				&detail,
			); err != nil || len(detail.Attempts) != 2 || len(detail.Reviews) != 1 ||
				detail.Item.State != "provider_accepted" {
				t.Fatalf("native history=%+v err%v", detail, err)
			}
			if err = worker.Reconcile(ctx); err != nil {
				t.Fatal(err)
			}
			if transport.count() != 2 {
				t.Fatal("reconciliation replayed reviewed effect")
			}
			if _, err = pool.Exec(
				ctx,
				"UPDATE events SET writes_allowed=false WHERE id=$1",
				id,
			); err != nil {
				t.Fatal(err)
			}
			archivedResponse := request(owner, "GET", detailPath, nil, 1, true, 200)
			var archived deliveryhistory.Detail
			if err = json.Unmarshal(
				archivedResponse.Body.Bytes(),
				&archived,
			); err != nil || !archived.Archived ||
				len(archived.Actions) != 0 {
				t.Fatalf("archive retained recovery capability %+v err%v", archived, err)
			}
			for _, action := range []string{"retry", "cancel", "cancel_resend", "resolve_no_resend", "resend_acknowledged"} {
				frozen := deliveryhistory.Command{
					RequestID: uuid.New(),
					Action:    action,
					Reason:    "Synthetic frozen decision must not be recorded",
				}
				request(owner, "POST", detailPath, frozen, archived.Item.Revision, true, 403)
			}
			request(owner, "POST", detailPath, input, original.Revision, true, 403)
			var decisions int
			if err = pool.QueryRow(ctx, "SELECT count(*) FROM communication_delivery_reviews WHERE event_id=$1", id).
				Scan(&decisions); err != nil ||
				decisions != 1 ||
				transport.count() != 2 {
				t.Fatal("archive changed delivery/audit decision state", err)
			}
			if _, err = pool.Exec(
				ctx,
				"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2",
				id,
				owner.ID,
			); err != nil {
				t.Fatal(err)
			}
			request(owner, "GET", base, nil, 1, true, 403)
			request(owner, "GET", detailPath, nil, 1, true, 403)
			for _, canary := range []string{"PRIVATE-DESTINATION@example.test", "PRIVATE-SUBJECT-CANARY", "PRIVATE-MESSAGE-BODY-CANARY"} {
				if strings.Contains(logs.String(), canary) {
					t.Fatal("provider payload entered generic logs")
				}
			}
		})
	}
}
