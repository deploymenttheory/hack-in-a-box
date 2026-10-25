package credentials

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"filippo.io/age"
	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/files"
	recoveryops "github.com/deploymenttheory/hack-in-a-box/internal/recovery"
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

// This rehearses the production encrypted bundle against an independent fresh
// PostgreSQL server, not another database on the source server.
func TestFreshEncryptedRecoveryPreservesNativeCredentialAndAttachment(t *testing.T) {
	source := testdb.NewRecovery(t)
	f := setupWithPool(t, source.Pool)
	ctx := t.Context()
	const value = "SYNTHETIC-ENCRYPTED-RESTORE-124"
	page := f.change(
		Command{
			Action:       "create",
			Label:        "Synthetic recovery credential",
			Value:        value,
			TeamIDs:      []uuid.UUID{f.team},
			ResourceType: "licence",
		},
	)
	id := page.Items[0].ID
	sourceFiles := t.TempDir()
	storage, err := files.OpenLocal(sourceFiles)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = storage.Close() }()
	content := []byte("Synthetic recovery attachment 124\n")
	blob, err := storage.Put(ctx, bytes.NewReader(content), 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = source.Pool.Exec(
		ctx,
		`INSERT INTO files(id,event_id,owner_id,idempotency_key,object_key,filename,media_type,checksum,size_bytes) VALUES($1,$2,$3,$1,$4,'synthetic.txt','text/plain',$5,$6)`,
		uuid.New(),
		f.event,
		f.owner.ID,
		blob.Key,
		blob.Checksum,
		blob.Size,
	); err != nil {
		t.Fatal(err)
	}
	verifyHistories := seedRecoveryHistories(t, f)
	archiveIdentity, err := age.GenerateHybridIdentity()
	if err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(t.TempDir(), "backup")
	backup, err := recoveryops.Capture(
		ctx,
		source.Pool,
		sourceFiles,
		bundle,
		[]age.Recipient{archiveIdentity.Recipient()},
		source.DumpStream,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"manifest.age", "database.age", "files.age"} {
		data, e := os.ReadFile(filepath.Join(bundle, name))
		if e != nil {
			t.Fatal(e)
		}
		if bytes.Contains(data, []byte(value)) || bytes.Contains(data, content) {
			t.Fatal("plaintext backup artifact")
		}
	}
	f.change(
		Command{
			Action: "replace",
			ID:     id,
			Label:  "Source after snapshot",
			Value:  "SYNTHETIC-AFTER-SNAPSHOT-124",
		},
	)
	target := testdb.NewRecoveryTarget(t)
	targetFiles := filepath.Join(t.TempDir(), "restored-files")
	wrong, err := age.GenerateHybridIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = recoveryops.Restore(
		ctx,
		target.Pool,
		bundle,
		targetFiles,
		[]age.Identity{wrong},
		target.RestoreStream,
	); err == nil {
		t.Fatal("wrong archive key accepted")
	}
	if _, err = os.Stat(targetFiles); !os.IsNotExist(err) {
		t.Fatal("wrong key exposed restored files")
	}
	var present bool
	if err = target.Pool.QueryRow(ctx, "SELECT to_regclass('public.events') IS NOT NULL").
		Scan(&present); err != nil ||
		present {
		t.Fatal("wrong key exposed database restore")
	}
	restored, err := recoveryops.Restore(
		ctx,
		target.Pool,
		bundle,
		targetFiles,
		[]age.Identity{archiveIdentity},
		target.RestoreStream,
	)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Hold.HoldID == nil || restored.Manifest.BackupID != backup.BackupID {
		t.Fatal("restored environment not held/bound to snapshot")
	}
	verifyHistories(target.Pool)
	actual, err := os.ReadFile(filepath.Join(targetFiles, blob.Key))
	if err != nil || !bytes.Equal(actual, content) {
		t.Fatal("restored attachment mismatch")
	}
	recovered := *f
	recovered.pool = target.Pool
	recovered.s = Service{Pool: target.Pool}
	if result, e := recovered.reveal(
		id,
	); !errors.Is(e, secretbox.ErrUnavailable) ||
		result.Value != "" {
		t.Fatal("credential disclosed without separate key")
	}
	safe, e := recovered.s.List(ctx, recovered.member.ID, recovered.event)
	if e != nil || len(safe.Items) != 1 {
		t.Fatal("safe native metadata unavailable without key")
	}
	recovered.s.Keys = keyring(t, "old", "other", "old")
	if result, e := recovered.reveal(
		id,
	); !errors.Is(e, secretbox.ErrUnavailable) ||
		result.Value != "" {
		t.Fatal("wrong material under original key ID decrypted old backup")
	}
	recovered.s.Keys = keyring(t, "new", "new")
	if result, e := recovered.reveal(
		id,
	); !errors.Is(e, secretbox.ErrUnavailable) ||
		result.Value != "" {
		t.Fatal("new-only key decrypted old backup")
	}
	recovered.s.Keys = keyring(t, "new", "old", "new")
	result, e := recovered.reveal(id)
	if e != nil || result.Value != value {
		t.Fatalf("retained exact key did not restore native access: %v", e)
	}
	if _, err = recoveryops.Restore(
		ctx,
		target.Pool,
		bundle,
		filepath.Join(t.TempDir(), "again"),
		[]age.Identity{archiveIdentity},
		target.RestoreStream,
	); err == nil {
		t.Fatal("restore overwrote existing installation")
	}
	t.Logf(
		"synthetic backup_id=%s files=%d bytes=%d capture=%s fresh_restore=%s snapshot_completed=%s; source subsequent replacement absent from restore",
		backup.BackupID,
		len(backup.Blobs),
		blob.Size,
		backup.CompletedAt.Sub(backup.StartedAt),
		restored.CompletedAt.Sub(restored.StartedAt),
		backup.CompletedAt.Format(time.RFC3339Nano),
	)
}
