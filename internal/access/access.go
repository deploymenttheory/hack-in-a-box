// Package access defines the application resource-policy boundary. Authentication
// supplies a person; policy evaluates current grants for each operation.
package access

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

var (
	ErrForbidden       = errors.New("access denied")
	ErrUnauthenticated = errors.New("authentication required")
	ErrNotFound        = errors.New("resource not found")
	ErrConflict        = errors.New("stale resource revision")
	ErrInvalid         = errors.New("invalid input")
)

type Action string

const (
	AuditRead   Action = "audit.read"
	AuditExport Action = "audit.export"
	FileWrite   Action = "file.write"
	FileRead    Action = "file.read"
)

type Policy interface {
	Authorize(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, Action) error
}
