// Package access defines the application resource-policy boundary. Authentication
// supplies a person; policy evaluates current grants for each operation.
package access

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
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

// AdminPolicy is deliberately restrictive until event-role grants land in #34.
// Callers provide event AND resource IDs so subsequent policies can narrow access.
type AdminPolicy struct{ Queries *dbgen.Queries }

func (p AdminPolicy) Authorize(
	ctx context.Context,
	actor, event, _ uuid.UUID,
	action Action,
) error {
	switch action {
	case AuditRead, AuditExport, FileWrite, FileRead:
	default:
		return ErrForbidden
	}
	person, err := p.Queries.Person(ctx, actor)
	if err != nil || !person.Active || !person.PlatformAdmin {
		return ErrForbidden
	}
	if _, err := p.Queries.Event(ctx, event); err != nil {
		return ErrNotFound
	}
	return nil
}
