package credentials

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/secretbox"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestCredentialLogicalRestoreRequiresExternalRetainedKeys(t *testing.T) {
	recovery := testdb.NewRecovery(t)
	source := setupWithPool(t, recovery.Pool)
	const value = "SYNTHETIC-RECOVERY-CREDENTIAL-76"
	now := time.Now().UTC().Truncate(time.Microsecond)
	available, expires := now.Add(-time.Hour), now.Add(time.Hour)
	page := source.change(
		Command{
			Action:       "create",
			Label:        "Synthetic restored service",
			Value:        value,
			TeamIDs:      []uuid.UUID{source.team},
			ResourceType: "licence", AvailableAt: &available, ExpiresAt: &expires,
		},
	)
	id := page.Items[0].ID
	ctx := t.Context()
	original, err := dbgen.New(source.pool).
		TeamCredential(ctx, dbgen.TeamCredentialParams{EventID: source.event, ID: id})
	if err != nil {
		t.Fatal(err)
	}
	restored, dump := recovery.Restore(t)
	for _, forbidden := range []string{value, base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32)), base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32))} {
		if bytes.Contains(dump, []byte(forbidden)) {
			t.Fatal("logical backup contains a credential value or deployment key")
		}
	}
	if !bytes.Contains(dump, []byte(hex.EncodeToString(original.Ciphertext))) {
		t.Fatal("backup evidence does not contain encrypted resource")
	}
	if err = database.CheckSchema(ctx, restored); err != nil {
		t.Fatal(err)
	}
	// Prove the restored installation is independent of subsequent source writes.
	source.change(
		Command{
			Action: "replace",
			ID:     id,
			Label:  "Changed after backup",
			Value:  "SYNTHETIC-SOURCE-AFTER-BACKUP",
		},
	)
	f := *source
	f.pool = restored
	f.s = Service{Pool: restored}
	row, err := dbgen.New(restored).
		TeamCredential(ctx, dbgen.TeamCredentialParams{EventID: f.event, ID: id})
	if err != nil || !bytes.Equal(row.Ciphertext, original.Ciphertext) ||
		row.Version != original.Version ||
		row.KeyID != original.KeyID || row.ResourceType != "licence" || !row.AvailableAt.Valid || !row.AvailableAt.Time.Equal(available) || !row.ExpiresAt.Valid || !row.ExpiresAt.Time.Equal(expires) {
		t.Fatal("restore did not preserve authenticated envelope")
	}
	revision := f.revision()
	assertUnavailable := func() {
		t.Helper()
		result, err := f.reveal(id)
		if !errors.Is(err, secretbox.ErrUnavailable) || result.Value != "" {
			t.Fatal("restored ciphertext disclosed without its exact external key")
		}
		if f.revision() != revision {
			t.Fatal("failed disclosure changed event revision")
		}
		safe, err := f.s.List(ctx, f.member.ID, f.event)
		encoded, _ := json.Marshal(safe)
		if err != nil || len(safe.Items) != 1 || strings.Contains(string(encoded), value) ||
			strings.Contains(string(encoded), "ciphertext") ||
			strings.Contains(string(encoded), "key_id") {
			t.Fatal("missing key prevented safe metadata or leaked envelope")
		}
	}
	assertUnavailable()
	// The identifier alone is insufficient: different material under the old ID
	// must authenticate unsuccessfully rather than silently invent a new secret.
	wrong, _ := json.Marshal(
		map[string]any{
			"active": "old",
			"keys": map[string]string{
				"old": base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{9}, 32)),
			},
		},
	)
	f.s.Keys, err = secretbox.Parse(string(wrong))
	if err != nil {
		t.Fatal(err)
	}
	assertUnavailable()
	// A new active deployment key cannot decrypt the old backup by itself.
	f.s.Keys = keyring(t, "new", "new")
	assertUnavailable()
	f.s.Keys = keyring(t, "new", "old", "new")
	result, err := f.reveal(id)
	if err != nil || result.Value != value {
		t.Fatal("retained external key did not recover restored resource")
	}
	f.change(Command{Action: "rotate", ID: id})
	newOnly, _ := json.Marshal(
		map[string]any{
			"active": "new",
			"keys": map[string]string{
				"new": base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32)),
			},
		},
	)
	f.s.Keys, err = secretbox.Parse(string(newOnly))
	if err != nil {
		t.Fatal(err)
	}
	result, err = f.reveal(id)
	if err != nil || result.Value != value || result.Version != 2 {
		t.Fatal("restored resource rotation did not permit safe old-key retirement")
	}
	var unavailable int
	var audit string
	if err = restored.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE action='credential.revealed' AND actor_id=$1 AND target_id=$2 AND metadata->>'result'='unavailable'", f.member.ID, id).
		Scan(&unavailable); err != nil ||
		unavailable != 3 {
		t.Fatal("restored key failures lack durable audit")
	}
	if err = restored.QueryRow(ctx, "SELECT string_agg(row_to_json(a)::text,' ') FROM audit_events a").
		Scan(&audit); err != nil ||
		strings.Contains(audit, value) ||
		strings.Contains(audit, "SYNTHETIC-SOURCE-AFTER-BACKUP") {
		t.Fatal("restored audit exposed credential")
	}
	if result, err = source.reveal(
		id,
	); err != nil ||
		result.Value != "SYNTHETIC-SOURCE-AFTER-BACKUP" {
		t.Fatal("recovery changed source installation")
	}
}
