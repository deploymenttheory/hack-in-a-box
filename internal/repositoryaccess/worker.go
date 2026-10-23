package repositoryaccess

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/githubapp"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
)

type effect struct {
	Action    Action
	PlanID    uuid.UUID
	TeamID    uuid.UUID
	State     string
	Attempted bool
	Dispatch  *uuid.UUID
}
type scope struct {
	Event      dbgen.Event
	Repository dbgen.TeamRepository
	Plan       Plan
	Effect     effect
}

func readEffect(ctx context.Context, tx pgx.Tx, eventID, id uuid.UUID) (effect, error) {
	var out effect
	var encoded []byte
	err := tx.QueryRow(ctx, `SELECT definition,plan_id,team_id,state,attempted,dispatch_id FROM repository_access_effects WHERE event_id=$1 AND id=$2 FOR UPDATE`, eventID, id).
		Scan(&encoded, &out.PlanID, &out.TeamID, &out.State, &out.Attempted, &out.Dispatch)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, access.ErrNotFound
	}
	if err != nil {
		return out, fmt.Errorf("read durable repository access effect: %w", err)
	}
	if err = json.Unmarshal(encoded, &out.Action); err != nil {
		return out, fmt.Errorf("decode durable repository access effect: %w", err)
	}
	return out, nil
}

func (s Service) currentScope(
	ctx context.Context,
	tx pgx.Tx,
	q *dbgen.Queries,
	d jobs.Delivery,
) (scope, bool, error) {
	var out scope
	e, err := q.LockEvent(ctx, d.EventID)
	if err != nil {
		return out, false, fmt.Errorf("lock repository access execution: %w", err)
	}
	out.Event = e
	out.Effect, err = readEffect(ctx, tx, e.ID, d.TargetID)
	if err != nil {
		return out, false, err
	}
	out.Repository, err = repository(ctx, q, e.ID, out.Effect.TeamID)
	if err != nil {
		return out, false, err
	}
	plan, err := readPlan(ctx, tx, e.ID, out.Repository.TeamID)
	if err != nil {
		return out, false, err
	}
	if plan == nil || plan.ID != out.Effect.PlanID || plan.ApprovedAt == nil ||
		plan.ApprovedBy == nil ||
		!e.WritesAllowed {
		return out, false, nil
	}
	out.Plan = *plan
	if err = authorize(ctx, q, *plan.ApprovedBy, e.ID, true); err != nil {
		return out, false, nil
	}
	if _, err = q.LockIdentity(ctx, out.Effect.Action.Member.PersonID); err != nil {
		return out, false, fmt.Errorf("lock current repository account authority: %w", err)
	}
	members, _, err := desired(ctx, tx, q, e, out.Repository, *plan.ApprovedBy, s.now())
	if err != nil {
		return out, false, err
	}
	if plan.Fingerprint != fingerprint(
		out.Repository.GithubRepositoryID.Int64,
		out.Repository.Provenance,
		members,
	) {
		return out, false, nil
	}
	action := out.Effect.Action
	if action.Kind == "invite" {
		if !grantsOpen(e, s.now()) ||
			!slices.ContainsFunc(
				members,
				func(member Member) bool { return sameMember(member, action.Member) },
			) {
			return out, false, nil
		}
		team, err := q.HackTeam(ctx, dbgen.HackTeamParams{EventID: e.ID, ID: out.Repository.TeamID})
		if err != nil {
			return out, false, fmt.Errorf("read current frozen access team: %w", err)
		}
		if !team.Frozen || team.State != "published" {
			return out, false, nil
		}
	} else if action.Kind != "remove" || action.GrantID == nil {
		return out, false, access.ErrInvalid
	}
	return out, true, nil
}

