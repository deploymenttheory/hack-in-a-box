package repositoryaccess

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/githubapp"
)

type Command struct {
	Action      string    `json:"action"`
	PlanID      uuid.UUID `json:"plan_id"`
	PlanVersion int64     `json:"plan_version"`
	Reason      string    `json:"reason"`
}
type Result struct {
	Receipt *Plan `json:"receipt"`
	Current View  `json:"current"`
}

func authorize(ctx context.Context, q *dbgen.Queries, actor, eventID uuid.UUID, write bool) error {
	action := access.StaffRead
	if write {
		action = access.EventEdit
	}
	return wrap(
		"authorize current repository access",
		(access.EventPolicy{Queries: q}).Authorize(ctx, actor, eventID, eventID, action),
	)
}

func repository(
	ctx context.Context,
	q *dbgen.Queries,
	eventID, teamID uuid.UUID,
) (dbgen.TeamRepository, error) {
	row, err := q.TeamRepository(ctx, dbgen.TeamRepositoryParams{EventID: eventID, TeamID: teamID})
	if errors.Is(err, pgx.ErrNoRows) {
		return row, access.ErrNotFound
	}
	if err != nil {
		return row, fmt.Errorf("read native repository mapping: %w", err)
	}
	if row.State != "ready" || !row.GithubRepositoryID.Valid {
		return row, access.ErrConflict
	}
	return row, nil
}

func (s Service) view(
	ctx context.Context,
	tx pgx.Tx,
	e dbgen.Event,
	r dbgen.TeamRepository,
	actor uuid.UUID,
) (View, error) {
	q := dbgen.New(tx)
	out := View{
		Revision:   e.Revision,
		TeamID:     r.TeamID,
		Grants:     []Grant{},
		Flags:      []Flag{},
		Execution:  []Execution{},
		CanManage:  e.WritesAllowed && authorize(ctx, q, actor, e.ID, true) == nil,
		GrantsOpen: grantsOpen(e, s.now()),
	}
	plan, err := readPlan(ctx, tx, e.ID, r.TeamID)
	if err != nil {
		return out, wrap("repository access request", err)
	}
	out.Plan = plan
	if plan != nil {
		rows, readErr := tx.Query(
			ctx,
			"SELECT id,state,diagnostic,attempted,updated_at FROM repository_access_effects WHERE plan_id=$1 ORDER BY id",
			plan.ID,
		)
		if readErr != nil {
			return out, fmt.Errorf("read current access execution: %w", readErr)
		}
		for rows.Next() {
			var effect Execution
			if readErr = rows.Scan(
				&effect.ActionID,
				&effect.State,
				&effect.Diagnostic,
				&effect.Attempted,
				&effect.UpdatedAt,
			); readErr != nil {
				rows.Close()
				return out, fmt.Errorf("read access effect status: %w", readErr)
			}
			out.Execution = append(out.Execution, effect)
		}
		rows.Close()
		if readErr = rows.Err(); readErr != nil {
			return out, fmt.Errorf("read access effect status page: %w", readErr)
		}
	}

	out.Grants, err = readGrants(ctx, tx, e.ID, r.TeamID)
	if err != nil {
		return out, wrap("repository access request", err)
	}
	source, observed, diagnostic, err := readObservation(ctx, tx, e.ID, r.TeamID)
	if err != nil {
		return out, wrap("repository access request", err)
	}
	out.ObservedAt = observed
	members, flags, err := desired(ctx, tx, q, e, r, actor, s.now())
	if err != nil {
		return out, wrap("repository access request", err)
	}
	out.Flags = flags
	_, providerFlags := propose(members, out.Grants, source)
	out.Flags = append(out.Flags, providerFlags...)
	if diagnostic != "" {
		out.Flags = append(out.Flags, Flag{Code: diagnostic})
	}
	if plan != nil &&
		plan.Fingerprint != fingerprint(r.GithubRepositoryID.Int64, r.Provenance, members) {
		out.Flags = append(out.Flags, Flag{Code: "plan_stale"})
	}
	if !out.GrantsOpen {
		out.Flags = append(out.Flags, Flag{Code: "challenge_publication_required_for_new_access"})
	}
	return out, nil
}

func (s Service) Read(ctx context.Context, actor, eventID, teamID uuid.UUID) (View, error) {
	var out View
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		e, err := q.LockEvent(ctx, eventID)
		if err != nil {
			return fmt.Errorf("lock repository access read: %w", err)
		}
		if err = authorize(ctx, q, actor, eventID, false); err != nil {
			return err
		}
		r, err := repository(ctx, q, eventID, teamID)
		if err != nil {
			return err
		}
		out, err = s.view(ctx, tx, e, r, actor)
		return err
	})
	return out, wrap("repository access request", err)
}

