package submissionfreeze

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/githubapp"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
)

const HandlerKind = "submission.capture"

var errUnavailable = errors.New("submission capture dependencies unavailable")

type App interface {
	ObserveBranch(
		context.Context,
		*dbgen.Queries,
		uuid.UUID,
		githubapp.RepositoryRequest,
		int64,
		string,
	) (githubapp.BranchObservation, error)
}
type Service struct {
	Pool *pgxpool.Pool
	App  App
	Jobs *jobs.Service
	Now  func() time.Time
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC().Truncate(time.Microsecond)
	}
	return time.Now().UTC().Truncate(time.Microsecond)
}

func config(row dbgen.Event) (event.Configuration, error) {
	var c event.Configuration
	err := json.Unmarshal(row.Configuration, &c)
	return c, failure("config", err)
}

func native(
	s Service,
) submissions.Service {
	return submissions.Service{Pool: s.Pool, Clock: s.now}
}

func failure(op string, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return access.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("submission freeze %s: %w", op, err)
	}
	return nil
}

type scope struct {
	event     dbgen.Event
	authority access.TeamAuthority
	native    submissions.Page
	config    event.Configuration
}

func (s Service) scoped(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, teamID uuid.UUID,
) (scope, error) {
	var v scope
	q := dbgen.New(tx)
	row, err := q.LockEvent(ctx, eventID)
	if err != nil {
		return v, failure("event", err)
	}
	v.event = row
	// Parent event visibility and native submission visibility are both required.
	if err = (access.EventPolicy{Queries: q, Clock: s.now}).Authorize(
		ctx,
		actor,
		eventID,
		eventID,
		access.EventRead,
	); err != nil {
		return v, failure("scoped", err)
	}
	v.native, err = native(s).ReadInTransaction(ctx, tx, actor, eventID, teamID)
	if err != nil {
		return v, failure("scoped", err)
	}
	v.authority, err = access.TeamPermissions(ctx, q, actor, eventID, teamID)
	if err != nil {
		return v, failure("scoped", err)
	}
	v.config, err = config(row)
	return v, failure("configuration", err)
}

func head(ctx context.Context, q *dbgen.Queries, eventID, teamID uuid.UUID) (int64, error) {
	n, err := q.SubmissionFreezeHead(
		ctx,
		dbgen.SubmissionFreezeHeadParams{EventID: eventID, TeamID: teamID},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return n, failure("head", err)
}

func (s Service) page(
	ctx context.Context,
	tx pgx.Tx,
	actor uuid.UUID,
	v scope,
	eventID, teamID uuid.UUID,
) (Page, error) {
	q := dbgen.New(tx)
	out, err := s.configurationPage(ctx, q, v, eventID, teamID)
	if err != nil {
		return out, failure("page", err)
	}
	if out.CurrentReferenceStatus == "not_applicable" && !v.authority.Member &&
		!v.authority.Organiser {
		out.Status = "not_applicable"
		out.RepositoryID = 0
		out.RepositoryURL = ""
		out.Branch = ""
		return out, nil
	}

	row, err := q.SubmissionFreezeSource(
		ctx,
		dbgen.SubmissionFreezeSourceParams{EventID: eventID, TeamID: teamID},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, failure("page", err)
	}
	out.Source = &Source{}
	if err = json.Unmarshal(row.Definition, out.Source); err != nil {
		return out, failure("page", err)
	}
	if !v.authority.Member && !v.authority.Organiser && out.Source.Status != "not_applicable" {
		visible := false
		if v.native.Final != nil {
			for _, e := range v.native.Final.Evidence {
				if e.Kind == "repository" {
					visible = true
				}
			}
		}
		if !visible {
			return Page{}, access.ErrForbidden
		}
	}
	out.CanConfigure = false
	out.Status = out.Source.Status
	out.Branch = out.Source.Branch
	out.RepositoryID = out.Source.RepositoryID
	out.RepositoryURL = out.Source.RepositoryURL
	out.Cutoff = out.Source.Cutoff
	if err = s.loadHistory(ctx, q, &out); err != nil {
		return out, failure("page", err)
	}
	s.projectCapabilities(v, &out)
	if err = s.projectPage(ctx, tx, actor, v, &out); err != nil {
		return Page{}, err
	}
	return out, nil
}

func (s Service) Read(ctx context.Context, actor, eventID, teamID uuid.UUID) (Page, error) {
	var out Page
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		v, err := s.scoped(ctx, tx, actor, eventID, teamID)
		if err != nil {
			return failure("Read", err)
		}
		out, err = s.page(ctx, tx, actor, v, eventID, teamID)
		return failure("Read", err)
	})
	return out, failure("read", err)
}

// PushEvidence pages the exact retained authenticated repository/ref evidence.
// UUID cursor order is stable storage order, never push occurrence order. This
// is not a claim that webhook delivery was complete, timely or chronological.
func (s Service) PushEvidence(
	ctx context.Context,
	actor, eventID, teamID, after uuid.UUID,
	limit int32,
) (PushPage, error) {
	out := PushPage{Items: []Push{}}
	if limit < 1 || limit > 100 {
		return out, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		v, err := s.scoped(ctx, tx, actor, eventID, teamID)
		if err != nil {
			return failure("PushEvidence", err)
		}
		if !v.authority.Organiser {
			return access.ErrForbidden
		}
		p, err := s.page(ctx, tx, actor, v, eventID, teamID)
		if err != nil {
			return failure("PushEvidence", err)
		}
		if p.Source == nil {
			return nil
		}
		rows, err := q.SubmissionFreezePushEvidence(
			ctx,
			dbgen.SubmissionFreezePushEvidenceParams{
				EventID:      eventID,
				TeamID:       teamID,
				RepositoryID: p.Source.RepositoryID,
				Provenance:   p.Source.RepositoryProvenance,
				Ref:          "refs/heads/" + p.Source.Branch,
				ID:           after,
				Limit:        limit + 1,
			},
		)
		if err != nil {
			return failure("PushEvidence", err)
		}
		for i, row := range rows {
			if i == int(limit) {
				id := out.Items[len(out.Items)-1].ID
				out.NextCursor = &id
				break
			}
			p := Push{
				ID:          row.ID,
				SHA:         row.AfterSha,
				Ref:         row.Ref,
				ReceivedAt:  row.ReceivedAt,
				PayloadHash: hex.EncodeToString(row.PayloadHash),
			}
			if row.ProviderAt.Valid {
				p.ProviderAt = &row.ProviderAt.Time
			}
			out.Items = append(out.Items, p)
		}
		return nil
	})
	return out, failure("push evidence", err)
}

