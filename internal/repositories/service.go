package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/githubapp"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
)

const HandlerKind = "repository.provision"

type App interface {
	LookupRepository(
		context.Context,
		*dbgen.Queries,
		uuid.UUID,
		githubapp.RepositoryRequest,
	) (githubapp.Repository, error)
	CreateRepository(
		context.Context,
		*dbgen.Queries,
		uuid.UUID,
		githubapp.RepositoryRequest,
	) (githubapp.Repository, error)
}
type Service struct {
	Pool *pgxpool.Pool
	App  App
	// Now optionally supplies domain eligibility time for new immediate intents.
	// Nil preserves the original database creation timestamp, including its precision.
	// Audit records, authentication, provider transport and job leases keep wall time.
	Now func() time.Time
}
type View struct {
	TeamID       uuid.UUID `json:"team_id"`
	Name         string    `json:"name"`
	Organization string    `json:"organization"`
	GitHubID     *int64    `json:"github_repository_id"`
	State        string    `json:"state"`
	Diagnostic   string    `json:"diagnostic"`
	Unused       bool      `json:"unused"`
}

func current(ctx context.Context, q *dbgen.Queries, r dbgen.TeamRepository) (bool, error) {
	event, err := q.Event(ctx, r.EventID)
	if err != nil {
		return false, repositoryError(err)
	}
	if !event.WritesAllowed || event.Stage == "cancelled" || event.Stage == "completed" ||
		event.Stage == "archived" {
		return false, nil
	}
	if err = (access.EventPolicy{Queries: q}).Authorize(
		ctx,
		r.RequestedBy,
		r.EventID,
		r.EventID,
		access.EventEdit,
	); err != nil {
		return false, repositoryError(err)
	}
	team, err := q.HackTeam(ctx, dbgen.HackTeamParams{EventID: r.EventID, ID: r.TeamID})
	if err != nil {
		return false, repositoryError(err)
	}
	if team.State != "published" {
		return false, nil
	}
	_, err = q.DBPublishedTeam(
		ctx,
		dbgen.DBPublishedTeamParams{EventID: r.EventID, TeamID: r.TeamID},
	)
	return err == nil, repositoryError(err)
}

// Queue records one independent operation per actual published team. Repetition
// retains successes and original provenance; it never grants a second create.
func (s Service) Queue(ctx context.Context, actor, eventID uuid.UUID, revision int64) error {
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		e, err := q.LockEvent(ctx, eventID)
		if err != nil {
			return repositoryError(err)
		}
		if e.Revision != revision {
			return access.ErrConflict
		}
		if err = (access.EventPolicy{Queries: q}).Authorize(
			ctx,
			actor,
			eventID,
			eventID,
			access.EventEdit,
		); err != nil {
			return repositoryError(err)
		}
		if err := s.queue(ctx, q, e, actor); err != nil {
			return err
		}
		return audit.Append(
			ctx,
			q,
			audit.Record{
				ActorID:  &actor,
				EventID:  &eventID,
				TargetID: eventID,
				Action:   audit.RepositoryProvisioningChanged,
				Reason:   audit.OrganiserDecision,
				Metadata: audit.Metadata{Result: new("success")},
			},
		)
	})
	return repositoryError(err)
}

func (s Service) queue(
	ctx context.Context,
	q *dbgen.Queries,
	e dbgen.Event,
	actor uuid.UUID,
) error {
	if !e.WritesAllowed || e.Stage == "cancelled" || e.Stage == "completed" ||
		e.Stage == "archived" {
		return access.ErrForbidden
	}
	a, err := q.GitHubAppIntegration(ctx, e.ID)
	if err != nil {
		return repositoryError(err)
	}
	if a.InstallationID <= 0 {
		return access.ErrConflict
	}
	teams, err := q.HackTeams(ctx, e.ID)
	if err != nil {
		return repositoryError(err)
	}
	var queuedAt time.Time
	if s.Now != nil {
		queuedAt = s.Now().UTC()
	}
	for _, t := range teams {
		if t.State != "published" {
			continue
		}
		if _, err = q.DBPublishedTeam(
			ctx,
			dbgen.DBPublishedTeamParams{EventID: e.ID, TeamID: t.ID},
		); errors.Is(
			err,
			pgx.ErrNoRows,
		) {
			continue
		} else if err != nil {
			return repositoryError(err)
		}
		err = q.InsertTeamRepository(
			ctx,
			dbgen.InsertTeamRepositoryParams{
				EventID:            e.ID,
				TeamID:             t.ID,
				OrganizationID:     a.OrganizationID,
				Organization:       a.Organization,
				Name:               Name(e.ID, t.ID),
				AppID:              a.AppID,
				InstallationID:     a.InstallationID,
				TemplateRepository: a.TemplateRepository,
				RequestedBy:        actor,
			},
		)
		if err != nil {
			return repositoryError(err)
		}
		r, err := q.TeamRepository(ctx, dbgen.TeamRepositoryParams{EventID: e.ID, TeamID: t.ID})
		if err != nil {
			return repositoryError(err)
		}
		due := r.CreatedAt
		if s.Now != nil {
			due = queuedAt
		}
		if _, err = q.InsertOperation(
			ctx,
			dbgen.InsertOperationParams{
				EventID:      e.ID,
				TargetID:     t.ID,
				Kind:         HandlerKind,
				OperationKey: "repository:" + t.ID.String(),
				DueAt:        due,
				Approved:     true,
			},
		); err != nil {
			return repositoryError(err)
		}
	}
	return nil
}

