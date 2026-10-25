package submissionfreeze

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/githubapp"
	"github.com/deploymenttheory/hack-in-a-box/internal/githubwebhooks"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/secretbox"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
	"github.com/google/uuid"
)

func TestRecoverySignedActivityCannotBecomeSubmissionOrRetargetCompletedReceipt(t *testing.T) {
	pool := testdb.New(t, true)
	f := newFixture(t, pool)
	other := newFixture(t, pool)
	// Select initial scenario time before any domain command. All following clock
	// changes move forward; no settled final or publication fact is rewound.
	f.now = f.cfg.LiveStart.Add(-time.Hour)
	keys, err := secretbox.Parse(`{"active":"test","keys":{"test":"` + base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32)) + `"}}`)
	if err != nil {
		t.Fatal(err)
	}
	secret := []byte("SYNTHETIC-RECOVERY-WEBHOOK-SECRET-00000000")
	// One App can span events; the same App secret must still not authorize a
	// delivery for another event's immutable numeric repository mapping.
	for _, target := range []*fixture{f, other} {
		keyID, cipher, sealErr := keys.Seal(secret, githubapp.WebhookSecretBinding(target.event, 42, 1))
		if sealErr != nil {
			t.Fatal(sealErr)
		}
		if err = target.q.SaveGitHubWebhookSecret(t.Context(), dbgen.SaveGitHubWebhookSecretParams{EventID: target.event, AppID: 42, Version: 1, KeyID: keyID, Encrypted: cipher}); err != nil {
			t.Fatal(err)
		}
	}
	webhooks := &githubwebhooks.Service{Pool: pool, Keys: keys}
	queue, err := jobs.New(pool, slog.New(slog.NewTextHandler(io.Discard, nil)), map[string]jobs.Handler{githubwebhooks.HandlerKind: webhooks, HandlerKind: f.service})
	if err != nil {
		t.Fatal(err)
	}
	webhooks.Jobs = queue
	f.jobs = queue
	f.service.Jobs = queue
	native := submissions.Service{Pool: pool, Clock: func() time.Time { return f.now }}
	evidence := []submissions.Evidence{{DeliverableID: f.deliverable, Kind: "repository", Label: "Synthetic early build", URL: "https://github.com/synthetic-org/freeze-" + f.team.String()}}
	draft, err := native.SaveDraft(t.Context(), f.member, f.event, f.team, 0, evidence)
	if err != nil || draft.Revision != 1 {
		t.Fatal("early working draft unavailable", draft, err)
	}
	if _, err = native.Submit(t.Context(), f.member, f.event, f.team, 0, submissions.Command{Evidence: evidence}); !errors.Is(err, event.ErrBlocked) {
		t.Fatal("early formal output accepted", err)
	}
	if _, err = native.Review(t.Context(), f.owner, f.event, f.team, 0, 1, "accepted", "Synthetic premature review"); !errors.Is(err, event.ErrBlocked) {
		t.Fatal("early review accepted", err)
	}
	payload := func(sha string) []byte {
		raw, marshalErr := json.Marshal(map[string]any{"installation": map[string]any{"id": 55}, "repository": map[string]any{"id": f.repository, "owner": map[string]any{"id": 99}, "pushed_at": f.now.Unix()}, "ref": "refs/heads/main", "after": sha, "commits": []any{map[string]any{"message": "PRIVATE-RECOVERY-COMMIT-NOTE"}}})
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		return raw
	}
	headers := func(id uuid.UUID, raw []byte) githubwebhooks.Headers {
		mac := hmac.New(sha256.New, secret)
		_, _ = mac.Write(raw)
		return githubwebhooks.Headers{DeliveryID: id.String(), Event: "push", Signature: "sha256=" + hex.EncodeToString(mac.Sum(nil))}
	}
	formalSnapshot := func() string {
		var value string
		if e := pool.QueryRow(t.Context(), "SELECT jsonb_build_object('states',(SELECT coalesce(jsonb_agg(to_jsonb(s) ORDER BY event_id,team_id),'[]') FROM submission_states s),'revisions',(SELECT coalesce(jsonb_agg(to_jsonb(r) ORDER BY event_id,team_id,revision),'[]') FROM submission_revisions r),'changes',(SELECT count(*) FROM submission_changes),'decisions',(SELECT count(*) FROM submission_freeze_decisions))::text").Scan(&value); e != nil {
			t.Fatal(e)
		}
		return value
	}
	before := formalSnapshot()
	early := payload(strings.Repeat("a", 40))
	id := uuid.New()
	h := headers(id, early)
	if err = webhooks.Receive(t.Context(), h, early); err != nil {
		t.Fatal(err)
	}
	var delivery dbgen.GithubWebhookDelivery
	delivery, err = f.q.GitHubWebhookDelivery(t.Context(), id)
	if err != nil || delivery.OperationID == nil {
		t.Fatal(delivery, err)
	}
	if err = queue.Execute(t.Context(), *delivery.OperationID); err != nil {
		t.Fatal(err)
	}
	if formalSnapshot() != before {
		t.Fatal("valid pre-live operational push mutated formal/assessment evidence")
	}
	if err = webhooks.Ingest(t.Context(), other.event, headers(uuid.New(), early), early); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("signed cross-event repository delivery accepted", err)
	}
	tampered := bytes.Replace(early, []byte(strings.Repeat("a", 40)), []byte(strings.Repeat("b", 40)), 1)
	if err = webhooks.Receive(t.Context(), h, tampered); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("forged body reused authentic signature", err)
	}
	if formalSnapshot() != before {
		t.Fatal("rejected deliveries changed either team's formal state")
	}
	f.now = f.cfg.LiveStart
	f.submit("repository")
	f.now = f.cfg.SubmissionCutoff
	f.capture()
	page := f.read(f.owner)
	if page.NativeOutcome != "frozen" || page.Status != "unresolved" || page.Current != nil {
		t.Fatal("cutoff depended on lifecycle advancement or push auto-verified", page)
	}
	storedEvent, err := f.q.Event(t.Context(), f.event)
	if err != nil || storedEvent.Stage != "preparation" {
		t.Fatal("fixture silently advanced lifecycle", storedEvent.Stage, err)
	}
	command := f.decision(page, "reconcile")
	command.PushIDs = []uuid.UUID{id}
	receipt, err := f.service.Change(t.Context(), f.owner, f.event, f.team, command)
	if err != nil {
		t.Fatal(err)
	}
	original, _ := json.Marshal(receipt)
	formal := formalSnapshot()
	f.now = f.now.Add(time.Hour)
	late := payload(strings.Repeat("b", 40))
	lateID := uuid.New()
	lateHeaders := headers(lateID, late)
	for range 2 {
		if err = webhooks.Receive(t.Context(), lateHeaders, late); err != nil {
			t.Fatal(err)
		}
	}
	delivery, err = f.q.GitHubWebhookDelivery(t.Context(), lateID)
	if err != nil || delivery.OperationID == nil {
		t.Fatal(delivery, err)
	}
	for range 2 {
		if err = queue.Execute(t.Context(), *delivery.OperationID); err != nil {
			t.Fatal(err)
		}
	}
	saved, err := f.service.OriginalReceipt(t.Context(), f.owner, f.event, f.team, command.RequestID)
	if err != nil || saved.Receipt == nil {
		t.Fatal(saved, err)
	}
	after, _ := json.Marshal(saved.Receipt)
	if !bytes.Equal(original, after) || formalSnapshot() != formal {
		t.Fatal("later signed push or replay mutated completed receipt/formal records")
	}
	var retained int
	if err = pool.QueryRow(t.Context(), "SELECT count(*) FROM github_webhook_deliveries WHERE event_id=$1", f.event).Scan(&retained); err != nil || retained != 2 {
		t.Fatal("delivery GUID replay duplicated activity", retained, err)
	}
}
