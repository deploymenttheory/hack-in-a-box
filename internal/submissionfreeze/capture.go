package submissionfreeze

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/githubapp"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
)

type branchRequest struct {
	Configuration  githubapp.Configuration `json:"configuration"`
	InstallationID int64                   `json:"installation_id"`
	Name           string                  `json:"name"`
}

func (r branchRequest) request() githubapp.RepositoryRequest {
	return githubapp.RepositoryRequest{
		Configuration:  r.Configuration,
		InstallationID: r.InstallationID,
		Name:           r.Name,
	}
}

type binding struct {
	Request           branchRequest `json:"request"`
	RequestedBy       uuid.UUID     `json:"requested_by"`
	CredentialVersion int64         `json:"credential_version"`
}

func (s Service) Maintain(ctx context.Context) error {
	ids, err := dbgen.New(s.Pool).SubmissionFreezeEvents(ctx)
	if err != nil {
		return failure("maintenance events", err)
	}
	for _, id := range ids {
		if err = s.Schedule(ctx, id); err != nil {
			return failure("Maintain", err)
		}
	}
	return nil
}

// Schedule queues one read-only operation per native entry and shared cutoff.
// Branch changes before cutoff do not duplicate operations or freeze early.
func (s Service) Schedule(ctx context.Context, eventID uuid.UUID) error {
	if s.Jobs == nil {
		return errUnavailable
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		row, err := q.LockEvent(ctx, eventID)
		if err != nil {
			return failure("Schedule", err)
		}
		c, err := config(row)
		if err != nil {
			return failure("Schedule", err)
		}
		if !row.WritesAllowed || row.Stage == "cancelled" || row.Stage == "completed" ||
			row.Stage == "archived" ||
			c.SubmissionCutoff.IsZero() ||
			!row.PublishedAt.Valid ||
			s.now().Before(row.PublishedAt.Time) {
			return nil
		}
		entries, err := q.SubmissionEntries(ctx, eventID)
		if err != nil {
			return failure("Schedule", err)
		}
		for _, entry := range entries {
			_, err = s.Jobs.Enqueue(
				ctx,
				tx,
				dbgen.InsertOperationParams{
					EventID:  eventID,
					Kind:     HandlerKind,
					TargetID: entry.TeamID,
					OperationKey: "submission-cutoff:" + entry.TeamID.String() + ":" + c.SubmissionCutoff.UTC().
						Format(time.RFC3339Nano),
					DueAt:    c.SubmissionCutoff,
					Approved: true,
				},
			)
			if err != nil {
				return failure("Schedule", err)
			}
		}
		return nil
	})
	return failure("schedule", err)
}

func (s Service) Eligible(ctx context.Context, d jobs.Delivery) (bool, error) {
	row, err := dbgen.New(s.Pool).Event(ctx, d.EventID)
	if err != nil {
		return false, failure("Eligible", err)
	}
	c, err := config(row)
	if err != nil {
		return false, failure("Eligible", err)
	}
	op, err := dbgen.New(s.Pool).Operation(ctx, d.ID)
	if err != nil {
		return false, failure("Eligible", err)
	}
	return row.WritesAllowed && row.Stage != "cancelled" && row.Stage != "completed" &&
		row.Stage != "archived" &&
		!c.SubmissionCutoff.IsZero() &&
		!s.now().Before(c.SubmissionCutoff) &&
		op.DueAt.Equal(c.SubmissionCutoff), nil
}

// Reconcile is safe to retry because this handler only reads GitHub. Existing
// immutable source/attempts remain; no external create, send or rewrite occurs.
func (s Service) Reconcile(context.Context, jobs.Delivery) jobs.Outcome { return jobs.Retryable }

