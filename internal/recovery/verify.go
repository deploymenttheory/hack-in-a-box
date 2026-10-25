package recovery

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"syscall"

	"filippo.io/age"
	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
)

// Verify authenticates every age stream to EOF and checks the complete object
// inventory before restore may consume it. It never extracts an archive path.
func Verify(ctx context.Context, path string, identities []age.Identity) (Manifest, error) {
	var m Manifest
	if len(identities) == 0 {
		return m, ErrArchive
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return m, fmt.Errorf("open recovery archive: %w", err)
	}
	defer func() { _ = root.Close() }()
	dir, err := root.Open(".")
	if err != nil {
		return m, fmt.Errorf("read archive directory: %w", err)
	}
	entries, err := dir.ReadDir(-1)
	_ = dir.Close()
	if err != nil {
		return m, fmt.Errorf("read archive directory: %w", err)
	}
	if len(entries) != 3 {
		return m, ErrArchive
	}
	for _, entry := range entries {
		if entry.Name() != "database.age" && entry.Name() != "files.age" &&
			entry.Name() != "manifest.age" {
			return m, ErrArchive
		}
	}
	manifest, err := openArtifact(root, "manifest.age", manifestLimit)
	if err != nil {
		return m, err
	}
	defer func() { _ = manifest.Close() }()
	plain, err := age.Decrypt(manifest, identities...)
	if err != nil {
		return m, ErrArchive
	}
	data, err := io.ReadAll(io.LimitReader(plain, manifestLimit+1))
	if err != nil || len(data) > manifestLimit {
		return m, ErrArchive
	}
	// ReadAll must observe the final authenticated age chunk, not merely one JSON object.
	if err = json.Unmarshal(data, &m); err != nil {
		return m, ErrArchive
	}
	if m.Format != 1 || m.BackupID == uuid.Nil || m.Schema != database.SchemaVersion ||
		m.StartedAt.IsZero() ||
		m.CompletedAt.Before(m.StartedAt) ||
		!validBlobs(m.Blobs) ||
		!objectKey.MatchString(m.DatabaseDigest) ||
		!objectKey.MatchString(m.FilesDigest) {
		return m, ErrArchive
	}
	if err = verifyArtifact(
		ctx,
		root,
		"database.age",
		m.DatabaseDigest,
		identities,
		func(reader io.Reader) error {
			if _, err := io.Copy(io.Discard, reader); err != nil {
				return fmt.Errorf("authenticate database stream: %w", err)
			}
			return nil
		},
	); err != nil {
		return m, err
	}
	if err = verifyArtifact(
		ctx,
		root,
		"files.age",
		m.FilesDigest,
		identities,
		func(reader io.Reader) error { return verifyFiles(ctx, reader, m.Blobs) },
	); err != nil {
		return m, err
	}
	return m, nil
}

func openArtifact(root *os.Root, name string, limit int64) (*os.File, error) {
	file, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, ErrArchive
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit ||
		info.Mode().Perm()&0o077 != 0 {
		_ = file.Close()
		return nil, ErrArchive
	}
	return file, nil
}

func verifyArtifact(
	ctx context.Context,
	root *os.Root,
	name, digest string,
	identities []age.Identity,
	consume func(io.Reader) error,
) error {
	file, err := openArtifact(root, name, artifactLimit)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	plain, err := age.Decrypt(io.TeeReader(file, hash), identities...)
	if err != nil {
		return ErrArchive
	}
	if err = consume(plain); err != nil {
		return err
	}
	// tar EOF occurs before encryption EOF. Drain and authenticate the remaining
	// stream too; reject nonzero trailing plaintext instead of silently ignoring it.
	remainder := make([]byte, 4096)
	for {
		if err = ctx.Err(); err != nil {
			return fmt.Errorf("verify cancelled: %w", err)
		}
		n, readErr := plain.Read(remainder)
		for _, b := range remainder[:n] {
			if b != 0 {
				return ErrArchive
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return ErrArchive
		}
	}
	if hex.EncodeToString(hash.Sum(nil)) != digest {
		return ErrArchive
	}
	return nil
}

func verifyFiles(ctx context.Context, reader io.Reader, blobs []Blob) error {
	archive := tar.NewReader(reader)
	for _, blob := range blobs {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("verify files cancelled: %w", err)
		}
		header, err := archive.Next()
		if err != nil || header.Name != blob.Key || header.Typeflag != tar.TypeReg ||
			header.Size != blob.Size ||
			header.Linkname != "" ||
			len(header.PAXRecords) != 0 {
			return ErrArchive
		}
		hash := sha256.New()
		n, err := io.Copy(hash, io.LimitReader(archive, blob.Size+1))
		if err != nil || n != blob.Size || hex.EncodeToString(hash.Sum(nil)) != blob.Checksum {
			return ErrArchive
		}
	}
	if _, err := archive.Next(); err != io.EOF {
		return ErrArchive
	}
	return nil
}
