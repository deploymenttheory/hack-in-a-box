package files

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

type Service struct {
	Pool                 *pgxpool.Pool
	Storage              Storage
	Policy               access.Policy
	MaxBytes, EventQuota int64
}
type Upload struct {
	Actor, Event, Key   uuid.UUID
	Revision            int64
	Filename, MediaType string
	Content             []byte
}

func (s Service) Upload(ctx context.Context, input Upload) (dbgen.File, error) {
	if s.Policy == nil {
		return dbgen.File{}, access.ErrForbidden
	}
	if err := s.Policy.Authorize(
		ctx,
		input.Actor,
		input.Event,
		input.Event,
		access.FileWrite,
	); err != nil {
		return dbgen.File{}, fmt.Errorf("authorize: %w", err)
	}
	if input.Key == uuid.Nil || input.Revision < 1 {
		return dbgen.File{}, access.ErrInvalid
	}
	if input.Filename == "" || len(input.Filename) > 160 ||
		filepath.Base(input.Filename) != input.Filename ||
		strings.ContainsAny(input.Filename, "/\\") ||
		strings.IndexFunc(input.Filename, unicode.IsControl) >= 0 ||
		input.Filename == "." ||
		input.Filename == ".." {
		return dbgen.File{}, ErrPath
	}
	if int64(len(input.Content)) > s.MaxBytes {
		return dbgen.File{}, ErrSize
	}
	blob, err := s.Storage.Put(ctx, bytes.NewReader(input.Content), s.MaxBytes)
	if err != nil {
		return dbgen.File{}, fmt.Errorf("put: %w", err)
	}
	// A commit error can have an unknown outcome. Only remove after positively
	// confirming metadata does not reference the object; otherwise GC handles it.
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		exists, err := dbgen.New(s.Pool).ObjectExists(cleanup, blob.Key)
		if err == nil && !exists {
			_ = s.Storage.Remove(blob.Key)
		}
	}()
	if blob.MediaType != input.MediaType {
		return dbgen.File{}, ErrMedia
	}
	var result dbgen.File
	err = database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		event, err := q.LockEvent(ctx, input.Event)
		if errors.Is(err, pgx.ErrNoRows) {
			return access.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock event: %w", err)
		}
		if err := s.authorizeUpload(ctx, q, input, event.WritesAllowed); err != nil {
			return err
		}
		existing, err := q.IdempotentFile(
			ctx,
			dbgen.IdempotentFileParams{
				EventID:        input.Event,
				OwnerID:        input.Actor,
				IdempotencyKey: input.Key,
			},
		)
		if err == nil {
			if existing.Checksum != blob.Checksum || existing.Filename != input.Filename ||
				existing.MediaType != input.MediaType {
				return ErrIdempotency
			}
			result = existing
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("idempotent file: %w", err)
		}
		if event.Revision != input.Revision {
			return access.ErrConflict
		}
		usage, err := q.EventFileUsage(ctx, input.Event)
		if err != nil {
			return fmt.Errorf("event file usage: %w", err)
		}
		if blob.Size > s.EventQuota-usage {
			return ErrQuota
		}
		result, err = q.InsertFile(
			ctx,
			dbgen.InsertFileParams{
				ID:             uuid.New(),
				EventID:        input.Event,
				OwnerID:        input.Actor,
				IdempotencyKey: input.Key,
				ObjectKey:      blob.Key,
				Filename:       input.Filename,
				MediaType:      blob.MediaType,
				Checksum:       blob.Checksum,
				SizeBytes:      blob.Size,
			},
		)
		if err != nil {
			return fmt.Errorf("insert file: %w", err)
		}
		return audit.Append(
			ctx,
			q,
			audit.Record{
				ActorID:  &input.Actor,
				EventID:  &input.Event,
				TargetID: result.ID,
				Action:   audit.FileUploaded,
				Reason:   audit.UserRequest,
			},
		)
	})
	if err != nil {
		return dbgen.File{}, fmt.Errorf("finalize upload: %w", err)
	}
	return result, nil
}

func (s Service) Download(
	ctx context.Context,
	actor, event, id uuid.UUID,
) (dbgen.File, []byte, error) {
	if s.Policy == nil {
		return dbgen.File{}, nil, access.ErrForbidden
	}
	if err := s.Policy.Authorize(ctx, actor, event, id, access.FileRead); err != nil {
		return dbgen.File{}, nil, fmt.Errorf("authorize: %w", err)
	}
	file, err := dbgen.New(s.Pool).File(ctx, dbgen.FileParams{EventID: event, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return dbgen.File{}, nil, access.ErrNotFound
	}
	if err != nil {
		return dbgen.File{}, nil, fmt.Errorf("find file metadata: %w", err)
	}
	data, err := s.Storage.Read(ctx, file.ObjectKey, file.SizeBytes)
	if err != nil {
		return dbgen.File{}, nil, fmt.Errorf("read: %w", err)
	}
	checksum := sha256.Sum256(data)
	if int64(len(data)) != file.SizeBytes || hex.EncodeToString(checksum[:]) != file.Checksum {
		return dbgen.File{}, nil, ErrMissing
	}
	return file, data, nil
}

func (s Service) authorizeUpload(
	ctx context.Context,
	q *dbgen.Queries,
	input Upload,
	writable bool,
) error {
	policy := s.Policy
	if scoped, ok := policy.(interface {
		WithQueries(*dbgen.Queries) access.Policy
	}); ok {
		policy = scoped.WithQueries(q)
	}
	if err := policy.Authorize(
		ctx,
		input.Actor,
		input.Event,
		input.Event,
		access.FileWrite,
	); err != nil {
		return fmt.Errorf("recheck upload authority: %w", err)
	}
	if !writable {
		return access.ErrForbidden
	}
	return nil
}