func (s Service) Deliver(ctx context.Context, d jobs.Delivery) jobs.Outcome {
	src, b, skip, err := s.prepareCapture(ctx, d)
	if err != nil {
		return jobs.Retryable
	}
	if skip || (src.Status != "unresolved" && src.Status != "repository_reference_mismatch") {
		return jobs.Delivered
	}
	// Current authorisation/configuration is captured under the event lock before
	// remote I/O, then fenced again before publishing. No private key escapes.
	o := Observation{
		ID:         uuid.New(),
		SourceID:   src.ID,
		Status:     "provider_unavailable",
		Provenance: "github_app_current_ref",
	}
	retry := false
	valid := false
	err = database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		row, err := q.LockEvent(ctx, d.EventID)
		if err != nil {
			return failure("Deliver", err)
		}
		valid, err = s.current(ctx, q, row, src, b)
		return failure("Deliver", err)
	})
	if err != nil {
		return jobs.Retryable
	}
	if !valid {
		o.Status = "authority_or_configuration_lost"
	} else {
		// Durable jobs.Execute owns the operation's connection-scoped claim. Remote
		// read-only I/O runs outside event/operation row locks; publish revalidates the
		// exact native source and current authority before retaining any result.
		value, observeErr := s.App.ObserveBranch(
			ctx,
			dbgen.New(s.Pool),
			d.EventID,
			b.Request.request(),
			src.RepositoryID,
			src.Branch,
		)
		o.ObservedAt = s.now()
		if observeErr != nil {
			retry = true
		} else {
			o.SHA = value.SHA
			o.Status = value.Status
			if (o.Status == "current_ref" && !shaPattern.MatchString(o.SHA)) ||
				(o.Status != "current_ref" && o.Status != "branch_missing" && o.Status != "empty_repository" && o.Status != "repository_unavailable") {
				o.Status = "provider_unavailable"
				o.SHA = ""
				retry = true
			}
		}
	}

	if o.ObservedAt.IsZero() {
		o.ObservedAt = s.now()
	}
	err = database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		row, err := q.LockEvent(ctx, d.EventID)
		if err != nil {
			return failure("Deliver", err)
		}
		if !row.WritesAllowed || row.Stage == "archived" || row.Stage == "cancelled" ||
			row.Stage == "completed" {
			return nil
		}
		valid, err := s.current(ctx, q, row, src, b)
		if err != nil {
			return failure("Deliver", err)
		}
		if !valid {
			o.Status = "authority_or_configuration_lost"
			o.SHA = ""
			retry = false
		}
		decisions, err := q.SubmissionFreezeDecisions(ctx, src.ID)
		if err != nil {
			return failure("Deliver", err)
		}
		if len(decisions) > 0 {
			return nil
		}
		encoded, err := json.Marshal(o)
		if err != nil {
			return failure("Deliver", err)
		}
		if err = q.InsertSubmissionFreezeObservation(
			ctx,
			dbgen.InsertSubmissionFreezeObservationParams{
				ID:         o.ID,
				SourceID:   src.ID,
				ObservedAt: o.ObservedAt,
				Definition: encoded,
			},
		); err != nil {
			return failure("Deliver", err)
		}
		_, err = q.AdvanceSubmissionFreezeHead(
			ctx,
			dbgen.AdvanceSubmissionFreezeHeadParams{EventID: d.EventID, TeamID: d.TargetID},
		)
		return failure("Deliver", err)
	})
	if err != nil {
		return jobs.Retryable
	}
	if retry {
		return jobs.Retryable
	}
	return jobs.Delivered
}

func (s Service) current(
	ctx context.Context,
	q *dbgen.Queries,
	row dbgen.Event,
	src Source,
	b binding,
) (bool, error) {
	c, err := config(row)
	if err != nil {
		return false, failure("current", err)
	}
	if !row.WritesAllowed || row.Stage == "archived" || row.Stage == "completed" ||
		row.Stage == "cancelled" ||
		!c.SubmissionCutoff.Equal(src.Cutoff) {
		return false, nil
	}
	if err = (access.EventPolicy{Queries: q, Clock: s.now}).Authorize(
		ctx,
		b.RequestedBy,
		row.ID,
		row.ID,
		access.EventEdit,
	); err != nil {
		if errors.Is(err, access.ErrForbidden) {
			return false, nil
		}
		return false, failure("current", err)
	}
	a, err := q.GitHubAppIntegration(ctx, row.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, failure("current", err)
	}
	if !integrationMatches(a, b) {
		return false, nil
	}
	r, err := q.TeamRepository(ctx, dbgen.TeamRepositoryParams{EventID: row.ID, TeamID: src.TeamID})
	if err != nil {
		return false, failure("current", err)
	}
	if !r.GithubRepositoryID.Valid || r.GithubRepositoryID.Int64 != src.RepositoryID ||
		r.Provenance != src.RepositoryProvenance ||
		r.State != "ready" {
		return false, nil
	}
	entry, err := q.CompetitionEntry(
		ctx,
		dbgen.CompetitionEntryParams{EventID: row.ID, TeamID: src.TeamID},
	)
	if err != nil {
		return false, failure("current", err)
	}
	t, err := q.HackTeam(ctx, dbgen.HackTeamParams{EventID: row.ID, ID: src.TeamID})
	if err != nil {
		return false, failure("current", err)
	}
	if !entry.Active || t.State == "disbanded" {
		return false, nil
	}
	state, err := q.SubmissionState(
		ctx,
		dbgen.SubmissionStateParams{EventID: row.ID, TeamID: src.TeamID},
	)
	if err != nil {
		return false, failure("current", err)
	}
	if state.Outcome != "frozen" || src.SubmissionRevision == nil || !state.ActiveRevision.Valid ||
		state.ActiveRevision.Int64 != *src.SubmissionRevision {
		return false, nil
	}
	if s.App == nil {
		return false, errUnavailable
	}
	return true, nil
}

