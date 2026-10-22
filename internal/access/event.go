package access

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

const (
	TeamFileWrite    Action = "team.file.write"
	EventRead        Action = "event.read"
	RosterRead       Action = "attendance.read"
	AttendanceManage Action = "attendance.manage"
	EventEdit        Action = "event.edit"
	StaffManage      Action = "staff.manage"
	StaffRead        Action = "staff.read"
	OwnershipManage  Action = "ownership.manage"
	TeamRead         Action = "team.read"
	AssessmentRead   Action = "assessment.read"
	CredentialRead   Action = "credential.read" // #nosec G101 -- Policy action name; contains no credential.
	BriefRead        Action = "brief.read"
	PresentationRead Action = "presentation.read"
	LogisticsRead    Action = "logistics.read"
	MediaRead        Action = "media.read"
	ProfileRead      Action = "profile.read"
	ProfileExport    Action = "profile.export"
	ProfileManage    Action = "profile.manage"
)

// EventPolicy always reads current authority. Platform administration permits
// ownership management, never implicit access to event content or assessments.
// Use WithQueries within a locked transaction for mutations.
type EventPolicy struct {
	Queries *dbgen.Queries
	Clock   func() time.Time
}

func (p EventPolicy) WithQueries(q *dbgen.Queries) Policy { p.Queries = q; return p }

func (p EventPolicy) Authorize(
	ctx context.Context,
	actor, event, target uuid.UUID,
	action Action,
) error {
	person, err := p.Queries.Person(ctx, actor)
	if err != nil || !person.Active {
		return ErrForbidden
	}
	record, err := p.Queries.Event(ctx, event)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("load policy event: %w", err)
	}
	roles, err := p.Queries.ActiveRoles(
		ctx,
		dbgen.ActiveRolesParams{EventID: event, PersonID: actor},
	)
	if err != nil {
		return fmt.Errorf("load current grants: %w", err)
	}
	organiser := slices.Contains(roles, "organiser")
	if action == OwnershipManage {
		if person.PlatformAdmin ||
			(record.OwnerID != nil && *record.OwnerID == actor && organiser) {
			return nil
		}
		return ErrForbidden
	}
	switch action {
	case RosterRead, AttendanceManage:
		return attendancePolicy(action, roles, record.WritesAllowed)
	}
	if action == EventRead {
		visible, err := p.Queries.VisibleEvent(
			ctx,
			dbgen.VisibleEventParams{EventID: event, PersonID: actor},
		)
		if err != nil {
			return fmt.Errorf("check event membership: %w", err)
		}
		if (visible.Valid && visible.Bool) || person.PlatformAdmin {
			return nil
		}
		return ErrForbidden
	}
	if action == EventEdit || action == StaffManage || action == FileWrite {
		if organiser && record.WritesAllowed {
			return nil
		}
		return ErrForbidden
	}
	if action == AuditRead || action == AuditExport || action == StaffRead {
		if organiser {
			return nil
		}
		return ErrForbidden
	}
	return p.content(
		ctx,
		actor,
		event,
		target,
		action,
		roles,
		record,
		organiser,
		person.PlatformAdmin,
	)
}

func (p EventPolicy) content(
	ctx context.Context,
	actor, event, target uuid.UUID,
	action Action,
	roles []string,
	record dbgen.Event,
	organiser, admin bool,
) error {
	if action == ProfileManage || action == ProfileRead || action == ProfileExport {
		return profilePolicy(action, roles, organiser, admin)
	}
	if action == TeamFileWrite {
		return p.teamFileWrite(ctx, actor, record, target)
	}
	if action == FileRead {
		return p.file(ctx, actor, record, target, organiser)
	}
	return p.resource(ctx, actor, event, target, action, roles)
}

func attendancePolicy(action Action, roles []string, writesAllowed bool) error {
	if !slices.Contains(roles, "organiser") && !slices.Contains(roles, "logistics") {
		return ErrForbidden
	}
	if action == AttendanceManage && !writesAllowed {
		return ErrForbidden
	}
	return nil
}

func (p EventPolicy) resource(
	ctx context.Context,
	actor, event, target uuid.UUID,
	action Action,
	roles []string,
) error {
	resource, err := p.Queries.PolicyResource(
		ctx,
		dbgen.PolicyResourceParams{EventID: event, ID: target},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("load resource scope: %w", err)
	}
	assigned, err := p.Queries.ResourceAssigned(
		ctx,
		dbgen.ResourceAssignedParams{EventID: event, ResourceID: target, PersonID: actor},
	)
	if err != nil {
		return fmt.Errorf("load resource assignment: %w", err)
	}
	permitted := false
	switch action {
	case TeamRead:
		permitted = resource.Kind == "team" &&
			(slices.Contains(roles, "organiser") || (assigned && slices.Contains(roles, "mentor")))
	case AssessmentRead:
		permitted = resource.Kind == "assessment" && assigned &&
			(slices.Contains(roles, "mentor") || slices.Contains(roles, "judge"))
	case CredentialRead:
		visible, err := p.Queries.VisibleEvent(
			ctx,
			dbgen.VisibleEventParams{EventID: event, PersonID: actor},
		)
		if err != nil {
			return fmt.Errorf("check credential membership: %w", err)
		}
		permitted = resource.Kind == "credential" && assigned && visible.Bool
	case BriefRead:
		permitted = resource.Kind == "brief" &&
			(slices.Contains(roles, "organiser") || slices.Contains(roles, "early_brief_access"))
	case PresentationRead:
		permitted = resource.Kind == "presentation" && slices.Contains(roles, "mc")
	case LogisticsRead:
		permitted = resource.Kind == "logistics" && slices.Contains(roles, "logistics")
	case MediaRead:
		permitted = resource.Kind == "media" && slices.Contains(roles, "photographer")
	}
	if permitted {
		return nil
	}
	return ErrForbidden
}

func (p EventPolicy) file(
	ctx context.Context,
	actor uuid.UUID,
	row dbgen.Event,
	target uuid.UUID,
	organiser bool,
) error {
	event := row.ID
	file, err := p.Queries.File(ctx, dbgen.FileParams{EventID: event, ID: target})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("check file scope: %w", err)
	}
	if file.TeamID != nil {
		return p.teamFileRead(ctx, actor, row, file)
	}
	if organiser && file.EventID == event {
		return nil
	}
	return ErrForbidden
}

func profilePolicy(action Action, roles []string, organiser, admin bool) error {
	if action == ProfileManage {
		if organiser || admin {
			return nil
		}
		return ErrForbidden
	}
	if action == ProfileRead && slices.Contains(roles, "photographer") {
		return nil
	}
	if action == ProfileRead || action == ProfileExport {
		if slices.Contains(roles, "logistics") {
			return nil
		}
		return ErrForbidden
	}
	return ErrForbidden
}

func (p EventPolicy) now() time.Time {
	if p.Clock != nil {
		return p.Clock()
	}
	return time.Now()
}
