package recovery

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"filippo.io/age"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
)

// RestoreSQL executes the supplied logical SQL stream in one transaction with
// stop-on-error enabled. The stream includes installation of the new recovery hold.
type RestoreSQL func(context.Context, io.Reader) error

// Restored records a verified fresh-target recovery, still held for operator review.
type Restored struct {
	Manifest    Manifest  `json:"manifest"`
	Hold        Status    `json:"hold"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
}

// Restore refuses existing application data/files. It snapshots ciphertext into
// its own private directory, authenticates it completely, then streams SQL into a
// fresh database and files into a private staging directory. Native services stay
// blocked by the exclusive lease and the hold committed with the SQL restoration.
func Restore(
	ctx context.Context,
	pool *pgxpool.Pool,
	archivePath, filesPath string,
	identities []age.Identity,
	restore RestoreSQL,
) (Restored, error) {
	result := Restored{StartedAt: time.Now().UTC()}
	if restore == nil {
		return result, ErrArchive
	}
	copyPath, err := copyEncryptedBundle(archivePath)
	if err != nil {
		return result, err
	}
	defer func() { _ = os.RemoveAll(copyPath) }()
	result.Manifest, err = Verify(ctx, copyPath, identities)
	if err != nil {
		return result, err
	}
	lease, err := Exclusive(ctx, pool)
	if err != nil {
		return result, err
	}
	defer func() { _ = lease.Close(context.WithoutCancel(ctx)) }()
	var relations int64
	err = pool.QueryRow(ctx, `SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname NOT IN ('pg_catalog','information_schema') AND n.nspname NOT LIKE 'pg_toast%' AND c.relkind IN ('r','p','v','m','S','f')`).
		Scan(&relations)
	if err != nil {
		return result, fmt.Errorf("inspect fresh restore target: %w", err)
	}
	if relations != 0 {
		return result, ErrArchive
	}
	if _, err = os.Lstat(filesPath); !errors.Is(err, os.ErrNotExist) {
		return result, ErrArchive
	}
	stage, err := os.MkdirTemp(filepath.Dir(filesPath), ".hackbox-restoring-")
	if err != nil {
		return result, fmt.Errorf("create private restored storage: %w", err)
	}
	defer func() { _ = os.RemoveAll(stage) }()
	root, err := os.OpenRoot(copyPath)
	if err != nil {
		return result, fmt.Errorf("open verified recovery: %w", err)
	}
	defer func() { _ = root.Close() }()
	if err = extractFiles(ctx, root, stage, result.Manifest.Blobs, identities); err != nil {
		return result, err
	}
	databaseFile, err := openArtifact(root, "database.age", artifactLimit)
	if err != nil {
		return result, err
	}
	defer func() { _ = databaseFile.Close() }()
	plain, err := age.Decrypt(databaseFile, identities...)
	if err != nil {
		return result, ErrArchive
	}
	hold := uuid.New()
	// Generated UUID is the only interpolation. Raw operator or archive strings are
	// never inserted into this SQL. The callback must commit this in the dump's txn.
	holdSQL := "\nUPDATE public.recovery_control SET hold_id='" + hold.String() + "',held_at=clock_timestamp() WHERE singleton;\nINSERT INTO public.recovery_history(hold_id,action) VALUES('" + hold.String() + "','hold');\n"
	if err = restore(ctx, io.MultiReader(plain, strings.NewReader(holdSQL))); err != nil {
		return result, fmt.Errorf("restore held database: %w", err)
	}
	if err = database.CheckSchema(ctx, pool); err != nil {
		return result, fmt.Errorf("verify restored schema: %w", err)
	}
	result.Hold, err = Read(ctx, pool)
	if err != nil {
		return result, err
	}
	if result.Hold.HoldID == nil || *result.Hold.HoldID != hold {
		return result, ErrArchive
	}
	if err = verifyRestoredObjects(ctx, pool, result.Manifest.Blobs); err != nil {
		return result, err
	}
	if err = syncDirectory(stage); err != nil {
		return result, err
	}
	if err = os.Rename(stage, filesPath); err != nil {
		return result, fmt.Errorf("publish restored storage: %w", err)
	}
	if err = syncDirectory(filepath.Dir(filesPath)); err != nil {
		return result, err
	}
	result.CompletedAt = time.Now().UTC()
	return result, nil
}

func copyEncryptedBundle(path string) (string, error) {
	root, err := os.OpenRoot(path)
	if err != nil {
		return "", fmt.Errorf("open encrypted source: %w", err)
	}
	defer func() { _ = root.Close() }()
	dir, err := root.Open(".")
	if err != nil {
		return "", fmt.Errorf("read encrypted source: %w", err)
	}
	entries, err := dir.ReadDir(-1)
	_ = dir.Close()
	if err != nil {
		return "", fmt.Errorf("read encrypted source: %w", err)
	}
	if len(entries) != 3 {
		return "", ErrArchive
	}
	target, err := os.MkdirTemp("", "hackbox-encrypted-restore-")
	if err != nil {
		return "", fmt.Errorf("create private encrypted copy: %w", err)
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.RemoveAll(target)
		}
	}()
	for _, entry := range entries {
		name := entry.Name()
		if name != "manifest.age" && name != "database.age" && name != "files.age" {
			return "", ErrArchive
		}
		if err = copyEncryptedArtifact(root, target, name); err != nil {
			return "", err
		}
	}
	complete = true
	return target, nil
}

func copyEncryptedArtifact(root *os.Root, target, name string) error {
	source, err := openArtifact(root, name, artifactLimit)
	if err != nil {
		return err
	}
	defer func() { _ = source.Close() }()
	file, err := os.OpenFile(filepath.Join(target, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create private encrypted artifact: %w", err)
	}
	defer func() { _ = file.Close() }()
	n, err := io.Copy(file, io.LimitReader(source, artifactLimit+1))
	if err != nil {
		return fmt.Errorf("copy encrypted artifact: %w", err)
	}
	if n > artifactLimit {
		return ErrArchive
	}
	if err = file.Sync(); err != nil {
		return fmt.Errorf("sync private encrypted artifact: %w", err)
	}
	return nil
}

func verifyRestoredObjects(ctx context.Context, pool *pgxpool.Pool, blobs []Blob) error {
	rows, err := pool.Query(
		ctx,
		"SELECT object_key,checksum,size_bytes FROM files ORDER BY object_key",
	)
	if err != nil {
		return fmt.Errorf("verify restored object metadata: %w", err)
	}
	defer rows.Close()
	index := 0
	for rows.Next() {
		var actual Blob
		if err = rows.Scan(&actual.Key, &actual.Checksum, &actual.Size); err != nil {
			return fmt.Errorf("read restored object metadata: %w", err)
		}
		if index >= len(blobs) || actual != blobs[index] {
			return ErrArchive
		}
		index++
	}
	if err = rows.Err(); err != nil {
		return fmt.Errorf("verify restored object metadata: %w", err)
	}
	if index != len(blobs) {
		return ErrArchive
	}
	return nil
}

func extractFiles(
	ctx context.Context,
	source *os.Root,
	target string,
	blobs []Blob,
	identities []age.Identity,
) error {
	file, err := openArtifact(source, "files.age", artifactLimit)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	plain, err := age.Decrypt(file, identities...)
	if err != nil {
		return ErrArchive
	}
	root, err := os.OpenRoot(target)
	if err != nil {
		return fmt.Errorf("open private restored storage: %w", err)
	}
	defer func() { _ = root.Close() }()
	archive := tar.NewReader(plain)
	for _, blob := range blobs {
		if err = ctx.Err(); err != nil {
			return fmt.Errorf("restore files cancelled: %w", err)
		}
		header, e := archive.Next()
		if e != nil || header.Name != blob.Key || header.Typeflag != tar.TypeReg ||
			header.Size != blob.Size {
			return ErrArchive
		}
		if err = extractBlob(root, archive, blob); err != nil {
			return err
		}
	}
	if _, err = archive.Next(); !errors.Is(err, io.EOF) {
		return ErrArchive
	}
	if _, err = io.Copy(io.Discard, plain); err != nil {
		return fmt.Errorf("authenticate restored files: %w", err)
	}
	return nil
}

func extractBlob(root *os.Root, reader io.Reader, blob Blob) error {
	file, err := root.OpenFile(blob.Key, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create restored object: %w", err)
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(file, hash), reader)
	if err != nil {
		return fmt.Errorf("restore object: %w", err)
	}
	if n != blob.Size || hex.EncodeToString(hash.Sum(nil)) != blob.Checksum {
		return ErrArchive
	}
	if err = file.Sync(); err != nil {
		return fmt.Errorf("sync restored object: %w", err)
	}
	return nil
}