func (s Service) Change(
	ctx context.Context,
	actor, eventID, teamID uuid.UUID,
	revision int64,
	command Command,
) (Result, error) {
	var out Result
	if command.Action != "review" && command.Action != "approve" && command.Action != "refresh" {
		return out, access.ErrInvalid
	}
	if strings.TrimSpace(command.Reason) == "" || len(command.Reason) > 2000 {
		return out, access.ErrInvalid
	}
	if command.Action != "refresh" && command.PlanID == uuid.Nil {
		return out, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		e, err := q.LockEvent(ctx, eventID)
		if err != nil {
			return fmt.Errorf("lock repository access change: %w", err)
		}
		if err = authorize(ctx, q, actor, eventID, true); err != nil {
			return err
		}
		if e.Revision != revision {
			return access.ErrConflict
		}
		r, err := repository(ctx, q, eventID, teamID)
		if err != nil {
			return err
		}
		out.Receipt, err = s.changeLocked(ctx, tx, q, e, r, actor, command)
		if err != nil {
			return err
		}
		metadata := audit.Metadata{}
		if out.Receipt != nil {
			metadata.ReferenceID = &out.Receipt.ID
		}
		if err = audit.Append(
			ctx,
			q,
			audit.Record{
				ActorID:  &actor,
				EventID:  &eventID,
				TargetID: teamID,
				Action:   audit.RepositoryAccessChanged,
				Reason:   audit.OrganiserDecision,
				Metadata: metadata,
			},
		); err != nil {
			return wrap("audit repository access review", err)
		}
		out.Current, err = s.view(ctx, tx, e, r, actor)
		return err
	})
	return out, wrap("repository access request", err)
}

func (s Service) refresh(
	ctx context.Context,
	tx pgx.Tx,
	q *dbgen.Queries,
	e dbgen.Event,
	r dbgen.TeamRepository,
) error {
	if s.App == nil {
		return access.ErrConflict
	}
	integration, err := q.GitHubAppIntegration(ctx, e.ID)
	if err != nil {
		return fmt.Errorf("read current repository App binding: %w", err)
	}
	source, err := s.App.InspectCollaborators(ctx, q, e.ID, request(r, integration))
	if err != nil {
		code := "provider_unavailable"
		if errors.Is(err, githubapp.ErrAccessRejected) {
			code = "organization_policy_restricted"
		}
		_, writeErr := tx.Exec(
			ctx,
			`INSERT INTO repository_access_observations(event_id,team_id,snapshot,diagnostic)VALUES($1,$2,'{"collaborators":[],"invitations":[]}',$3)ON CONFLICT(event_id,team_id)DO UPDATE SET diagnostic=EXCLUDED.diagnostic`,
			e.ID,
			r.TeamID,
			code,
		)
		if writeErr != nil {
			return fmt.Errorf("retain provider access diagnostic: %w", writeErr)
		}
		return nil
	}
	grants, err := readGrants(ctx, tx, e.ID, r.TeamID)
	if err != nil {
		return err
	}
	if err = observeGrants(ctx, tx, grants, source, s.now()); err != nil {
		return err
	}
	return saveObservation(ctx, tx, e.ID, r.TeamID, source, s.now())
}

func (s Service) createPlan(
	ctx context.Context,
	tx pgx.Tx,
	q *dbgen.Queries,
	e dbgen.Event,
	r dbgen.TeamRepository,
	actor, id uuid.UUID,
	force bool,
	reason string,
) (*Plan, error) {
	previous, err := readPlan(ctx, tx, e.ID, r.TeamID)
	if err != nil {
		return nil, err
	}
	members, flags, err := desired(ctx, tx, q, e, r, actor, s.now())
	if err != nil {
		return nil, err
	}
	hash := fingerprint(r.GithubRepositoryID.Int64, r.Provenance, members)
	source, _, diagnostic, err := readObservation(ctx, tx, e.ID, r.TeamID)
	if err != nil {
		return nil, err
	}
	grants, err := readGrants(ctx, tx, e.ID, r.TeamID)
	if err != nil {
		return nil, err
	}
	actions, providerFlags := propose(members, grants, source)
	flags = append(flags, providerFlags...)
	if diagnostic != "" {
		flags = append(flags, Flag{Code: diagnostic})
		actions = []Action{}
	}
	if previous != nil && previous.Fingerprint == hash && !force {
		if previous.ApprovedAt != nil ||
			proposalFingerprint(
				previous.Actions,
				previous.Flags,
			) == proposalFingerprint(
				actions,
				flags,
			) {
			return previous, nil
		}
	}
	version := int64(1)
	if previous != nil {
		version = previous.Version + 1
	}
	plan := Plan{
		ID:           id,
		Version:      version,
		EventID:      e.ID,
		TeamID:       r.TeamID,
		RepositoryID: r.GithubRepositoryID.Int64,
		Fingerprint:  hash,
		Members:      members,
		Actions:      actions,
		Flags:        flags,
		CreatedAt:    s.now(),
		Reason:       reason,
	}
	if err = savePlan(ctx, tx, actor, plan); err != nil {
		return nil, err
	}
	return &plan, nil
}