func (s Service) prepareCapture(
	ctx context.Context,
	d jobs.Delivery,
) (Source, binding, bool, error) {
	var src Source
	var b binding
	skip := false
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		row, err := q.LockEvent(ctx, d.EventID)
		if err != nil {
			return failure("prepareCapture", err)
		}
		c, err := config(row)
		if err != nil {
			return failure("prepareCapture", err)
		}
		op, err := q.Operation(ctx, d.ID)
		if err != nil {
			return failure("prepareCapture", err)
		}
		if !row.WritesAllowed || row.Stage == "cancelled" || row.Stage == "completed" ||
			row.Stage == "archived" ||
			c.SubmissionCutoff.IsZero() ||
			s.now().Before(c.SubmissionCutoff) ||
			!op.DueAt.Equal(c.SubmissionCutoff) {
			skip = true
			return nil
		}
		existing, err := q.SubmissionFreezeSource(
			ctx,
			dbgen.SubmissionFreezeSourceParams{EventID: d.EventID, TeamID: d.TargetID},
		)
		if err == nil {
			if err = json.Unmarshal(existing.Definition, &src); err != nil {
				return failure("prepareCapture", err)
			}
			if err = json.Unmarshal(existing.Binding, &b); err != nil {
				return failure("prepareCapture", err)
			}
			skip, err = s.captureAlreadySettled(ctx, q, src, c.SubmissionCutoff)
			return failure("prepareCapture", err)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return failure("prepareCapture", err)
		}
		src, b, err = s.createSource(ctx, tx, q, row, d)
		return failure("prepareCapture", err)
	})
	return src, b, skip, failure("prepare capture", err)
}

func (s Service) captureAlreadySettled(
	ctx context.Context,
	q *dbgen.Queries,
	src Source,
	cutoff time.Time,
) (bool, error) {
	if !src.Cutoff.Equal(cutoff) {
		return true, nil
	}
	decisions, err := q.SubmissionFreezeDecisions(ctx, src.ID)
	if err != nil {
		return false, failure("captureAlreadySettled", err)
	}
	if len(decisions) > 0 {
		return true, nil
	}
	observations, err := q.SubmissionFreezeObservations(ctx, src.ID)
	if err != nil {
		return false, failure("captureAlreadySettled", err)
	}
	for _, row := range observations {
		var o Observation
		if err = json.Unmarshal(row.Definition, &o); err != nil {
			return false, failure("captureAlreadySettled", err)
		}
		if o.Status != "provider_unavailable" {
			return true, nil
		}
	}
	return false, nil
}

