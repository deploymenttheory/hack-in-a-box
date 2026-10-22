package teams

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

type AllocationRequest struct {
	Seed         string                `json:"seed"`
	Limits       AllocationLimits      `json:"limits"`
	Declarations []AllocationCandidate `json:"declarations"`
}

// Suggest reads one current roster under the same lock as registration,
// membership and freeze changes. It writes neither draft nor published data.
// Declarations are explicitly selected skills/experience/pairings, never raw
// registration answers or operational profile fields inferred by their labels.
func (s Service) Suggest(
	ctx context.Context,
	actor, eventID uuid.UUID,
	revision int64,
	input AllocationRequest,
) (AllocationSuggestion, error) {
	var result AllocationSuggestion
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		row, err := q.LockEvent(ctx, eventID)
		if errors.Is(err, pgx.ErrNoRows) {
			return access.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock allocation snapshot: %w", err)
		}
		if err = (access.EventPolicy{Queries: q}).Authorize(
			ctx,
			actor,
			eventID,
			eventID,
			access.EventEdit,
		); err != nil {
			return fmt.Errorf("authorize allocation suggestion: %w", err)
		}
		if row.Revision != revision {
			return access.ErrConflict
		}
		c, err := config(row)
		if err != nil {
			return err
		}
		if frozen(row, c, s.now()) {
			return ErrFrozen
		}
		if !slices.Contains([]string{"registration", "preparation"}, row.Stage) {
			return access.ErrInvalid
		}
		if err = verifyAllocationLocks(ctx, q, row.ID, uuid.Nil); err != nil {
			return err
		}
		snapshot, err := allocationRoster(ctx, q, row, input.Declarations)
		if err != nil {
			return err
		}
		result, err = allocate(snapshot, input.Seed, input.Limits)
		return err
	})
	if err != nil {
		return result, fmt.Errorf("suggest team allocation: %w", err)
	}
	return result, nil
}

func allocationRoster(
	ctx context.Context,
	q *dbgen.Queries,
	row dbgen.Event,
	declarations []AllocationCandidate,
) (allocationSnapshot, error) {
	result := allocationSnapshot{
		EventID:  row.ID,
		Revision: row.Revision,
		People:   []AllocationCandidate{},
		Groups:   []AllocationGroup{},
	}
	roster, err := q.ConfirmedTeamPeople(ctx, row.ID)
	if err != nil {
		return result, fmt.Errorf("read confirmed allocation pool: %w", err)
	}
	eligible := make(map[uuid.UUID]bool, len(roster))
	for _, person := range roster {
		eligible[person.ID] = true
	}
	features := make(map[uuid.UUID]AllocationCandidate, len(declarations))
	for _, declaration := range declarations {
		if _, duplicate := features[declaration.ID]; duplicate || !eligible[declaration.ID] {
			return result, access.ErrInvalid
		}
		features[declaration.ID] = declaration
	}
	for _, person := range roster {
		candidate, provided := features[person.ID]
		if !provided {
			candidate = AllocationCandidate{
				ID:         person.ID,
				Skills:     []string{},
				PreferWith: []uuid.UUID{},
			}
		}
		result.People = append(result.People, candidate)
	}
	groups, err := q.HackTeams(ctx, row.ID)
	if err != nil {
		return result, fmt.Errorf("read allocation groups: %w", err)
	}
	members, err := q.HackTeamMembers(ctx, row.ID)
	if err != nil {
		return result, fmt.Errorf("read allocation memberships: %w", err)
	}
	for _, group := range groups {
		if group.State == "disbanded" {
			continue
		}
		if group.Frozen {
			return result, ErrFrozen
		}
		g := AllocationGroup{
			ID:       group.ID,
			Locked:   group.AllocationLocked,
			Members:  []uuid.UUID{},
			Capacity: capacityPointer(group.Capacity),
		}
		for _, member := range members {
			if member.TeamID == group.ID && member.Snapshot == "draft" {
				if !eligible[member.PersonID] {
					return result, ErrUnready
				}
				g.Members = append(g.Members, member.PersonID)
			}
		}
		result.Groups = append(result.Groups, g)
	}
	return result, nil
}

func capacityPointer(capacity pgtype.Int4) *int {
	if !capacity.Valid {
		return nil
	}
	value := int(capacity.Int32)
	return &value
}

