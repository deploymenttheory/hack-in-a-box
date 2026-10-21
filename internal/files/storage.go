// Package files owns private object storage and transactional file metadata.
package files

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"time"
)

var (
	ErrSize    = errors.New("file exceeds the upload limit")
	ErrQuota   = errors.New("event file quota exceeded")
	ErrMedia   = errors.New("unsupported or mismatched file type")
	ErrPath    = errors.New("invalid filename or object key")
	ErrMissing = errors.New(
		"stored object missing or corrupt; restore the file volume from backup",
	)
	ErrIdempotency = errors.New("idempotency key reused with different file content")
)

type Blob struct {
	Key, Checksum, MediaType string
	Size                     int64
}
type Storage interface {
	Put(context.Context, io.Reader, int64) (Blob, error)
	Read(context.Context, string, int64) ([]byte, error)
	Remove(string) error
	Ready(context.Context) error
}
type Local struct{ root *os.Root }

var keyPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func OpenLocal(path string) (*Local, error) {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return nil, fmt.Errorf("create storage root: %w", err)
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, fmt.Errorf("open storage root: %w", err)
	}
	return &Local{root: root}, nil
}

func (s *Local) Close() error {
	if err := s.root.Close(); err != nil {
		return fmt.Errorf("close storage root: %w", err)
	}
	return nil
}

func (s *Local) Put(ctx context.Context, reader io.Reader, limit int64) (Blob, error) {
	if limit < 1 {
		return Blob{}, ErrSize
	}
	var random [32]byte
	if _, err := rand.Read(random[:]); err != nil {
		return Blob{}, fmt.Errorf("object key: %w", err)
	}
	key := hex.EncodeToString(random[:])
	file, err := s.root.OpenFile(key, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return Blob{}, fmt.Errorf("create private object: %w", err)
	}
	complete := false
	defer func() {
		_ = file.Close()
		if !complete {
			_ = s.root.Remove(key)
		}
	}()
	hash := sha256.New()
	source := &contextReader{ctx: ctx, reader: io.LimitReader(reader, limit+1)}
	prefix := make([]byte, 512)
	n, err := io.ReadFull(source, prefix)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return Blob{}, fmt.Errorf("read upload: %w", err)
	}
	if n == 0 {
		return Blob{}, ErrSize
	}
	writer := io.MultiWriter(file, hash)
	if _, err := writer.Write(prefix[:n]); err != nil {
		return Blob{}, fmt.Errorf("write upload: %w", err)
	}
	remainder, err := io.Copy(writer, source)
	if err != nil {
		return Blob{}, fmt.Errorf("write upload: %w", err)
	}
	size := int64(n) + remainder
	if size > limit {
		return Blob{}, ErrSize
	}
	media := http.DetectContentType(prefix[:n])
	if media == "text/plain; charset=utf-8" {
		media = "text/plain"
	}
	switch media {
	case "application/pdf", "image/png", "image/jpeg", "text/plain":
	default:
		return Blob{}, ErrMedia
	}
	if err := file.Sync(); err != nil {
		return Blob{}, fmt.Errorf("sync object: %w", err)
	}
	if err := file.Close(); err != nil {
		return Blob{}, fmt.Errorf("close object: %w", err)
	}
	directory, err := s.root.Open(".")
	if err != nil {
		return Blob{}, fmt.Errorf("open storage directory: %w", err)
	}
	syncErr := directory.Sync()
	_ = directory.Close()
	if syncErr != nil {
		return Blob{}, fmt.Errorf("sync storage directory: %w", syncErr)
	}
	complete = true
	return Blob{
		Key:       key,
		Checksum:  hex.EncodeToString(hash.Sum(nil)),
		MediaType: media,
		Size:      size,
	}, nil
}

func (s *Local) Read(ctx context.Context, key string, limit int64) ([]byte, error) {
	if !keyPattern.MatchString(key) {
		return nil, ErrPath
	}
	file, err := s.root.Open(key)
	if err != nil {
		return nil, ErrMissing
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(&contextReader{ctx: ctx, reader: io.LimitReader(file, limit+1)})
	if err != nil {
		return nil, fmt.Errorf("read stored object: %w", err)
	}
	if int64(len(data)) > limit {
		return nil, ErrMissing
	}
	return data, nil
}

func (s *Local) Remove(key string) error {
	if !keyPattern.MatchString(key) {
		return ErrPath
	}
	if err := s.root.Remove(key); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove unreferenced object: %w", err)
	}
	return nil
}

func (s *Local) Ready(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("err: %w", err)
	}
	key := hex.EncodeToString([]byte(rand.Text()))
	probe, err := s.root.OpenFile(".probe-"+key, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("storage is not writable: %w", err)
	}
	_ = probe.Close()
	if err := s.root.Remove(".probe-" + key); err != nil {
		return fmt.Errorf("remove storage probe: %w", err)
	}
	return nil
}

// Cleanup keeps a grace period for in-flight uploads and checks metadata before
// removing crash leftovers. Database failures stop cleanup rather than delete.
func (s *Local) Cleanup(
	ctx context.Context,
	referenced func(context.Context, string) (bool, error),
) error {
	directory, err := s.root.Open(".")
	if err != nil {
		return fmt.Errorf("open storage: %w", err)
	}
	defer func() { _ = directory.Close() }()
	entries, err := directory.ReadDir(-1)
	if err != nil {
		return fmt.Errorf("list storage: %w", err)
	}
	for _, entry := range entries {
		if !keyPattern.MatchString(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("inspect object: %w", err)
		}
		if time.Since(info.ModTime()) < 24*time.Hour {
			continue
		}
		exists, err := referenced(ctx, entry.Name())
		if err != nil {
			return err
		}
		if !exists {
			if err := s.Remove(entry.Name()); err != nil {
				return err
			}
		}
	}
	return nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, fmt.Errorf("err: %w", err)
	}
	return r.reader.Read(p) //nolint:wrapcheck // io.Reader adapters preserve io.EOF exactly.
}