func (s Service) Eligible(ctx context.Context, d jobs.Delivery) (bool, error) {
	allowed := false
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		current, ok, err := s.currentScope(ctx, tx, q, d)
		if err != nil {
			return err
		}
		allowed = ok && (current.Effect.State == "planned" || current.Effect.State == "ambiguous")
		return nil
	})
	return allowed, wrap("check current repository effect eligibility", err)
}

func markEffect(ctx context.Context, tx pgx.Tx, id uuid.UUID, state, diagnostic string) error {
	_, err := tx.Exec(
		ctx,
		`UPDATE repository_access_effects SET state=$2,diagnostic=$3,updated_at=now()WHERE id=$1`,
		id,
		state,
		diagnostic,
	)
	if err != nil {
		return fmt.Errorf("retain repository access effect outcome: %w", err)
	}
	return nil
}

func (s Service) retainGrant(
	ctx context.Context,
	tx pgx.Tx,
	current scope,
	state, diagnostic string,
	managed bool,
	invitation *int64,
) error {
	action := current.Effect.Action
	_, err := tx.Exec(
		ctx,
		`INSERT INTO repository_access_grants(id,event_id,team_id,person_id,github_id,link_revision,role,permission,app_managed,state,invitation_id,diagnostic,observed_at)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)ON CONFLICT(id)DO UPDATE SET app_managed=repository_access_grants.app_managed OR EXCLUDED.app_managed,state=EXCLUDED.state,invitation_id=COALESCE(repository_access_grants.invitation_id,EXCLUDED.invitation_id),diagnostic=EXCLUDED.diagnostic,observed_at=EXCLUDED.observed_at,updated_at=now()`,
		action.ID,
		current.Event.ID,
		current.Repository.TeamID,
		action.Member.PersonID,
		action.Member.GitHubID,
		action.Member.LinkRevision,
		action.Member.Role,
		action.Member.Permission,
		managed,
		state,
		invitation,
		diagnostic,
		s.now(),
	)
	if err != nil {
		return fmt.Errorf("retain observed repository grant: %w", err)
	}
	return nil
}

func (s Service) source(
	ctx context.Context,
	tx pgx.Tx,
	q *dbgen.Queries,
	current scope,
) (githubapp.AccessSnapshot, error) {
	integration, err := q.GitHubAppIntegration(ctx, current.Event.ID)
	if err != nil {
		return githubapp.AccessSnapshot{}, fmt.Errorf("read current access installation: %w", err)
	}
	if s.App == nil {
		return githubapp.AccessSnapshot{}, access.ErrConflict
	}
	source, err := s.App.InspectCollaborators(
		ctx,
		q,
		current.Event.ID,
		request(current.Repository, integration),
	)
	if err != nil {
		return source, wrap("read current provider access", err)
	}
	if err = saveObservation(
		ctx,
		tx,
		current.Event.ID,
		current.Repository.TeamID,
		source,
		s.now(),
	); err != nil {
		return source, wrap("read current provider access", err)
	}
	return source, nil
}

