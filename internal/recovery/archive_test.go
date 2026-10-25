package recovery_test

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"filippo.io/age"
	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/recovery"
)

func TestArchiveAuthenticatesCompleteStreamsAndRejectsUnsafeObjects(t *testing.T) {
	identity, err := age.GenerateHybridIdentity()
	if err != nil {
		t.Fatal(err)
	}
	recipient := identity.Recipient()
	for _, scenario := range []string{"valid", "wrong-key", "truncated-database", "truncated-files", "unsafe-path", "symlink-header", "wrong-blob-checksum", "symlink-artifact", "unexpected-artifact", "public-artifact"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			content := []byte("synthetic protected attachment\n")
			checksum := sha256.Sum256(content)
			key := hex.EncodeToString(checksum[:])
			blob := recovery.Blob{Key: key, Checksum: key, Size: int64(len(content))}
			var tarBytes bytes.Buffer
			tw := tar.NewWriter(&tarBytes)
			header := &tar.Header{Name: key, Mode: 0o600, Typeflag: tar.TypeReg, Size: blob.Size}
			if scenario == "unsafe-path" {
				header.Name = "../escape"
			}
			if scenario == "symlink-header" {
				header.Typeflag = tar.TypeSymlink
				header.Linkname = "outside"
				header.Size = 0
			}
			if err = tw.WriteHeader(header); err != nil {
				t.Fatal(err)
			}
			if header.Typeflag == tar.TypeReg {
				if _, err = tw.Write(content); err != nil {
					t.Fatal(err)
				}
			}
			if err = tw.Close(); err != nil {
				t.Fatal(err)
			}
			databaseDigest := fixtureCiphertext(
				t,
				root,
				"database.age",
				recipient,
				[]byte("-- synthetic logical database\n"),
			)
			filesDigest := fixtureCiphertext(t, root, "files.age", recipient, tarBytes.Bytes())
			if scenario == "wrong-blob-checksum" {
				blob.Checksum = hex.EncodeToString(make([]byte, 32))
			}
			manifest := recovery.Manifest{
				Format:         1,
				BackupID:       uuid.New(),
				Schema:         database.SchemaVersion,
				StartedAt:      time.Now().UTC(),
				CompletedAt:    time.Now().UTC(),
				DatabaseDigest: databaseDigest,
				FilesDigest:    filesDigest,
				Blobs:          []recovery.Blob{blob},
			}
			data, err := json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
			fixtureCiphertext(t, root, "manifest.age", recipient, data)
			identities := []age.Identity{identity}
			switch scenario {
			case "wrong-key":
				wrong, e := age.GenerateHybridIdentity()
				if e != nil {
					t.Fatal(e)
				}
				identities = []age.Identity{wrong}
			case "truncated-database", "truncated-files":
				name := "database.age"
				if scenario == "truncated-files" {
					name = "files.age"
				}
				path := filepath.Join(root, name)
				info, e := os.Stat(path)
				if e != nil {
					t.Fatal(e)
				}
				if e = os.Truncate(path, info.Size()-1); e != nil {
					t.Fatal(e)
				}
			case "symlink-artifact":
				path := filepath.Join(root, "files.age")
				outside := filepath.Join(t.TempDir(), "ciphertext")
				if e := os.Rename(path, outside); e != nil {
					t.Fatal(e)
				}
				if e := os.Symlink(outside, path); e != nil {
					t.Fatal(e)
				}
			case "unexpected-artifact":
				if e := os.WriteFile(
					filepath.Join(root, "extra"),
					[]byte("unexpected"),
					0o600,
				); e != nil {
					t.Fatal(e)
				}
			case "public-artifact":
				if e := os.Chmod(filepath.Join(root, "database.age"), 0o644); e != nil {
					t.Fatal(e)
				}
			}
			verified, err := recovery.Verify(t.Context(), root, identities)
			if scenario == "valid" {
				if err != nil || verified.BackupID != manifest.BackupID {
					t.Fatalf("valid bundle rejected: %+v %v", verified, err)
				}
			} else if err == nil {
				t.Fatalf("unsafe %s bundle accepted", scenario)
			}
			if _, e := os.Stat(filepath.Join(filepath.Dir(root), "escape")); !os.IsNotExist(e) {
				t.Fatalf("verification extracted unsafe path: %v", e)
			}
		})
	}
}

func fixtureCiphertext(
	t *testing.T,
	root, name string,
	recipient age.Recipient,
	data []byte,
) string {
	t.Helper()
	var encrypted bytes.Buffer
	writer, err := age.Encrypt(&encrypted, recipient)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = writer.Write(data); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, name), encrypted.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(encrypted.Bytes())
	return hex.EncodeToString(hash[:])
}