func (s Service) createSource(
	ctx context.Context,
	tx pgx.Tx,
	q *dbgen.Queries,
	row dbgen.Event,
	d jobs.Delivery,
) (src Source, b binding, err error) {
	if err = native(s).Settle(ctx, tx, row, s.now()); err != nil {
		return src, b, failure("createSource", err)
	}
	entry, err := q.CompetitionEntry(
		ctx,
		dbgen.CompetitionEntryParams{EventID: d.EventID, TeamID: d.TargetID},
	)
	if err != nil {
		return src, b, failure("createSource", err)
	}

	if err = q.EnsureSubmissionFreezeHead(
		ctx,
		dbgen.EnsureSubmissionFreezeHeadParams{EventID: d.EventID, TeamID: d.TargetID},
	); err != nil {
		return src, b, failure("createSource", err)
	}
	cutoff, err := native(s).CutoffInTransaction(ctx, tx, d.EventID, d.TargetID)
	if err != nil {
		return src, b, failure("createSource", err)
	}
	src = Source{
		ID:                 uuid.New(),
		EventID:            d.EventID,
		TeamID:             d.TargetID,
		EntryID:            entry.ID,
		NativeOutcome:      cutoff.Outcome,
		Cutoff:             cutoff.Cutoff,
		RecordedAt:         s.now(),
		Branch:             "main",
		Status:             "unresolved",
		SubmissionRevision: cutoff.Revision,
	}

	b, err = s.bindSourceRepository(ctx, q, &src)
	if err != nil {
		return src, b, failure("createSource", err)
	}
	if err = sourceApplicability(ctx, q, &src); err != nil {
		return src, b, failure("createSource", err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		return src, b, failure("createSource", err)
	}
	bound, err := json.Marshal(b)
	if err != nil {
		return src, b, failure("createSource", err)
	}
	if err = q.InsertSubmissionFreezeSource(
		ctx,
		dbgen.InsertSubmissionFreezeSourceParams{
			ID:         src.ID,
			EventID:    d.EventID,
			TeamID:     d.TargetID,
			Cutoff:     src.Cutoff,
			Binding:    bound,
			Definition: data,
		},
	); err != nil {
		return src, b, failure("createSource", err)
	}
	_, err = q.AdvanceSubmissionFreezeHead(
		ctx,
		dbgen.AdvanceSubmissionFreezeHeadParams{EventID: d.EventID, TeamID: d.TargetID},
	)
	return src, b, failure("createSource", err)
}

func (s Service) bindSourceRepository(
	ctx context.Context,
	q *dbgen.Queries,
	src *Source,
) (b binding, err error) {
	repo, err := q.TeamRepository(
		ctx,
		dbgen.TeamRepositoryParams{EventID: src.EventID, TeamID: src.TeamID},
	)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return b, failure("bindSourceRepository", err)
	}
	if err == nil && repo.GithubRepositoryID.Valid {
		src.RepositoryID = repo.GithubRepositoryID.Int64
		src.RepositoryURL = "https://github.com/" + repo.Organization + "/" + repo.Name
		src.RepositoryProvenance = repo.Provenance
		integration, err := q.GitHubAppIntegration(ctx, src.EventID)
		if err == nil {
			b = binding{
				Request: branchRequest{
					Configuration: githubapp.Configuration{
						AppID:              repo.AppID,
						AppSlug:            integration.AppSlug,
						OrganizationID:     repo.OrganizationID,
						Organization:       repo.Organization,
						TemplateRepository: repo.TemplateRepository,
					},
					InstallationID: repo.InstallationID,
					Name:           repo.Name,
				},
				RequestedBy:       repo.RequestedBy,
				CredentialVersion: integration.CredentialVersion,
			}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return b, failure("bindSourceRepository", err)
		}
	} else if src.Status == "unresolved" {
		src.Status = "managed_repository_missing"
	}
	branch, err := q.SubmissionBranchConfiguration(
		ctx,
		dbgen.SubmissionBranchConfigurationParams{EventID: src.EventID, TeamID: src.TeamID},
	)
	if err == nil {
		src.Branch = branch.Branch
		src.ConfigurationRevision = branch.Revision
		if branch.RepositoryID != src.RepositoryID ||
			branch.Provenance != src.RepositoryProvenance {
			src.Status = "configuration_mismatch"
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return b, failure("bindSourceRepository", err)
	}
	return b, nil
}

func integrationMatches(a dbgen.GithubAppIntegration, b binding) bool {
	return a.AppID == b.Request.Configuration.AppID &&
		a.AppSlug == b.Request.Configuration.AppSlug &&
		a.OrganizationID == b.Request.Configuration.OrganizationID &&
		a.Organization == b.Request.Configuration.Organization &&
		a.TemplateRepository == b.Request.Configuration.TemplateRepository &&
		a.InstallationID == b.Request.InstallationID &&
		a.CredentialVersion == b.CredentialVersion
}
