package files

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	TeamID              *uuid.UUID
	Reason              string
	Revision            int64
	Filename, MediaType string
	Content             []byte
}

func (s Service) Upload(ctx context.Context, input Upload) (dbgen.File, error) {
	if s.Policy == nil {
		return dbgen.File{}, access.ErrForbidden
	}
	target, action := uploadScope(input)
	if err := s.Policy.Authorize(
		ctx,
		input.Actor,
		input.Event,
		target,
		action,
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
			if !sameUpload(existing, blob.Checksum, input) {
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
				TeamID:         input.TeamID,
			},
		)
		if err != nil {
			return fmt.Errorf("insert file: %w", err)
		}
		return recordUpload(ctx, q, input, event, result)
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
	data, err := readStored(ctx, s.Storage, file)
	if err != nil {
		return dbgen.File{}, nil, err
	}
	return file, data, nil
}

// VerifyStored checks immutable file metadata against storage without acquiring a
// database connection. Callers must first authorise the file and load its metadata
// using their current transaction, so verification cannot deadlock a saturated pool.
func VerifyStored(ctx context.Context, storage Storage, file dbgen.File) error {
	_, err := readStored(ctx, storage, file)
	return err
}

// ReadAuthorizedStored verifies immutable metadata and returns stored bytes. The
// native resource owner must first authorise its current reader and exact file
// association using its existing transaction; this helper grants no file access.
func ReadAuthorizedStored(ctx context.Context, storage Storage, file dbgen.File) ([]byte, error) {
	if storage == nil {
		return nil, ErrMissing
	}
	return readStored(ctx, storage, file)
}

func readStored(ctx context.Context, storage Storage, file dbgen.File) ([]byte, error) {
	data, err := storage.Read(ctx, file.ObjectKey, file.SizeBytes)
	if err != nil {
		return nil, fmt.Errorf("read stored file: %w", err)
	}
	checksum := sha256.Sum256(data)
	if int64(len(data)) != file.SizeBytes || hex.EncodeToString(checksum[:]) != file.Checksum {
		return nil, ErrMissing
	}
	return data, nil
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
	target, action := uploadScope(input)
	if err := policy.Authorize(
		ctx,
		input.Actor,
		input.Event,
		target,
		action,
	); err != nil {
		return fmt.Errorf("recheck upload authority: %w", err)
	}
	if input.TeamID != nil {
		row, err := q.Event(ctx, input.Event)
		if err != nil {
			return fmt.Errorf("read upload cutoff: %w", err)
		}
		var dates struct {
			Cutoff time.Time `json:"submission_cutoff"`
		}
		if err := json.Unmarshal(row.Configuration, &dates); err != nil {
			return fmt.Errorf("decode upload cutoff: %w", err)
		}
		now := time.Now()
		if timed, ok := policy.(access.EventPolicy); ok && timed.Clock != nil {
			now = timed.Clock()
		}
		if len(input.Reason) > 2000 ||
			(!now.Before(dates.Cutoff) && strings.TrimSpace(input.Reason) == "") {
			return access.ErrInvalid
		}
	}
	if !writable {
		return access.ErrForbidden
	}
	return nil
}

func uploadScope(input Upload) (uuid.UUID, access.Action) {
	if input.TeamID != nil {
		return *input.TeamID, access.TeamFileWrite
	}
	return input.Event, access.FileWrite
}

func sameTeam(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func recordUpload(
	ctx context.Context,
	q *dbgen.Queries,
	input Upload,
	event dbgen.Event,
	result dbgen.File,
) error {
	if input.TeamID != nil && strings.TrimSpace(input.Reason) != "" {
		if _, err := q.RecordEventDecision(
			ctx,
			dbgen.RecordEventDecisionParams{
				EventID:  input.Event,
				ActorID:  input.Actor,
				Kind:     "submission_file_exception",
				Reason:   input.Reason,
				Revision: event.Revision,
			},
		); err != nil {
			return fmt.Errorf("record file exception: %w", err)
		}
	}
	if err := audit.Append(
		ctx,
		q,
		audit.Record{
			ActorID:  &input.Actor,
			EventID:  &input.Event,
			TargetID: result.ID,
			Action:   audit.FileUploaded,
			Reason:   audit.UserRequest,
		},
	); err != nil {
		return fmt.Errorf("audit uploaded file: %w", err)
	}
	return nil
}

func sameUpload(file dbgen.File, checksum string, input Upload) bool {
	return file.Checksum == checksum && file.Filename == input.Filename &&
		file.MediaType == input.MediaType &&
		sameTeam(file.TeamID, input.TeamID)
}