// Maintain catches publication/configuration in either order and relies on the
// normal jobs reconciler to enqueue durable operations, never on draft rosters.
func (s Service) Maintain(ctx context.Context) error {
	events, err := dbgen.New(s.Pool).ProvisioningEvents(ctx)
	if err != nil {
		return repositoryError(err)
	}
	for _, e := range events {
		if e.OwnerID == nil {
			continue
		}
		err = database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
			locked, err := q.LockEvent(ctx, e.ID)
			if err != nil {
				return repositoryError(err)
			}
			if err = (access.EventPolicy{Queries: q}).Authorize(
				ctx,
				*e.OwnerID,
				e.ID,
				e.ID,
				access.EventEdit,
			); err != nil {
				return nil
			}
			return s.queue(ctx, q, locked, *e.OwnerID)
		})
		if err != nil {
			return fmt.Errorf("reconcile published repository intents: %w", err)
		}
	}
	return nil
}

func (s Service) List(ctx context.Context, actor, eventID uuid.UUID) ([]View, error) {
	result := []View{}
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		e, err := q.LockEvent(ctx, eventID)
		if err != nil {
			return repositoryError(err)
		}
		staff := (access.EventPolicy{Queries: q}).Authorize(
			ctx,
			actor,
			eventID,
			eventID,
			access.StaffRead,
		) == nil
		var own uuid.UUID
		if !staff {
			if !e.PublishedAt.Valid || e.PublishedAt.Time.After(time.Now()) {
				return access.ErrForbidden
			}
			assignment, err := q.TeamAssignment(
				ctx,
				dbgen.TeamAssignmentParams{
					EventID:  eventID,
					PersonID: actor,
					Snapshot: "published",
				},
			)
			if err != nil {
				return access.ErrForbidden
			}
			authority, err := access.TeamPermissions(ctx, q, actor, eventID, assignment.TeamID)
			if err != nil || !authority.Member || !authority.Active {
				return access.ErrForbidden
			}
			own = assignment.TeamID
		}
		rows, err := q.EventRepositories(ctx, eventID)
		if err != nil {
			return repositoryError(err)
		}
		for _, r := range rows {
			if !staff && (r.TeamID != own || r.TeamState != "published") {
				continue
			}
			v := View{
				TeamID:       r.TeamID,
				Name:         r.Name,
				Organization: r.Organization,
				State:        r.State,
				Diagnostic:   r.Diagnostic,
				Unused:       r.TeamState == "disbanded",
			}
			if r.GithubRepositoryID.Valid {
				v.GitHubID = &r.GithubRepositoryID.Int64
			}
			result = append(result, v)
		}
		return nil
	})
	return result, repositoryError(err)
}

func request(r dbgen.TeamRepository, slug string) githubapp.RepositoryRequest {
	return githubapp.RepositoryRequest{
		Configuration: githubapp.Configuration{
			AppID:              r.AppID,
			AppSlug:            slug,
			OrganizationID:     r.OrganizationID,
			Organization:       r.Organization,
			TemplateRepository: r.TemplateRepository,
		},
		InstallationID: r.InstallationID,
		Name:           r.Name,
		Marker:         "hackbox-provisioning:" + r.Provenance.String(),
	}
}

