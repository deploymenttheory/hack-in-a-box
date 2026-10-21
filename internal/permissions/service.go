// Package permissions owns event grants and lasting competition eligibility.
package permissions

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

var (
	ErrWithdrawal = errors.New(
		"withdraw the active attendee registration before granting this role",
	)
	ErrIneligible = errors.New(
		"previous organiser or early brief access permanently excludes competition in this event",
	)
	ErrLastOrganiser = errors.New(
		"assign another organiser and transfer ownership before removing this organiser",
	)
	ErrAssessment = errors.New("competing and assessing in the same event are incompatible")
)

var Roles = []string{
	"organiser",
	"mentor",
	"judge",
	"mc",
	"logistics",
	"photographer",
	"early_brief_access",
}

type (
	Service struct{ Pool *pgxpool.Pool }
	Change  struct {
		Actor, Event, Person              uuid.UUID
		Role                              string
		Active                            bool
		AcknowledgeCompetitionRestriction bool
		Revision                          int64
	}
)

func (s Service) List(ctx context.Context, actor, event uuid.UUID) ([]dbgen.MembershipsRow, error) {
	q := dbgen.New(s.Pool)
	if err := (access.EventPolicy{Queries: q}).Authorize(
		ctx,
		actor,
		event,
		event,
		access.StaffRead,
	); err != nil {
		return nil, fmt.Errorf("authorize staff: %w", err)
	}
	rows, err := q.Memberships(ctx, event)
	if err != nil {
		return nil, fmt.Errorf("list staff: %w", err)
	}
	return rows, nil
}

func (s Service) Change(ctx context.Context, c Change) (dbgen.Event, error) {
	var result dbgen.Event
	if !slices.Contains(Roles, c.Role) || c.Person == uuid.Nil {
		return result, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		event, err := q.LockEvent(ctx, c.Event)
		if err != nil {
			return fmt.Errorf("lock event: %w", err)
		}
		if err := (access.EventPolicy{Queries: q}).Authorize(
			ctx,
			c.Actor,
			c.Event,
			c.Event,
			access.StaffManage,
		); err != nil {
			return fmt.Errorf("authorize grant: %w", err)
		}
		if event.Revision != c.Revision {
			return access.ErrConflict
		}
		target, err := q.Person(ctx, c.Person)
		if err != nil || !target.Active {
			return access.ErrNotFound
		}
		if c.Active {
			if err := checkGrant(ctx, q, c); err != nil {
				return err
			}
		} else if c.Role == "organiser" {
			roles, err := q.ActiveRoles(
				ctx,
				dbgen.ActiveRolesParams{EventID: c.Event, PersonID: c.Person},
			)
			if err != nil {
				return fmt.Errorf("read roles: %w", err)
			}
			count, err := q.ActiveOrganisers(ctx, c.Event)
			if err != nil {
				return fmt.Errorf("count organisers: %w", err)
			}
			if (event.OwnerID != nil && *event.OwnerID == c.Person) ||
				(slices.Contains(roles, "organiser") && count <= 1) {
				return ErrLastOrganiser
			}
		}
		if err := q.SetMembership(
			ctx,
			dbgen.SetMembershipParams{
				EventID:  c.Event,
				PersonID: c.Person,
				Role:     c.Role,
				Active:   c.Active,
			},
		); err != nil {
			return fmt.Errorf("save grant: %w", err)
		}
		result, err = q.BumpEventRevision(ctx, c.Event)
		if err != nil {
			return fmt.Errorf("advance revision: %w", err)
		}
		action := audit.RoleRevoked
		if c.Active {
			action = audit.RoleGranted
		}
		return audit.Append(
			ctx,
			q,
			audit.Record{
				ActorID:  &c.Actor,
				EventID:  &c.Event,
				TargetID: c.Person,
				Action:   action,
				Reason:   audit.OrganiserDecision,
				Metadata: audit.Metadata{Revision: &result.Revision, Role: &c.Role},
			},
		)
	})
	if err != nil {
		return result, fmt.Errorf("change event authority: %w", err)
	}
	return result, nil
}

