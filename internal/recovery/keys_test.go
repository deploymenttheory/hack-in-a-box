package recovery_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"filippo.io/age"

	"github.com/deploymenttheory/hack-in-a-box/internal/recovery"
)

func TestArchiveIdentityIsPrivateSeparateAndNeverOverwritten(t *testing.T) {
	root := t.TempDir()
	privatePath, publicPath := filepath.Join(root, "identity"), filepath.Join(root, "recipients")
	recipient, err := recovery.InitializeArchiveIdentity(privatePath, publicPath)
	if err != nil {
		t.Fatal(err)
	}
	private, err := os.ReadFile(privatePath)
	if err != nil {
		t.Fatal(err)
	}
	public, err := os.ReadFile(publicPath)
	if err != nil {
		t.Fatal(err)
	}
	identities, err := age.ParseIdentities(bytes.NewReader(private))
	if err != nil || len(identities) != 1 {
		t.Fatal("private archive identity unusable")
	}
	recipients, err := age.ParseRecipients(bytes.NewReader(public))
	if err != nil || len(recipients) != 1 || strings.TrimSpace(string(public)) != recipient {
		t.Fatal("public recipient mismatch")
	}
	if bytes.Contains(public, private) || strings.Contains(recipient, "SECRET") {
		t.Fatal("private material disclosed as public recipient")
	}
	for _, path := range []string{privatePath, publicPath} {
		info, e := os.Stat(path)
		if e != nil || info.Mode().Perm() != 0o600 {
			t.Fatal("key custody file is not private")
		}
	}
	if _, err = recovery.InitializeArchiveIdentity(privatePath, publicPath); err == nil {
		t.Fatal("existing archive identity overwritten")
	}
	after, err := os.ReadFile(privatePath)
	if err != nil || !bytes.Equal(after, private) {
		t.Fatal("original archive identity lost")
	}
	if _, err = recovery.InitializeArchiveIdentity(
		filepath.Join(root, "missing", "identity"),
		filepath.Join(root, "new-public"),
	); err == nil {
		t.Fatal("missing private storage accepted")
	}
	if _, err = os.Stat(filepath.Join(root, "new-public")); !os.IsNotExist(err) {
		t.Fatal("failed identity creation left an unrelated public recipient")
	}
}
