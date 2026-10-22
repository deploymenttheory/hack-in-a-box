package httpapi

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/identity"
	"github.com/deploymenttheory/hack-in-a-box/internal/smtpintegration"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestSMTPReceiptHTTPIsCurrentRequesterOnlyAndNeverRevealsPayload(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	owner, other, outsider := testdb.Person(t, pool), testdb.Person(t, pool), testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, owner.ID)
	campaignID, testID := uuid.New(), uuid.New()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(
		"INSERT INTO event_memberships(event_id,person_id,role) VALUES($1,$2,'organiser')",
		eventID,
		other.ID,
	)
	// Narrow receipt fixtures retain valid scope/ownership references. The actual
	// campaign resolver/provider/outbox path is exercised by SMTP integration tests.
	exec(
		"INSERT INTO communication_campaign_versions(event_id,id,version,document,actor_id,reason) VALUES($1,$2,1,'{}',$3,'Synthetic receipt fixture')",
		eventID,
		campaignID,
		owner.ID,
	)
	exec(
		"INSERT INTO communication_test_requests(event_id,id,campaign_id,campaign_version,actor_id,channel,snapshot,fingerprint,reason) VALUES($1,$2,$3,1,$4,'smtp',$5,'synthetic','Synthetic receipt fixture')",
		eventID,
		testID,
		campaignID,
		owner.ID,
		[]byte(`{"subject":"SYNTHETIC-PRIVATE-SUBJECT","body":"SYNTHETIC-PRIVATE-BODY"}`),
	)
	q := dbgen.New(pool)
	operation, err := q.InsertOperation(
		ctx,
		dbgen.InsertOperationParams{
			EventID:      eventID,
			TargetID:     testID,
			Kind:         "communication_test_smtp",
			OperationKey: "communication_test:" + testID.String() + ":smtp",
			Approved:     true,
			DueAt:        time.Now(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	err = q.SaveSMTPMessageSnapshot(
		ctx,
		dbgen.SaveSMTPMessageSnapshotParams{
			OperationID:   operation.ID,
			EventID:       eventID,
			PersonID:      owner.ID,
			TestRequestID: testID,
			Mailbox:       "private-organiser@example.test",
			MessageID:     "<synthetic-private-message@example.test>",
			Snapshot: []byte(
				`{"recipient":"private-organiser@example.test","subject":"SYNTHETIC-PRIVATE-SUBJECT","body":"SYNTHETIC-PRIVATE-BODY"}`,
			),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err = q.RecordSMTPReceipt(
		ctx,
		dbgen.RecordSMTPReceiptParams{
			OperationID: operation.ID,
			State:       "accepted",
			ReceiptCode: "relay_accepted",
		},
	); err != nil {
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
	router := NewRouter(
		cfg,
		slog.New(slog.NewTextHandler(&logs, nil)),
		Dependencies{Pool: pool, Auth: auth, SMTP: &smtpintegration.Service{Pool: pool}},
	)
	tokens := map[uuid.UUID]string{}
	request := func(person dbgen.Person, event uuid.UUID, want int) *httptest.ResponseRecorder {
		t.Helper()
		path := "/api/v1/events/" + event.String() + "/communications/tests/" + testID.String() + "/delivery"
		r := httptest.NewRequest("GET", path, nil)
		if person.ID != uuid.Nil {
			if tokens[person.ID] == "" {
				session, err := auth.Sessions.Load(ctx, "")
				if err != nil {
					t.Fatal(err)
				}
				auth.Sessions.Put(session, "person_id", person.ID.String())
				auth.Sessions.Put(session, "session_revision", person.SessionRevision)
				token, _, err := auth.Sessions.Commit(session)
				if err != nil {
					t.Fatal(err)
				}
				tokens[person.ID] = token
			}
			r.AddCookie(&http.Cookie{Name: "hackbox_session", Value: tokens[person.ID]})
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, r)
		validateResponse(t, r, response)
		if response.Code != want {
			t.Fatalf("receipt status%d want%d", response.Code, want)
		}
		if !strings.Contains(response.Header().Get("Cache-Control"), "no-store") {
			t.Fatal("receipt response cacheable")
		}
		for _, private := range []string{"private-organiser@example.test", "SYNTHETIC-PRIVATE", "synthetic-private-message", "snapshot", "mailbox"} {
			if strings.Contains(response.Body.String(), private) {
				t.Fatal("bounded receipt exposed private envelope")
			}
		}
		return response
	}
	request(dbgen.Person{}, eventID, 401)
	request(outsider, eventID, 403)
	request(other, eventID, 403)
	response := request(owner, eventID, 200)
	var status smtpintegration.DeliveryStatus
	if err = json.Unmarshal(
		response.Body.Bytes(),
		&status,
	); err != nil || status.State != "accepted" ||
		status.Reason != "relay_accepted" {
		t.Fatal("accepted transport receipt not represented honestly")
	}
	otherEvent := testdb.Event(t, pool, owner.ID)
	request(owner, otherEvent, 404)
	// Historic receipts remain readable by the current requester on a read-only
	// event, but loss of organiser authority removes that private view immediately.
	exec("UPDATE events SET writes_allowed=false WHERE id=$1", eventID)
	request(owner, eventID, 200)
	exec(
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='organiser'",
		eventID,
		owner.ID,
	)
	request(owner, eventID, 403)
	exec("UPDATE people SET active=false WHERE id=$1", owner.ID)
	request(owner, eventID, 401)
	if strings.Contains(logs.String(), "private-organiser@example.test") ||
		strings.Contains(logs.String(), "SYNTHETIC-PRIVATE") {
		t.Fatal("receipt endpoint logged private envelope")
	}
}