func (s Service) Eligible(ctx context.Context, d jobs.Delivery) (bool, error) {
	r, err := dbgen.New(s.Pool).
		TeamRepository(ctx, dbgen.TeamRepositoryParams{EventID: d.EventID, TeamID: d.TargetID})
	if err != nil {
		return false, repositoryError(err)
	}
	return current(ctx, dbgen.New(s.Pool), r)
}

func result(
	ctx context.Context,
	q *dbgen.Queries,
	r dbgen.TeamRepository,
	state, code string,
	id int64,
) error {
	githubID := pgtype.Int8{Int64: id, Valid: id > 0}
	_, err := q.SetRepositoryResult(
		ctx,
		dbgen.SetRepositoryResultParams{
			EventID:            r.EventID,
			TeamID:             r.TeamID,
			State:              state,
			Diagnostic:         code,
			GithubRepositoryID: githubID,
		},
	)
	return repositoryError(err)
}

func proven(r dbgen.TeamRepository, p githubapp.Repository) bool {
	return p.ID > 0 && p.OrganizationID == r.OrganizationID && p.Name == r.Name && p.Private &&
		!p.Archived &&
		p.Description == "hackbox-provisioning:"+r.Provenance.String() &&
		r.TemplateRepositoryID.Valid &&
		p.TemplateID == r.TemplateRepositoryID.Int64 &&
		(!r.GithubRepositoryID.Valid || r.GithubRepositoryID.Int64 == p.ID)
}

func (s Service) Deliver(ctx context.Context, d jobs.Delivery) jobs.Outcome {
	outcome := jobs.Retryable
	attempted := false
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		if _, err := q.LockEvent(ctx, d.EventID); err != nil {
			return repositoryError(err)
		}
		r, err := q.TeamRepository(
			ctx,
			dbgen.TeamRepositoryParams{EventID: d.EventID, TeamID: d.TargetID},
		)
		if err != nil {
			return repositoryError(err)
		}
		ok, err := current(ctx, q, r)
		if err != nil {
			return repositoryError(err)
		}
		if !ok {
			return nil
		}
		if r.GithubRepositoryID.Valid {
			outcome = jobs.Delivered
			return nil
		}
		if r.Attempted {
			attempted = true
			return nil
		}
		a, err := q.GitHubAppIntegration(ctx, r.EventID)
		if err != nil {
			return repositoryError(err)
		}
		req := request(r, a.AppSlug)
		templateReq := req
		templateReq.Name = r.TemplateRepository
		template, err := s.App.LookupRepository(ctx, q, r.EventID, templateReq)
		if err != nil {
			return result(ctx, q, r, "pending", "template_unavailable", 0)
		}
		if template.ID <= 0 || template.OrganizationID != r.OrganizationID || template.Archived {
			outcome = jobs.Permanent
			return result(ctx, q, r, "review", "template_invalid", 0)
		}
		_, err = s.App.LookupRepository(ctx, q, r.EventID, req)
		if err == nil {
			outcome = jobs.Permanent
			return result(ctx, q, r, "review", "name_collision", 0)
		}
		if !errors.Is(err, githubapp.ErrRepositoryAbsent) {
			return result(ctx, q, r, "pending", "provider_unavailable", 0)
		}
		if _, err = q.MarkRepositoryAttempt(
			ctx,
			dbgen.MarkRepositoryAttemptParams{
				EventID:              r.EventID,
				TeamID:               r.TeamID,
				TemplateRepositoryID: pgtype.Int8{Int64: template.ID, Valid: true},
			},
		); err != nil {
			return repositoryError(err)
		}
		return nil
	})
	if err != nil {
		return outcome
	}
	if attempted {
		return s.Reconcile(ctx, d)
	}
	if outcome == jobs.Permanent || outcome == jobs.Delivered {
		return outcome
	}
	// The create intent must commit before the network call. A crash from this point
	// only reconciles; absence/inaccessibility can never justify a second create.
	err = database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		if _, err := q.LockEvent(ctx, d.EventID); err != nil {
			return repositoryError(err)
		}
		r, err := q.TeamRepository(
			ctx,
			dbgen.TeamRepositoryParams{EventID: d.EventID, TeamID: d.TargetID},
		)
		if err != nil {
			return repositoryError(err)
		}
		if !r.Attempted {
			return nil
		}
		ok, err := current(ctx, q, r)
		if err != nil {
			return repositoryError(err)
		}
		if !ok {
			return nil
		}
		a, err := q.GitHubAppIntegration(ctx, r.EventID)
		if err != nil {
			return repositoryError(err)
		}
		p, err := s.App.CreateRepository(ctx, q, r.EventID, request(r, a.AppSlug))
		outcome = jobs.Ambiguous
		if errors.Is(err, githubapp.ErrRepositoryRejected) {
			outcome = jobs.Permanent
			return result(ctx, q, r, "review", "create_rejected", 0)
		}
		if err != nil {
			return result(ctx, q, r, "ambiguous", "create_uncertain", 0)
		}
		if !proven(r, p) {
			return result(ctx, q, r, "ambiguous", "identity_unproven", 0)
		}
		outcome = jobs.Delivered
		return result(ctx, q, r, "ready", "", p.ID)
	})
	if err != nil {
		return jobs.Ambiguous
	}
	return outcome
}