func (s Service) preflight(
	ctx context.Context,
	tx pgx.Tx,
	current scope,
	source githubapp.AccessSnapshot,
) (bool, error) {
	action := current.Effect.Action
	if action.Kind == "invite" {
		for _, user := range source.Collaborators {
			if user.ID == action.Member.GitHubID {
				if err := s.retainGrant(
					ctx,
					tx,
					current,
					"accepted",
					"ownership_unproven",
					false,
					nil,
				); err != nil {
					return false, err
				}
				return false, markEffect(
					ctx,
					tx,
					action.ID,
					"delivered",
					"already_present_not_app_managed",
				)
			}
		}
		for _, invitation := range source.Invitations {
			if invitation.AccountID == action.Member.GitHubID {
				if err := s.retainGrant(
					ctx,
					tx,
					current,
					"pending",
					"ownership_unproven",
					false,
					&invitation.ID,
				); err != nil {
					return false, err
				}
				return false, markEffect(
					ctx,
					tx,
					action.ID,
					"delivered",
					"already_pending_not_app_managed",
				)
			}
		}
		return true, nil
	}
	grants, err := readGrants(ctx, tx, current.Event.ID, current.Repository.TeamID)
	if err != nil {
		return false, err
	}
	index := slices.IndexFunc(
		grants,
		func(g Grant) bool { return action.GrantID != nil && g.ID == *action.GrantID },
	)
	if index < 0 || !grants[index].Managed {
		return false, markEffect(ctx, tx, action.ID, "failed", "grant_not_app_managed")
	}
	grant := grants[index]
	for _, user := range source.Collaborators {
		if user.ID == action.Member.GitHubID {
			if user.Permission == permissionName(grant.Member.Permission) {
				return true, nil
			}
			return false, markEffect(ctx, tx, action.ID, "failed", "managed_permission_changed")
		}
	}
	for _, invitation := range source.Invitations {
		if invitation.AccountID == action.Member.GitHubID {
			if grant.InvitationID != nil && invitation.ID == *grant.InvitationID &&
				invitation.Permission == permissionName(grant.Member.Permission) {
				return true, nil
			}
			return false, markEffect(ctx, tx, action.ID, "failed", "invitation_ownership_changed")
		}
	}
	if err = updateGrant(
		ctx,
		tx,
		grant.ID,
		"revoked",
		"not_currently_present",
		s.now(),
	); err != nil {
		return false, err
	}
	return false, markEffect(ctx, tx, action.ID, "delivered", "")
}

func (s Service) barrier(ctx context.Context, tx pgx.Tx, current scope, dispatch uuid.UUID) error {
	action := current.Effect.Action
	changed, err := tx.Exec(
		ctx,
		`UPDATE repository_access_effects SET attempted=true,dispatch_id=$2,state='ambiguous',diagnostic='external_effect_uncertain',updated_at=now()WHERE id=$1 AND NOT attempted AND state='planned'`,
		action.ID,
		dispatch,
	)
	if err != nil {
		return fmt.Errorf("persist access dispatch barrier: %w", err)
	}
	if changed.RowsAffected() != 1 {
		return access.ErrConflict
	}
	if action.Kind == "invite" {
		if err = s.retainGrant(
			ctx,
			tx,
			current,
			"ambiguous",
			"external_effect_uncertain",
			false,
			nil,
		); err != nil {
			return err
		}
		_, err = tx.Exec(
			ctx,
			`INSERT INTO github_invitation_bindings(id,event_id,person_id,github_id,link_revision)VALUES($1,$2,$3,$4,$5)ON CONFLICT(id)DO NOTHING`,
			action.ID,
			current.Event.ID,
			action.Member.PersonID,
			action.Member.GitHubID,
			action.Member.LinkRevision,
		)
	} else {
		err = updateGrant(
			ctx,
			tx,
			*action.GrantID,
			"ambiguous",
			"external_effect_uncertain",
			s.now(),
		)
	}
	if err != nil {
		return fmt.Errorf("bind durable repository access attempt: %w", err)
	}
	return nil
}

// Deliver commits an immutable effect barrier before any provider write. Only
// the in-memory instance which won that barrier may cross the write boundary;
// process retries and crashes use read-only reconciliation instead.
func (s Service) Deliver(ctx context.Context, d jobs.Delivery) jobs.Outcome {
	dispatch := uuid.New()
	won, previous := false, false
	outcome := jobs.Retryable
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		current, allowed, err := s.currentScope(ctx, tx, q, d)
		if err != nil {
			return err
		}
		if !allowed {
			outcome = jobs.Permanent
			return nil
		}
		if current.Effect.Attempted {
			previous = true
			return nil
		}
		if current.Effect.State != "planned" {
			outcome = jobs.Delivered
			return nil
		}
		source, err := s.source(ctx, tx, q, current)
		if err != nil {
			return err
		}
		needed, err := s.preflight(ctx, tx, current, source)
		if err != nil {
			return err
		}
		if !needed {
			outcome = jobs.Delivered
			return nil
		}
		if err = s.barrier(ctx, tx, current, dispatch); err != nil {
			return err
		}
		won = true
		return nil
	})
	if err != nil {
		return jobs.Retryable
	}
	if previous {
		return s.Reconcile(ctx, d)
	}
	if !won {
		return outcome
	}
	return s.dispatch(ctx, d, dispatch)
}

