// Package audit stores a deliberately narrow, append-only decision history.
package audit

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

type Action string

const (
	AdminBootstrapped       Action = "admin.bootstrapped"
	SignedIn                Action = "identity.signed_in"
	FileUploaded            Action = "file.uploaded"
	OrganiserGranted        Action = "organiser.granted"
	PrivilegedAccessGranted Action = "privileged_access.granted"
	AttendanceException     Action = "attendance.exception"
	TeamFrozen              Action = "team.frozen"
	TeamRebalanced          Action = "team.rebalanced"
	ChallengeApproved       Action = "challenge.approved"
	ChallengePostponed      Action = "challenge.postponed"
	SubmissionFinalized     Action = "submission.finalized"
	RoleGranted             Action = "role.granted"
	RoleRevoked             Action = "role.revoked"
	OwnerTransferred        Action = "owner.transferred"
)

type Reason string

const (
	Bootstrap          Reason = "deployment_bootstrap"
	Authentication     Reason = "authentication"
	UserRequest        Reason = "user_request"
	OrganiserDecision  Reason = "organiser_decision"
	ThresholdException Reason = "post_publication_withdrawal"
)

// Metadata has no arbitrary strings/maps: credentials, brief text, accessibility
// needs and assessment prose cannot accidentally enter generic audit payloads.
type Metadata struct {
	ReferenceID      *uuid.UUID `json:"reference_id,omitempty"`
	PreviousRevision *int64     `json:"previous_revision,omitempty"`
	Revision         *int64     `json:"revision,omitempty"`
	Count            *int       `json:"count,omitempty"`
	Role             *string    `json:"role,omitempty"`
}
type Record struct {
	ActorID, EventID *uuid.UUID
	TargetID         uuid.UUID
	Action           Action
	Reason           Reason
	Metadata         Metadata
}

func Append(ctx context.Context, q *dbgen.Queries, r Record) error {
	switch r.Action {
	case AdminBootstrapped,
		SignedIn,
		FileUploaded,
		OrganiserGranted,
		PrivilegedAccessGranted,
		AttendanceException,
		TeamFrozen,
		TeamRebalanced,
		ChallengeApproved,
		ChallengePostponed,
		SubmissionFinalized, RoleGranted, RoleRevoked, OwnerTransferred:
	default:
		return access.ErrInvalid
	}
	switch r.Reason {
	case Bootstrap, Authentication, UserRequest, OrganiserDecision, ThresholdException:
	default:
		return access.ErrInvalid
	}
	if r.TargetID == uuid.Nil {
		return access.ErrInvalid
	}
	if r.Metadata.Role != nil {
		switch *r.Metadata.Role {
		case "organiser",
			"mentor",
			"judge",
			"mc",
			"logistics",
			"photographer",
			"early_brief_access":
		default:
			return access.ErrInvalid
		}
	}
	metadata, err := json.Marshal(r.Metadata)
	if err != nil {
		return fmt.Errorf("encode audit references: %w", err)
	}
	_, err = q.AppendAudit(
		ctx,
		dbgen.AppendAuditParams{
			ActorID:    r.ActorID,
			EventID:    r.EventID,
			TargetID:   r.TargetID,
			Action:     string(r.Action),
			ReasonCode: string(r.Reason),
			Metadata:   metadata,
		},
	)
	if err != nil {
		return fmt.Errorf("append audit event: %w", err)
	}
	return nil
}

type Service struct {
	Queries *dbgen.Queries
	Policy  access.Policy
}

func (s Service) List(
	ctx context.Context,
	actor, event uuid.UUID,
	cursor int64,
	limit int32,
	export bool,
) ([]dbgen.AuditEvent, error) {
	action := access.AuditRead
	if export {
		action = access.AuditExport
	}
	if s.Policy == nil {
		return nil, access.ErrForbidden
	}
	if err := s.Policy.Authorize(ctx, actor, event, event, action); err != nil {
		return nil, fmt.Errorf("authorize: %w", err)
	}
	if cursor < 0 || limit < 1 || limit > 100 {
		return nil, access.ErrInvalid
	}
	rows, err := s.Queries.ListAudit(
		ctx,
		dbgen.ListAuditParams{EventID: &event, Sequence: cursor, Limit: limit},
	)
	if err != nil {
		return nil, fmt.Errorf("query audit events: %w", err)
	}
	return rows, nil
}