func checkGrant(ctx context.Context, q *dbgen.Queries, c Change) error {
	state, err := q.CompetitionState(
		ctx,
		dbgen.CompetitionStateParams{EventID: c.Event, PersonID: c.Person},
	)
	if err != nil {
		return fmt.Errorf("check registration: %w", err)
	}
	active := state != "" && state != "withdrawn"
	switch c.Role {
	case "organiser", "early_brief_access":
		if active {
			return ErrWithdrawal
		}
		if !c.AcknowledgeCompetitionRestriction {
			return access.ErrInvalid
		}
		if err := q.ExcludeCompetition(
			ctx,
			dbgen.ExcludeCompetitionParams{EventID: c.Event, PersonID: c.Person, Source: c.Role},
		); err != nil {
			return fmt.Errorf("persist competition exclusion: %w", err)
		}
	case "mentor", "judge":
		if active {
			return ErrAssessment
		}
	}
	return nil
}

// SetRegistrationState is the registration domain's transactional boundary.
// Call with its existing transaction; grant changes take this same event lock.
// No public endpoint can manufacture confirmed attendance through this function.
func SetRegistrationState(
	ctx context.Context,
	q *dbgen.Queries,
	event, person uuid.UUID,
	state string,
) error {
	if !slices.Contains([]string{"received", "waitlisted", "confirmed", "withdrawn"}, state) {
		return access.ErrInvalid
	}
	if _, err := q.LockEvent(ctx, event); err != nil {
		return fmt.Errorf("lock eligibility: %w", err)
	}
	if state != "withdrawn" {
		excluded, err := q.CompetitionExcluded(
			ctx,
			dbgen.CompetitionExcludedParams{EventID: event, PersonID: person},
		)
		if err != nil {
			return fmt.Errorf("check exclusion: %w", err)
		}
		if excluded {
			return ErrIneligible
		}
		roles, err := q.ActiveRoles(ctx, dbgen.ActiveRolesParams{EventID: event, PersonID: person})
		if err != nil {
			return fmt.Errorf("check assessment grants: %w", err)
		}
		if slices.Contains(roles, "mentor") || slices.Contains(roles, "judge") {
			return ErrAssessment
		}
	}
	if err := q.SetCompetitionState(
		ctx,
		dbgen.SetCompetitionStateParams{EventID: event, PersonID: person, State: state},
	); err != nil {
		return fmt.Errorf("save registration fact: %w", err)
	}
	return nil
}

// AssignOwner is also used atomically when creating an event. The caller holds
// the event lock and has checked ownership authority. It never removes grants.
func AssignOwner(ctx context.Context, q *dbgen.Queries, event, person uuid.UUID) error {
	if err := checkGrant(
		ctx,
		q,
		Change{
			Event:                             event,
			Person:                            person,
			Role:                              "organiser",
			Active:                            true,
			AcknowledgeCompetitionRestriction: true,
		},
	); err != nil {
		return err
	}
	if err := q.SetMembership(
		ctx,
		dbgen.SetMembershipParams{
			EventID:  event,
			PersonID: person,
			Role:     "organiser",
			Active:   true,
		},
	); err != nil {
		return fmt.Errorf("grant owner role: %w", err)
	}
	if err := q.SetEventOwner(
		ctx,
		dbgen.SetEventOwnerParams{ID: event, OwnerID: &person},
	); err != nil {
		return fmt.Errorf("assign owner: %w", err)
	}
	return nil
}

func (s Service) Transfer(ctx context.Context, c Change) (dbgen.Event, error) {
	var result dbgen.Event
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		event, err := q.LockEvent(ctx, c.Event)
		if err != nil {
			return fmt.Errorf("lock ownership: %w", err)
		}
		if err := (access.EventPolicy{Queries: q}).Authorize(
			ctx,
			c.Actor,
			c.Event,
			c.Event,
			access.OwnershipManage,
		); err != nil {
			return fmt.Errorf("authorize transfer: %w", err)
		}
		if !event.WritesAllowed {
			return access.ErrForbidden
		}
		if event.Revision != c.Revision {
			return access.ErrConflict
		}
		person, err := q.Person(ctx, c.Person)
		if err != nil || !person.Active {
			return access.ErrNotFound
		}
		if !c.AcknowledgeCompetitionRestriction {
			return access.ErrInvalid
		}
		if err := AssignOwner(ctx, q, c.Event, c.Person); err != nil {
			return err
		}
		result, err = q.BumpEventRevision(ctx, c.Event)
		if err != nil {
			return fmt.Errorf("advance owner revision: %w", err)
		}
		return audit.Append(
			ctx,
			q,
			audit.Record{
				ActorID:  &c.Actor,
				EventID:  &c.Event,
				TargetID: c.Person,
				Action:   audit.OwnerTransferred,
				Reason:   audit.OrganiserDecision,
			},
		)
	})
	if err != nil {
		return result, fmt.Errorf("change event authority: %w", err)
	}
	return result, nil
}
