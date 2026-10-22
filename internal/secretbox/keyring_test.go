package secretbox

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func ring(t *testing.T, active string, keys map[string]string) *Keyring {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"active": active, "keys": keys})
	r, e := Parse(string(raw))
	if e != nil {
		t.Fatal(e)
	}
	return r
}

func TestContextBoundEncryptionAndRotationRecovery(t *testing.T) {
	k1 := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))
	k2 := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32))
	first := ring(t, "old", map[string]string{"old": k1})
	id, encrypted, err := first.Seal(
		[]byte("SYNTHETIC-ONLY"),
		[]byte("event:A:resource:B:version:1"),
	)
	if err != nil {
		t.Fatal(err)
	}
	_, second, _ := first.Seal([]byte("SYNTHETIC-ONLY"), []byte("event:A:resource:B:version:1"))
	if bytes.Equal(second, encrypted) || bytes.Contains(encrypted, []byte("SYNTHETIC-ONLY")) {
		t.Fatal("encryption reused nonce or contains plaintext")
	}
	both := ring(t, "new", map[string]string{"old": k1, "new": k2})
	plain, err := both.Open(id, encrypted, []byte("event:A:resource:B:version:1"))
	if err != nil || string(plain) != "SYNTHETIC-ONLY" {
		t.Fatal("old key recovery failed")
	}
	nextID, rotated, err := both.Seal(plain, []byte("event:A:resource:B:version:2"))
	if err != nil || nextID != "new" {
		t.Fatal("active key not used")
	}
	newOnly := ring(t, "new", map[string]string{"new": k2})
	if _, err = newOnly.Open(nextID, rotated, []byte("event:A:resource:B:version:2")); err != nil {
		t.Fatal(err)
	}
	tampered := bytes.Clone(encrypted)
	tampered[len(tampered)-1] ^= 1
	cases := []struct {
		name            string
		ring            *Keyring
		key             string
		cipher, binding []byte
	}{
		{"tampered", first, id, tampered, []byte("event:A:resource:B:version:1")},
		{"other event", first, id, encrypted, []byte("event:X:resource:B:version:1")},
		{"other resource", first, id, encrypted, []byte("event:A:resource:X:version:1")},
		{"other version", first, id, encrypted, []byte("event:A:resource:B:version:2")},
		{
			"wrong key",
			ring(t, "old", map[string]string{"old": k2}),
			id,
			encrypted,
			[]byte("event:A:resource:B:version:1"),
		},
		{"removed key", newOnly, id, encrypted, []byte("event:A:resource:B:version:1")},
		{"truncated", first, id, []byte{1}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, e := tc.ring.Open(tc.key, tc.cipher, tc.binding); !errors.Is(e, ErrUnavailable) {
				t.Fatalf("did not fail closed: %v", e)
			}
		})
	}
}

func TestInvalidKeyringNeverEchoesConfiguration(t *testing.T) {
	for _, raw := range []string{"SECRET-INPUT", `{"active":"missing","keys":{}}`, `{"active":"x","keys":{"x":"SECRET-INPUT"}}`, `{"active":"x","keys":{"x":"` + base64.StdEncoding.EncodeToString([]byte("short")) + `"}}`} {
		_, err := Parse(raw)
		if !errors.Is(err, ErrConfiguration) || strings.Contains(err.Error(), "SECRET-INPUT") {
			t.Fatalf("unsafe parse: %v", err)
		}
	}
	var missing *Keyring
	if _, _, err := missing.Seal([]byte("secret"), nil); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
}
