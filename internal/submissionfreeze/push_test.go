package submissionfreeze

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/githubapp"
	"github.com/deploymenttheory/hack-in-a-box/internal/githubwebhooks"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/secretbox"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestSignedDelayedOutOfOrderEvidenceIsExactPagedAndNeverAutomaticProof(t *testing.T) {
	pool := testdb.New(t, true)
	f := newFixture(t, pool)
	f.submit("repository")
	f.now = f.cfg.SubmissionCutoff.Add(time.Hour)
	f.capture()
	p := f.read(f.owner)
	keys, err := secretbox.Parse(
		`{"active":"test","keys":{"test":"` + base64.StdEncoding.EncodeToString(
			bytes.Repeat([]byte{7}, 32),
		) + `"}}`,
	)
	if err != nil {
		t.Fatal(err)
	}
	secret := []byte("SYNTHETIC-ONLY-WEBHOOK-SECRET-0000000000")
	keyID, encrypted, err := keys.Seal(secret, githubapp.WebhookSecretBinding(f.event, 42, 1))
	if err != nil {
		t.Fatal(err)
	}
	if err = f.q.SaveGitHubWebhookSecret(
		t.Context(),
		dbgen.SaveGitHubWebhookSecretParams{
			EventID:   f.event,
			AppID:     42,
			Version:   1,
			KeyID:     keyID,
			Encrypted: encrypted,
		},
	); err != nil {
		t.Fatal(err)
	}
	webhooks := &githubwebhooks.Service{Pool: pool, Keys: keys}
	queue, err := jobs.New(
		pool,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		map[string]jobs.Handler{githubwebhooks.HandlerKind: webhooks},
	)
	if err != nil {
		t.Fatal(err)
	}
	webhooks.Jobs = queue
	oldID := uuid.New()
	for i := range 105 {
		id := uuid.New()
		if i == 0 {
			id = oldID
		}
		at := f.cfg.SubmissionCutoff.Add(-time.Minute)
		if i == 103 {
			at = f.cfg.SubmissionCutoff.Add(time.Minute)
		}
		if i == 104 {
			at = f.cfg.SubmissionCutoff.Add(-5 * time.Minute)
		}
		raw, _ := json.Marshal(
			map[string]any{
				"installation": map[string]any{"id": 55},
				"repository": map[string]any{
					"id":        f.repository,
					"owner":     map[string]any{"id": 99},
					"pushed_at": at.Unix(),
				},
				"ref":    "refs/heads/main",
				"after":  strings.Repeat("a", 40),
				"before": strings.Repeat("b", 40),
				"forced": true,
				"commits": []any{
					map[string]any{
						"timestamp": "2000-01-01T00:00:00Z",
						"message":   "PRIVATE-COMMIT-PROSE",
					},
				},
			},
		)
		mac := hmac.New(sha256.New, secret)
		_, _ = mac.Write(raw)
		h := githubwebhooks.Headers{
			DeliveryID: id.String(),
			Event:      "push",
			Signature:  "sha256=" + hex.EncodeToString(mac.Sum(nil)),
		}
		if err = webhooks.Receive(t.Context(), h, raw); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			if err = webhooks.Receive(t.Context(), h, raw); err != nil {
				t.Fatal("signed duplicate", err)
			}
		}
	}
	retained, err := f.q.ListGitHubWebhookDeliveries(t.Context(), f.event)
	if err != nil || len(retained) != 100 {
		t.Fatal("staff status bound changed", len(retained), err)
	}
	all := []Push{}
	cursor := uuid.Nil
	for {
		page, err := f.service.PushEvidence(t.Context(), f.owner, f.event, f.team, cursor, 60)
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, page.Items...)
		if page.NextCursor == nil {
			break
		}
		cursor = *page.NextCursor
	}
	if len(all) != 105 {
		t.Fatal("bounded status used as complete evidence history", len(all))
	}
	found := false
	for _, push := range all {
		if push.ID == oldID {
			found = true
		}
		if !push.ReceivedAt.After(p.Source.Cutoff) || push.ProviderAt == nil {
			t.Fatal("lost separate observation/provider times", push)
		}
	}
	if !found {
		t.Fatal("exact older retained evidence lost")
	}
	p = f.read(f.owner)
	if p.Status != "unresolved" || p.Current != nil {
		t.Fatal("delayed/out-of-order webhook fabricated cutoff proof", p)
	}
	c := f.decision(p, "reconcile")
	c.PushIDs = []uuid.UUID{oldID}
	c.Reason = "Reviewed retained push and independent cutoff evidence; explicit organiser decision"
	if _, err = f.service.Change(t.Context(), f.owner, f.event, f.team, c); err != nil {
		t.Fatal(err)
	}
	p = f.read(f.owner)
	if p.Current == nil || p.Current.PushIDs[0] != oldID || len(p.Observations) != 1 {
		t.Fatal(p)
	}
}
