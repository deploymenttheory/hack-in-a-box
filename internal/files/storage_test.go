package files_test

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/files"
)

type interrupted struct{}

func (interrupted) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func TestLocalRejectsTraversalSizeTypesAndInterruptedWrites(t *testing.T) {
	directory := t.TempDir()
	storage, err := files.OpenLocal(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	for _, key := range []string{"../secret", "/etc/passwd", "a/b", "..", ""} {
		if _, err := storage.Read(t.Context(), key, 100); !errors.Is(err, files.ErrPath) {
			t.Fatalf("path %q accepted: %v", key, err)
		}
	}
	for _, tc := range []struct {
		reader io.Reader
		limit  int64
	}{{strings.NewReader("too large"), 2}, {strings.NewReader("<html>unsafe</html>"), 100}, {interrupted{}, 100}} {
		if _, err := storage.Put(t.Context(), tc.reader, tc.limit); err == nil {
			t.Fatal("invalid upload accepted")
		}
		entries, err := os.ReadDir(directory)
		if err != nil || len(entries) != 0 {
			t.Fatalf("incomplete upload remained: %v %v", entries, err)
		}
	}
	blob, err := storage.Put(t.Context(), strings.NewReader("safe text"), 100)
	if err != nil {
		t.Fatal(err)
	}
	data, err := storage.Read(t.Context(), blob.Key, blob.Size)
	if err != nil || !bytes.Equal(data, []byte("safe text")) {
		t.Fatalf("stored content changed: %q %v", data, err)
	}
	if err := storage.Remove(blob.Key); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.Read(t.Context(), blob.Key, 100); !errors.Is(err, files.ErrMissing) {
		t.Fatalf("missing object: %v", err)
	}
}
