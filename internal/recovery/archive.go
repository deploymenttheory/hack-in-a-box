package recovery

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"syscall"
	"time"

	"filippo.io/age"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
)

const (
	manifestLimit       = 16 << 20
	artifactLimit int64 = 1 << 40
)

var (
	ErrArchive = errors.New("recovery archive failed validation")
	objectKey  = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// Blob contains only storage identity and integrity metadata, not person/file names.
type Blob struct {
	Key      string `json:"key"`
	Checksum string `json:"checksum"`
	Size     int64  `json:"size"`
}

// Manifest is encrypted with the archive. Deployment decryption keys are excluded.
type Manifest struct {
	Format         int       `json:"format"`
	BackupID       uuid.UUID `json:"backup_id"`
	Schema         int64     `json:"schema"`
	StartedAt      time.Time `json:"started_at"`
	CompletedAt    time.Time `json:"completed_at"`
	DatabaseDigest string    `json:"database_digest"`
	FilesDigest    string    `json:"files_digest"`
	Blobs          []Blob    `json:"blobs"`
}

// Dump must stream a consistent logical dump without creating a plaintext file.
type Dump func(context.Context, io.Writer) error

// Capture requires quiesced native services and an operator-controlled output
// parent. Its temporary and completed artifacts contain ciphertext only.
func Capture(
	ctx context.Context,
	pool *pgxpool.Pool,
	filesPath, output string,
	recipients []age.Recipient,
	dump Dump,
) (Manifest, error) {
	var m Manifest
	if len(recipients) == 0 || dump == nil {
		return m, ErrArchive
	}
	lease, err := Exclusive(ctx, pool)
	if err != nil {
		return m, err
	}
	defer func() { _ = lease.Close(context.WithoutCancel(ctx)) }()
	if err = database.CheckSchema(ctx, pool); err != nil {
		return m, fmt.Errorf("check capture schema: %w", err)
	}
	if _, err = os.Lstat(output); !errors.Is(err, os.ErrNotExist) {
		return m, ErrArchive
	}
	stage, err := os.MkdirTemp(filepath.Dir(output), ".hackbox-encrypted-")
	if err != nil {
		return m, fmt.Errorf("create encrypted capture: %w", err)
	}
	defer func() { _ = os.RemoveAll(stage) }()
	m = Manifest{
		Format:    1,
		BackupID:  uuid.New(),
		Schema:    database.SchemaVersion,
		StartedAt: time.Now().UTC(),
		Blobs:     []Blob{},
	}
	rows, err := pool.Query(
		ctx,
		"SELECT object_key,checksum,size_bytes FROM files ORDER BY object_key",
	)
	if err != nil {
		return m, fmt.Errorf("read capture objects: %w", err)
	}
	for rows.Next() {
		var blob Blob
		if err = rows.Scan(&blob.Key, &blob.Checksum, &blob.Size); err != nil {
			rows.Close()
			return m, fmt.Errorf("scan capture objects: %w", err)
		}
		m.Blobs = append(m.Blobs, blob)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return m, fmt.Errorf("read capture objects: %w", err)
	}
	if !validBlobs(m.Blobs) {
		return m, ErrArchive
	}
	root, err := os.OpenRoot(filesPath)
	if err != nil {
		return m, fmt.Errorf("open capture storage: %w", err)
	}
	defer func() { _ = root.Close() }()
	m.DatabaseDigest, err = encryptFile(
		filepath.Join(stage, "database.age"),
		recipients,
		func(w io.Writer) error { return dump(ctx, w) },
	)
	if err != nil {
		return m, err
	}
	m.FilesDigest, err = encryptFile(
		filepath.Join(stage, "files.age"),
		recipients,
		func(w io.Writer) error {
			tw := tar.NewWriter(w)
			for _, blob := range m.Blobs {
				if err := ctx.Err(); err != nil {
					return fmt.Errorf("capture cancelled: %w", err)
				}
				if err := captureBlob(root, tw, blob); err != nil {
					return err
				}
			}
			if err := tw.Close(); err != nil {
				return fmt.Errorf("finish file archive: %w", err)
			}
			return nil
		},
	)
	if err != nil {
		return m, err
	}
	m.CompletedAt = time.Now().UTC()
	_, err = encryptFile(filepath.Join(stage, "manifest.age"), recipients, func(w io.Writer) error {
		if err := json.NewEncoder(w).Encode(m); err != nil {
			return fmt.Errorf("encode encrypted manifest: %w", err)
		}
		return nil
	})
	if err != nil {
		return m, err
	}
	if err = syncDirectory(stage); err != nil {
		return m, err
	}
	if err = os.Rename(stage, output); err != nil {
		return m, fmt.Errorf("publish encrypted backup: %w", err)
	}
	if err = syncDirectory(filepath.Dir(output)); err != nil {
		return m, err
	}
	return m, nil
}

func validBlobs(blobs []Blob) bool {
	if len(blobs) > 100000 {
		return false
	}
	previous := ""
	var total int64
	for _, blob := range blobs {
		if !objectKey.MatchString(blob.Key) || !objectKey.MatchString(blob.Checksum) ||
			blob.Key <= previous ||
			blob.Size < 0 ||
			blob.Size > artifactLimit {
			return false
		}
		previous = blob.Key
		total += blob.Size
		if total > artifactLimit {
			return false
		}
	}
	return true
}

func captureBlob(root *os.Root, tw *tar.Writer, blob Blob) error {
	file, err := root.OpenFile(blob.Key, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return fmt.Errorf("open capture object: %w", err)
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != blob.Size {
		return ErrArchive
	}
	if err = tw.WriteHeader(
		&tar.Header{Name: blob.Key, Mode: 0o600, Size: blob.Size, Typeflag: tar.TypeReg},
	); err != nil {
		return fmt.Errorf("write capture object header: %w", err)
	}
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(tw, hash), io.LimitReader(file, blob.Size+1))
	if err != nil {
		return fmt.Errorf("copy capture object: %w", err)
	}
	if n != blob.Size || hex.EncodeToString(hash.Sum(nil)) != blob.Checksum {
		return ErrArchive
	}
	return nil
}

func encryptFile(
	path string,
	recipients []age.Recipient,
	write func(io.Writer) error,
) (string, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", fmt.Errorf("create encrypted artifact: %w", err)
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	writer, err := age.Encrypt(io.MultiWriter(file, hash), recipients...)
	if err != nil {
		return "", fmt.Errorf("start archive encryption: %w", err)
	}
	if err = write(writer); err != nil {
		return "", err
	}
	if err = writer.Close(); err != nil {
		return "", fmt.Errorf("authenticate encrypted artifact: %w", err)
	}
	if err = file.Sync(); err != nil {
		return "", fmt.Errorf("sync encrypted artifact: %w", err)
	}
	if err = file.Close(); err != nil {
		return "", fmt.Errorf("close encrypted artifact: %w", err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open recovery directory: %w", err)
	}
	defer func() { _ = dir.Close() }()
	if err = dir.Sync(); err != nil {
		return fmt.Errorf("sync recovery directory: %w", err)
	}
	return nil
}
