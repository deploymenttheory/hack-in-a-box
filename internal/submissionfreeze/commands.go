package submissionfreeze

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
)

func fingerprint(c Command) (string, error) {
	// Include the reviewed resource revision; a new GET never becomes an ACK.
	b, err := json.Marshal(struct {
		Command
		Expected int64 `json:"expected_revision"`
	}{c, c.ExpectedRevision})
	if err != nil {
		return "", failure("fingerprint", err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func (s Service) Change(
	ctx context.Context,
	actor, eventID, teamID uuid.UUID,
	c Command,
) (Receipt, error) {
	var out Receipt
	if c.RequestID == uuid.Nil || c.ExpectedRevision < 0 {
		return out, access.ErrInvalid
	}
	c.Reason = strings.TrimSpace(c.Reason)
	c.ExternalEvidence = strings.TrimSpace(c.ExternalEvidence)
	if len(c.Reason) > 4000 || len(c.ExternalEvidence) > 8000 || len(c.ObservationIDs) > 50 ||
		len(c.PushIDs) > 50 {
		return out, access.ErrInvalid
	}
	if c.Action != "configure" && c.Action != "reconcile" && c.Action != "amend" {
		return out, access.ErrInvalid
	}
	f, err := fingerprint(c)
	if err != nil {
		return out, failure("Change", err)
	}
	err = database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		v, err := s.scoped(ctx, tx, actor, eventID, teamID)
		if err != nil {
			return failure("Change", err)
		}
		original, err := q.SubmissionFreezeCommand(
			ctx,
			dbgen.SubmissionFreezeCommandParams{EventID: eventID, RequestID: c.RequestID},
		)
		if err == nil {
			if original.ActorID != actor || original.TeamID != teamID || original.Fingerprint != f {
				return access.ErrConflict
			}
			if err = json.Unmarshal(original.Result, &out); err != nil {
				return failure("decode original receipt", err)
			}
			return s.projectReceipt(ctx, tx, actor, eventID, teamID, v, &out)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return failure("Change", err)
		}
		p, err := s.page(ctx, tx, actor, v, eventID, teamID)
		if err != nil {
			return failure("Change", err)
		}
		if p.Revision != c.ExpectedRevision {
			return access.ErrConflict
		}
		if err = q.EnsureSubmissionFreezeHead(
			ctx,
			dbgen.EnsureSubmissionFreezeHeadParams{EventID: eventID, TeamID: teamID},
		); err != nil {
			return failure("Change", err)
		}
		if c.Action == "configure" {
			if err = s.configure(ctx, q, actor, eventID, teamID, p, c); err != nil {
				return failure("Change", err)
			}
		} else {
			if v.native.Final == nil {
				return access.ErrForbidden
			}
			out.Decision, err = s.decide(
				ctx,
				q,
				actor,
				eventID,
				teamID,
				p,
				c,
				v.native.Final.Document,
			)
			if err != nil {
				return failure("Change", err)
			}
		}

		revision, err := q.AdvanceSubmissionFreezeHead(
			ctx,
			dbgen.AdvanceSubmissionFreezeHeadParams{EventID: eventID, TeamID: teamID},
		)
		if err != nil {
			return failure("Change", err)
		}
		out.Branch = c.Branch
		out.ExpectedRevision = c.ExpectedRevision
		out.RequestID = c.RequestID
		out.Action = c.Action
		out.Revision = revision
		out.Fingerprint = f
		encoded, err := json.Marshal(out)
		if err != nil {
			return failure("Change", err)
		}
		if err = q.InsertSubmissionFreezeCommand(
			ctx,
			dbgen.InsertSubmissionFreezeCommandParams{
				EventID:     eventID,
				RequestID:   c.RequestID,
				TeamID:      teamID,
				ActorID:     actor,
				Fingerprint: f,
				Result:      encoded,
			},
		); err != nil {
			return failure("Change", err)
		}
		err = audit.Append(
			ctx,
			q,
			audit.Record{
				ActorID:  &actor,
				EventID:  &eventID,
				TargetID: teamID,
				Action:   audit.SubmissionRepositoryChanged,
				Reason:   audit.OrganiserDecision,
				Metadata: audit.Metadata{ReferenceID: &c.RequestID},
			},
		)
		if err != nil {
			return failure("audit command", err)
		}
		return s.projectReceipt(ctx, tx, actor, eventID, teamID, v, &out)
	})
	return out, failure("change", err)
}

// OriginalReceipt returns only a currently authorised actor's exact original
// ACK. Absence is unknown, never proof of failure or permission to replay.
func (s Service) OriginalReceipt(
	ctx context.Context,
	actor, eventID, teamID, requestID uuid.UUID,
) (Recovery, error) {
	var out Recovery
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		v, err := s.scoped(ctx, tx, actor, eventID, teamID)
		if err != nil {
			return failure("OriginalReceipt", err)
		}
		row, err := q.SubmissionFreezeCommand(
			ctx,
			dbgen.SubmissionFreezeCommandParams{EventID: eventID, RequestID: requestID},
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return failure("OriginalReceipt", err)
		}
		if row.ActorID != actor || row.TeamID != teamID {
			return nil
		}
		out.Receipt = &Receipt{}
		if err = json.Unmarshal(row.Result, out.Receipt); err != nil {
			return failure("decode original receipt", err)
		}
		return s.projectReceipt(ctx, tx, actor, eventID, teamID, v, out.Receipt)
	})
	return out, failure("original receipt", err)
}