func (s Service) configurationPage(
	ctx context.Context,
	q *dbgen.Queries,
	v scope,
	eventID, teamID uuid.UUID,
) (Page, error) {
	out := Page{
		Branch:       "main",
		Cutoff:       v.config.SubmissionCutoff,
		Status:       "pending",
		Observations: []Observation{},
		Decisions:    []Decision{},
	}
	out.NativeRevision = v.native.Revision
	out.NativeOutcome = v.native.Outcome
	if v.native.Final != nil {
		revision := v.native.Final.Revision
		out.CurrentSubmissionRevision = &revision
	}
	var err error
	out.Revision, err = head(ctx, q, eventID, teamID)
	if err != nil {
		return out, failure("configurationPage", err)
	}
	r, err := q.TeamRepository(ctx, dbgen.TeamRepositoryParams{EventID: eventID, TeamID: teamID})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return out, failure("configurationPage", err)
	}
	if err == nil && r.GithubRepositoryID.Valid {
		out.RepositoryID = r.GithubRepositoryID.Int64
		out.RepositoryURL = "https://github.com/" + r.Organization + "/" + r.Name
	}
	c, err := q.SubmissionBranchConfiguration(
		ctx,
		dbgen.SubmissionBranchConfigurationParams{EventID: eventID, TeamID: teamID},
	)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return out, failure("configurationPage", err)
	}
	if err == nil {
		out.Branch = c.Branch
	}
	out.CurrentReferenceStatus, err = currentReferences(ctx, q, v, out.RepositoryURL)
	if err != nil {
		return out, err
	}
	if v.native.Outcome == "frozen" && out.CurrentReferenceStatus == "not_applicable" {
		out.Status = "not_applicable"
	}

	out.CanConfigure = v.event.WritesAllowed && v.event.Stage != "cancelled" &&
		v.authority.Active &&
		(v.authority.Member || v.authority.Organiser) &&
		v.event.PublishedAt.Valid &&
		!s.now().Before(v.event.PublishedAt.Time) &&
		!out.Cutoff.IsZero() &&
		s.now().Before(out.Cutoff) &&
		out.RepositoryID > 0 && r.State == "ready"
	return out, nil
}

func (s Service) projectCapabilities(v scope, out *Page) {
	// Reconciliation cannot change native outcomes or attach old evidence to a
	// later organiser-replaced submission. The original binding remains readable.
	target := out.Source.SubmissionRevision
	if out.Current != nil {
		target = &out.Current.SubmissionRevision
	}
	matches := target != nil && v.native.Final != nil && *target == v.native.Final.Revision
	out.BindingCurrent = matches && v.native.Outcome == "frozen" && v.authority.Active
	managedIDs := []uuid.UUID{}
	if v.native.Final != nil {
		managedIDs, _, _ = referenceFacts(v.native.Final.Document, out.Source.RepositoryURL)
	}
	writable := len(managedIDs) > 0 && v.event.WritesAllowed && v.event.Stage != "cancelled" &&
		v.event.Stage != "archived" &&
		v.authority.Organiser &&
		v.authority.Active &&
		v.native.Outcome == "frozen" &&
		v.native.Final != nil &&
		out.Source.RepositoryID > 0
	originalMatches := out.Source.SubmissionRevision != nil && v.native.Final != nil &&
		*out.Source.SubmissionRevision == v.native.Final.Revision
	out.CanReconcile = writable && out.Current == nil && originalMatches &&
		out.Source.Status == "unresolved"
	out.CanAmend = writable &&
		(out.Current != nil || !originalMatches || out.Source.Status == "repository_reference_mismatch")
	if !matches && v.native.Outcome == "frozen" && v.native.Final != nil {
		out.Status = "native_revision_mismatch"
	}
	if out.CurrentReferenceStatus == "not_applicable" && v.native.Outcome == "frozen" {
		out.Status = "not_applicable"
	}
}

func (s Service) loadHistory(ctx context.Context, q *dbgen.Queries, out *Page) error {
	observations, err := q.SubmissionFreezeObservations(ctx, out.Source.ID)
	if err != nil {
		return failure("loadHistory", err)
	}
	for _, row := range observations {
		var o Observation
		if err = json.Unmarshal(row.Definition, &o); err != nil {
			return failure("loadHistory", err)
		}
		out.Observations = append(out.Observations, o)
	}
	decisions, err := q.SubmissionFreezeDecisions(ctx, out.Source.ID)
	if err != nil {
		return failure("loadHistory", err)
	}
	for _, row := range decisions {
		var d Decision
		if err = json.Unmarshal(row.Definition, &d); err != nil {
			return failure("loadHistory", err)
		}
		out.Decisions = append(out.Decisions, d)
	}
	if len(out.Decisions) > 0 {
		out.Current = &out.Decisions[len(out.Decisions)-1]
		out.Status = "completed"
	}
	return nil
}
