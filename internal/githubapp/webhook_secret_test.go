package githubapp

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

func TestWebhookSecretPurposeRotationReplacementAndDisconnect(t *testing.T) {
	f := setupApp(t)
	ctx := t.Context()
	input := f.configuration()
	input.WebhookSecret = "SYNTHETIC-WEBHOOK-SECRET-PRIVATE-CANARY"
	if err := f.s.Configure(ctx, f.owner.ID, f.event, f.revision(), input); err != nil {
		t.Fatal(err)
	}
	q := dbgen.New(f.pool)
	row, err := q.GitHubWebhookSecret(ctx, f.event)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(row.Encrypted, []byte(input.WebhookSecret)) || row.Version <= 0 {
		t.Fatal("plaintext secret persisted")
	}
	initialVersion := row.Version
	plain, err := f.s.Keys.Open(
		row.KeyID,
		row.Encrypted,
		WebhookSecretBinding(f.event, 42, row.Version),
	)
	if err != nil || string(plain) != input.WebhookSecret {
		t.Fatal("secret did not roundtrip")
	}
	clear(plain)
	if _, err = f.s.Keys.Open(
		row.KeyID,
		row.Encrypted,
		[]byte("hackbox:github-app-private-key:v1"),
	); err == nil {
		t.Fatal("secret accepted under App signing key purpose")
	}
	status, err := f.s.Status(ctx, f.owner.ID, f.event)
	if err != nil || !status.WebhookConfigured {
		t.Fatalf("missing safe configuration status: %+v %v", status, err)
	}
	data, _ := json.Marshal(status)
	if strings.Contains(string(data), input.WebhookSecret) {
		t.Fatal("secret returned in status")
	}
	if err = f.s.Configure(
		ctx,
		f.other.ID,
		f.event,
		f.revision(),
		input,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatalf("unrelated actor configured secret: %v", err)
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
	row, err = q.GitHubWebhookSecret(ctx, f.event)
	if err != nil || row.KeyID != "new" || row.Version != initialVersion+1 {
		t.Fatalf("secret not rewrapped: %+v %v", row, err)
	}
	plain, err = f.s.Keys.Open(
		row.KeyID,
		row.Encrypted,
		WebhookSecretBinding(f.event, 42, row.Version),
	)
	if err != nil || string(plain) != input.WebhookSecret {
		t.Fatal("rotation changed shared secret")
	}
	clear(plain)
	input.WebhookSecret = "SYNTHETIC-REPLACEMENT-SECRET-PRIVATE-CANARY"
	if err = f.s.Configure(ctx, f.owner.ID, f.event, f.revision(), input); err != nil {
		t.Fatal(err)
	}
	row, err = q.GitHubWebhookSecret(ctx, f.event)
	if err != nil || row.Version != initialVersion+2 {
		t.Fatal("replacement did not invalidate old secret version")
	}
	input.WebhookSecret = "too-short"
	before := f.revision()
	if err = f.s.Configure(
		ctx,
		f.owner.ID,
		f.event,
		before,
		input,
	); !errors.Is(
		err,
		access.ErrInvalid,
	) {
		t.Fatalf("short secret: %v", err)
	}
	if before != f.revision() {
		t.Fatal("invalid secret partially mutated configuration")
	}
	input = f.configuration()
	input.AppID = 43
	if err = f.s.Configure(ctx, f.owner.ID, f.event, f.revision(), input); err != nil {
		t.Fatal(err)
	}
	if _, err = q.GitHubWebhookSecret(ctx, f.event); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("new App retained another App webhook secret")
	}
	input.WebhookSecret = "SYNTHETIC-WEBHOOK-SECRET-PRIVATE-CANARY"
	if err = f.s.Configure(ctx, f.owner.ID, f.event, f.revision(), input); err != nil {
		t.Fatal(err)
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
	if _, err = q.GitHubWebhookSecret(ctx, f.event); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("disconnect retained secret")
	}
}