func (s Service) queue(
	ctx context.Context,
	tx pgx.Tx,
	q *dbgen.Queries,
	e dbgen.Event,
	r dbgen.TeamRepository,
	plan Plan,
) error {
	if plan.ApprovedAt == nil || plan.ApprovedBy == nil {
		return nil
	}
	if err := authorize(ctx, q, *plan.ApprovedBy, e.ID, true); err != nil {
		return nil
	}
	members, _, err := desired(ctx, tx, q, e, r, *plan.ApprovedBy, s.now())
	if err != nil {
		return err
	}
	if plan.Fingerprint != fingerprint(r.GithubRepositoryID.Int64, r.Provenance, members) {
		return nil
	}
	for _, action := range plan.Actions {
		if action.Kind == "invite" {
			if !grantsOpen(e, s.now()) {
				continue
			}
			grants, err := readGrants(ctx, tx, e.ID, r.TeamID)
			if err != nil {
				return err
			}
			blocked := false
			for _, grant := range grants {
				if grant.Member.GitHubID == action.Member.GitHubID && grant.ID != action.ID &&
					(grant.State == "pending" || grant.State == "accepted" || grant.State == "ambiguous" || grant.State == "expired") {
					blocked = true
				}
			}
			if blocked {
				continue
			}
		}
		var state string
		if err = tx.QueryRow(ctx, `SELECT state FROM repository_access_effects WHERE id=$1`, action.ID).
			Scan(&state); err != nil {
			return fmt.Errorf("read planned access effect: %w", err)
		}
		if state != "planned" {
			continue
		}
		if _, err = q.InsertOperation(
			ctx,
			dbgen.InsertOperationParams{
				EventID:      e.ID,
				TargetID:     action.ID,
				Kind:         HandlerKind,
				OperationKey: "repository-access:" + action.ID.String(),
				DueAt:        plan.CreatedAt,
				Approved:     true,
			},
		); err != nil {
			return fmt.Errorf("queue approved repository access: %w", err)
		}
	}
	return nil
}

// Maintain records changed native rosters as unapproved plans. It never treats
// polling or a new role/account as approval to grant external access.

func (s Service) Maintain(ctx context.Context) error {
	rows, err := s.Pool.Query(
		ctx,
		"SELECT DISTINCT r.event_id FROM team_repositories r JOIN events e ON e.id=r.event_id WHERE e.writes_allowed AND r.state='ready' ORDER BY r.event_id",
	)
	if err != nil {
		return fmt.Errorf("read repository access maintenance events: %w", err)
	}
	ids := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return fmt.Errorf("read repository access event reference: %w", err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return fmt.Errorf("read repository access event references: %w", err)
	}
	for _, id := range ids {
		err = database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
			e, err := q.LockEvent(ctx, id)
			if err != nil {
				return fmt.Errorf("lock repository access maintenance: %w", err)
			}
			if e.OwnerID == nil || authorize(ctx, q, *e.OwnerID, e.ID, true) != nil {
				return nil
			}
			mappings, err := q.EventRepositories(ctx, e.ID)
			if err != nil {
				return fmt.Errorf("read repository access mappings: %w", err)
			}
			for _, mapping := range mappings {
				if mapping.State != "ready" || !mapping.GithubRepositoryID.Valid {
					continue
				}
				r, err := repository(ctx, q, e.ID, mapping.TeamID)
				if err != nil {
					return err
				}
				_, observed, _, err := readObservation(ctx, tx, e.ID, r.TeamID)
				if err != nil {
					return err
				}
				if observed == nil || !s.now().Before(observed.Add(time.Minute)) {
					due := s.now().Truncate(time.Minute)
					if _, err = q.InsertOperation(
						ctx,
						dbgen.InsertOperationParams{
							EventID:  e.ID,
							TargetID: r.TeamID,
							Kind:     PollKind,
							OperationKey: fmt.Sprintf(
								"repository-access-poll:%s:%s",
								r.TeamID,
								due.Format(time.RFC3339),
							),
							DueAt:    due,
							Approved: true,
						},
					); err != nil {
						return fmt.Errorf("queue bounded repository access observation: %w", err)
					}
				}
				plan, err := s.createPlan(
					ctx,
					tx,
					q,
					e,
					r,
					*e.OwnerID,
					uuid.New(),
					false,
					"Current published roster and verified accounts",
				)
				if err != nil {
					return err
				}
				// queue independently requires this exact plan's approval, current approver
				// authority and a matching native roster fingerprint before any write job.
				if err = s.queue(ctx, tx, q, e, r, *plan); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return wrap("maintain current repository access", err)
		}
	}
	return nil
}