func (s Service) configure(
	ctx context.Context,
	q *dbgen.Queries,
	actor, eventID, teamID uuid.UUID,
	p Page,
	c Command,
) error {
	if !p.CanConfigure {
		return access.ErrForbidden
	}
	if !validBranch(c.Branch) || c.SourceID != uuid.Nil || c.SHA != "" ||
		len(c.ObservationIDs) > 0 ||
		len(c.PushIDs) > 0 ||
		c.ExternalEvidence != "" ||
		c.PriorDecisionID != nil || c.NativeRevision != 0 || c.SubmissionRevision != 0 {
		return access.ErrInvalid
	}
	r, err := q.TeamRepository(
		ctx,
		dbgen.TeamRepositoryParams{EventID: eventID, TeamID: teamID},
	)
	if err != nil {
		return failure("configure", err)
	}
	if r.State != "ready" || !r.GithubRepositoryID.Valid {
		return access.ErrConflict
	}
	err = q.SaveSubmissionBranchConfiguration(
		ctx,
		dbgen.SaveSubmissionBranchConfigurationParams{
			EventID:      eventID,
			TeamID:       teamID,
			Revision:     p.Revision + 1,
			RepositoryID: r.GithubRepositoryID.Int64,
			Provenance:   r.Provenance,
			Branch:       c.Branch,
			ActorID:      actor,
		},
	)
	if err != nil {
		return failure("configure", err)
	}
	return nil
}

func validateDecision(p Page, c Command) error {
	if (c.Action == "reconcile" && !p.CanReconcile) ||
		(c.Action == "amend" && !p.CanAmend) {
		return access.ErrForbidden
	}
	if p.Source == nil || c.SourceID != p.Source.ID || c.Branch != "" ||
		!shaPattern.MatchString(c.SHA) ||
		strings.Trim(c.SHA, "0") == "" ||
		c.Reason == "" {
		return access.ErrInvalid
	}
	if c.NativeRevision != p.NativeRevision || p.CurrentSubmissionRevision == nil ||
		c.SubmissionRevision != *p.CurrentSubmissionRevision {
		return access.ErrConflict
	}
	if c.Action == "amend" {
		if (p.Current == nil && c.PriorDecisionID != nil) ||
			(p.Current != nil && (c.PriorDecisionID == nil || *c.PriorDecisionID != p.Current.ID)) {
			return access.ErrConflict
		}
	} else if c.PriorDecisionID != nil {
		return access.ErrInvalid
	}

	if len(c.ObservationIDs) == 0 && len(c.PushIDs) == 0 && c.ExternalEvidence == "" {
		return access.ErrInvalid
	}
	return nil
}

