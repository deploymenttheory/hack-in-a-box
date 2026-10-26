package loadprofile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadOnlyObserverRejectsRetargetedOrPrivilegedConfigurationBeforeDial(t *testing.T) {
	for _, dsn := range []string{
		"postgres://load123_observer:synthetic@remote.invalid:15433/load123",
		"postgres://load123:synthetic@127.0.0.1:15433/load123",
		"postgres://load123_observer@127.0.0.1:15433/load123",
		"postgres://load123_observer:synthetic@127.0.0.1:15433/load123?host=remote.invalid",
		"postgres://load123_observer:synthetic@127.0.0.1:15433/load123?port=15434",
		"postgres://load123_observer:synthetic@127.0.0.1:15433/load123?user=load123",
		"postgres://load123_observer:synthetic@127.0.0.1:15433/load123?password=alternate",
	} {
		if pool, err := OpenReadOnlyObserver(t.Context(), dsn); err == nil || pool != nil {
			if pool != nil {
				pool.Close()
			}
			t.Fatal("untrusted observer configuration accepted")
		}
	}
}

func TestPrivateInputRejectsReadableSecretsAndOversizedFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private.json")
	if err := os.WriteFile(path, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadPrivateInput(path, 16); err != nil {
		t.Fatal("owner-only bounded input rejected", err)
	}
	if _, err := ReadPrivateInput(path, 2); err == nil {
		t.Fatal("oversized secret input accepted")
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadPrivateInput(path, 16); err == nil {
		t.Fatal("world-readable private input accepted")
	}
}
