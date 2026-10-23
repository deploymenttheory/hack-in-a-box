package repositoryaccess

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/githubapp"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func readPlan(ctx context.Context, tx pgx.Tx, eventID, teamID uuid.UUID) (*Plan, error) {
	var encoded []byte
	var approvedAt *time.Time
	var approvedBy *uuid.UUID
	err := tx.QueryRow(ctx, `SELECT definition,approved_at,approved_by FROM repository_access_plans WHERE event_id=$1 AND team_id=$2 ORDER BY version DESC LIMIT 1`, eventID, teamID).Scan(&encoded, &approvedAt, &approvedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read current repository access plan: %w", err)
	}
	var out Plan
	if err = json.Unmarshal(encoded, &out); err != nil {
		return nil, fmt.Errorf("decode repository access plan: %w", err)
	}
	out.ApprovedAt = approvedAt
	out.ApprovedBy = approvedBy
	return &out, nil
}
func readGrants(ctx context.Context, tx pgx.Tx, eventID, teamID uuid.UUID) ([]Grant, error) {
	rows, err := tx.Query(ctx, `SELECT g.id,g.person_id,g.github_id,g.link_revision,g.role,g.permission,g.app_managed,g.state,g.invitation_id,g.diagnostic,g.observed_at,EXISTS(SELECT 1 FROM repository_access_effects x WHERE x.event_id=g.event_id AND x.team_id=g.team_id AND x.attempted AND x.state='ambiguous' AND x.definition->'member'->>'github_id'=g.github_id::text) FROM repository_access_grants g WHERE g.event_id=$1 AND g.team_id=$2 ORDER BY g.created_at,g.id`, eventID, teamID)
	if err != nil {
		return nil, fmt.Errorf("read repository grants: %w", err)
	}
	defer rows.Close()
	out := []Grant{}
	for rows.Next() {
		var grant Grant
		if err = rows.Scan(&grant.ID, &grant.Member.PersonID, &grant.Member.GitHubID, &grant.Member.LinkRevision, &grant.Member.Role, &grant.Member.Permission, &grant.Managed, &grant.State, &grant.InvitationID, &grant.Diagnostic, &grant.ObservedAt, &grant.Uncertain); err != nil {
			return nil, fmt.Errorf("read repository grant row: %w", err)
		}
		out = append(out, grant)
	}
	return out, rows.Err()
}
func savePlan(ctx context.Context, tx pgx.Tx, actor uuid.UUID, plan Plan) error {
	encoded, err := json.Marshal(plan)
	if err != nil {
		return fmt.Errorf("encode repository access plan: %w", err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO repository_access_plans(id,event_id,team_id,version,fingerprint,definition,created_by,created_at)VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, plan.ID, plan.EventID, plan.TeamID, plan.Version, plan.Fingerprint, encoded, actor, plan.CreatedAt); err != nil {
		return fmt.Errorf("retain repository access plan: %w", err)
	}
	for _, action := range plan.Actions {
		encoded, err = json.Marshal(action)
		if err != nil {
			return fmt.Errorf("encode repository access effect: %w", err)
		}
		if _, err = tx.Exec(ctx, `INSERT INTO repository_access_effects(id,plan_id,event_id,team_id,definition)VALUES($1,$2,$3,$4,$5)`, action.ID, plan.ID, plan.EventID, plan.TeamID, encoded); err != nil {
			return fmt.Errorf("retain repository access effect: %w", err)
		}
	}
	return nil
}
func readObservation(ctx context.Context, tx pgx.Tx, eventID, teamID uuid.UUID) (githubapp.AccessSnapshot, *time.Time, string, error) {
	out := githubapp.AccessSnapshot{Collaborators: []githubapp.Collaborator{}, Invitations: []githubapp.Invitation{}}
	var encoded []byte
	var observed *time.Time
	var diagnostic string
	err := tx.QueryRow(ctx, `SELECT snapshot,observed_at,diagnostic FROM repository_access_observations WHERE event_id=$1 AND team_id=$2`, eventID, teamID).Scan(&encoded, &observed, &diagnostic)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil, "not_observed", nil
	}
	if err != nil {
		return out, nil, "", fmt.Errorf("read current repository access observation: %w", err)
	}
	if err = json.Unmarshal(encoded, &out); err != nil {
		return out, nil, "", fmt.Errorf("decode repository access observation: %w", err)
	}
	return out, observed, diagnostic, nil
}
func saveObservation(ctx context.Context, tx pgx.Tx, eventID, teamID uuid.UUID, source githubapp.AccessSnapshot, at time.Time) error {
	encoded, err := json.Marshal(source)
	if err != nil {
		return fmt.Errorf("encode repository access observation: %w", err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO repository_access_observations(event_id,team_id,snapshot,observed_at,diagnostic)VALUES($1,$2,$3,$4,'')ON CONFLICT(event_id,team_id)DO UPDATE SET snapshot=EXCLUDED.snapshot,observed_at=EXCLUDED.observed_at,diagnostic=''`, eventID, teamID, encoded, at)
	if err != nil {
		return fmt.Errorf("retain repository access observation: %w", err)
	}
	return nil
}
func updateGrant(ctx context.Context, tx pgx.Tx, id uuid.UUID, state, diagnostic string, at time.Time) error {
	_, err := tx.Exec(ctx, `UPDATE repository_access_grants SET state=$2,diagnostic=$3,observed_at=$4,updated_at=now()WHERE id=$1`, id, state, diagnostic, at)
	if err != nil {
		return fmt.Errorf("retain current repository grant status: %w", err)
	}
	return nil
}
func observeGrants(ctx context.Context, tx pgx.Tx, grants []Grant, source githubapp.AccessSnapshot, at time.Time) error {
	for _, grant := range grants {
		if grant.State == "revoked" || grant.State == "failed" {
			continue
		}
		state, diagnostic := grant.State, grant.Diagnostic
		found := false
		for _, user := range source.Collaborators {
			if user.ID == grant.Member.GitHubID {
				found = true
				state = "accepted"
				if !grant.Managed {
					diagnostic = "ownership_unproven"
				} else if user.Permission != permissionName(grant.Member.Permission) {
					diagnostic = "permission_changed"
				} else {
					diagnostic = ""
				}
				break
			}
		}
		if !found {
			for _, invitation := range source.Invitations {
				if invitation.AccountID == grant.Member.GitHubID {
					found = true
					state = "pending"
					if invitation.Expired {
						state = "expired"
					}
					if !grant.Managed || grant.InvitationID == nil || *grant.InvitationID != invitation.ID {
						diagnostic = "ownership_unproven"
					} else if invitation.Permission != permissionName(grant.Member.Permission) {
						diagnostic = "permission_changed"
					} else {
						diagnostic = ""
					}
					break
				}
			}
		}
		if !found && grant.State != "ambiguous" {
			state = "revoked"
			diagnostic = "not_currently_present"
		}
		if err := updateGrant(ctx, tx, grant.ID, state, diagnostic, at); err != nil {
			return err
		}
	}
	return nil
}