func allocationControl(
	ctx context.Context,
	q *dbgen.Queries,
	eventID uuid.UUID,
	command Command,
) error {
	group, err := q.HackTeam(ctx, dbgen.HackTeamParams{EventID: eventID, ID: command.TeamID})
	if errors.Is(err, pgx.ErrNoRows) || group.State == "disbanded" {
		return access.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("load allocation control: %w", err)
	}
	if group.Frozen {
		return ErrFrozen
	}
	if command.Action != "capacity" {
		if command.Action == "lock" {
			if err = verifyAllocationLocks(ctx, q, eventID, group.ID); err != nil {
				return err
			}
		}
		if err = q.SetAllocationLock(
			ctx,
			dbgen.SetAllocationLockParams{
				EventID:          eventID,
				ID:               group.ID,
				AllocationLocked: command.Action == "lock",
			},
		); err != nil {
			return fmt.Errorf("set allocation lock: %w", err)
		}
		return nil
	}
	capacity := pgtype.Int4{}
	if command.Capacity != nil {
		if *command.Capacity < 3 || *command.Capacity > 10000 {
			return access.ErrInvalid
		}
		count, err := draftMemberCount(ctx, q, eventID, group.ID)
		if err != nil {
			return err
		}
		if count > *command.Capacity {
			return ErrCapacity
		}
		capacity = pgtype.Int4{Int32: int32(*command.Capacity), Valid: true}
	}
	if err = q.SetTeamCapacity(
		ctx,
		dbgen.SetTeamCapacityParams{EventID: eventID, ID: group.ID, Capacity: capacity},
	); err != nil {
		return fmt.Errorf("set team capacity: %w", err)
	}
	return nil
}

// Published locked cohorts cannot be replaced by a previously edited draft.
// Withdrawals remove both snapshots and therefore do not require an unlock.
func verifyAllocationLocks(
	ctx context.Context,
	q *dbgen.Queries,
	eventID, locking uuid.UUID,
) error {
	groups, err := q.HackTeams(ctx, eventID)
	if err != nil {
		return fmt.Errorf("read publication locks: %w", err)
	}
	locked := map[uuid.UUID]bool{}
	for _, group := range groups {
		locked[group.ID] = group.AllocationLocked || group.ID == locking
	}
	members, err := q.HackTeamMembers(ctx, eventID)
	if err != nil {
		return fmt.Errorf("read locked membership: %w", err)
	}
	draft := map[uuid.UUID]uuid.UUID{}
	for _, member := range members {
		if member.Snapshot == "draft" {
			draft[member.PersonID] = member.TeamID
		}
	}
	for _, member := range members {
		if member.Snapshot == "published" && locked[member.TeamID] &&
			draft[member.PersonID] != member.TeamID {
			return ErrLocked
		}
	}
	return nil
}

func draftMemberCount(
	ctx context.Context,
	q *dbgen.Queries,
	eventID, teamID uuid.UUID,
) (int, error) {
	members, err := q.HackTeamMembers(ctx, eventID)
	if err != nil {
		return 0, fmt.Errorf("count draft members: %w", err)
	}
	count := 0
	for _, member := range members {
		if member.TeamID == teamID && member.Snapshot == "draft" {
			count++
		}
	}
	return count, nil
}

func checkAllocationMove(
	ctx context.Context,
	q *dbgen.Queries,
	eventID uuid.UUID,
	destination dbgen.HackTeam,
	person uuid.UUID,
) error {
	if destination.Frozen {
		return ErrFrozen
	}
	current, err := q.TeamAssignment(
		ctx,
		dbgen.TeamAssignmentParams{EventID: eventID, PersonID: person, Snapshot: "draft"},
	)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("read source assignment: %w", err)
	}
	if err == nil {
		if current.TeamID == destination.ID {
			return nil
		}
		source, err := q.HackTeam(ctx, dbgen.HackTeamParams{EventID: eventID, ID: current.TeamID})
		if err != nil {
			return fmt.Errorf("read source allocation lock: %w", err)
		}
		if source.Frozen {
			return ErrFrozen
		}
		if source.AllocationLocked {
			return ErrLocked
		}
	}
	if destination.Capacity.Valid {
		count, err := draftMemberCount(ctx, q, eventID, destination.ID)
		if err != nil {
			return err
		}
		if count >= int(destination.Capacity.Int32) {
			return ErrCapacity
		}
	}
	return nil
}