func (s Service) dispatch(ctx context.Context, d jobs.Delivery, dispatch uuid.UUID) jobs.Outcome {
	outcome := jobs.Ambiguous
	beforeWrite := true
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		current, allowed, err := s.currentScope(ctx, tx, q, d)
		if err != nil {
			return err
		}
		if !current.Effect.Attempted || current.Effect.Dispatch == nil ||
			*current.Effect.Dispatch != dispatch {
			return access.ErrConflict
		}
		if !allowed {
			if err = s.noEffect(
				ctx,
				tx,
				d.EventID,
				current.Effect.Action,
				"authority_changed_before_dispatch",
			); err != nil {
				return err
			}
			outcome = jobs.Permanent
			return markEffect(ctx, tx, d.TargetID, "cancelled", "authority_changed_before_dispatch")
		}

		source, err := s.source(ctx, tx, q, current)
		if err != nil {
			return err
		}
		needed, err := s.preflight(ctx, tx, current, source)
		if err != nil {
			return err
		}
		if !needed {
			outcome = jobs.Delivered
			return nil
		}
		integration, err := q.GitHubAppIntegration(ctx, d.EventID)
		if err != nil {
			return fmt.Errorf("read execution access installation: %w", err)
		}
		change := githubapp.AccessChange{
			AccountID:  current.Effect.Action.Member.GitHubID,
			Permission: current.Effect.Action.Member.Permission,
			Remove:     current.Effect.Action.Kind == "remove",
		}
		if change.Remove {
			for _, invitation := range source.Invitations {
				if invitation.AccountID == change.AccountID {
					change.InvitationID = invitation.ID
					break
				}
			}
		}
		beforeWrite = false
		receipt, err := s.App.ChangeCollaborator(
			ctx,
			q,
			d.EventID,
			request(current.Repository, integration),
			change,
		)
		if errors.Is(err, githubapp.ErrAccessNotSent) {
			beforeWrite = true
			return wrap("repository write was not sent", err)
		}
		if errors.Is(err, githubapp.ErrAccessRejected) {
			outcome = jobs.Permanent
			return s.noEffect(
				ctx,
				tx,
				d.EventID,
				current.Effect.Action,
				"organization_policy_restricted",
			)
		}
		if err != nil {
			return nil
		}
		if err = s.acceptReceipt(ctx, tx, current, receipt); err != nil {
			return err
		}
		outcome = jobs.Delivered
		return markEffect(ctx, tx, d.TargetID, "delivered", "")
	})
	if err != nil {
		if beforeWrite {
			return s.finishNoEffect(ctx, d, dispatch)
		}
		return jobs.Ambiguous
	}
	return outcome
}

func (s Service) acceptReceipt(
	ctx context.Context,
	tx pgx.Tx,
	current scope,
	receipt githubapp.AccessResult,
) error {
	action := current.Effect.Action
	if action.Kind == "remove" {
		if err := updateGrant(ctx, tx, *action.GrantID, "revoked", "", s.now()); err != nil {
			return err
		}
		_, err := tx.Exec(
			ctx,
			`UPDATE github_invitation_bindings SET state='delivered',updated_at=now()WHERE id=$1`,
			*action.GrantID,
		)
		if err != nil {
			return fmt.Errorf("complete revoked invitation binding: %w", err)
		}
		return nil
	}
	if receipt.Invitation != nil && receipt.Invitation.AccountID == action.Member.GitHubID &&
		receipt.Invitation.Permission == permissionName(action.Member.Permission) {
		return s.retainGrant(ctx, tx, current, "pending", "", true, &receipt.Invitation.ID)
	}
	if receipt.Accepted {
		return s.retainGrant(ctx, tx, current, "accepted", "ownership_unproven", false, nil)
	}
	return access.ErrConflict
}