func (s Service) Reconcile(ctx context.Context, d jobs.Delivery) jobs.Outcome {
	outcome, err := s.reconcile(ctx, d, nil, nil)
	if err != nil {
		return jobs.Ambiguous
	}
	return outcome
}

func (s Service) reconcile(
	ctx context.Context,
	d jobs.Delivery,
	reviewer *uuid.UUID,
	revision *int64,
) (jobs.Outcome, error) {
	outcome := jobs.Ambiguous
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		event, err := q.LockEvent(ctx, d.EventID)
		if err != nil {
			return repositoryError(err)
		}
		if reviewer != nil {
			if revision == nil || event.Revision != *revision {
				return access.ErrConflict
			}
			if err = (access.EventPolicy{Queries: q}).Authorize(
				ctx,
				*reviewer,
				d.EventID,
				d.EventID,
				access.EventEdit,
			); err != nil {
				return repositoryError(err)
			}
			if err = q.AuthorizeRepositoryRecovery(
				ctx,
				dbgen.AuthorizeRepositoryRecoveryParams{
					EventID:     d.EventID,
					TeamID:      d.TargetID,
					RequestedBy: *reviewer,
				},
			); err != nil {
				return repositoryError(err)
			}
		}
		r, err := q.TeamRepository(
			ctx,
			dbgen.TeamRepositoryParams{EventID: d.EventID, TeamID: d.TargetID},
		)
		if err != nil {
			return repositoryError(err)
		}
		ok, err := current(ctx, q, r)
		if err != nil {
			return repositoryError(err)
		}
		if !ok {
			return access.ErrForbidden
		}
		switch {
		case !r.Attempted:
			outcome = jobs.Retryable
		default:
			a, err := q.GitHubAppIntegration(ctx, r.EventID)
			if err != nil {
				return repositoryError(err)
			}
			p, err := s.App.LookupRepository(ctx, q, r.EventID, request(r, a.AppSlug))
			switch {
			case err != nil:
				err = result(ctx, q, r, "ambiguous", "reconciliation_required", 0)
			case !proven(r, p):
				outcome = jobs.Permanent
				err = result(ctx, q, r, "review", "identity_collision", 0)
			default:
				outcome = jobs.Delivered
				err = result(ctx, q, r, "ready", "", p.ID)
			}
			if err != nil {
				return err
			}
		}
		if outcome == jobs.Delivered {
			if err = q.ResolveRepositoryOperation(
				ctx,
				dbgen.ResolveRepositoryOperationParams{EventID: r.EventID, TargetID: r.TeamID},
			); err != nil {
				return repositoryError(err)
			}
		}
		if reviewer != nil {
			auditResult := "unavailable"
			if outcome == jobs.Delivered {
				auditResult = "success"
			}
			return audit.Append(
				ctx,
				q,
				audit.Record{
					ActorID:  reviewer,
					EventID:  &r.EventID,
					TargetID: r.TeamID,
					Action:   audit.RepositoryProvisioningChanged,
					Reason:   audit.OrganiserDecision,
					Metadata: audit.Metadata{Result: &auditResult},
				},
			)
		}
		return nil
	})
	return outcome, repositoryError(err)
}

// Recover performs read-only provider reconciliation under current organiser
// authority. Even a missing repository never grants permission for another create.
func (s Service) Recover(
	ctx context.Context,
	actor, eventID, teamID uuid.UUID,
	revision int64,
) error {
	_, err := s.reconcile(ctx, jobs.Delivery{EventID: eventID, TargetID: teamID}, &actor, &revision)
	return err
}

func repositoryError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("repository operation: %w", err)
}