func selectedEvidence(
	ctx context.Context,
	q *dbgen.Queries,
	eventID, teamID uuid.UUID,
	p Page,
	c Command,
) error {
	// A decision binds retained facts explicitly. Neither received order nor
	// author timestamps are promoted to proof; external review evidence is prose
	// retained only for current organisers, alongside their explicit reason.
	observations := map[uuid.UUID]Observation{}
	for _, o := range p.Observations {
		observations[o.ID] = o
	}
	seen := map[uuid.UUID]bool{}
	for _, id := range c.ObservationIDs {
		o, ok := observations[id]
		if !ok || seen[id] || o.SHA != c.SHA {
			return access.ErrConflict
		}
		seen[id] = true
	}
	seen = map[uuid.UUID]bool{}
	for _, id := range c.PushIDs {
		push, err := q.SubmissionFreezeExactPush(
			ctx,
			dbgen.SubmissionFreezeExactPushParams{EventID: eventID, TeamID: teamID, ID: id},
		)
		if err != nil {
			return failure("exact push", err)
		}
		if seen[id] || push.RepositoryID != p.Source.RepositoryID ||
			push.Provenance != p.Source.RepositoryProvenance ||
			push.Ref != "refs/heads/"+p.Source.Branch ||
			push.AfterSha != c.SHA {
			return access.ErrConflict
		}
		seen[id] = true
	}
	return nil
}

func (s Service) decide(
	ctx context.Context,
	q *dbgen.Queries,
	actor, eventID, teamID uuid.UUID,
	p Page,
	c Command,
	document submissions.Document,
) (*Decision, error) {
	if err := validateDecision(p, c); err != nil {
		return nil, failure("decide", err)
	}
	if err := selectedEvidence(ctx, q, eventID, teamID, p, c); err != nil {
		return nil, failure("decide", err)
	}

	// Selecting facts alone is not an automatic historical guarantee. Require
	// the organiser to explain how those facts support the cutoff decision.
	managedIDs, uncapturedIDs, audiences := referenceFacts(document, p.Source.RepositoryURL)
	d := Decision{
		ManagedEvidenceIDs: managedIDs, UncapturedEvidenceIDs: uncapturedIDs, Audiences: audiences,
		SubmissionRevision: c.SubmissionRevision, NativeRevision: c.NativeRevision,
		ID:               uuid.New(),
		SourceID:         c.SourceID,
		Revision:         p.Revision + 1,
		ActorID:          actor,
		At:               s.now(),
		Action:           c.Action,
		SHA:              c.SHA,
		Reason:           c.Reason,
		ObservationIDs:   c.ObservationIDs,
		PushIDs:          c.PushIDs,
		ExternalEvidence: c.ExternalEvidence,
		PriorDecisionID:  c.PriorDecisionID,
	}
	if d.ObservationIDs == nil {
		d.ObservationIDs = []uuid.UUID{}
	}
	if d.PushIDs == nil {
		d.PushIDs = []uuid.UUID{}
	}
	encoded, err := json.Marshal(d)
	if err != nil {
		return nil, failure("decide", err)
	}
	if err = q.InsertSubmissionFreezeDecision(
		ctx,
		dbgen.InsertSubmissionFreezeDecisionParams{
			ID:         d.ID,
			SourceID:   d.SourceID,
			Revision:   d.Revision,
			ActorID:    actor,
			DecidedAt:  d.At,
			Definition: encoded,
		},
	); err != nil {
		return nil, failure("decide", err)
	}
	return &d, nil
}