func (s Service) Reconcile(ctx context.Context, d jobs.Delivery) jobs.Outcome {
	outcome := jobs.Ambiguous
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		e, err := q.LockEvent(ctx, d.EventID)
		if err != nil {
			return fmt.Errorf("lock access reconciliation: %w", err)
		}
		effect, err := readEffect(ctx, tx, e.ID, d.TargetID)
		if err != nil {
			return err
		}
		r, err := repository(ctx, q, e.ID, effect.TeamID)
		if err != nil {
			return err
		}
		if e.OwnerID == nil || authorize(ctx, q, *e.OwnerID, e.ID, false) != nil {
			return access.ErrForbidden
		}
		if err = s.refresh(ctx, tx, q, e, r); err != nil {
			return err
		}
		source, _, diagnostic, err := readObservation(ctx, tx, e.ID, r.TeamID)
		if err != nil {
			return err
		}
		if diagnostic != "" {
			return nil
		}
		if effect.Action.Kind == "remove" && effect.Action.GrantID != nil {
			present := slices.ContainsFunc(
				source.Collaborators,
				func(c githubapp.Collaborator) bool { return c.ID == effect.Action.Member.GitHubID },
			) ||
				slices.ContainsFunc(
					source.Invitations,
					func(i githubapp.Invitation) bool { return i.AccountID == effect.Action.Member.GitHubID },
				)
			if !present {
				if err = updateGrant(
					ctx,
					tx,
					*effect.Action.GrantID,
					"revoked",
					"",
					s.now(),
				); err != nil {
					return err
				}
				outcome = jobs.Delivered
				return markEffect(ctx, tx, d.TargetID, "delivered", "")
			}
		}
		if effect.State == "delivered" {
			outcome = jobs.Delivered
		}
		return nil
	})
	if err != nil {
		return jobs.Ambiguous
	}
	return outcome
}

func (s Service) noEffect(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
	action Action,
	diagnostic string,
) error {
	if action.Kind == "invite" {
		if _, err := tx.Exec(
			ctx,
			`UPDATE repository_access_grants SET state='failed',diagnostic=$2,updated_at=now()WHERE id=$1`,
			action.ID,
			diagnostic,
		); err != nil {
			return fmt.Errorf("retain rejected invitation: %w", err)
		}
	} else if action.GrantID != nil && (action.PreviousState == "pending" || action.PreviousState == "accepted" || action.PreviousState == "expired") {
		if _, err := tx.Exec(
			ctx,
			`UPDATE repository_access_grants SET state=$2,diagnostic=$3,updated_at=now()WHERE id=$1`,
			*action.GrantID,
			action.PreviousState,
			diagnostic,
		); err != nil {
			return fmt.Errorf("retain unchanged managed grant: %w", err)
		}
	}
	return markEffect(ctx, tx, action.ID, "failed", diagnostic)
}

func (s Service) finishNoEffect(
	ctx context.Context,
	d jobs.Delivery,
	dispatch uuid.UUID,
) jobs.Outcome {
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		if _, err := q.LockEvent(ctx, d.EventID); err != nil {
			return fmt.Errorf("lock unsent access result: %w", err)
		}
		current, err := readEffect(ctx, tx, d.EventID, d.TargetID)
		if err != nil {
			return err
		}
		if current.Dispatch == nil || *current.Dispatch != dispatch {
			return access.ErrConflict
		}
		return s.noEffect(ctx, tx, d.EventID, current.Action, "provider_unavailable_before_write")
	})
	if err != nil {
		return jobs.Ambiguous
	}
	return jobs.Permanent
}
